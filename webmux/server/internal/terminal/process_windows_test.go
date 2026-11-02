package terminal

import (
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func childBlockInput() error { return nil }

func childSize() (int, int, error) {
	var info windows.ConsoleScreenBufferInfo
	err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info)
	return int(info.Size.X), int(info.Size.Y), err
}

// Match browser exec sessions: cmd.exe owns a raw-input Node child that stays
// alive until the terminal is closed. Killing the shell alone must not hang.
func TestWindowsCloseRawShellChild(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the raw terminal fixture")
	}
	fixture := filepath.Join(t.TempDir(), "raw-child.cjs")
	if err := os.WriteFile(fixture, []byte(`process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('\x1b[?1003h\x1b[?1006hraw-ready:'+process.pid+';');
process.stdin.on('data', d => process.stdout.write('input:'+d.toString('hex')));
`), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		p, err := Start(Command{Path: "cmd.exe", ShellCommand: fmt.Sprintf(`"%s" "%s"`, node, fixture), Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		c := &capture{changed: make(chan struct{}, 1), done: make(chan struct{})}
		go func() { _, c.err = io.Copy(c, p); close(c.done) }()
		c.expect(t, ";")
		match := regexp.MustCompile(`raw-ready:(\d+);`).FindStringSubmatch(c.text())
		if len(match) != 2 {
			t.Fatalf("missing child PID: %q", c.text())
		}
		pid, err := strconv.ParseUint(match[1], 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		childHandle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(childHandle)
		if _, err := p.Write([]byte("h")); err != nil {
			t.Fatal(err)
		}
		c.expect(t, "input:68")
		if _, err := p.Write([]byte("\x1b[<0;3;1M\x1b[<0;3;1m")); err != nil {
			t.Fatal(err)
		}
		// Browsers send fit/resize messages concurrently with reconnect/delete.
		stopResize, resizeDone := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(resizeDone)
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for width := 80; ; width = 80 + (width+1)%20 {
				select {
				case <-stopResize:
					return
				case <-ticker.C:
					_ = p.Resize(width, 24)
				}
			}
		}()
		time.Sleep(2 * time.Millisecond)
		closed := make(chan struct{})
		go func() { _ = p.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(10 * time.Second):
			stacks := make([]byte, 1<<20)
			n := runtime.Stack(stacks, true)
			t.Fatalf("raw shell close hung on iteration %d\n%s", i, stacks[:n])
		}
		close(stopResize)
		await(t, resizeDone)
		await(t, p.Done())
		await(t, c.done)
		status, err := windows.WaitForSingleObject(childHandle, 5000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("raw child survived terminal close: status=%d err=%v", status, err)
		}
	}
}

// ConPTY interprets application output before WebMux receives it. Its OSC 52
// handler forwards clipboard writes but consumes read queries. Retain native
// evidence for this backend limit rather than mocking a query into browser tests.
func TestWindowsConPTYClipboardQueryBoundary(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the ConPTY clipboard regression")
	}
	fixture := filepath.Join(t.TempDir(), "clipboard-boundary.cjs")
	if err := os.WriteFile(fixture, []byte(`process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('CLIPBOARD_BEFORE\x1b]52;c;Y29weQ==\x1b\\CLIPBOARD_BETWEEN\x1b]52;c;?\x07CLIPBOARD_MIDDLE\x1b]52;c;?\x1b\\CLIPBOARD_AFTER');
`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Start(Command{Path: "cmd.exe", ShellCommand: fmt.Sprintf(`"%s" "%s"`, node, fixture), Cols: 160, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	c := &capture{changed: make(chan struct{}, 1), done: make(chan struct{})}
	go func() { _, c.err = io.Copy(c, p); close(c.done) }()
	t.Cleanup(func() { _ = p.Close(); await(t, p.Done()); await(t, c.done) })
	c.expect(t, "CLIPBOARD_AFTER")
	output := c.text()
	if !strings.Contains(output, "\x1b]52;c;Y29weQ==") {
		t.Fatalf("ConPTY lost clipboard write; output: %q", output)
	}
	if strings.Contains(output, "52;c;?") {
		t.Fatalf("ConPTY now preserves clipboard queries: enable and qualify the Windows application-read workflows; output: %q", output)
	}
	if !strings.Contains(output, "CLIPBOARD_BETWEEN") || !strings.Contains(output, "CLIPBOARD_MIDDLE") {
		t.Fatalf("ConPTY lost surrounding output: %q", output)
	}
	t.Logf("ConPTY clipboard boundary: write forwarded; BEL/ST queries consumed; output: %q", output)
}
