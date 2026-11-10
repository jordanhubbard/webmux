package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func sendRecovery(t *testing.T, b *Broker, id string, p *fakeProcess, value recovery.Event) {
	t.Helper()
	b.mu.Lock()
	key := b.entries[id].value.RecoveryKey
	b.mu.Unlock()
	packet, err := recovery.SignedPacket(value, key)
	if err != nil {
		t.Fatal(err)
	}
	p.output(t, packet)
}

func TestRecoveryEvidenceSurvivesWithoutBrokerShutdown(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "previous work 😀\n")
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "command", Instance: "original-shell", Cwd: "/workspace/project", Command: "codex resume previous"})
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "cli", Instance: "original-shell", Cwd: "/workspace/project", CLI: "codex", Version: "0.167.0", SessionID: "native-session", Argv: []string{"codex", "resume", "previous"}, Configuration: &recovery.NativeConfiguration{Roots: map[string]string{"CODEX_HOME": "/original-native-state"}}})
	eventually(t, func() bool {
		v, _ := b.Recovery("owner", s.ID)
		return v.State != nil && v.State.Activity != nil && v.State.Activity.NativeSessionID == "native-session" && strings.Contains(v.History, "previous work 😀")
	})
	// Read the persisted state while the first broker is still running. No
	// graceful Close, logger drain, or shutdown checkpoint is used as evidence.
	next, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	v, err := next.Recovery("owner", s.ID)
	if err != nil || v.State == nil || !v.State.Pending || v.State.Activity.NativeSessionID != "native-session" || !v.HistoryAvailable {
		t.Fatalf("lost persisted recovery evidence: %+v %v", v, err)
	}
	if v.State.Activity.Configuration == nil || v.State.Activity.Configuration.Roots["CODEX_HOME"] != "/original-native-state" {
		t.Fatal("lost original CLI configuration before broker shutdown")
	}
	if _, err := next.Recovery("other", s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("recovery crossed owner boundary")
	}
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(text, recovery.Prefix) || strings.Contains(text, "native-session") {
		t.Fatal("metadata entered terminal output")
	}
	if err := os.Remove(filepath.Join(store.Home, "logs", "sessions", v.State.Transcripts[0])); err != nil {
		t.Fatal(err)
	}
	v, err = next.Recovery("owner", s.ID)
	if err != nil || v.HistoryAvailable || v.State.Activity.NativeSessionID != "native-session" {
		t.Fatal("rotated log destroyed native recovery metadata")
	}
}

func TestPausedLoggingDoesNotCaptureCommandsAndNewShellCannotCompleteOldActivity(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	s := create(t, b, "owner")
	p := <-launched
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "command", Instance: "before-reboot", Cwd: "/workspace", Command: "long-running-work"})
	eventually(t, func() bool { s, _ := b.Get("owner", s.ID); return s.Recovery != nil && s.Recovery.Activity != nil })
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "complete", Instance: "different-shell"})
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "shell", Instance: "different-shell", Cwd: "/new-shell"})
	eventually(t, func() bool { v, _ := b.Recovery("owner", s.ID); return v.ShellReady })
	v, _ := b.Recovery("owner", s.ID)
	if !v.State.Activity.Running || v.State.Activity.Command != "long-running-work" {
		t.Fatal("fresh shell overwrote interrupted activity")
	}
	viewer, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ToggleTranscript("owner", s.ID, viewer.ID); err != nil {
		t.Fatal(err)
	}
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "command", Instance: "different-shell", Command: "private-unrecorded-command"})
	p.output(t, "paused-output-marker\n")
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.Contains(text, "paused-output-marker")
	})
	v, _ = b.Recovery("owner", s.ID)
	if v.State.Activity.Command != "long-running-work" {
		t.Fatal("paused logging captured activity")
	}
}
