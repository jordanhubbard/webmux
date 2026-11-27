//go:build linux

package recovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This qualification deliberately triggers a kernel OOM inside a private,
// swap-disabled 64 MiB cgroup. It never pressures the host's memory pool.
func TestProviderWorkloadOOMPreservesOtherSessions(t *testing.T) {
	if os.Getenv("WEBMUX_QUALIFY_PROVIDER_OOM") != "1" {
		t.Skip("set WEBMUX_QUALIFY_PROVIDER_OOM=1 for bounded native systemd OOM qualification")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	hostFixture(t)
	dir, err := os.MkdirTemp("/tmp", "wm-oom-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	// Some tmux builds create a separate systemd scope for each pane. Disable
	// that optional integration in this fixture to cover hosts where workloads
	// inherit the provider scope, as they did in the reported incident.
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	wrapper := filepath.Join(dir, "tmux")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexport DBUS_SESSION_BUS_ADDRESS=unix:path="+dir+"/no-bus\nexec "+quote(tmux)+" -f /dev/null \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := scopedProviderCommand(ctx, wrapper, testHostSession, os.Environ())
	if command == nil {
		t.Fatal("native systemd user scope unavailable")
	}
	unit := ""
	for _, arg := range command.Args {
		if strings.HasPrefix(arg, "--unit=") {
			unit = strings.TrimPrefix(arg, "--unit=") + ".scope"
		}
	}
	if unit == "" {
		t.Fatal("missing provider scope")
	}
	t.Cleanup(func() { _ = exec.Command("systemctl", "--user", "stop", unit).Run() })
	// Apply limits before tmux starts, while using the production scope policy.
	for i, arg := range command.Args {
		if arg == "--" {
			args := append([]string{}, command.Args[:i]...)
			args = append(args, "--property=MemoryMax=64M", "--property=MemorySwapMax=0")
			command.Args = append(args, command.Args[i:]...)
			break
		}
	}
	command.Stdin = strings.NewReader("new-session -d -s keeper 'sleep 60'\nnew-session -d -s unrelated 'sleep 60'\n")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start provider: %s: %v", output, err)
	}
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, wrapper, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %s: %v", args, output, err)
		}
		return strings.TrimSpace(string(output))
	}
	pids := run("list-panes", "-a", "-F", "#{session_name}:#{pane_pid}")
	group, err := exec.CommandContext(ctx, "systemctl", "--user", "show", unit, "--property=ControlGroup", "--value").Output()
	if err != nil {
		t.Fatal(err)
	}
	events := filepath.Join("/sys/fs/cgroup", strings.TrimSpace(string(group)), "memory.events")
	hog := filepath.Join(dir, "hog.py")
	if err := os.WriteFile(hog, []byte("from pathlib import Path\nPath('/proc/self/oom_score_adj').write_text('1000')\nx = bytearray(128 * 1024 * 1024)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("new-session", "-d", "-s", "hog", quote(python)+" "+quote(hog))
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		data, err := os.ReadFile(events)
		if err != nil {
			t.Fatalf("provider scope disappeared after workload OOM: %v", err)
		}
		var kills int
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "oom_kill ") {
				_, _ = fmt.Sscanf(line, "oom_kill %d", &kills)
			}
		}
		if kills > 0 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("workload did not trigger a bounded cgroup OOM: %s", data)
		case <-tick.C:
		}
	}
	// Synchronize through the manager after its OOM notification, then verify
	// the original pane processes and a reconnect to the same tmux server.
	policy, err := exec.CommandContext(ctx, "systemctl", "--user", "show", unit, "--property=OOMPolicy", "--value").Output()
	if err != nil || strings.TrimSpace(string(policy)) != "continue" {
		t.Fatalf("provider did not retain continue policy: %s: %v", policy, err)
	}
	after := run("list-panes", "-a", "-F", "#{session_name}:#{pane_pid}")
	for _, pid := range strings.Split(pids, "\n") {
		if !strings.Contains(after, pid) {
			t.Fatalf("original process lost: %s; after: %s", pid, after)
		}
	}
	run("has-session", "-t", "=keeper")
	run("has-session", "-t", "=unrelated")
}
