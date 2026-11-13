//go:build !windows

package recovery

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// discardWaitingInput disables echo and discards keys while the recovery worker
// owns the TTY. Stop joins the reader and drains pending input before restoring
// the terminal, so partial drafts cannot reach a subsequently launched CLI.
func discardWaitingInput(input *os.File) (func() error, error) {
	fd := int(input.Fd())
	previous, err := term.MakeRaw(fd)
	if err != nil {
		return nil, errors.New("recovery requires an interactive terminal")
	}
	tty, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = term.Restore(fd, previous)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	drain := func() {
		buffer := make([]byte, 4096)
		for {
			n, err := unix.Read(tty, buffer)
			if err != nil || n == 0 {
				return
			}
		}
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				drain()
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		drain()
		return errors.Join(unix.Close(tty), term.Restore(fd, previous))
	}, nil
}

// Keep the path short enough for macOS sockaddr_un; no user supplied path.
func newWaitingSocket() (*net.UnixListener, string, func(), error) {
	dir, err := os.MkdirTemp("/tmp", "wm-recovery-")
	if err != nil {
		return nil, "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err == nil {
		err = os.Chmod(endpoint, 0600)
	}
	if err != nil {
		if listener != nil {
			_ = listener.Close()
		}
		cleanup()
		return nil, "", nil, err
	}
	return listener, endpoint, func() { _ = listener.Close(); cleanup() }, nil
}
