package coordination

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLaunchProvisionIsAtomicAcrossMembershipRefresh(t *testing.T) {
	h := fixture(t)
	inactive := append([]MemberInput(nil), h.inputs...)
	inactive[0].Active = false
	if err := h.s.Sync(testNode, inactive); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Provision(testNode, h.inputs[0]); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("control must refuse issuance from stale inactive membership", err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	clock := h.s.Now
	h.s.Now = func() time.Time {
		once.Do(func() { close(entered); <-release })
		return clock()
	}
	type outcome struct {
		token string
		err   error
	}
	result, done := make(chan outcome, 1), make(chan struct{})
	go func() {
		defer close(done)
		token, err := h.s.ProvisionLaunch(testNode, h.inputs[0])
		result <- outcome{token, err}
	}()
	locked := false
	var refreshed chan error
	defer func() {
		unblock()
		if locked {
			h.s.mu.Unlock()
		}
		<-done
		if refreshed != nil {
			<-refreshed
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provision transaction did not start")
	}
	// Once database() returned, the transaction must finish without another
	// Service lock acquisition. A split Sync/Provision would block here, giving
	// a concurrent refresh an opportunity to invalidate the pending member.
	h.s.mu.Lock()
	locked = true
	refreshed = make(chan error, 1)
	go func() { defer close(refreshed); refreshed <- h.s.Sync(testNode, inactive) }()
	unblock()
	var granted outcome
	select {
	case granted = <-result:
		if granted.err != nil || granted.token == "" {
			t.Fatal("atomic launch issuance failed", granted.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("launch issuance reacquired the service lock between transactions")
	}
	h.s.mu.Unlock()
	locked = false
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	refreshed = nil
	// Issuance is not admission: a later inactive broker observation still
	// refuses every operation until a current live snapshot is supplied.
	if _, err := h.s.Subject(testNode, granted.token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("pending capability acquired live authority", err)
	}
	if _, err := h.s.AgentView(testNode, granted.token, inactive, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("inactive member read project context", err)
	}
	if _, err := h.s.Execute(testNode, granted.token, inactive, Request{ID: "pending-work", Action: "work", Text: "must be refused"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("inactive member acquired work authority", err)
	}
	if err := h.s.Sync(testNode, h.inputs); err != nil {
		t.Fatal(err)
	}
	if ref, err := h.s.Subject(testNode, granted.token); err != nil || ref != h.inputs[0].Ref {
		t.Fatal("live membership did not admit its issued capability", err)
	}
}

func TestAtomicLaunchProvisionRetainsAuthorityGuards(t *testing.T) {
	for _, control := range []string{"disabled", "revoked", "invalid", "foreign-node"} {
		t.Run(control, func(t *testing.T) {
			h := fixture(t)
			input := h.inputs[0]
			want := ErrUnauthorized
			switch control {
			case "disabled":
				if err := h.s.SetEnabled(testNode, false); err != nil {
					t.Fatal(err)
				}
				want = ErrDisabled
			case "revoked":
				if err := h.s.Sync(testNode, nil); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				input.Ref.Session = ""
				want = ErrInvalid
			case "foreign-node":
				input.Ref.Node = strings.Repeat("b", 32)
				want = ErrInvalid
			}
			token, err := h.s.ProvisionLaunch(testNode, input)
			if token != "" || !errors.Is(err, want) {
				t.Fatalf("%s guard: token present=%v err=%v", control, token != "", err)
			}
		})
	}
}

func TestLaunchProvisionDoesNotWithdrawUnrelatedMembers(t *testing.T) {
	h := fixture(t)
	pending := h.inputs[0]
	pending.Ref.Session = "pending"
	pending.Ref.Incarnation = strings.Repeat("5", 32)
	token, err := h.s.ProvisionLaunch(testNode, pending)
	if err != nil || token == "" {
		t.Fatal("pending interface was not issued", err)
	}
	for i, existing := range h.tokens {
		if ref, err := h.s.Subject(testNode, existing); err != nil || ref != h.inputs[i].Ref {
			t.Fatal("a partial launch observation withdrew another participant", err)
		}
	}
	if _, err := h.s.Subject(testNode, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("launch input's active flag granted pending authority", err)
	}
	// A participant absent from the authoritative broker inventory cannot act
	// and its withdrawn incarnation cannot be revived by a late launch result.
	if _, err := h.s.AgentView(testNode, token, h.inputs, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unregistered participant acquired authority", err)
	}
	if value, err := h.s.ProvisionLaunch(testNode, pending); value != "" || !errors.Is(err, ErrUnauthorized) {
		t.Fatal("withdrawn participant was revived", err)
	}
}
