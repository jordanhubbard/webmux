package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func codexSelectionFixture(t *testing.T, home, cwd, id string) string {
	t.Helper()
	directory := filepath.Join(home, "sessions", "fixture")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "rollout-"+id+".jsonl")
	data, err := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]string{"id": id, "cwd": cwd}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestCodexSelectionRequiresCurrentAccountStoreAndOriginalProject(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", home)
	first := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	second := "01a1c383-71b0-7631-99d9-e0e5eb52a7a8"
	foreign := "12345678-1234-4123-a123-123456789abd"
	codexSelectionFixture(t, home, project, first)
	secondFile := codexSelectionFixture(t, home, project, second)
	codexSelectionFixture(t, home, t.TempDir(), foreign)
	event := Event{Kind: "cli", CLI: "codex", Cwd: project, SessionID: first, NativeEvidence: "launch-selector"}
	observeCodexSelection(&event, codexSelectionObservation{ThreadID: &second}, project)
	if event.SessionID != second || event.StateFile != secondFile || event.NativeEvidence != "native-selection" {
		t.Fatal("same-process switch was not observed", event)
	}
	packet, err := SignedPacket(event, "selected-terminal-key")
	if err != nil {
		t.Fatal(err)
	}
	var decoder Decoder
	_, events := decoder.Feed(packet)
	if len(events) != 1 || !Verify(events[0], "selected-terminal-key") || Verify(events[0], "another-terminal-key") {
		t.Fatal("native selection lost its terminal authentication", events)
	}
	for _, id := range []*string{&foreign, nil, &first} {
		if id == &first {
			t.Setenv("CODEX_HOME", t.TempDir())
		}
		observeCodexSelection(&event, codexSelectionObservation{ThreadID: id}, project)
		if event.SessionID != second || event.NativeEvidence != "unavailable" {
			t.Fatal("unavailable/foreign observation changed the manual reference or retained confidence", event)
		}
	}
}

func TestCodexSelectionRejectsWrongProcessReplayAndUnsupportedProtocol(t *testing.T) {
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	good := codexSelectionObservation{Schema: "codex.tui.selection.v1", PID: 12, Sequence: 2, ThreadID: &id}
	data, _ := json.Marshal(good)
	if _, ok := decodeCodexSelection(data, 12, 1); !ok {
		t.Fatal("valid record rejected")
	}
	for _, test := range []struct {
		pid      int
		previous uint64
	}{{13, 1}, {12, 2}, {12, 3}} {
		if _, ok := decodeCodexSelection(data, test.pid, test.previous); ok {
			t.Fatal("foreign or replayed selection accepted", test)
		}
	}
	good.Schema = "unsupported"
	data, _ = json.Marshal(good)
	if _, ok := decodeCodexSelection(data, 12, 1); ok {
		t.Fatal("unqualified protocol accepted")
	}
	if _, ok := decodeCodexSelection(make([]byte, 513), 12, 1); ok {
		t.Fatal("oversized selection accepted")
	}
}
