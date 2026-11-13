//go:build windows

package recovery

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// Chmod does not restrict Windows directory access. Apply a protected DACL
// before creating the socket, including when TEMP is shared.
func privateWaitingDirectory(dir string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func newWaitingSocket() (*net.UnixListener, string, func(), error) {
	dir, err := os.MkdirTemp("", "wmr-")
	if err != nil {
		return nil, "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := privateWaitingDirectory(dir); err != nil {
		cleanup()
		return nil, "", nil, err
	}
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		cleanup()
		return nil, "", nil, err
	}
	return listener, endpoint, func() { _ = listener.Close(); cleanup() }, nil
}

var flushConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("FlushConsoleInputBuffer")

func flushWaitingInput(handle windows.Handle) error {
	result, _, err := flushConsoleInput.Call(uintptr(handle))
	if result == 0 {
		return err
	}
	return nil
}

// The worker owns this console while waiting. No application reads input and
// every queued event is deliberately discarded, including partial drafts.
// Flush is nonblocking, so Stop joins the drainer before restoring modes and
// handing the same console to a native CLI or the following shell.
func discardWaitingInput(input *os.File) (func() error, error) {
	fd := int(input.Fd())
	previous, err := term.MakeRaw(fd)
	if err != nil {
		return nil, errors.New("recovery requires an interactive terminal")
	}
	handle := windows.Handle(input.Fd())
	if err := flushWaitingInput(handle); err != nil {
		_ = term.Restore(fd, previous)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var drainErr error
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if drainErr = flushWaitingInput(handle); drainErr != nil {
					return
				}
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		return errors.Join(drainErr, flushWaitingInput(handle), term.Restore(fd, previous))
	}, nil
}
