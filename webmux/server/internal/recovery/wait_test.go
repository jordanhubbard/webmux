package recovery

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

const testActionID = "257284bf-cd7b-4357-8b8e-507a46a6739b"

func waitingFixture(t *testing.T) RPCRequest {
	t.Helper()
	hostFixture(t)
	t.Setenv("WEBMUX_RECOVERY_INSTANCE", "current-worker")
	state, err := updateHostState(testHostSession, testHostKey, false, func(state *HostState) error {
		state.Control = &Event{Kind: "control", Mode: "waiting", Instance: "current-worker"}
		state.Command = &Event{Kind: "command", Instance: "old-shell", Cwd: t.TempDir(), Command: "printf approved"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "action", Action: &RecoveryAction{ID: testActionID, Instance: "current-worker", Revision: state.Revision, Kind: "shell"}}
}

func TestWaitingWorkerConsumesBeforeReturningExecutionAndRejectsDuplicates(t *testing.T) {
	request := waitingFixture(t)
	prepared, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", preflightAction)
	if err != nil || prepared.Kind != "shell" {
		t.Fatalf("consume: %+v %v", prepared, err)
	}
	// A new process reading the file sees consumption even if the actor dies
	// immediately after this point and never starts the approved child.
	state := hostSnapshot(t)
	if len(state.Consumed) != 1 || state.Consumed[0] != testActionID || state.Control.Mode != "consumed" {
		t.Fatal("execution returned before intent was durable")
	}
	if _, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", preflightAction); err == nil {
		t.Fatal("duplicate action was executable")
	}
	response := HostRPC(context.Background(), request)
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatal("reconnect lost the consumed outcome", response)
	}
}

func TestWaitingWorkerRechecksRevisionAfterPreflight(t *testing.T) {
	request := waitingFixture(t)
	_, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", func(state HostState, action RecoveryAction) (preparedAction, error) {
		// A logging-policy update or another worker may supersede a preview
		// during slow native metadata validation. It cannot authorize this action.
		_, err := updateHostState(testHostSession, testHostKey, false, func(state *HostState) error { state.Recording = false; return nil })
		return preparedAction{Kind: "shell"}, err
	})
	if err == nil || !strings.Contains(err.Error(), "changed") || len(hostSnapshot(t).Consumed) != 0 {
		t.Fatal("stale preview was committed", err)
	}
}

func TestWaitingWorkerRejectsForeignOrStaleRequestsBeforePreflight(t *testing.T) {
	request := waitingFixture(t)
	original := *request.Action
	called := false
	prepare := func(HostState, RecoveryAction) (preparedAction, error) { called = true; return preparedAction{}, nil }
	for _, mutate := range []func(*RPCRequest){
		func(r *RPCRequest) { r.Key = "00000000000000000000000000000000" },
		func(r *RPCRequest) { r.SessionID = testActionID },
		func(r *RPCRequest) { r.Action.Instance = "previous-worker" },
		func(r *RPCRequest) { r.Action.Revision-- },
		func(r *RPCRequest) { r.Action.ID = "missing" },
	} {
		r := request
		a := original
		r.Action = &a
		mutate(&r)
		if _, err := acceptWaitingAction(r, testHostSession, testHostKey, "current-worker", prepare); err == nil || called {
			t.Fatal("untrusted action reached preflight", r)
		}
	}
}

func TestFailedNativePreflightLeavesWorkerAvailableAndEvidenceIntact(t *testing.T) {
	request := waitingFixture(t)
	_, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", func(HostState, RecoveryAction) (preparedAction, error) {
		return preparedAction{}, errors.New("native session unavailable")
	})
	state := hostSnapshot(t)
	if err == nil || len(state.Consumed) != 0 || state.Control.Mode != "waiting" || state.Command.Command != "printf approved" || state.Revision != request.Action.Revision {
		t.Fatal("failed preflight damaged recovery", state, err)
	}
}

func TestApprovedReplayIsReadOnceAndOnlyByOwningStartupShell(t *testing.T) {
	request := waitingFixture(t)
	request.Action.Kind = "replay"
	if _, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", preflightAction); err == nil {
		t.Fatal("unconfirmed command was replayable")
	}
	request.Action.Confirm = true
	prepared, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", preflightAction)
	if err != nil || prepared.Command != "printf approved" {
		t.Fatal(prepared, err)
	}
	t.Setenv("WEBMUX_RECOVERY_INSTANCE", "foreign-shell")
	if _, err := ReadReplay(); err == nil {
		t.Fatal("different startup shell read approved replay")
	}
	t.Setenv("WEBMUX_RECOVERY_INSTANCE", "current-worker")
	script, err := ReadReplay()
	if err != nil || !strings.HasPrefix(script, "cd -- '") || !strings.Contains(script, "eval 'printf approved'") {
		t.Fatal(script, err)
	}
	if _, err := ReadReplay(); err == nil {
		t.Fatal("approved replay could be read twice")
	}
	if hostSnapshot(t).Replay != nil {
		t.Fatal("approved replay was not consumed before returning bytes")
	}
}

func TestReplayAndTranscriptConsentAreIndependent(t *testing.T) {
	state := HostState{Command: &Event{Kind: "command", Cwd: os.TempDir(), Command: "[redacted]", Redacted: true}}
	if _, err := preflightAction(state, RecoveryAction{Kind: "replay", Confirm: true}); err == nil {
		t.Fatal("confirmed redacted command was replayable")
	}
	if _, err := preflightAction(state, RecoveryAction{Kind: "native", Context: "prior output"}); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatal("transcript handoff did not require confirmation", err)
	}
	if _, err := preflightAction(state, RecoveryAction{Kind: "shell", Context: "prior output", Confirm: true}); err == nil {
		t.Fatal("shell action accepted transcript as executable context")
	}
}

func TestCLIOnlyActivityRetainsItsLifecycleIdentity(t *testing.T) {
	event := &Event{Kind: "cli", Platform: "windows", Instance: "native-wrapper", Cwd: `C:\project`, CLI: "codex", Version: "0.153.0", SessionID: "12345678-1234-4234-a234-123456789abc", NativeEvidence: "launch-selector", Argv: []string{"codex", "resume", "12345678-1234-4234-a234-123456789abc"}}
	for _, tc := range []struct {
		complete *Event
		running  bool
	}{{nil, true}, {&Event{Kind: "complete", Instance: "other-shell"}, true}, {&Event{Kind: "complete", Instance: event.Instance}, false}} {
		a := cachedActivity(HostState{CLI: event, Complete: tc.complete})
		if a.ShellInstance != event.Instance || a.Platform != "windows" || a.Cwd != event.Cwd || a.Running != tc.running || a.NativeSessionID != event.SessionID || a.Replayable {
			t.Fatalf("CLI-only lifecycle evidence was lost: %+v", a)
		}
	}
}
