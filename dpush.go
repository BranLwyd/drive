package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
)

var (
	deadline = flag.Duration("deadline", 2*time.Minute, "How long to try pushing before timing out.")
)

func homeDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("couldn't get current user: %v", err)
	}
	if usr.HomeDir == "" {
		return "", errors.New("user has no home directory")
	}
	return usr.HomeDir, nil
}

func tokenFile() (string, error) {
	hd, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("couldn't get home directory: %v", err)
	}
	return filepath.Join(hd, ".dpush/token"), nil
}

func config() (*oauth2.Config, error) {
	hd, err := homeDir()
	if err != nil {
		return nil, fmt.Errorf("couldn't get home directory: %v", err)
	}
	csb, err := ioutil.ReadFile(filepath.Join(hd, ".dpush/client_secret"))
	if err != nil {
		return nil, fmt.Errorf("couldn't read client secret: %v", err)
	}
	cfg, err := google.ConfigFromJSON(csb, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("couldn't create config from client secret: %v", err)
	}
	return cfg, nil
}

func client(ctx context.Context, cfg *oauth2.Config) (*http.Client, error) {
	tf, err := tokenFile()
	if err != nil {
		return nil, fmt.Errorf("couldn't get token filename: %v", err)
	}

	var tok *oauth2.Token
	if _, err := os.Stat(tf); !os.IsNotExist(err) {
		// Token file exists, just read it.
		tb, err := ioutil.ReadFile(tf)
		if err != nil {
			return nil, fmt.Errorf("couldn't read token file: %v", err)
		}
		tok = &oauth2.Token{}
		if err := json.Unmarshal(tb, tok); err != nil {
			return nil, fmt.Errorf("couldn't parse token file: %v", err)
		}
	} else {
		// No token file. Request that the user gain access.
		// TODO: generate random state token & validate?
		fmt.Printf("No authorization token found. Visit the following link in your browser, then type the authorization code:\n  %v\n\nCode: ", cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline))

		var code string
		if _, err := fmt.Scan(&code); err != nil {
			return nil, fmt.Errorf("couldn't get code from user: %v", err)
		}
		var err error
		tok, err = cfg.Exchange(oauth2.NoContext, code)
		if err != nil {
			return nil, fmt.Errorf("couldn't retrieve token from web: %v", err)
		}

		tokBytes, err := json.Marshal(tok)
		if err != nil {
			return nil, fmt.Errorf("couldn't marshal token to JSON: %v", err)
		}
		if err := ioutil.WriteFile(tf, tokBytes, 0600); err != nil {
			return nil, fmt.Errorf("couldn't write token file: %v", err)
		}
	}

	return cfg.Client(ctx, tok), nil
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprint(os.Stderr, "\n")
	os.Exit(1)
}

func main() {
	// Parse & sanity-check flags & command-line arguments.
	flag.Parse()
	if *deadline <= 0 {
		fail("--deadline must be positive")
	}

	if len(flag.Args()) != 2 {
		fail("Usage: %s <local-file> <remote-file>", os.Args[0])
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fail("Couldn't open local file: %v", err)
	}
	defer f.Close()
	driveFile := flag.Arg(1)

	// Create Google Drive client.
	ctx, cancel := context.WithTimeout(context.Background(), *deadline)
	defer cancel()
	cfg, err := config()
	if err != nil {
		fail("Couldn't get configuration: %v", err)
	}
	client, err := client(ctx, cfg)
	if err != nil {
		fail("Couldn't get HTTP client: %v", err)
	}
	drv, err := drive.New(client)
	if err != nil {
		fail("Couldn't create Google Drive client: %v", err)
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
			fail("Couldn't list directory %q: %v", strings.Join(path[:i], "/"), err)
		}
		if len(lst.Files) == 0 {
			fail("Couldn't find Google Drive directory %q", strings.Join(path[:i+1], "/"))
		}
		if len(lst.Files) > 1 {
			fail("Directory specification %q is ambiguous", strings.Join(path[:i+1], "/"))
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
			fail("Failed to create file: %v", err)
		}

	case 1:
		// File already exists; update it.
		id := lst.Files[0].Id
		if _, err := drv.Files.Update(id, nil).Context(ctx).Media(f).Do(); err != nil {
			fail("Failed to update file: %v", err)
		}

	default:
		fail("File specification %q is ambiguous", driveFile)
	}
}
