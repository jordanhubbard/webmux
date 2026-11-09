package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testHostSession = "6b06908a-83c3-4050-9e98-a83bbd412681"
const testHostKey = "cb14a8b8274769763bb4e2e942ed624c"

func hostFixture(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	t.Setenv("WEBMUX_RECOVERY_SESSION", testHostSession)
	t.Setenv("WEBMUX_RECOVERY_KEY", testHostKey)
	if err := InitializeHostState(testHostSession, testHostKey, true); err != nil {
		t.Fatal(err)
	}
}

func hostSnapshot(t *testing.T) HostState {
	t.Helper()
	state, err := updateHostState(testHostSession, testHostKey, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestHostCapturePauseRetainsEvidenceAndSuppressesNewActivity(t *testing.T) {
	hostFixture(t)
	command := Event{Kind: "command", Instance: "old", Cwd: "/project", Command: "git status"}
	if err := RecordHostEvent(command); err != nil {
		t.Fatal(err)
	}
	saved := hostSnapshot(t)
	paused := false
	response := HostRPC(context.Background(), RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "policy", Recording: &paused})
	if response.Error != "" || response.Recording {
		t.Fatal(response)
	}
	for _, event := range []Event{
		{Kind: "command", Command: "private activity", Cwd: "/private"},
		{Kind: "cli", CLI: "codex", SessionID: testHostSession},
		{Kind: "complete"}, {Kind: "shell", Cwd: "/private"},
	} {
		if err := RecordHostEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	state := hostSnapshot(t)
	want, _ := json.Marshal(saved.Command)
	got, _ := json.Marshal(state.Command)
	if !bytes.Equal(want, got) || state.CLI != nil || state.Complete != nil || state.Shell != nil || state.Revision != response.Revision {
		t.Fatalf("paused capture changed evidence: %+v", state)
	}
	if err := RecordHostEvent(Event{Kind: "control", Instance: "new", Mode: "waiting"}); err != nil {
		t.Fatal(err)
	}
	if hostSnapshot(t).Control == nil {
		t.Fatal("pause disabled recovery control")
	}
	if err := InitializeHostState(testHostSession, testHostKey, true); err != nil {
		t.Fatal(err)
	}
	if err := RecordHostEvent(Event{Kind: "command", Instance: "new", Command: "pwd"}); err != nil {
		t.Fatal(err)
	}
	if hostSnapshot(t).Command.Command != "pwd" {
		t.Fatal("resume did not append new activity to this session")
	}
}

func TestHostSnapshotAuthenticatesAndPreservesRedaction(t *testing.T) {
	hostFixture(t)
	synthetic := strings.Repeat("s", 24)
	if err := RecordHostEvent(Event{Kind: "command", Command: "curl --token " + synthetic}); err != nil {
		t.Fatal(err)
	}
	request := RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "snapshot"}
	response := HostRPC(context.Background(), request)
	if response.Error != "" || len(response.Events) != 1 || !Verify(response.Events[0], testHostKey) || !response.Events[0].Redacted {
		t.Fatal(response)
	}
	data, _ := json.Marshal(response)
	if strings.Contains(string(data), synthetic) || strings.Contains(string(data), "key_digest") || strings.Contains(string(data), testHostKey) {
		t.Fatal("snapshot exposed private data")
	}
	if cachedActivity(hostSnapshot(t)).Replayable {
		t.Fatal("redacted cached command became replayable")
	}
	request.Key = "00000000000000000000000000000000"
	response = HostRPC(context.Background(), request)
	if response.Error == "" || len(response.Events) != 0 || response.Revision != 0 {
		t.Fatal("foreign key read host evidence")
	}
}

func TestHostStateConcurrentWritersDoNotLoseCommittedIntents(t *testing.T) {
	hostFixture(t)
	var group sync.WaitGroup
	for i := 0; i < 24; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := updateHostState(testHostSession, testHostKey, false, func(state *HostState) error {
				state.Consumed = append(state.Consumed, "owned-intent")
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	state := hostSnapshot(t)
	if len(state.Consumed) != 24 || state.Revision != 25 {
		t.Fatalf("lost durable updates: %+v", state)
	}
}

func TestEmptyHostCacheCanBeSeededWithoutOverwritingNewerEvidenceOrCapturePolicy(t *testing.T) {
	hostFixture(t)
	seed := &Activity{Command: "old command", Cwd: "/project", Running: true, Replayable: true, ShellInstance: "old"}
	if err := InitializeHostEvidence(testHostSession, testHostKey, false, seed); err != nil {
		t.Fatal(err)
	}
	state := hostSnapshot(t)
	if !state.Recording || state.Command == nil || state.Command.Command != seed.Command {
		t.Fatal("bootstrap changed an existing recording policy or lost saved evidence")
	}
	if err := RecordHostEvent(Event{Kind: "command", Command: "newer command", Instance: "current"}); err != nil {
		t.Fatal(err)
	}
	if err := InitializeHostEvidence(testHostSession, testHostKey, false, seed); err != nil {
		t.Fatal(err)
	}
	if hostSnapshot(t).Command.Command != "newer command" {
		t.Fatal("controller seed replaced newer host evidence")
	}
}

func TestHostStateRejectsTraversalLinksAndOversizedRPC(t *testing.T) {
	hostFixture(t)
	if _, err := hostStateDirectory("../other"); err == nil {
		t.Fatal("accepted directory traversal")
	}
	dir, _ := hostStateDirectory(testHostSession)
	if err := os.Rename(filepath.Join(dir, "state.json"), filepath.Join(dir, "other.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.json", filepath.Join(dir, "state.json")); err == nil {
		if _, err := updateHostState(testHostSession, testHostKey, false, nil); err == nil {
			t.Fatal("accepted substituted state symlink")
		}
	}
	var response bytes.Buffer
	if err := ServeRPC(context.Background(), strings.NewReader(strings.Repeat(" ", maxRPCBytes+1)), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.String(), "exceeds its limit") {
		t.Fatal("unbounded RPC input")
	}
	response.Reset()
	_ = ServeRPC(context.Background(), strings.NewReader(`{} {}`), &response)
	if !strings.Contains(response.String(), "invalid recovery control message") {
		t.Fatal("accepted multiple control requests")
	}
}
