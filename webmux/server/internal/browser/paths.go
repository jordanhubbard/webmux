package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Browser files must share the browser's filesystem namespace. In particular,
// Snap Chromium cannot see the worker's private /tmp or arbitrary hidden home
// directories, but can see its own common data directory.
func browserDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".config", "webmux", "browser")
	if runtime.GOOS == "windows" {
		root = filepath.Join(home, "AppData", "Local", "webmux", "browser")
	} else if runtime.GOOS == "linux" {
		for _, mount := range []string{"/snap/chromium/current", "/var/lib/snapd/snap/chromium/current"} {
			if _, err := os.Stat(mount); err == nil {
				root = filepath.Join(home, "snap", "chromium", "common", "webmux", "browser")
				break
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return "", err
	}
	if err := privateDirectory(root); err != nil {
		return "", err
	}
	return root, nil
}

func browserDirectory(name string) (string, error) {
	if name != "profiles" && name != "tmp" {
		return "", errors.New("invalid private browser directory")
	}
	root, err := browserDataRoot()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, name)
	if err := privateDirectory(directory); err != nil {
		return "", err
	}
	return directory, nil
}
