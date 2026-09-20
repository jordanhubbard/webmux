package session

import (
	"errors"
	"testing"
)

func TestSlaveResetClosesAllOwnersAndAgentPanes(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	first := create(t, b, "owner")
	second := create(t, b, "other")
	if _, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 80, 24); err != nil {
		t.Fatal(err)
	}
	processes := []*fakeProcess{<-launched, <-launched, <-launched}
	viewer, err := b.Join("owner", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	console, err := b.ResetForSlave("console.local", 1234)
	if err != nil {
		t.Fatal(err)
	}
	if console.Owner != "system" || console.Username != "console" || console.Transport != "exec" || console.Title != "console.local:1234" || console.Port != 1234 || console.Row != 0 || console.Col != 0 {
		t.Fatal(console)
	}
	for _, old := range []Session{first, second} {
		if _, err := b.Get(old.Owner, old.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("old terminal survived", err)
		}
	}
	for _, p := range processes {
		select {
		case <-p.done:
		default:
			t.Fatal("old process survived")
		}
	}
	select {
	case <-viewer.Done():
	default:
		t.Fatal("old viewer survived")
	}
	var saved document
	if err := store.ReadSessions(&saved); err != nil || len(saved.Sessions) != 1 || saved.Sessions[0].ID != console.ID {
		t.Fatal(saved, err)
	}
	(<-launched).output(t, "console ready")
	eventually(t, func() bool { value, _ := b.Get("system", console.ID); return value.State == "connected" })
	if _, err := b.ResetForSlave("console.local", -1); err == nil {
		t.Fatal("invalid port accepted")
	}
	if _, err := b.Get("system", console.ID); err != nil {
		t.Fatal("invalid configuration deleted console", err)
	}
	zero, err := b.ResetForSlave("console.local", 0)
	if err != nil || zero.Port != 22 {
		t.Fatal(zero, err)
	}
}
