package session

import (
	"testing"
)

func TestManagedNativeEvidenceFollowsLoggingPolicyAndSurvivesRestart(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	first := RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:seat", ManagerProfile: "work", Runtime: "claude", Host: "localhost", Workspace: "/project", NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeStateFile: "/native/first.jsonl"}
	value, err := b.EnsureRigTerminal("owner", first.Target, "coder", []string{"attach"}, first)
	if err != nil {
		t.Fatal(err)
	}
	original := *value.Rig
	if original.NativeSessionID != first.NativeSessionID || original.NativeSource != "agent-deck-inventory" || original.NativeObservedAt == "" {
		t.Fatal("missing native provenance", original)
	}
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	second := first
	second.NativeSessionID = "18bbfe0e-d31c-4cb6-9d18-546eea56a08c"
	second.NativeStateFile = "/native/second.jsonl"
	paused, err := b.EnsureRigTerminal("owner", first.Target, "coder", []string{"attach"}, second)
	if err != nil || *paused.Rig != original {
		t.Fatal("logging pause captured a new native conversation", paused.Rig, err)
	}
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	resumed, err := b.EnsureRigTerminal("owner", first.Target, "coder", []string{"attach"}, second)
	if err != nil || resumed.Rig.NativeSessionID != second.NativeSessionID {
		t.Fatal("logging resume lost current native evidence", resumed.Rig, err)
	}
	captured := *resumed.Rig
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	view, err := restored.Recovery("owner", value.ID)
	if err != nil || view.ManagerProfile != "work" || view.ManagerNativeSession != captured.NativeSessionID || view.ManagerNativeStateFile != captured.NativeStateFile || view.ManagerNativeObservedAt != captured.NativeObservedAt || view.ManagerNativeSource != "agent-deck-inventory" {
		t.Fatal("native evidence did not survive restart", view, err)
	}
}

func TestManagedNativeEvidenceDoesNotMistakeRigRegistrationForConversation(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	identity := RigIdentity{Target: "seat", Runtime: "codex", Host: "localhost", NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c"}
	value, err := b.EnsureRigTerminal("owner", identity.Target, "coder", []string{"attach"}, identity)
	if err != nil || value.Rig.NativeSessionID != "" {
		t.Fatal("coordinator registration became native conversation evidence", value, err)
	}
}
