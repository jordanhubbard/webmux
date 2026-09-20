package terminal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run the test binary inside a real terminal, without depending on installed
// SSH servers, shell startup files, or network access.
func TestTerminalChild(t *testing.T) {
	if os.Getenv("WEBMUX_PTY_TEST_CHILD") != "1" {
		return
	}
	fmt.Println("child-ready")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "size":
			cols, rows, err := childSize()
			fmt.Printf("size-result:%d,%d,%v\n", cols, rows, err)
		case "unicode":
			fmt.Println("unicode-result:λ 日本語 😀")
		case "bulk":
			for i := 0; i < 4096; i++ {
				fmt.Println("bulk-result:0123456789abcdef")
			}
			fmt.Println("bulk-finished")
			os.Exit(7)
		case "exit":
			os.Exit(7)
		case "block":
			fmt.Println("blocked-ready")
			for {
				time.Sleep(time.Hour)
			}
		}
	}
	os.Exit(0)
}

type capture struct {
	mu      sync.Mutex
	data    bytes.Buffer
	changed chan struct{}
	done    chan struct{}
	err     error
}

func (c *capture) Write(data []byte) (int, error) {
	c.mu.Lock()
	n, err := c.data.Write(data)
	c.mu.Unlock()
	select {
	case c.changed <- struct{}{}:
	default:
	}
	return n, err
}
func (c *capture) text() string { c.mu.Lock(); defer c.mu.Unlock(); return c.data.String() }
func (c *capture) expect(t *testing.T, text string) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for !strings.Contains(c.text(), text) {
		select {
		case <-c.changed:
		case <-c.done:
			t.Fatalf("output ended before %q: %q (%v)", text, c.text(), c.err)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %q: %q", text, c.text())
		}
	}
}
func child(t *testing.T) (*Process, *capture) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(Command{Path: executable, Args: []string{"-test.run=^TestTerminalChild$"}, Env: append(os.Environ(), "WEBMUX_PTY_TEST_CHILD=1"), Cols: 91, Rows: 33})
	if err != nil {
		t.Fatal(err)
	}
	c := &capture{changed: make(chan struct{}, 1), done: make(chan struct{})}
	go func() { _, c.err = io.Copy(c, p); close(c.done) }()
	t.Cleanup(func() { _ = p.Close(); await(t, p.Done()); await(t, c.done) })
	c.expect(t, "child-ready")
	return p, c
}
func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("terminal cleanup timed out")
	}
}
func send(t *testing.T, p *Process, command string) {
	t.Helper()
	if _, err := p.Write([]byte(command + "\r")); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalInputResizeAndExit(t *testing.T) {
	p, c := child(t)
	send(t, p, "size")
	c.expect(t, "size-result:91,33,<nil>")
	if err := p.Resize(120, 44); err != nil {
		t.Fatal(err)
	}
	send(t, p, "size")
	c.expect(t, "size-result:120,44,<nil>")
	send(t, p, "unicode")
	c.expect(t, "unicode-result:λ 日本語 😀")
	send(t, p, "exit")
	await(t, p.Done())
	await(t, c.done)
	if exit := p.Wait(); exit.Code != 7 || exit.Err != nil {
		t.Fatalf("exit: %+v", exit)
	}
	if next := p.Wait(); next.Code != 7 {
		t.Fatalf("repeated wait: %+v", next)
	}
}

func TestTerminalDrainsOutputOnExit(t *testing.T) {
	p, c := child(t)
	send(t, p, "bulk")
	await(t, p.Done())
	await(t, c.done)
	if count := strings.Count(c.text(), "bulk-result:0123456789abcdef"); count != 4096 {
		t.Fatalf("lost output: %d of 4096 lines", count)
	}
	if !strings.Contains(c.text(), "bulk-finished") {
		t.Fatal("lost final output")
	}
}

func TestTerminalCloseCancelsBlockedIO(t *testing.T) {
	p, c := child(t)
	send(t, p, "block")
	c.expect(t, "blocked-ready")
	written := make(chan struct{})
	go func() { _, _ = p.Write(bytes.Repeat([]byte("x"), 8<<20)); close(written) }()
	select {
	case <-written:
		t.Fatal("expected input to block while child is not reading")
	case <-time.After(50 * time.Millisecond):
	}
	closed := make(chan struct{})
	go func() { _ = p.Close(); close(closed) }()
	await(t, closed)
	await(t, written)
	await(t, p.Done())
	await(t, c.done)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Resize(80, 24); err == nil {
		t.Fatal("resize after close succeeded")
	}
}

func TestTerminalRejectsInvalidLaunch(t *testing.T) {
	for _, spec := range []Command{
		{Path: "missing-webmux-test-executable", Cols: 80, Rows: 24},
		{Path: "invalid\x00command", Cols: 80, Rows: 24},
		{Path: "unused", Cols: 0, Rows: 24},
		{Path: "unused", Cols: 501, Rows: 24},
		{Path: "unused", Cols: 80, Rows: 201},
		{Path: "unused", Cols: 80, Rows: 24, Args: []string{"\x00"}},
	} {
		if p, err := Start(spec); err == nil {
			p.Close()
			t.Errorf("invalid launch succeeded: %#v", spec)
		}
	}
}
