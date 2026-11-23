//go:build !windows

package maccoord

import (
	"os"
	"syscall"
)

func privateOwnerFile(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid()) && info.Mode().Perm()&0077 == 0
}
