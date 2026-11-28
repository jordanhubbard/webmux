//go:build windows

package discovery

import "golang.org/x/sys/windows"

func setReuseAddr(fd uintptr) error {
	return windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
}
