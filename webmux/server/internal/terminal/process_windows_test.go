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
		closed := make(chan struct{})
		go func() { _ = p.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(10 * time.Second):
			stacks := make([]byte, 1<<20)
			n := runtime.Stack(stacks, true)
			t.Fatalf("raw shell close hung on iteration %d\n%s", i, stacks[:n])
		}
		await(t, p.Done())
		await(t, c.done)
		status, err := windows.WaitForSingleObject(childHandle, 5000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("raw child survived terminal close: status=%d err=%v", status, err)
		}
	}
}
