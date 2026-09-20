package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func TestAgentActivitySuppressesReplayAndFlushesOnShutdown(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	s, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	<-launched
	b.mu.Lock()
	r := b.entries[s.ID].run
	b.mu.Unlock()
	b.output(s.ID, r, "initial replay")
	b.mu.Lock()
	if len(b.activity) != 0 {
		t.Fatal("replay changed status")
	}
	r.replayUntil = time.Now().Add(-time.Second)
	b.mu.Unlock()
	b.output(s.ID, r, "live output")
	if err := b.Input("owner", s.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	pending := b.activity[agentActivityKey{"alpha", "task"}]
	b.mu.Unlock()
	if pending.update.LastInputAt == nil || pending.update.LastOutputAt == nil || pending.update.LastOutputSource == nil {
		t.Fatal("activity not merged", pending)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.Home, "data", "agent-status", "alpha", base64.RawURLEncoding.EncodeToString([]byte("task"))+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["status"] != "working" || saved["source"] != "webmux" || saved["last_output_source"] != "live" || saved["last_input_at"] != *pending.update.LastInputAt || saved["last_output_at"] != *pending.update.LastOutputAt {
		t.Fatal(saved)
	}
}

func TestAgentActivityDebouncesAndIgnoresOldLaunch(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	s, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	<-launched
	b.mu.Lock()
	old := b.entries[s.ID].run
	old.replayUntil = time.Now().Add(-time.Second)
	b.mu.Unlock()
	if _, _, err := b.EnsureAgentAttach("owner", "alpha", "other-task", 120, 40); err != nil {
		t.Fatal(err)
	}
	<-launched
	b.output(s.ID, old, "obsolete output")
	b.mu.Lock()
	if len(b.activity) != 0 {
		t.Fatal("old launch changed activity")
	}
	b.mu.Unlock()
	if err := b.Input("owner", s.ID, "live input"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.Home, "data", "agent-status", "alpha", base64.RawURLEncoding.EncodeToString([]byte("other-task"))+".json")
	eventually(t, func() bool { _, err := os.Stat(file); return err == nil })
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["last_input_at"] == nil || saved["last_output_at"] != nil {
		t.Fatal(saved)
	}
}

func configureAgents(t *testing.T, store *storage.Store, enabled bool) {
	t.Helper()
	if err := store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"agents": map[string]any{"enabled": enabled, "disable_in_multi_user_mode": false, "definitions": []any{map[string]any{"id": "alpha", "tmux_socket": "isolated-test", "workspace": "Agents"}}}}}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAttachReuseReplacementAndIsolation(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	first, created, err := b.EnsureAgentAttach("owner", "alpha", "task-a", 100, 30)
	if err != nil || !created || first.State != "connected" || first.Persistent || first.AgentSessionName != "task-a" {
		t.Fatal(first, created, err)
	}
	p1 := <-launched
	if len(b.List("owner")) != 0 {
		t.Fatal("agent leaked into terminal grid")
	}
	viewer, err := b.Join("owner", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := b.EnsureAgentAttach("owner", "alpha", "task-a", 120, 40)
	if err != nil || created || first.ID != second.ID || len(launched) != 0 {
		t.Fatal(second, created, err)
	}
	other, created, err := b.EnsureAgentAttach("other", "alpha", "task-a", 120, 40)
	if err != nil || !created || other.ID == first.ID {
		t.Fatal(other, created, err)
	}
	<-launched
	third, created, err := b.EnsureAgentAttach("owner", "alpha", "task-b", 120, 40)
	if err != nil || created || third.ID != first.ID || third.Title != "task-b" {
		t.Fatal(third, created, err)
	}
	p2 := <-launched
	select {
	case <-p1.done:
	default:
		t.Fatal("replaced process still running")
	}
	p2.output(t, "new agent output")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", first.ID); return text == "new agent output" })
	select {
	case <-viewer.Done():
		t.Fatal("reattach closed viewer")
	default:
	}
	if _, err := b.Reconnect("owner", first.ID, ""); err != nil {
		t.Fatal(err)
	}
	<-launched
	var layout map[string]any
	if err := store.ReadConfig("layout.yaml", &layout); err != nil {
		t.Fatal(err)
	}
	if len(layout["layout"].(map[string]any)["tiles"].([]any)) != 0 {
		t.Fatal(layout)
	}
}

func TestConcurrentAgentEnsureHasOnePane(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	var workers sync.WaitGroup
	ids := make(chan string, 16)
	for range 16 {
		workers.Go(func() {
			s, _, err := b.EnsureAgentAttach("owner", "alpha", "same", 120, 40)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- s.ID
		})
	}
	workers.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate agent panes")
		}
	}
	if len(launched) != 1 {
		t.Fatal("unexpected launches", len(launched))
	}
}

func TestScratchReuseDoesNotRestartForCwd(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	first, created, err := b.EnsureAgentScratch("owner", "alpha", 100, 30, t.TempDir())
	if err != nil || !created || first.AgentRole != "scratch" || first.Col != 1 || first.Title != "Scratch shell" {
		t.Fatal(first, created, err)
	}
	p := <-launched
	cwd := t.TempDir()
	second, created, err := b.EnsureAgentScratch("owner", "alpha", 110, 35, cwd)
	if err != nil || created || second.ID != first.ID || second.ExecCwd != cwd || len(launched) != 0 {
		t.Fatal(second, created, err)
	}
	p.exit()
	eventually(t, func() bool { s, _ := b.Get("owner", first.ID); return s.State == "disconnected" })
	third, created, err := b.EnsureAgentScratch("owner", "alpha", 110, 35, cwd)
	if err != nil || created || third.ID != first.ID {
		t.Fatal(third, created, err)
	}
	if len(launched) != 1 {
		t.Fatal("scratch did not relaunch")
	}
}

func TestAgentPolicyRevokesLiveSessionAndWatcherStops(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	s, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	p := <-launched
	viewer, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	configureAgents(t, store, false)
	if _, err := b.Join("owner", s.ID); err == nil {
		t.Fatal("disabled agent admitted viewer")
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err == nil {
		t.Fatal("disabled agent reconnected")
	}
	eventually(t, func() bool { _, err := b.Get("owner", s.ID); return errors.Is(err, ErrNotFound) })
	code, reason := viewer.CloseReason()
	if code != 1008 || reason != "Agent sessions are not enabled" {
		t.Fatal(code, reason)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("revoked process running")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentPolicyDiskFailureStillRevokesProcess(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	s, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	p := <-launched
	viewer, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.Home, "data", "sessions", "sessions.yaml")
	backup := file + ".saved"
	if err := os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(file); _ = os.Rename(backup, file) }()
	configureAgents(t, store, false)
	if err := b.EnforceAgentAccess(); err == nil {
		t.Fatal("lost persistence error")
	}
	code, _ := viewer.CloseReason()
	if code != 1008 {
		t.Fatal(code)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("disk failure kept revoked process running")
	}
}

func TestMalformedPolicyPreservesSavedAgentsAndBlocksNewAccess(t *testing.T) {
	b, store, _ := fixture(t)
	configureAgents(t, store, true)
	s, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ConfigPath("app.yaml"), []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.EnforceAgentAccess(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get("owner", s.ID); err != nil {
		t.Fatal("config error destroyed saved state", err)
	}
	if _, err := b.Join("owner", s.ID); err == nil {
		t.Fatal("invalid policy allowed join")
	}
}

func TestAgentFailedCreatePersistenceClosesProcess(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	file := filepath.Join(store.Home, "data", "sessions", "sessions.yaml")
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file)
	_, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
	if err == nil {
		t.Fatal("failed persistence accepted")
	}
	p := <-launched
	select {
	case <-p.done:
	default:
		t.Fatal("orphaned agent process")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) != 0 {
		t.Fatal("orphaned agent record")
	}
}
