package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexUnknownProcessRetainsSelectedConversationWithoutAutomaticConfidence(t *testing.T) {
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	event := Event{SessionID: id, StateFile: "/selected/rollout.jsonl", NativeEvidence: "launch-selector"}
	observeCodexProcess(&event, "", "")
	if event.SessionID != id || event.StateFile != "/selected/rollout.jsonl" || event.NativeEvidence != "launch-selector" {
		t.Fatal("missing TUI-owned file erased the explicit native selection", event)
	}
	activity := &Activity{CLI: "codex", NativeSessionID: event.SessionID, NativeEvidence: event.NativeEvidence}
	if !activity.NativeRecoverable() || activity.CanAutoResumeNative() {
		t.Fatal("manual evidence lost or incorrectly eligible for automatic recovery", activity)
	}
	// A subsequent observed switch replaces the selector with the owned state.
	switched := "01a1c383-71b0-7631-99d9-e0e5eb52a7a8"
	observeCodexProcess(&event, switched, "/switched/rollout.jsonl")
	if event.SessionID != switched || event.NativeEvidence != "process-state" {
		t.Fatal("verified native switch did not replace the selector", event)
	}
	observeCodexProcess(&event, "", "")
	if event.SessionID != switched || event.NativeEvidence != "unavailable" {
		t.Fatal("lost ownership discarded history or retained current-state confidence", event)
	}
	activity.NativeSessionID, activity.NativeEvidence = event.SessionID, event.NativeEvidence
	if !activity.NativeRecoverable() || activity.CanAutoResumeNative() {
		t.Fatal("unknown current conversation was automatically recoverable", activity)
	}
}

func TestCodexSelectedStateFileRequiresExactIDAndOriginalProject(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	dir := filepath.Join(root, "sessions", "dated")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	file := filepath.Join(dir, "rollout-"+id+".jsonl")
	data, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]string{"id": id, "cwd": project}})
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if found := codexStateFile(id, project, ""); found != file {
		t.Fatal("native selection did not retain its verified file", found)
	}
	if found := codexStateFile(id, t.TempDir(), file); found != "" {
		t.Fatal("native file from another project accepted", found)
	}
	if found := codexStateFile("01a1c383-71b0-7631-99d9-e0e5eb52a7a8", project, file); found != "" {
		t.Fatal("another native conversation was selected", found)
	}
}
