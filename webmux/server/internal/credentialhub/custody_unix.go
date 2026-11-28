//go:build linux || darwin

package credentialhub

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func checkPrivateDir(path string) error {
	fi, e := os.Lstat(path)
	if e != nil {
		return e
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil || s.Uid != uint32(os.Geteuid()) || fi.Mode().Perm()&0077 != 0 || !fi.IsDir() {
		return ErrUnavailable
	}
	return nil
}
func makePrivateDir(path string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if e := os.Mkdir(path, 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	return checkPrivateDir(path)
}
func readPrivate(path string, limit int) ([]byte, error) {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil || s.Uid != uint32(os.Geteuid()) || s.Mode&unix.S_IFMT != unix.S_IFREG || s.Mode&0077 != 0 || s.Nlink != 1 || s.Size > int64(limit) {
		return nil, ErrUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if e != nil || len(b) > limit {
		return nil, ErrUnavailable
	}
	return b, nil
}
func createPrivate(path string, value []byte) error {
	fd, e := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), path)
	_, e = f.Write(value)
	if e == nil {
		e = f.Sync()
	}
	return errors.Join(e, f.Close())
}

func syncPrivateDir(path string) error {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}
