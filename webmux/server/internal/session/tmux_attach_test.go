package session

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestExternalTmuxAttachAcceptsOnlyReattach(t *testing.T) {
	for _, argv := range [][]string{
		{"tmux", "attach-session", "-t", "kept"},
		{"/usr/bin/tmux", "attach", "-t", "kept"},
		{"tmux", "-L", "webmux", "a", "-t", "kept"},
	} {
		if !ExternalTmuxAttach(argv) {
			t.Fatalf("rejected reattach %q", argv)
		}
	}
	for _, argv := range [][]string{
		{"agent-deck", "session", "attach", "deck"},
		{"ssh", "host", "tmux", "attach-session", "-t", "kept"},
		{"tmux", "new-session", "-s", "kept"},
		{"tmux", "kill-session", "-t", "kept"},
		{"tmux", "send-keys", "-t", "kept", "exit", "Enter"},
		{"tmux"},
		nil,
	} {
		if ExternalTmuxAttach(argv) {
			t.Fatalf("accepted %q", argv)
		}
	}
}

func TestRestoreReattachesSavedTmuxAndLeavesOtherManagersStopped(t *testing.T) {
	b, store, _ := fixture(t)
	kept := create(t, b, "owner")
	deck := create(t, b, "owner")
	b.mu.Lock()
	b.entries[kept.ID].value.ExecArgv = []string{"/usr/bin/tmux", "attach-session", "-t", "kept"}
	b.entries[kept.ID].value.Rig = &RigIdentity{Provider: "rig", Target: "kept", Project: "Canonical", Name: "worker", Host: "localhost", Runtime: "shell"}
	b.entries[deck.ID].value.ExecArgv = []string{"agent-deck", "session", "attach", "deck"}
	b.entries[deck.ID].value.Rig = &RigIdentity{Provider: "agent-deck", Target: "agent-deck:deck", Project: "Deck", Name: "seat", Host: "localhost", Runtime: "shell"}
	if err := b.persistLocked(); err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	b.mu.Unlock()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	var launched []terminal.LaunchRequest
	next, err := newBroker(store, slog.New(slog.NewTextHandler(io.Discard, nil)), func(request terminal.LaunchRequest, _ string) (process, error) {
		launched = append(launched, request)
		return fake(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 1 || !ExternalTmuxAttach(launched[0].ExecArgv) || launched[0].ExecArgv[3] != "kept" {
		t.Fatalf("restore launched %v", launched)
	}
	if _, err := next.Get("owner", deck.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Reconnect("owner", deck.ID, ""); err == nil || len(launched) != 1 {
		t.Fatalf("agent-deck reconnect launched or succeeded: %v %d", err, len(launched))
	}
	if _, err := next.Get("owner", deck.ID); err != nil {
		t.Fatal("rejected reconnect removed the saved viewer")
	}
	if _, err := next.Reconnect("owner", kept.ID, ""); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 2 || !ExternalTmuxAttach(launched[1].ExecArgv) {
		t.Fatalf("tmux reconnect launched %v", launched)
	}
	view, err := next.Recovery("owner", deck.ID)
	if err != nil || view.CanResumeManaged || view.LifecycleOwner == "agent-deck" || view.LifecycleOwner == "rig" {
		t.Fatalf("recovery still offers a removed manager: %+v %v", view, err)
	}
}
