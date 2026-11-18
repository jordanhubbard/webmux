//go:build solaris || darwin || freebsd || netbsd || openbsd || dragonfly
// +build solaris darwin freebsd netbsd openbsd dragonfly

package uv

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestSelectReader(t *testing.T) {
	testCancelableFileReader(t, newSelectPollReader)
}

// Stop inside Poll after its canceled check, before the kernel wait. This
// forces cancellation after Read has started without a scheduler-dependent
// sleep, and verifies that the cancellation pipe releases that wait.
type pollFDGate struct {
	*os.File
	armed   bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *pollFDGate) Fd() uintptr {
	if f.armed {
		f.once.Do(func() { close(f.entered) })
		<-f.release
	}
	return f.File.Fd()
}

func TestSelectReaderCancelAfterPollCheck(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	gate := &pollFDGate{File: pr, entered: make(chan struct{}), release: make(chan struct{})}
	reader, err := newSelectPollReader(gate)
	if err != nil {
		pr.Close()
		pw.Close()
		t.Fatal(err)
	}
	gate.armed = true
	type readResult struct {
		n   int
		err error
	}
	result := make(chan readResult, 1)
	done := make(chan struct{})
	var release sync.Once
	defer func() {
		release.Do(func() { close(gate.release) })
		reader.Cancel()
		pw.Close() // EOF releases the old implementation too.
		<-done
		reader.Close()
		pr.Close()
	}()
	go func() {
		defer close(done)
		n, err := reader.Read(make([]byte, 1))
		result <- readResult{n, err}
	}()
	select {
	case <-gate.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Read did not enter the cancellable descriptor poll")
	}
	if !reader.Cancel() {
		t.Fatal("cancel failed")
	}
	release.Do(func() { close(gate.release) })
	select {
	case got := <-result:
		if got.n != 0 || got.err != ErrCanceled {
			t.Fatalf("canceled read: got %d, %v", got.n, got.err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cancel did not release the descriptor poll")
	}
}
