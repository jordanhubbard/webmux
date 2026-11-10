//go:build !windows

package recovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestProviderReattachesAndSerializesConcurrentCreation(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	hostFixture(t)
	dir, err := os.MkdirTemp("/tmp", "wm-provider-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("SHELL", "/bin/bash")
	t.Cleanup(func() {
		_ = exec.Command(tmux, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	// Exercise the native tmux parser with a helper path requiring quoting.
	// The sleeping child is a provider-lifetime fixture, not a coding CLI.
	helper := filepath.Join(dir, "helper with 'quote")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	var mutex sync.Mutex
	created := 0
	for i := 0; i < 5; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			event, err := ensureProvider(context.Background(), testHostSession, testHostKey, helper)
			if err != nil {
				t.Error(err)
				return
			}
			if !event.Survived {
				mutex.Lock()
				created++
				mutex.Unlock()
			}
		}()
	}
	group.Wait()
	if created != 1 {
		t.Fatalf("concurrent reconnects created %d providers", created)
	}
	target := "=webmux-" + testHostSession
	pid, err := exec.Command(tmux, "list-panes", "-t", target, "-F", "#{pane_pid}").Output()
	if err != nil || strings.TrimSpace(string(pid)) == "" {
		t.Fatal("provider did not start", err)
	}
	event, err := ensureProvider(context.Background(), testHostSession, testHostKey, helper)
	if err != nil || !event.Survived {
		t.Fatal("surviving provider was replaced", event, err)
	}
	after, err := exec.Command(tmux, "list-panes", "-t", target, "-F", "#{pane_pid}").Output()
	if err != nil || string(after) != string(pid) {
		t.Fatal("reconnect changed the provider process", err)
	}
	if err := RecordHostEvent(Event{Kind: "command", Command: "git status", Cwd: dir}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(tmux, "kill-server").Run(); err != nil {
		t.Fatal(err)
	}
	event, err = ensureProvider(context.Background(), testHostSession, testHostKey, helper)
	if err != nil || event.Survived || event.Mode != "waiting" {
		t.Fatal("lost provider did not select explicit recovery", event, err)
	}
	flag, err := exec.Command(tmux, "show-environment", "-t", target, "WEBMUX_RECOVERY_WAIT").Output()
	if err != nil || strings.TrimSpace(string(flag)) != "WEBMUX_RECOVERY_WAIT=1" {
		t.Fatal("replacement provider was not gated before shell input", string(flag), err)
	}
}
