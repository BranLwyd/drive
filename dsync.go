package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BranLwyd/drive/client"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	drive "google.golang.org/api/drive/v3"
)

// TODO: protect against path traversal (Drive allows a folder named "..")
// TODO: handle moves by copying local file rather than re-downloading

var (
	// Where to download from.
	fromFolderID = flag.String("from_folder_id", "", "The ID of the root folder to download from.")

	// Where to download to.
	toDir = flag.String("to_dir", "", "The directory to download to.")

	// How to sync.
	dryRun          = flag.Bool("dry_run", false, "If set, do not actually change filesystem state.")
	removeLocalOnly = flag.Bool("remove_local_only", false, "If set, remove files that are local-only.")

	// Miscellaneous options.
	concurrency = flag.Int("concurrency", 0, "The amount of concurrency to use for certain operations. By default, use GOMAXPROCS.")
	rateLimit   = flag.Float64("rate_limit", 5, "The rate limit (in QPS) to send to Google Drive.")
	isQuiet     = flag.Bool("quiet", false, "If set, do not output anything except errors.")
	isVerbose   = flag.Bool("verbose", false, "If set, output extra information.")

	// Global variables.
	lim *rate.Limiter
)

type localFileInfo struct {
	md5 []byte
}

func localFileInfos(ctx context.Context, baseDir string) (map[string]localFileInfo, error) {
	eg, ctx := errgroup.WithContext(ctx)
	fileCh := make(chan string)
	var rsltMu sync.Mutex // protects rslt
	rslt := map[string]localFileInfo{}

	// Consumer (of file-paths) goroutines.
	for i := 0; i < *concurrency; i++ {
		eg.Go(func() error {
			for {
				var fn string
				var ok bool
				select {
				case <-ctx.Done():
					return ctx.Err()
				case fn, ok = <-fileCh:
				}
				if !ok {
					return nil
				}

				if err := func() error {
					f, err := os.Open(fn)
					if err != nil {
						return fmt.Errorf("could not open %q: %v", fn, err)
					}
					defer f.Close()
					md5 := md5.New()
					if _, err := io.Copy(md5, f); err != nil {
						return fmt.Errorf("could not read %q: %v", fn, err)
					}
					baseFn := strings.TrimPrefix(fn, baseDir)
					if baseFn[0] == '/' {
						baseFn = baseFn[1:]
					}
					ls := localFileInfo{md5.Sum(nil)}
					rsltMu.Lock()
					defer rsltMu.Unlock()
					rslt[baseFn] = ls
					return nil
				}(); err != nil {
					return err
				}
			}
		})
	}

	// Producer goroutine.
	eg.Go(func() error {
		defer close(fileCh)
		return filepath.Walk(baseDir, func(path string, info os.FileInfo, walkErr error) error {
			switch {
			case walkErr != nil:
				return fmt.Errorf("error walking at %q: %v", path, walkErr)
			case info.Mode().IsRegular():
				select {
				case <-ctx.Done():
					return ctx.Err()
				case fileCh <- path:
				}
			}
			return nil
		})
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return rslt, nil
}

type remoteFileInfo struct {
	id   string
	md5  []byte
	size int64
}

func remoteFileInfos(ctx context.Context, drv *drive.Service, remoteFolderID string) (map[string]remoteFileInfo, error) {
	// TODO: handle multiple files w/ same name
	// TODO: handle recursive directory structures(?)
	eg, ctx := errgroup.WithContext(ctx)

	type workItem struct{ path, folderID string }
	getWI, sendWI, wiDone := make(chan workItem), make(chan workItem), make(chan struct{})
	var rsltMu sync.Mutex // protects rslt
	rslt := map[string]remoteFileInfo{}

	// Middle-man goroutine: receives work items, passes them to consumers, keeps track of when we are done.
	eg.Go(func() error {
		defer close(getWI)

		var wis []workItem
		var outstandingWIs uint64
		for {
			var wi workItem
			var ch chan workItem
			if len(wis) > 0 {
				wi, ch = wis[0], getWI
			}

			select {
			case ch <- wi:
				wis = wis[1:]
			case wi := <-sendWI:
				outstandingWIs++
				wis = append(wis, wi)
			case <-wiDone:
				outstandingWIs--
				if outstandingWIs == 0 {
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	// Consumer goroutines: receives work items, handles them; may produce more work items if directories are discovered.
	for i := 0; i < *concurrency; i++ {
		eg.Go(func() error {
			for {
				var wi workItem
				var ok bool
				select {
				case <-ctx.Done():
					return ctx.Err()
				case wi, ok = <-getWI:
				}
				if !ok {
					return nil
				}

				if err := func() (retErr error) {
					defer func() {
						select {
						case wiDone <- struct{}{}:
						case <-ctx.Done():
							retErr = ctx.Err()
						}
					}()
					var pageToken string
					q := fmt.Sprintf(`'%s' in parents and not trashed`, wi.folderID)
					verbose("Listing remote folder %q", wi.path)
					for {
						if err := lim.Wait(ctx); err != nil {
							return err
						}
						lst, err := drv.Files.List().Context(ctx).Q(q).PageToken(pageToken).Fields("nextPageToken", "files/id", "files/mimeType", "files/name", "files/md5Checksum", "files/size").Do()
						if err != nil {
							return fmt.Errorf("could not list folder %q: %v", wi.path, err)
						}
						for _, f := range lst.Files {
							switch {
							case f.MimeType == "application/vnd.google-apps.folder":
								// This is a folder. Enqueue a new work item to walk into it.
								select {
								case sendWI <- workItem{path.Join(wi.path, f.Name), f.Id}:
								case <-ctx.Done():
									return ctx.Err()
								}

							case f.Md5Checksum != "":
								// This is a regular file. Add it to the result list.
								path := path.Join(wi.path, f.Name)
								md5, err := hex.DecodeString(f.Md5Checksum)
								if err != nil {
									return fmt.Errorf("could not hex-decode checksum for %q: %v", path, err)
								}
								rsltMu.Lock()
								rslt[path] = remoteFileInfo{f.Id, md5, f.Size}
								rsltMu.Unlock()
							}
						}

						pageToken = lst.NextPageToken
						if pageToken == "" {
							break
						}
					}
					return nil
				}(); err != nil {
					return err
				}
			}
		})
	}

	// Priming goroutine: insert the first work item.
	eg.Go(func() error {
		select {
		case sendWI <- workItem{"", remoteFolderID}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return rslt, nil
}

type diffType byte

const (
	REMOTE_ONLY diffType = 0
	LOCAL_ONLY  diffType = 1
	MODIFIED    diffType = 2
)

func (dt diffType) String() string {
	switch dt {
	case REMOTE_ONLY:
		return "remote-only"
	case LOCAL_ONLY:
		return "local-only"
	case MODIFIED:
		return "modified"
	default:
		panic("unknown change type")
	}
}

func diff(lfis map[string]localFileInfo, rfis map[string]remoteFileInfo) map[string]diffType {
	diffs := map[string]diffType{}
	for p, rfi := range rfis {
		lfi, ok := lfis[p]
		switch {
		case !ok:
			diffs[p] = REMOTE_ONLY
		case !bytes.Equal(lfi.md5, rfi.md5):
			diffs[p] = MODIFIED
		}
	}
	for p := range lfis {
		if _, ok := rfis[p]; !ok {
			diffs[p] = LOCAL_ONLY
		}
	}
	return diffs
}

func download(ctx context.Context, drv *drive.Service, localFN, driveID string) error {
	if err := lim.Wait(ctx); err != nil {
		return err
	}
	resp, err := drv.Files.Get(driveID).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("could not start download: %v", err)
	}
	defer resp.Body.Close()

	localDir := filepath.Dir(localFN)
	if err := os.MkdirAll(localDir, 0770); err != nil {
		return fmt.Errorf("could not create directory: %v", err)
	}
	tmpFile, err := ioutil.TempFile(filepath.Dir(localFN), ".dsync_tmp_")
	if err != nil {
		return fmt.Errorf("could not open temp file: %v", err)
	}
	tmpFN := tmpFile.Name()
	defer os.Remove(tmpFN)
	defer tmpFile.Close()
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("could not download: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("could not close temp file: %v", err)
	}
	if err := os.Rename(tmpFN, localFN); err != nil {
		return fmt.Errorf("could not rename temp file: %v", err)
	}
	return nil
}

func info(format string, args ...interface{}) {
	if *isQuiet {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func verbose(format string, args ...interface{}) {
	if *isQuiet || !*isVerbose {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func warning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func dieWithUsage(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n\n", args...)
	flag.Usage()
	os.Exit(1)
}

func size(sz int64) string {
	switch {
	case sz >= 1<<40:
		return fmt.Sprintf("%.02f TiB", float64(sz)/(1<<40))
	case sz >= 1<<30:
		return fmt.Sprintf("%.02f GiB", float64(sz)/(1<<30))
	case sz >= 1<<20:
		return fmt.Sprintf("%.02f MiB", float64(sz)/(1<<20))
	case sz >= 1<<10:
		return fmt.Sprintf("%.02f KiB", float64(sz)/(1<<10))
	default:
		return fmt.Sprintf("%d B", sz)
	}
}

func main() {
	// Parse & sanity-check flags & command-line arguments.
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s --from_folder_id=abc --to_dir=foo/bar ...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *fromFolderID == "" {
		dieWithUsage("--from_folder_id is required")
	}
	if *toDir == "" {
		dieWithUsage("--to_dir is required")
	}
	if *isQuiet && *isVerbose {
		dieWithUsage("--quiet and --verbose are mutually exclusive")
	}
	if *concurrency == 0 {
		*concurrency = runtime.GOMAXPROCS(0)
	}

	lim = rate.NewLimiter(rate.Every(time.Duration(float64(time.Second) / *rateLimit)), 1)

	// Create Google Drive client.
	drv, err := client.Client(context.Background())
	if err != nil {
		die("Couldn't create Google Drive client: %v", err)
	}

	// List local & remote.
	info("Retrieving & diffing local & remote repositories")
	eg, ctx := errgroup.WithContext(context.Background())

	var lfis map[string]localFileInfo
	eg.Go(func() error {
		var err error
		lfis, err = localFileInfos(ctx, *toDir)
		return err
	})

	var rfis map[string]remoteFileInfo
	eg.Go(func() error {
		var err error
		rfis, err = remoteFileInfos(ctx, drv, *fromFolderID)
		return err
	})

	if err := eg.Wait(); err != nil {
		die("Could not retrieve local & remote repositories: %v", err)
	}

	diffs := diff(lfis, rfis)
	var dlPaths, rmPaths []string
	for p, typ := range diffs {
		switch typ {
		case REMOTE_ONLY, MODIFIED:
			dlPaths = append(dlPaths, p)
		case LOCAL_ONLY:
			rmPaths = append(rmPaths, p)
		}
	}
	sort.Strings(dlPaths)
	sort.Strings(rmPaths)
	var totalSize int64
	for _, p := range dlPaths {
		sz := rfis[p].size
		verbose("File %q is %v, will download [size = %s]", p, diffs[p], size(sz))
		totalSize += sz
	}
	for _, p := range rmPaths {
		suffix := ""
		if *removeLocalOnly {
			suffix = ", will remove"
		}
		verbose("File %q is %v%s", p, diffs[p], suffix)
	}

	if *dryRun {
		info("Dry-run -- not changing filesystem state.")
		return
	}

	// Download files.
	ctx = context.Background()
	stats := struct {
		sync.Mutex // protects all fields
		dlCount    int
		dlSize     int64
		errors     int
	}{}
	var wg sync.WaitGroup
	ch := make(chan string) // paths to download
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range ch {
				rfi := rfis[p]

				// Update stats.
				stats.Lock()
				stats.dlCount++
				stats.dlSize += rfi.size
				cnt, sz := stats.dlCount, stats.dlSize
				stats.Unlock()

				info("[%d / %d, %s / %s] Downloading %q", cnt, len(dlPaths), size(sz), size(totalSize), p)
				if err := download(ctx, drv, filepath.Join(*toDir, p), rfi.id); err != nil {
					stats.Lock()
					stats.errors++
					stats.Unlock()
					warning("Could not download %q: %v", p, err)
				}
			}
		}()
	}

	for _, p := range dlPaths {
		ch <- p
	}
	close(ch)
	wg.Wait()
	if stats.errors > 0 {
		die("Encountered %d errors while downloading", stats.errors)
	}

	// Remove files (if requested).
	var rmErrors int
	if *removeLocalOnly {
		for i, p := range rmPaths {
			info("[%d / %d] Removing %q", i+1, len(rmPaths), p)
			if err := os.Remove(filepath.Join(*toDir, p)); err != nil {
				rmErrors++
				warning("Could not remove %q: %v", p, err)
			}
		}
	}
	if rmErrors > 0 {
		die("Encountered %d errors while removing", rmErrors)
	}
}
