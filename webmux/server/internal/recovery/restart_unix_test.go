//go:build !windows

package recovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLiveRestartSelectsOnlyOriginalPaneAndRetainsEvidence(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	hostFixture(t)
	dir, err := os.MkdirTemp("/tmp", "wm-restart-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Cleanup(func() { _ = exec.Command(tmux, "kill-server").Run(); _ = os.RemoveAll(dir) })
	helper := filepath.Join(dir, "helper ' quoted")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nif [ \"$WEBMUX_RECOVERY_WAIT\" = 1 ]; then IFS= read -r text; printf '%s' \"$text\" > "+filepath.Join(dir, "received")+"; fi\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureProvider(context.Background(), testHostSession, testHostKey, helper); err != nil {
		t.Fatal(err)
	}
	_, err = updateHostState(testHostSession, testHostKey, false, func(s *HostState) error {
		s.CLI = &Event{Kind: "cli", CLI: "codex", SessionID: "saved-conversation", Cwd: dir}
		s.Control = &Event{Kind: "control", Mode: "active", Instance: "original-shell"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	original, selection, err := liveRestartTarget(context.Background(), testHostSession)
	if err != nil {
		t.Fatal(err)
	}
	request := RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "restart", Action: &RecoveryAction{ID: "11111111-1111-4111-8111-111111111111", Kind: "restart", Instance: selection, Confirm: true}}
	// Confirmation, authentication and preview identity are independent gates.
	bad := *request.Action
	bad.Confirm = false
	copyRequest := request
	copyRequest.Action = &bad
	if _, err := restartLiveProviderWithHelper(context.Background(), copyRequest, helper); err == nil {
		t.Fatal("unconfirmed restart")
	}
	copyRequest = request
	copyRequest.Key = strings.Repeat("0", 32)
	if _, err := restartLiveProviderWithHelper(context.Background(), copyRequest, helper); err == nil {
		t.Fatal("crossed authentication boundary")
	}
	bad = *request.Action
	bad.Instance = strings.Repeat("0", 64)
	copyRequest = request
	copyRequest.Action = &bad
	if _, err := restartLiveProviderWithHelper(context.Background(), copyRequest, helper); err == nil {
		t.Fatal("accepted stale target")
	}
	unchanged, _, _ := liveRestartTarget(context.Background(), testHostSession)
	if unchanged != original {
		t.Fatal("rejected request killed work")
	}
	// A second session must survive the selected pane's replacement.
	sibling := "22222222-2222-4222-8222-222222222222"
	if err := InitializeHostState(sibling, testHostKey, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureProvider(context.Background(), sibling, testHostKey, helper); err != nil {
		t.Fatal(err)
	}
	siblingBefore, _, err := liveRestartTarget(context.Background(), sibling)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(tmux, "send-keys", "-t", strings.Split(original, "|")[4], "OLD-PENDING-INPUT", "Enter").Run(); err != nil {
		t.Fatal(err)
	}
	response, err := restartLiveProviderWithHelper(context.Background(), request, helper)
	if err != nil || response.Outcome != "consumed" {
		t.Fatal(response, err)
	}
	after, newSelection, err := liveRestartTarget(context.Background(), testHostSession)
	if err != nil || after == original || newSelection == selection {
		t.Fatal("provider was not replaced", after, err)
	}
	siblingAfter, _, err := liveRestartTarget(context.Background(), sibling)
	if err != nil || siblingAfter != siblingBefore {
		t.Fatal("affected sibling", err)
	}
	oldPID, _ := strconv.Atoi(strings.Split(original, "|")[5])
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(oldPID, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if syscall.Kill(oldPID, 0) == nil {
		t.Fatal("old foreground process survived restart")
	}
	if err := exec.Command(tmux, "send-keys", "-t", strings.Split(after, "|")[4], "NEW-INPUT", "Enter").Run(); err != nil {
		t.Fatal(err)
	}
	var received []byte
	for time.Now().Before(deadline) {
		received, _ = os.ReadFile(filepath.Join(dir, "received"))
		if len(received) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if string(received) != "NEW-INPUT" {
		t.Fatalf("pending old input crossed PTY restart: %q", received)
	}
	// A lost reply/repeated click must not kill the replacement.
	if result, err := restartLiveProviderWithHelper(context.Background(), request, helper); err != nil || result.Outcome != "consumed" {
		t.Fatal(result, err)
	}
	stable, _, _ := liveRestartTarget(context.Background(), testHostSession)
	if stable != after {
		t.Fatal("retry killed replacement")
	}
	state, err := updateHostState(testHostSession, testHostKey, false, nil)
	if err != nil || state.CLI == nil || state.CLI.SessionID != "saved-conversation" || state.Control.Mode != "waiting" || state.Replay != nil {
		t.Fatal("lost evidence or replayed work", state, err)
	}
	// A different request with the old selection also cannot kill new work.
	bad = *request.Action
	bad.ID = "33333333-3333-4333-8333-333333333333"
	copyRequest = request
	copyRequest.Action = &bad
	if _, err := restartLiveProviderWithHelper(context.Background(), copyRequest, helper); err == nil {
		t.Fatal("stale selection killed successor")
	}
	if output, err := exec.Command(tmux, "split-window", "-t", strings.Split(after, "|")[4], "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("split fixture: %s %v", output, err)
	}
	if _, err := liveRestartSelection(context.Background(), testHostSession); err == nil {
		t.Fatal("offered destructive restart for ambiguous multi-pane provider")
	}

}
