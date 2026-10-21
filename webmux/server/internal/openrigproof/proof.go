// Package openrigproof supplies an explicit, owner-scoped live acceptance test.
package openrigproof

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

//go:embed runner.mts
var runner []byte

type Location struct{ Directory, Runner, Team string }

func Locate(home, owner string) Location {
	hash := sha256.Sum256([]byte(owner))
	key := hex.EncodeToString(hash[:12])
	directory := filepath.Join(home, "data", "openrig-proof", key)
	// Distinct WebMux installations sharing one daemon must not stop each
	// other's pair, even when both use trusted mode's anonymous owner.
	absolute, err := filepath.Abs(home)
	if err != nil {
		absolute = filepath.Clean(home)
	}
	scope := sha256.Sum256([]byte(absolute + "\x00" + owner))
	team := "webmux-proof-" + hex.EncodeToString(scope[:12])
	return Location{directory, filepath.Join(directory, "runner.mts"), team}
}

func Prepare(home, owner string) (Location, error) {
	value := Locate(home, owner)
	if err := os.MkdirAll(value.Directory, 0700); err != nil {
		return value, err
	}
	file, err := os.CreateTemp(value.Directory, ".runner-*.mts")
	if err != nil {
		return value, err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(runner)
	err = firstError(err, file.Close())
	if err == nil {
		err = os.Rename(file.Name(), value.Runner)
	}
	return value, err
}

func firstError(first, second error) error {
	if first != nil {
		return first
	}
	return second
}

func Status(home, owner string) map[string]any {
	file := filepath.Join(Locate(home, owner).Directory, "status.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return nil
	}
	return result
}

// Starting invalidates a previous PASS before a new runner is launched.
func Starting(location Location) error {
	data, err := json.Marshal(map[string]string{"phase": "preparing", "message": "Starting a fresh challenge…", "run_id": "", "team": location.Team})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(location.Directory, ".status-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	err = firstError(err, file.Close())
	if err == nil {
		err = os.Rename(file.Name(), filepath.Join(location.Directory, "status.json"))
	}
	return err
}
