package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BranLwyd/drive/cli"
	"github.com/BranLwyd/drive/client"
	drive "google.golang.org/api/drive/v3"
)

var (
	deadline = flag.Duration("deadline", 2*time.Minute, "How long to try pushing before timing out.")
)

func main() {
	// Parse & sanity-check flags & command-line arguments.
	flag.Parse()
	if *deadline <= 0 {
		cli.DieWithUsage("--deadline must be positive")
	}

	if len(flag.Args()) != 2 {
		cli.DieWithUsage("Usage: %s <local-file> <remote-file>", os.Args[0])
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		cli.DieWithUsage("Couldn't open local file: %v", err)
	}
	defer f.Close()
	driveFile := flag.Arg(1)

	// Create Google Drive client.
	ctx, cancel := context.WithTimeout(context.Background(), *deadline)
	defer cancel()
	drv, err := client.Client(ctx)
	if err != nil {
		cli.Die("Couldn't create Google Drive client: %v", err)
	}

	// Traverse into the Google Drive path to find the ID of the parent of the file.
	parentID := "root"
	path := strings.Split(driveFile, "/")
	filename := path[len(path)-1]
	path = path[:len(path)-1]
	for i, pe := range path {
		q := fmt.Sprintf(`name = '%s' and '%s' in parents and mimeType = 'application/vnd.google-apps.folder' and not trashed`, pe, parentID)
		lst, err := drv.Files.List().Context(ctx).Q(q).Fields("files/id").Do()
		if err != nil {
			cli.Die("Couldn't list directory %q: %v", strings.Join(path[:i], "/"), err)
		}
		if len(lst.Files) == 0 {
			cli.Die("Couldn't find Google Drive directory %q", strings.Join(path[:i+1], "/"))
		}
		if len(lst.Files) > 1 {
			cli.Die("Directory specification %q is ambiguous", strings.Join(path[:i+1], "/"))
		}
		parentID = lst.Files[0].Id
	}

	// Figure out if the file already exists or not.
	q := fmt.Sprintf(`name = '%s' and '%s' in parents and mimeType != 'application/vnd.google-apps.folder' and not trashed`, filename, parentID)
	lst, err := drv.Files.List().Context(ctx).Q(q).Fields("files/id").Do()
	switch len(lst.Files) {
	case 0:
		// File does not exist; create it.
		if _, err := drv.Files.Create(&drive.File{
			Name:    filename,
			Parents: []string{parentID},
		}).Context(ctx).Media(f).Do(); err != nil {
			cli.Die("Failed to create file: %v", err)
		}

	case 1:
		// File already exists; update it.
		id := lst.Files[0].Id
		if _, err := drv.Files.Update(id, nil).Context(ctx).Media(f).Do(); err != nil {
			cli.Die("Failed to update file: %v", err)
		}

	default:
		cli.Die("File specification %q is ambiguous", driveFile)
	}
}
