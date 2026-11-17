package browser

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

var profileSyntax = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"-"`
}

func privateDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("browser profile directory is not private")
	}
	return nil
}

func savedProfile(key string) (string, *os.File, error) {
	if !profileSyntax.MatchString(key) {
		return "", nil, errors.New("invalid saved browser profile identity")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	root := filepath.Join(home, "webmux-browser-profiles")
	if err := privateDirectory(root); err != nil {
		return "", nil, err
	}
	lockPath := filepath.Join(root, key+".lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return "", nil, errors.New("invalid browser profile lock")
	}
	lock, err := lockProfile(lockPath)
	if err != nil {
		return "", nil, errors.New("saved browser profile is already in use or unavailable")
	}
	path := filepath.Join(root, key)
	if err := privateDirectory(path); err != nil {
		lock.Close()
		return "", nil, err
	}
	return path, lock, nil
}

// RemoveProfile is a fixed worker mode. A controller sends only a scoped opaque
// key over the private pipe; filesystem paths are never accepted from HTTP.
func RemoveProfile(in io.Reader, out io.Writer) error {
	var req Request
	if err := json.NewDecoder(io.LimitReader(in, 4096)).Decode(&req); err != nil {
		return err
	}
	path, lock, err := savedProfile(req.ProfileKey)
	if err == nil {
		defer lock.Close()
		err = os.RemoveAll(path)
	}
	frame := Frame{Protocol: Protocol, ProfileKey: req.ProfileKey}
	if err != nil {
		frame.Error = "Saved browser profile is in use or cannot be removed on the terminal host."
	}
	if encodeErr := json.NewEncoder(out).Encode(frame); encodeErr != nil {
		return encodeErr
	}
	return err
}
