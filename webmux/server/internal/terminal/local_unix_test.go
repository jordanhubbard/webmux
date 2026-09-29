//go:build !windows

package terminal

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLocalTmuxShellSurvivesClientClose(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	// Keep this test away from the user's tmux server and configuration.
	dir, err := os.MkdirTemp("", "wm-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	p := planner{platform: "linux", home: dir, environment: append(os.Environ(), "TMUX_TMPDIR="+dir), lookup: exec.LookPath}
	r := LaunchRequest{SessionID: "12345678-1234-1234-1234-123456789abc", Hostname: "localhost", Transport: "local", Cols: 80, Rows: 24}
	c, err := p.build(r, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.Args = append([]string{"-f", "/dev/null"}, c.Args...)
	run := func(args ...string) (string, error) {
		cmd := exec.Command(tmux, args...)
		cmd.Env = c.Env
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	defer run("kill-server")
	start := func() *Process {
		proc, err := Start(c)
		if err != nil {
			t.Fatal(err)
		}
		go io.Copy(io.Discard, proc)
		t.Cleanup(func() { _ = proc.Close() })
		return proc
	}
	first := start()
	target := "webmux-" + r.SessionID
	var pid string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pid, err = run("display-message", "-p", "-t", target, "#{pane_pid}")
		if err == nil && pid != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || pid == "" {
		t.Fatalf("tmux session did not start: %s, %v", pid, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := run("display-message", "-p", "-t", target, "#{pane_pid}"); err != nil || got != pid {
		t.Fatalf("shell lost after detach: %s, %v", got, err)
	}
	second := start()
	defer second.Close()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		attached, err := run("display-message", "-p", "-t", target, "#{session_attached}")
		if err == nil && attached == "1" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, err := run("display-message", "-p", "-t", target, "#{pane_pid}:#{session_attached}"); err != nil || got != pid+":1" {
		t.Fatalf("reattach lost shell: %s, %v", got, err)
	}
}
