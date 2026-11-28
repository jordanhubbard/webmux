//go:build unix

package discovery

import "golang.org/x/sys/unix"

func setReuseAddr(fd uintptr) error {
	return unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
}
