package main

import (
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

	"github.com/BranLwyd/drive/cli"
	"github.com/BranLwyd/drive/client"
	"github.com/golang/protobuf/proto"
	"github.com/kirsle/configdir"
	"github.com/thomaso-mirodin/intmath/i64"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
	drive "google.golang.org/api/drive/v3"

	pb "github.com/BranLwyd/drive/dsync_proto"
)

// TODO: protect against path traversal (Drive allows a folder named "..")
// TODO: handle multiple remote files with same name, recursive directory structure(?)
// TODO: handle moves by copying local file rather than re-downloading

var (
	// Where to download from.
	fromFolderID = flag.String("from_folder_id", "", "The ID of the root folder to download from.")

	// Where to download to.
	toDir = flag.String("to_dir", "", "The directory to download to.")

	// Cache options.
	cacheFile = flag.String("cache", "", "Location of the cache file. Will be created if it does not exist. Defaults ")

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
	md5 string
}

func localFileInfos(ctx context.Context, fsc *filesystemCache, baseDir string) (map[string]localFileInfo, error) {
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
					md5, err := fsc.ContentHash(fn)
					if err != nil {
						return fmt.Errorf("could not determine content hash for %q: %w", fn, err)
					}
					baseFn := strings.TrimPrefix(fn, baseDir)
					if baseFn[0] == '/' {
						baseFn = baseFn[1:]
					}
					ls := localFileInfo{md5}
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
	md5  string
	size int64
}

func remoteFileInfos(ctx context.Context, drv *drive.Service, remoteFolderID string) (map[string]remoteFileInfo, error) {
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
					cli.Verbose("Listing remote folder %q", wi.path)
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
								rslt[path] = remoteFileInfo{f.Id, string(md5), f.Size}
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
		case lfi.md5 != rfi.md5:
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

func sizes(szs ...int64) []string {
	if len(szs) == 0 {
		return nil
	}
	maxSz := szs[0]
	for _, sz := range szs[1:] {
		maxSz = i64.Max(maxSz, sz)
	}

	unit, div := "B", int64(1)
	for _, v := range []struct {
		unit string
		div  int64
	}{
		{"KiB", 1 << 10},
		{"MiB", 1 << 20},
		{"GiB", 1 << 30},
		{"TiB", 1 << 40},
	} {
		if maxSz < v.div {
			break
		}
		unit, div = v.unit, v.div
	}

	rslt := make([]string, 0, len(szs))
	for _, sz := range szs {
		rslt = append(rslt, fmt.Sprintf("%.02f %s", float64(sz)/float64(div), unit))
	}
	return rslt
}

func size(sz int64) string {
	return sizes(sz)[0]
}

type filesystemCache struct {
	mu sync.Mutex // Protects c.
	c  *pb.LocalFilesystemCache
}

func newFSCache() (*filesystemCache, error) {
	// Read config.
	fn := cacheFilename()
	cBytes, err := ioutil.ReadFile(fn)
	if os.IsNotExist(err) {
		// If the file wasn't there, pretend it was empty.
		return &filesystemCache{
			c: &pb.LocalFilesystemCache{Entries: map[string]*pb.LocalFilesystemCache_Entry{}},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't read cache %q: %w", fn, err)
	}

	// Parse & return config.
	c := &pb.LocalFilesystemCache{}
	if err := proto.Unmarshal(cBytes, c); err != nil {
		return nil, fmt.Errorf("couldn't parse cache %q: %w", fn, err)
	}
	return &filesystemCache{c: c}, nil
}

func (fc *filesystemCache) ContentHash(fn string) (string, error) {
	absFN, err := filepath.Abs(fn)
	if err != nil {
		return "", fmt.Errorf("couldn't get absolute path for %q: %w", fn, err)
	}
	fn = absFN

	ctim, err := ctime(fn)
	if err != nil {
		return "", fmt.Errorf("couldn't get ctime for %q: %w", fn, err)
	}

	// Fast path: if the file path is in the cache, and the ctime matches, return the cached hash.
	fc.mu.Lock()
	if e, ok := fc.c.Entries[fn]; ok {
		if e.Time == ctim {
			fc.mu.Unlock()
			return string(e.ContentHash), nil
		}
	}
	fc.mu.Unlock()

	// Slow path: the cache didn't have the entry, or it was stale. Compute the hash by reading the file.
	f, err := os.Open(fn)
	if err != nil {
		return "", fmt.Errorf("couldn't open %q: %w", fn, err)
	}
	defer f.Close()
	md5 := md5.New()
	if _, err := io.Copy(md5, f); err != nil {
		return "", fmt.Errorf("couldn't read %q: %w", fn, err)
	}
	e := &pb.LocalFilesystemCache_Entry{
		Time:        ctim,
		ContentHash: md5.Sum(nil),
	}

	// Recheck ctime after reading content to avoid TOCTOU issues with the ctime check.
	// The correctness of this check requires ctime to be monotonically increasing.
	ctim, err = ctime(fn)
	if err != nil {
		return "", fmt.Errorf("couldn't get ctime for %q: %w", fn, err)
	}
	if ctim != e.Time {
		return "", fmt.Errorf("file %q modified during hashing", fn)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.c.Entries[fn] = e
	return string(e.ContentHash), nil
}

func (fc *filesystemCache) Remove(fn string) error {
	absFN, err := filepath.Abs(fn)
	if err != nil {
		return fmt.Errorf("couldn't get absolute path for %q: %w", fn, err)
	}
	fn = absFN

	fc.mu.Lock()
	defer fc.mu.Unlock()
	delete(fc.c.Entries, fn)
	return nil
}

func (fc *filesystemCache) Write() error {
	// Serialize cache.
	cBytes, err := proto.Marshal(fc.c)
	if err != nil {
		return fmt.Errorf("couldn't serialize cache: %w", err)
	}

	// Write cache.
	fn := cacheFilename()
	cDir := filepath.Dir(fn)
	if err := os.MkdirAll(cDir, 0750); err != nil {
		return fmt.Errorf("couldn't create config directory %q: %w", cDir, err)
	}
	tempF, err := ioutil.TempFile(cDir, ".fs.cache_")
	if err != nil {
		return fmt.Errorf("couldn't create temporary file: %w", err)
	}
	tempFN := tempF.Name()
	defer os.Remove(tempFN)
	defer tempF.Close()
	if err := os.Chmod(tempFN, 0640); err != nil {
		return fmt.Errorf("couldn't set permission: %w", err)
	}
	if _, err := tempF.Write(cBytes); err != nil {
		return fmt.Errorf("couldn't write content: %w", err)
	}
	if err := tempF.Close(); err != nil {
		return fmt.Errorf("couldn't close: %w", err)
	}
	if err := os.Rename(tempFN, fn); err != nil {
		return fmt.Errorf("couldn't rename: %w", err)
	}
	return nil

}

func ctime(fn string) (int64, error) {
	var stat unix.Stat_t
	if err := unix.Stat(fn, &stat); err != nil {
		return 0, fmt.Errorf("couldn't stat %q: %w", fn, err)
	}
	return time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec).UnixNano(), nil
}

func cacheFilename() string {
	if *cacheFile != "" {
		return *cacheFile
	}
	return filepath.Join(configdir.LocalCache("dsync"), "fs.cache")
}

func main() {
	// Parse & sanity-check flags & command-line arguments.
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s --from_folder_id=abc --to_dir=foo/bar ...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := cli.Setup(*isQuiet, *isVerbose); err != nil {
		cli.DieWithUsage("CLI setup: %v", err)
	}
	if *fromFolderID == "" {
		cli.DieWithUsage("--from_folder_id is required")
	}
	if *toDir == "" {
		cli.DieWithUsage("--to_dir is required")
	}
	if *concurrency == 0 {
		*concurrency = runtime.GOMAXPROCS(0)
	}

	lim = rate.NewLimiter(rate.Every(time.Duration(float64(time.Second) / *rateLimit)), 1)

	fsc, err := newFSCache()
	if err != nil {
		cli.Die("Couldn't retrieve cache: %v", err)
	}

	// Create Google Drive client.
	drv, err := client.Client(context.Background())
	if err != nil {
		cli.Die("Couldn't create Google Drive client: %v", err)
	}

	// List local & remote.
	cli.Info("Retrieving & diffing local & remote repositories")
	eg, ctx := errgroup.WithContext(context.Background())

	var lfis map[string]localFileInfo
	eg.Go(func() error {
		var err error
		lfis, err = localFileInfos(ctx, fsc, *toDir)
		return err
	})

	var rfis map[string]remoteFileInfo
	eg.Go(func() error {
		var err error
		rfis, err = remoteFileInfos(ctx, drv, *fromFolderID)
		return err
	})

	if err := eg.Wait(); err != nil {
		cli.Die("Could not retrieve local & remote repositories: %v", err)
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
		cli.Verbose("File %q is %v, will download [size = %s]", p, diffs[p], size(sz))
		totalSize += sz
	}
	for _, p := range rmPaths {
		suffix := ""
		if *removeLocalOnly {
			suffix = ", will remove"
		}
		cli.Verbose("File %q is %v%s", p, diffs[p], suffix)
	}

	if *dryRun {
		cli.Info("Dry-run -- not changing filesystem state.")
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

				szs := sizes(sz, totalSize)
				cli.Info("[%d / %d, %s / %s] Downloading %q", cnt, len(dlPaths), szs[0], szs[1], p)
				if err := download(ctx, drv, filepath.Join(*toDir, p), rfi.id); err != nil {
					stats.Lock()
					stats.errors++
					stats.Unlock()
					cli.Warning("Could not download %q: %v", p, err)
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
		cli.Warning("Encountered %d errors while downloading", stats.errors)
	}

	// Remove files (if requested).
	var rmErrors int
	if *removeLocalOnly {
		for i, p := range rmPaths {
			cli.Info("[%d / %d] Removing %q", i+1, len(rmPaths), p)
			fn := filepath.Join(*toDir, p)
			if err := os.Remove(fn); err != nil {
				rmErrors++
				cli.Warning("Could not remove %q: %v", p, err)
				continue
			}
			if err := fsc.Remove(fn); err != nil {
				rmErrors++
				cli.Warning("Could not remove %q from cache: %v", p, err)
				continue
			}
		}
	}
	if rmErrors > 0 {
		cli.Warning("Encountered %d errors while removing", rmErrors)
	}

	fscWriteErr := fsc.Write()
	if fscWriteErr != nil {
		cli.Warning("Could not write filesystem cache back to disk: %v", err)
	}

	if stats.errors > 0 || rmErrors > 0 || fscWriteErr != nil {
		os.Exit(1)
	}
}
