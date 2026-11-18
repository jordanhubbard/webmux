//go:build !windows
// +build !windows

package uv

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestReader(t *testing.T) {
	testCancelableFileReader(t, newPollReader)
}

func testCancelableFileReader(t *testing.T, create func(io.Reader) (pollReader, error)) {
	for _, before := range []bool{true, false} {
		name := "cancel-after-read-start"
		if before {
			name = "cancel-before-read"
		}
		t.Run(name, func(t *testing.T) {
			pr, pw, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer pr.Close()
			defer pw.Close()
			reader, err := create(pr)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if n, err := reader.Read(nil); n != 0 || err != nil {
				t.Fatalf("empty buffer: got %d, %v", n, err)
			}
			if before && !reader.Cancel() {
				t.Fatal("cancel before read failed")
			}
			type readResult struct {
				n   int
				err error
			}
			result := make(chan readResult, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				n, err := reader.Read(make([]byte, 1))
				result <- readResult{n, err}
			}()
			<-started
			if !before && !reader.Cancel() {
				// Always release and join the reader before leaving this stage.
				pw.Close()
				<-result
				t.Fatal("cancel after read start failed")
			}
			select {
			case got := <-result:
				if got.n != 0 || got.err != ErrCanceled {
					t.Fatalf("canceled read: got %d, %v", got.n, got.err)
				}
			case <-time.After(100 * time.Millisecond):
				// EOF also releases the old broken implementation. Do not
				// continue and let it steal data from the next reader.
				pw.Close()
				<-result
				t.Fatal("expected cancellation to unblock reader")
			}

			// Cancellation belongs to the reader, not the underlying file.
			fresh, err := create(pr)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			msg := "hello"
			if n, err := pw.Write([]byte(msg)); n != len(msg) || err != nil {
				t.Fatalf("write: got %d, %v", n, err)
			}
			p := make([]byte, len(msg))
			if n, err := fresh.Read(p); n != len(msg) || err != nil || string(p) != msg {
				t.Fatalf("fresh read: got %q, %d, %v", p, n, err)
			}
			pw.Close()
			if n, err := fresh.Read(p); n != 0 || err != io.EOF {
				t.Fatalf("EOF read: got %d, %v", n, err)
			}
		})
	}
}
