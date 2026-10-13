//go:build !windows

package httpapi

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func verifyManagerSocket(path string) error {
	invalid := errors.New("unsafe session-manager socket")
	if !filepath.IsAbs(path) {
		return invalid
	}
	for current := path; current != filepath.Dir(current); current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return invalid
		}
		if current == path || current == filepath.Dir(path) {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0077 != 0 {
				return invalid
			}
		}
		if current == path && info.Mode()&os.ModeSocket == 0 {
			return invalid
		}
	}
	return nil
}
