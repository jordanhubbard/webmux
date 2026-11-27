//go:build linux

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const linuxBrowserExitChild = "WEBMUX_PRIVATE_LINUX_BROWSER_EXIT_CHILD"

func TestLinuxBrowserExitChild(t *testing.T) {
	if os.Getenv(linuxBrowserExitChild) != "1" {
		t.Skip("private exit-wait fixture child")
	}
	var completed string
	if json.NewDecoder(io.LimitReader(os.Stdin, 8192)).Decode(&completed) != nil {
		os.Exit(2)
	}
	if json.NewEncoder(os.Stdout).Encode(true) != nil {
		os.Exit(2)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if os.WriteFile(completed, []byte("owned child finished its file operation"), 0600) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestLinuxBrowserExitRetainsOwnedProcessAndPreservesSibling(t *testing.T) {
	type child struct {
		identity browserExitIdentity
		finish   func()
		release  func()
		path     string
	}
	start := func() child {
		t.Helper()
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestLinuxBrowserExitChild$")
		cmd.Env = append(os.Environ(), linuxBrowserExitChild+"=1")
		input, err := cmd.StdinPipe()
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		var closeOnce, waitOnce sync.Once
		finish := func() { closeOnce.Do(func() { _ = input.Close() }) }
		release := func() { finish(); waitOnce.Do(func() { _ = cmd.Wait(); cancel() }) }
		t.Cleanup(release)
		path := filepath.Join(t.TempDir(), "child-completed")
		if json.NewEncoder(input).Encode(path) != nil {
			t.Fatal("fixture input unavailable")
		}
		var ready bool
		if json.NewDecoder(io.LimitReader(output, 1024)).Decode(&ready) != nil || !ready {
			t.Fatal("fixture child not ready")
		}
		identity, err := processExitIdentity(cmd.Process.Pid)
		if err != nil {
			t.Fatal("cannot establish child identity")
		}
		return child{identity, finish, release, path}
	}
	selected, sibling := start(), start()
	for _, mutate := range []func(*browserExitIdentity){
		func(p *browserExitIdentity) { p.created++ },
		func(p *browserExitIdentity) { p.image += "-unrelated" },
		func(p *browserExitIdentity) { p.uid++ },
	} {
		wrong := selected.identity
		mutate(&wrong)
		if wait, err := retainBrowserExitHandles([]browserExitIdentity{wrong}); err == nil || wait != nil {
			t.Fatal("mismatched process identity accepted")
		}
	}
	for _, invalid := range [][]browserExitIdentity{nil, {{}}, {selected.identity, selected.identity}, make([]browserExitIdentity, 65)} {
		if wait, err := retainBrowserExitHandles(invalid); err == nil || wait != nil {
			t.Fatal("invalid process set accepted")
		}
	}
	wait, err := retainBrowserExitHandles([]browserExitIdentity{selected.identity})
	if err != nil {
		t.Fatal("cannot retain selected child")
	}
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = wait(short)
	cancel()
	if !errors.Is(err, errBrowserExitWait) {
		t.Fatal("live child did not fail the event-wait deadline")
	}
	if _, err := os.Stat(selected.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("waiting released or changed the selected child")
	}
	if actual, err := processExitIdentity(sibling.identity.pid); err != nil || actual != sibling.identity {
		t.Fatal("waiting changed the unrelated sibling")
	}
	wait, err = retainBrowserExitHandles([]browserExitIdentity{selected.identity})
	if err != nil {
		t.Fatal("cannot retain child for final exit")
	}
	selected.finish()
	deadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = wait(deadline)
	cancel()
	if err != nil {
		t.Fatal("owned child exit was not observed")
	}
	if _, err := os.Stat(selected.path); err != nil {
		t.Fatal("exit was reported before the child's final file operation")
	}
	selected.release()
	if actual, err := processExitIdentity(sibling.identity.pid); err != nil || actual != sibling.identity {
		t.Fatal("owned-child completion affected sibling")
	}
	if _, err := os.Stat(sibling.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("sibling was released by another process's exit wait")
	}
	sibling.release()
}
