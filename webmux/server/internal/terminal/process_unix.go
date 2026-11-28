//go:build !windows

package terminal

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type unixProcess struct {
	file    *os.File
	command *exec.Cmd
}

func startNative(spec Command) (nativeProcess, error) {
	command := exec.Command(spec.Path, spec.Args...)
	command.Dir, command.Env = spec.Dir, spec.Env
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: uint16(spec.Cols), Rows: uint16(spec.Rows)})
	if err != nil {
		return nil, err
	}
	// creack's setup uses File.Fd, which can put a terminal back in blocking
	// mode. Give the I/O layer its own nonblocking, pollable descriptor so Close
	// cancels reads/writes even when a descendant still holds the slave open.
	fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err == nil {
		err = unix.SetNonblock(fd, true)
		if err != nil {
			_ = unix.Close(fd)
		}
	}
	_ = file.Close()
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}
	return &unixProcess{os.NewFile(uintptr(fd), "terminal-master"), command}, nil
}

func (p *unixProcess) Read(data []byte) (int, error) {
	n, err := p.file.Read(data)
	// Linux reports EIO when the final slave descriptor closes; BSD returns EOF.
	if errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}
func (p *unixProcess) Write(data []byte) (int, error) { return p.file.Write(data) }
func (p *unixProcess) Close() error                   { return p.file.Close() }
func (p *unixProcess) resize(cols, rows int) error {
	connection, err := p.file.SyscallConn()
	if err != nil {
		return err
	}
	var resizeErr error
	err = connection.Control(func(fd uintptr) {
		resizeErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)})
	})
	return errors.Join(err, resizeErr)
}
func (p *unixProcess) kill() error { return p.command.Process.Kill() }
func (p *unixProcess) wait() Exit {
	err := p.command.Wait()
	var status *exec.ExitError
	if errors.As(err, &status) {
		err = nil
	}
	code := -1
	if p.command.ProcessState != nil {
		code = p.command.ProcessState.ExitCode()
	}
	return Exit{Code: code, Err: err}
}
