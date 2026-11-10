package recovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeHookEligibilityPreservesSettingsAndNativeOptOuts(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	if !claudeHookEligible("2.1.322 (Claude Code)", []string{"--restricted", "--strict-mcp-config", "--tools", ""}) {
		t.Fatal("qualified restricted invocation excluded")
	}
	for _, args := range [][]string{{"--safe-mode"}, {"--bare"}, {"--settings", "custom.json"}, {"--settings={}"}, {"--setting-sources", ""}, {"--print"}, {"-p"}, {"--agent", "custom"}} {
		if claudeHookEligible("2.1.322 (Claude Code)", args) {
			t.Fatal("native opt-out or settings override was replaced", args)
		}
	}
	if claudeHookEligible("2.1.323 (Claude Code)", nil) {
		t.Fatal("unqualified native version accepted")
	}
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "1")
	if claudeHookEligible("2.1.322 (Claude Code)", nil) {
		t.Fatal("native safe-mode environment bypassed")
	}
}

func claudeObservationFixture(t *testing.T) (claudeSessionObservation, string) {
	t.Helper()
	root, project := t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	dir := filepath.Join(root, "projects", "encoded-project")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id := "08bbfe0e-d31c-4cb6-9d18-546eea56a08c"
	file := filepath.Join(dir, id+".jsonl")
	data, _ := json.Marshal(map[string]string{"sessionId": id, "cwd": project})
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	return claudeSessionObservation{Event: "SessionStart", SessionID: id, Transcript: file, Cwd: project, Source: "resume"}, project
}

func TestClaudeHookReportsChangedNativeSessionAndRetainsManualConfidence(t *testing.T) {
	observation, project := claudeObservationFixture(t)
	event := Event{Kind: "cli", CLI: "claude", SessionID: "18bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "launch-selector"}
	observeClaudeSession(&event, observation, project)
	if event.SessionID != observation.SessionID || event.StateFile != observation.Transcript || event.NativeEvidence != "native-hook" {
		t.Fatal("native selection did not replace launch selector", event)
	}
	a := &Activity{CLI: event.CLI, NativeSessionID: event.SessionID, NativeEvidence: event.NativeEvidence}
	if !a.NativeRecoverable() || a.CanAutoResumeNative() {
		t.Fatal("hook observation lost native recovery or gained automatic confidence", a)
	}
	packet, err := SignedPacket(event, "terminal-key")
	if err != nil {
		t.Fatal(err)
	}
	var decoder Decoder
	_, events := decoder.Feed(packet)
	if len(events) != 1 || !Verify(events[0], "terminal-key") || Verify(events[0], "other-key") {
		t.Fatal("native hook provenance did not retain terminal authentication")
	}
	foreign := observation
	foreign.Cwd = t.TempDir()
	observeClaudeSession(&event, foreign, project)
	if event.SessionID != observation.SessionID || event.NativeEvidence != "unavailable" {
		t.Fatal("cross-project observation kept active confidence or erased last evidence", event)
	}
}

func TestClaudeHookRejectsSubagentsUnknownEventsAndForeignTranscriptPaths(t *testing.T) {
	observation, project := claudeObservationFixture(t)
	original := Event{CLI: "claude", SessionID: "selected", NativeEvidence: "launch-selector"}
	for _, change := range []func(*claudeSessionObservation){
		func(o *claudeSessionObservation) { o.AgentID = "subagent" }, func(o *claudeSessionObservation) { o.Event = "PreToolUse" }, func(o *claudeSessionObservation) { o.Source = "unknown" }, func(o *claudeSessionObservation) { o.SessionID = "not-native" },
	} {
		bad := observation
		change(&bad)
		event := original
		observeClaudeSession(&event, bad, project)
		if event.SessionID != original.SessionID || event.NativeEvidence != original.NativeEvidence {
			t.Fatal("untrusted observation changed native state", event)
		}
	}
	if got := claudeObservedStateFile(observation.SessionID, project, filepath.Join(t.TempDir(), "unrelated.jsonl")); got != "" {
		t.Fatal("unverified native path accepted", got)
	}
}

func TestClaudeHookEvidenceHonorsLoggingPause(t *testing.T) {
	hostFixture(t)
	first := Event{Kind: "cli", CLI: "claude", Instance: "native-cli", SessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "native-hook"}
	if err := RecordHostEvent(first); err != nil {
		t.Fatal(err)
	}
	if got := hostSnapshot(t).CLI; got == nil || got.NativeEvidence != "native-hook" {
		t.Fatal("hook provenance lost in host snapshot", got)
	}
	paused := false
	response := HostRPC(context.Background(), RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "policy", Recording: &paused})
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	second := first
	second.SessionID = "18bbfe0e-d31c-4cb6-9d18-546eea56a08c"
	if err := RecordHostEvent(second); err != nil {
		t.Fatal(err)
	}
	if got := hostSnapshot(t).CLI; got == nil || got.SessionID != first.SessionID {
		t.Fatal("paused hook capture replaced retained evidence", got)
	}
	if err := InitializeHostState(testHostSession, testHostKey, true); err != nil {
		t.Fatal(err)
	}
	if err := RecordHostEvent(second); err != nil {
		t.Fatal(err)
	}
	if got := hostSnapshot(t).CLI; got == nil || got.SessionID != second.SessionID || got.NativeEvidence != "native-hook" {
		t.Fatal("resumed hook capture lost current evidence", got)
	}
}
