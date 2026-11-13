package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/screenstate"
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

func TestWindowsNativeHistoryPrecedesSnapshotAndViewerReset(t *testing.T) {
	b, store, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	const old = "OLD_HISTORY_MARKER\nSECOND_HISTORY_LINE"
	name := "session-native-history.log"
	dir := filepath.Join(store.Home, "logs", "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	e := b.entries[s.ID]
	e.value.Cols, e.value.Rows = 40, 10
	e.value.Recovery = &recovery.State{Version: 1, Pending: false, Transcripts: []string{name}}
	key := e.value.RecoveryKey
	b.mu.Unlock()
	viewer, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	take := func() Event {
		select {
		case <-viewer.Ready():
			return viewer.Take()
		default:
			return nil
		}
	}
	for take() != nil {
	}
	const before = "\x1b[2J\x1b[HNATIVE_WAIT_READY"
	const future = "\x1b[2;1HCURRENT_OUTPUT"
	screen, err := screenstate.New(40, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer screen.Close()
	if err = screen.Feed([]byte(before)); err != nil {
		t.Fatal(err)
	}
	image, err := screen.Capture()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := image.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(snapshot, []byte("\x18\x1bc")) {
		t.Fatal("provider snapshot reset contract changed")
	}
	// The local application bridge removes this one reset because the viewer
	// reset/status precedes plain history and the native snapshot.
	snapshot = bytes.TrimPrefix(snapshot, []byte("\x18\x1bc"))
	provider, err := recovery.SignedPacket(recovery.Event{Kind: "provider", Platform: "windows", Mode: "waiting"}, key)
	if err != nil {
		t.Fatal(err)
	}
	p.output(t, provider+string(snapshot))
	sendRecovery(t, b, s.ID, p, recovery.Event{Kind: "control", Platform: "windows", Mode: "waiting", Instance: "new-worker"})
	p.output(t, future)
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return strings.Contains(text, "CURRENT_OUTPUT") })
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Count(text, "OLD_HISTORY_MARKER") != 1 || !strings.Contains(text, "OLD_HISTORY_MARKER\r\nSECOND_HISTORY_LINE") {
		t.Fatal("history was duplicated or lost line boundaries")
	}
	if strings.Index(text, "OLD_HISTORY_MARKER") > strings.Index(text, "NATIVE_WAIT_READY") {
		t.Fatal("history arrived after the native snapshot")
	}
	connected, history, snapshotSeen := -1, -1, -1
	for index := 0; ; index++ {
		event := take()
		if event == nil {
			break
		}
		if event["type"] == "status" && event["state"] == "connected" {
			connected = index
		}
		if data, ok := event["data"].(string); ok {
			if strings.Contains(data, "OLD_HISTORY_MARKER") {
				history = index
			}
			if strings.Contains(data, "NATIVE_WAIT_READY") {
				snapshotSeen = index
			}
		}
	}
	if connected < 0 || history <= connected || snapshotSeen < history {
		t.Fatalf("viewer reset/history/snapshot order: %d %d %d", connected, history, snapshotSeen)
	}
	if target := os.Getenv("WEBMUX_RECOVERY_HISTORY_CAPTURE"); target != "" {
		data, err := json.Marshal(map[string]any{"cols": 40, "rows": 10, "original": before + future, "snapshot": text, "marker": "OLD_HISTORY_MARKER"})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
