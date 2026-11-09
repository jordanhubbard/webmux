//go:build !windows

package recovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

func WaitForRecovery(ctx context.Context) error {
	id, key, instance := os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY"), os.Getenv("WEBMUX_RECOVERY_INSTANCE")
	if instance == "" {
		return errors.New("recovery startup identity is missing")
	}
	if _, err := updateHostState(id, key, false, nil); err != nil {
		return err
	}
	// Keep the path short enough for macOS sockaddr_un; no user supplied path.
	dir, err := os.MkdirTemp("/tmp", "wm-recovery-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(endpoint, 0600); err != nil {
		return err
	}
	restore, err := discardWaitingInput(os.Stdin)
	if err != nil {
		return err
	}
	defer func() {
		if restore != nil {
			_ = restore()
		}
	}()
	control := Event{Kind: "control", Mode: "waiting", Endpoint: endpoint, Instance: instance, Helper: os.Getenv("WEBMUX_RECOVERY_HELPER"), Platform: runtime.GOOS}
	_, err = updateHostState(id, key, false, func(state *HostState) error { state.Control = &control; return nil })
	if err != nil {
		return err
	}
	_ = Emit(control, os.Stdout)
	fmt.Fprint(os.Stdout, "\r\n[WebMux] Previous work was interrupted. Open Recover work to review and continue.\r\n")
	prepared, err := serveWaiting(ctx, listener, id, key, instance, preflightAction)
	_ = listener.Close()
	restoreErr := restore()
	restore = nil
	if err != nil || restoreErr != nil {
		return errors.Join(err, restoreErr)
	}
	switch prepared.Kind {
	case "replay":
		return ExitStatusError(10)
	case "native":
		if err := os.Chdir(prepared.Cwd); err != nil {
			return err
		}
		if err := RunCLI(prepared.Argv[0], prepared.Argv[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "\r\n[WebMux] Native recovery exited:", err)
		}
	}
	return nil
}
