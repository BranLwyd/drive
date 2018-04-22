package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
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
	return filepath.Join(hd, ".drive/token"), nil
}

func config() (*oauth2.Config, error) {
	hd, err := homeDir()
	if err != nil {
		return nil, fmt.Errorf("couldn't get home directory: %v", err)
	}
	csb, err := ioutil.ReadFile(filepath.Join(hd, ".drive/client_secret"))
	if err != nil {
		return nil, fmt.Errorf("couldn't read client secret: %v", err)
	}
	cfg, err := google.ConfigFromJSON(csb, drive.DriveReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("couldn't create config from client secret: %v", err)
	}
	return cfg, nil
}

func httpClient(ctx context.Context, cfg *oauth2.Config) (*http.Client, error) {
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

func Client(ctx context.Context) (*drive.Service, error) {
	cfg, err := config()
	if err != nil {
		return nil, fmt.Errorf("couldn't get configuration: %v", err)
	}
	cli, err := httpClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't get HTTP client: %v", err)
	}
	drv, err := drive.New(cli)
	if err != nil {
		return nil, fmt.Errorf("couldn't create Drive API client: %v", err)
	}
	return drv, nil
}
