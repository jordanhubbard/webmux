package session

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLiveTerminalRestartRejectsStaleTargetsAndDropsQueuedInput(t *testing.T) {
	b, p, s := controlFixture(t, false)
	p.mu.Lock()
	p.mode = "active"
	p.restartSelection = strings.Repeat("a", 64)
	p.revision++
	p.mu.Unlock()
	eventually(t, func() bool { v, _ := b.Recovery("owner", s.ID); return v.CanRestart && !v.Waiting })
	view, err := b.Recovery("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.Message, "history preview") {
		t.Fatal("active process implied successful recovery", view.Message)
	}
	id, _ := uuid()
	request := RecoveryRequest{ID: id, Kind: "restart", Revision: view.State.Revision, HostRevision: view.HostRevision, Instance: view.RestartSelection, Confirm: true}
	if _, err := b.Recover("other", s.ID, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("crossed owner boundary", err)
	}
	unapproved := request
	unapproved.Confirm = false
	if _, err := b.Recover("owner", s.ID, unapproved); err == nil {
		t.Fatal("unconfirmed restart")
	}
	stale := request
	stale.Revision--
	if _, err := b.Recover("owner", s.ID, stale); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("accepted stale state", err)
	}
	b.mu.Lock()
	r := b.entries[s.ID].run
	b.mu.Unlock()
	r.inputGate <- struct{}{}
	if err := b.Input("owner", s.ID, "old pending command\n"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := b.Recover("owner", s.ID, request); done <- err }()
	eventually(t, func() bool { return r.restarting.Load() })
	if err := b.Input("owner", s.ID, "new input while replacing\n"); !errors.Is(err, ErrRecoveryWaiting) {
		t.Fatal("accepted input during restart", err)
	}
	<-r.inputGate
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restart did not complete")
	}
	if result, err := b.Recover("owner", s.ID, request); err != nil || result.Outcome != "consumed" {
		t.Fatal("idempotent retry failed", result, err)
	}
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	if actions != 1 {
		t.Fatal("restart repeated", actions)
	}
	// Once the replacement snapshot arrives, fresh input is admitted. It must
	// be the first packet delivered; an old queued command must never follow it.
	eventually(t, func() bool { return !r.restarting.Load() })
	if err := b.Input("owner", s.ID, "fresh"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-p.input:
		if got != "fresh" {
			t.Fatalf("stale input reached replacement: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement did not accept input")
	}
}

func TestLiveRestartRefusesRemoteAndManagedLifecycles(t *testing.T) {
	for _, kind := range []string{"ssh", "agent", "rig", "native-owner"} {
		t.Run(kind, func(t *testing.T) {
			b, p, s := controlFixture(t, false)
			p.mu.Lock()
			p.mode = "active"
			p.revision++
			p.mu.Unlock()
			b.mu.Lock()
			e := b.entries[s.ID]
			switch kind {
			case "ssh":
				e.value.Transport = "ssh"
			case "agent":
				e.value.AgentID = "managed"
			case "rig":
				e.value.Companion = "openrig"
			case "native-owner":
				e.value.LocalOwnerManaged = true
			}
			b.mu.Unlock()
			id, _ := uuid()
			_, err := b.Recover("owner", s.ID, RecoveryRequest{ID: id, Kind: "restart", Confirm: true, Instance: strings.Repeat("a", 64)})
			if err == nil {
				t.Fatal("crossed lifecycle boundary")
			}
			p.mu.Lock()
			actions := p.actions
			p.mu.Unlock()
			if actions != 0 {
				t.Fatal("dispatched restart to another lifecycle")
			}
		})
	}
}
