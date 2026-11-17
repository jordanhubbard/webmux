//go:build !windows

package agentdeckattach

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fake(t *testing.T, script string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tool directory")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent-deck")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n[ \"$1\" = version ]\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestVersionChildCannotReadTerminalInputAndPreservesParentTERM(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	path := fake(t, "[ \"$TERM\" = dumb ]\n! test -t 0\nprintf '%s\\n' 'Agent Deck v1.16.39'\n")
	if err := check(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TERM") != "xterm-256color" {
		t.Fatal("changed parent TERM")
	}
}
func TestUnsupportedAndOversizeOutputRefused(t *testing.T) {
	for _, script := range []string{"printf '%s\\n' 'Agent Deck v1.16.34'\n", "printf '%01024d' 0\n", "exit 7\n"} {
		if err := check(context.Background(), fake(t, script)); err == nil {
			t.Fatal("unsupported version response accepted")
		}
	}
}
func TestStalledVersionChildGroupIsTerminated(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("WM_ATTACH_CHILD", marker)
	path := fake(t, "sleep 30 &\necho $! > \"$WM_ATTACH_CHILD\"\nwait\n")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := check(ctx, path); err == nil {
		t.Fatal("stalled version accepted")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("version check exceeded cancellation bound")
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// Some Unix init implementations briefly retain the killed child as a zombie.
	if syscall.Kill(pid, 0) == nil {
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		if err == nil && strings.Contains(string(data), ") Z ") {
			return
		}
		t.Fatal("version child survived cancellation", pid)
	}
}
