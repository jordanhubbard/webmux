package gitobserve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// These files test constructor resolution only; they are never executed.
func lookupFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
		t.Setenv("PATHEXT", ".EXE")
		t.Setenv("NoDefaultCurrentDirectoryInExePath", "1")
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("lookup fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommandScopeRetainsExecutableOnlyWithinObservation(t *testing.T) {
	firstPath, secondPath := lookupFixture(t), lookupFixture(t)
	t.Setenv("PATH", filepath.Dir(firstPath))
	ctx := WithCommandScope(t.Context())
	first, initial := CommandContext(ctx, "first")
	if first.Err != nil || first.Path != firstPath || initial.resolution != "initial" {
		t.Fatal(first.Path, first.Err, initial)
	}
	t.Setenv("PATH", filepath.Dir(secondPath))
	reused, preparation := CommandContext(EnsureCommandScope(ctx), "second")
	if reused.Err != nil || reused.Path != firstPath || reused.Args[0] != "git" || reused.Args[1] != "second" || preparation.resolution != "reused" {
		t.Fatal(reused.Path, reused.Args, reused.Err, preparation)
	}
	fresh, _ := CommandContext(WithCommandScope(ctx), "third")
	if fresh.Err != nil || fresh.Path != secondPath {
		t.Fatal("executable leaked across observations", fresh.Path, fresh.Err)
	}
}

func TestCommandScopeDoesNotRetryFailedResolution(t *testing.T) {
	path := lookupFixture(t)
	t.Setenv("PATH", t.TempDir())
	ctx := WithCommandScope(t.Context())
	first, _ := CommandContext(ctx)
	if first.Err == nil {
		t.Fatal("missing executable unexpectedly resolved")
	}
	t.Setenv("PATH", filepath.Dir(path))
	reused, _ := CommandContext(ctx)
	if reused.Err != first.Err || reused.Start() != first.Err {
		t.Fatal("failed resolution was retried", reused.Err)
	}
	fresh, _ := CommandContext(WithCommandScope(ctx))
	if fresh.Err != nil || fresh.Path != path {
		t.Fatal("failure leaked into new observation", fresh.Path, fresh.Err)
	}
}

func TestReusedCommandRetainsPerCallCancellation(t *testing.T) {
	path := lookupFixture(t)
	t.Setenv("PATH", filepath.Dir(path))
	ctx := WithCommandScope(t.Context())
	first, _ := CommandContext(ctx)
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	command, _ := CommandContext(canceled)
	if err := command.Run(); !errors.Is(err, context.Canceled) || command.Process != nil {
		t.Fatal("reused executable lost per-command cancellation", err)
	}
}
