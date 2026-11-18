package nativeobserve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const ReferenceFile = ".native-codex-observer.json"
const Contract = "webmux-native-codex-observer/v1"

type Reference struct {
	Contract         string `json:"contract"`
	Executable       string `json:"executable"`
	ExecutableSHA256 string `json:"executableSHA256"`
}

func privateDirectory(name string) error {
	if err := os.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe native observer directory")
	}
	return nil
}
func syncDirectory(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
func verifySnapshot(name, expected string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0500 || info.Size() > 512*1024*1024 {
		return errors.New("native observer snapshot changed")
	}
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return err
	}
	if hex.EncodeToString(digest.Sum(nil)) != expected {
		return errors.New("native observer snapshot changed")
	}
	return nil
}

// WriteReference snapshots the running controller's own helper into an immutable
// owner-private directory. A later installed-path replacement cannot change the
// selected helper between account lookup and policy inspection. This reference
// never enters the coding launch environment.
func WriteReference(directory string) error {
	if err := privateDirectory(directory); err != nil {
		return err
	}
	executable := "/proc/self/exe"
	if runtime.GOOS != "linux" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return err
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return err
		}
	}
	file, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 512*1024*1024 {
		return errors.New("native observer helper unavailable")
	}
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return err
	}
	expected := hex.EncodeToString(digest.Sum(nil))
	snapshots := filepath.Join(directory, "controller-observers")
	if err = privateDirectory(snapshots); err != nil {
		return err
	}
	slot := filepath.Join(snapshots, expected)
	if err = privateDirectory(slot); err != nil {
		return err
	}
	destination := filepath.Join(slot, "webmux")
	if err = verifySnapshot(destination, expected); os.IsNotExist(err) {
		temp, e := os.CreateTemp(slot, ".snapshot-*")
		if e != nil {
			return e
		}
		defer os.Remove(temp.Name())
		defer temp.Close()
		if _, e = file.Seek(0, 0); e != nil {
			return e
		}
		if _, e = io.Copy(temp, file); e != nil {
			return e
		}
		if e = temp.Chmod(0500); e != nil {
			return e
		}
		if e = temp.Sync(); e != nil {
			return e
		}
		if e = temp.Close(); e != nil {
			return e
		}
		if e = os.Link(temp.Name(), destination); e != nil && !os.IsExist(e) {
			return e
		}
		if err = verifySnapshot(destination, expected); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = syncDirectory(slot); err != nil {
		return err
	}
	if err = syncDirectory(snapshots); err != nil {
		return err
	}
	destination, err = filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	data, err := json.Marshal(Reference{Contract: Contract, Executable: destination, ExecutableSHA256: expected})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".native-observer-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), filepath.Join(directory, ReferenceFile)); err != nil {
		return err
	}
	return syncDirectory(directory)
}
