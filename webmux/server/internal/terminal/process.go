// Package terminal owns native terminal processes independently of HTTP viewers.
package terminal

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

// Command contains only launch-time state. Callers must not persist Env: it may
// contain an SSH password. Args excludes the executable name.
type Command struct {
	Path       string
	Args       []string
	Dir        string
	Env        []string
	Cols, Rows int
	// ShellCommand is a raw cmd.exe command on Windows. It is unused on Unix,
	// where shell commands are passed as a single -c argument.
	ShellCommand string
}

type Exit struct {
	Code int
	Err  error
}

type nativeProcess interface {
	io.ReadWriteCloser
	resize(int, int) error
	wait() Exit
	kill() error
}

// Process supports one output reader, serialized input writers, concurrent
// resize and idempotent Close. Always drain Read concurrently with Wait; ConPTY
// emits a final display frame during shutdown. Close cancels blocked I/O and
// discards remaining output; Wait alone preserves output for the reader.
type Process struct {
	native    nativeProcess
	done      chan struct{}
	exit      Exit
	writeMu   sync.Mutex
	controlMu sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func dimensions(cols, rows int) error {
	if cols < 1 || cols > 500 || rows < 1 || rows > 200 {
		return errors.New("terminal size must be 1-500 columns and 1-200 rows")
	}
	return nil
}

func Start(command Command) (*Process, error) {
	if err := dimensions(command.Cols, command.Rows); err != nil {
		return nil, err
	}
	if command.Path == "" || strings.ContainsRune(command.Path, 0) || strings.ContainsRune(command.Dir, 0) || strings.ContainsRune(command.ShellCommand, 0) {
		return nil, errors.New("invalid terminal command")
	}
	for _, value := range append(append([]string{}, command.Args...), command.Env...) {
		if strings.ContainsRune(value, 0) {
			return nil, errors.New("invalid terminal argument or environment")
		}
	}
	native, err := startNative(command)
	if err != nil {
		return nil, err
	}
	p := &Process{native: native, done: make(chan struct{})}
	go func() { p.exit = native.wait(); close(p.done) }()
	return p, nil
}

func (p *Process) Read(data []byte) (int, error) { return p.native.Read(data) }
func (p *Process) Write(data []byte) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.native.Write(data)
}
func (p *Process) Resize(cols, rows int) error {
	if err := dimensions(cols, rows); err != nil {
		return err
	}
	p.controlMu.Lock()
	closed := p.closed
	p.controlMu.Unlock()
	if closed {
		return os.ErrClosed
	}
	return p.native.resize(cols, rows)
}
func (p *Process) Wait() Exit            { <-p.done; return p.exit }
func (p *Process) Done() <-chan struct{} { return p.done }
func (p *Process) Close() error {
	p.closeOnce.Do(func() {
		p.controlMu.Lock()
		p.closed = true
		p.controlMu.Unlock()
		err := p.native.kill()
		if errors.Is(err, os.ErrProcessDone) {
			err = nil
		}
		p.closeErr = errors.Join(err, p.native.Close())
	})
	return p.closeErr
}
