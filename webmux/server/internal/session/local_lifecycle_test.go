package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type localOwnedProcess struct {
	*fakeProcess
	target terminal.LocalOwnerTarget
}

func (p *localOwnedProcess) LocalOwnerTarget() *terminal.LocalOwnerTarget { v := p.target; return &v }
func localFixture(t *testing.T) (*Broker, *storage.Store, *atomic.Int32) {
	t.Helper()
	b, store, _ := fixture(t)
	b.localOwners = true
	b.localOwnerScope = "fixture-scope"
	original := b.launch
	b.launch = func(req terminal.LaunchRequest, password string) (process, error) {
		p, err := original(req, password)
		if err != nil {
			return nil, err
		}
		return &localOwnedProcess{p.(*fakeProcess), terminal.LocalOwnerTarget{Version: 1, Scope: "fixture-scope", Session: req.SessionID, Instance: strings.Repeat("a", 32), Endpoint: "private-owner-endpoint", OwnerPID: 1234, RootPID: 1235, OwnerCreation: 42, BinarySHA256: strings.Repeat("b", 64), Replay: "screen-v3"}}, nil
	}
	calls := &atomic.Int32{}
	b.stopLocal = func(ctx context.Context, target terminal.LocalOwnerTarget, key string) error {
		var saved document
		if err := store.ReadSessions(&saved); err != nil {
			t.Error(err)
			return err
		}
		found := false
		for _, s := range saved.Sessions {
			if s.ID == target.Session {
				found = s.LocalStop != nil && s.LocalStop.Target == target && s.LocalStop.Outcome == "requested" && !s.Persistent && s.RecoveryKey == key
			}
		}
		if !found {
			t.Error("stop dispatched without its original durable intent")
		}
		calls.Add(1)
		return nil
	}
	b.inspectLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { return nil }
	return b, store, calls
}
func localSession(t *testing.T, b *Broker) Session {
	t.Helper()
	s, err := b.Create("owner", CreateRequest{Transport: "local", Hostname: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if s.LocalLifecycle == nil || s.LocalLifecycle.Selection == "" {
		t.Fatal("missing observed native owner")
	}
	return s
}
func localRequest(t *testing.T, s Session) LocalStopRequest {
	t.Helper()
	id, err := uuid()
	if err != nil {
		t.Fatal(err)
	}
	return LocalStopRequest{ID: id, Selection: s.LocalLifecycle.Selection, Mode: "stop", Confirm: true}
}
func blockSessionSave(t *testing.T, store *storage.Store) func() {
	t.Helper()
	file := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	backup := file + ".saved"
	if err := os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backup, file); err != nil {
			t.Fatal(err)
		}
	}
}
func TestLocalStopPersistsBeforeDispatchAndDuplicatesOnlyInspect(t *testing.T) {
	b, _, calls := localFixture(t)
	s := localSession(t, b)
	sibling := localSession(t, b)
	request := localRequest(t, s)
	if err := b.Delete("owner", s.ID); !errors.Is(err, ErrLocalStopRequired) {
		t.Fatalf("live job entry deleted: %v", err)
	}
	hidden := true
	if _, err := b.Patch("owner", s.ID, Patch{Minimized: &hidden}); err != nil {
		t.Fatal(err)
	}
	if len(b.List("owner")) != 2 || calls.Load() != 0 {
		t.Fatal("hide discarded access or stopped work")
	}
	if _, err := b.StopLocal(context.Background(), "other", s.ID, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign stop accepted", err)
	}
	for n := 0; n < 2; n++ {
		r, err := b.StopLocal(context.Background(), "owner", s.ID, request)
		if err != nil || r.Outcome != "stopped" {
			t.Fatal(r, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("stop dispatched more than once")
	}
	if _, err := b.Reconnect("owner", s.ID, ""); !errors.Is(err, ErrLocalStopRequired) {
		t.Fatal("stopped work re-created", err)
	}
	if err := b.Input("owner", s.ID, "unsafe\n"); !errors.Is(err, ErrLocalStopRequired) {
		t.Fatal("input accepted after stop", err)
	}
	kept, _ := b.Get("owner", sibling.ID)
	if kept.LocalLifecycle.Selection != sibling.LocalLifecycle.Selection || kept.LocalStop != nil {
		t.Fatal("sibling changed")
	}
	if err := b.Delete("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	if len(b.List("owner")) != 1 {
		t.Fatal("wrong entry removed")
	}
}
func TestLocalStopFailedSaveDoesNotDispatchOrFreezeInput(t *testing.T) {
	b, store, calls := localFixture(t)
	s := localSession(t, b)
	restore := blockSessionSave(t, store)
	_, err := b.StopLocal(context.Background(), "owner", s.ID, localRequest(t, s))
	restore()
	if err == nil || calls.Load() != 0 {
		t.Fatal("stop happened before persistence")
	}
	value, _ := b.Get("owner", s.ID)
	if value.LocalStop != nil || !value.Persistent {
		t.Fatal("failed save changed lifecycle")
	}
	if err := b.Input("owner", s.ID, "safe\n"); err != nil {
		t.Fatal("failed save froze terminal", err)
	}
}
func TestLocalStopUncertaintySurvivesReconstructionWithoutLaunching(t *testing.T) {
	b, store, calls := localFixture(t)
	s := localSession(t, b)
	request := localRequest(t, s)
	b.stopLocal = func(context.Context, terminal.LocalOwnerTarget, string) error {
		calls.Add(1)
		return errors.New("lost stop reply")
	}
	r, err := b.StopLocal(context.Background(), "owner", s.ID, request)
	if err != nil || r.Outcome != "uncertain" {
		t.Fatal(r, err)
	}
	b.inspectLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { return errors.New("still running") }
	if _, err := b.StopLocal(context.Background(), "owner", s.ID, request); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("ordinary duplicate resent stop")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	next, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { launches.Add(1); return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	next.localOwnerScope = "fixture-scope"
	for _, e := range next.entries {
		e.value.LocalOwnerAvailable = true
	}
	next.stopLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { calls.Add(1); return nil }
	next.inspectLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { return nil }
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 0 {
		t.Fatal("pending stop restored a provider")
	}
	request.Mode = "inspect"
	request.Confirm = false
	r, err = next.StopLocal(context.Background(), "owner", s.ID, request)
	if err != nil || r.Outcome != "stopped" || calls.Load() != 1 {
		t.Fatal("inspection dispatched or lost original", r, err)
	}
}
func TestLocalStopConcurrentRequestsAndContinuationStayOriginal(t *testing.T) {
	b, _, calls := localFixture(t)
	s := localSession(t, b)
	request := localRequest(t, s)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	b.stopLocal = func(context.Context, terminal.LocalOwnerTarget, string) error {
		calls.Add(1)
		close(entered)
		<-release
		return errors.New("uncertain")
	}
	go func() { _, err := b.StopLocal(context.Background(), "owner", s.ID, request); done <- err }()
	<-entered
	r, err := b.StopLocal(context.Background(), "owner", s.ID, request)
	if err != nil || r.Outcome != "uncertain" || calls.Load() != 1 {
		t.Fatal(r, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.Selection = strings.Repeat("c", 64)
	if _, err := b.StopLocal(context.Background(), "owner", s.ID, changed); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("changed target accepted", err)
	}
	b.stopLocal = func(_ context.Context, target terminal.LocalOwnerTarget, _ string) error {
		if target != *s.LocalOwner {
			t.Error("continuation changed target")
		}
		calls.Add(1)
		return nil
	}
	request.Mode = "continue"
	r, err = b.StopLocal(context.Background(), "owner", s.ID, request)
	if err != nil || r.Outcome != "stopped" || calls.Load() != 2 {
		t.Fatal(r, err)
	}
}
func TestLocalStopRejectsOtherLifecycleOwnersAndPrivateMetadata(t *testing.T) {
	b, _, calls := localFixture(t)
	s := localSession(t, b)
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), "private-owner-endpoint") || strings.Contains(string(data), "owner_creation") || strings.Contains(string(data), "recovery_key") {
		t.Fatal("private owner metadata escaped")
	}
	for _, edit := range []func(*Session){func(v *Session) { v.Rig = &RigIdentity{} }, func(v *Session) { v.AgentID = "partial-manager" }, func(v *Session) { v.Transport = "ssh" }, func(v *Session) { v.Companion = "openrig" }} {
		b.mu.Lock()
		old := b.entries[s.ID].value
		edit(&b.entries[s.ID].value)
		b.mu.Unlock()
		if _, err := b.StopLocal(context.Background(), "owner", s.ID, localRequest(t, s)); err == nil {
			t.Fatal("another lifecycle owner stopped")
		}
		b.mu.Lock()
		b.entries[s.ID].value = old
		b.mu.Unlock()
	}
	if calls.Load() != 0 {
		t.Fatal("other lifecycle dispatch")
	}
}
func TestLocalCreationBanksKeyBeforeLaunchAndRetainsPostLaunchSaveFailure(t *testing.T) {
	b, store, _ := localFixture(t)
	original := b.launch
	var restore func()
	var target terminal.LaunchRequest
	b.launch = func(req terminal.LaunchRequest, password string) (process, error) {
		target = req
		var saved document
		if err := store.ReadSessions(&saved); err != nil {
			t.Fatal(err)
		}
		if len(saved.Sessions) != 1 || saved.Sessions[0].RecoveryKey != req.RecoveryKey || !saved.Sessions[0].LocalOwnerManaged {
			t.Fatal("job launched before recoverable entry/key")
		}
		p, err := original(req, password)
		restore = blockSessionSave(t, store)
		return p, err
	}
	s, err := b.Create("owner", CreateRequest{Transport: "local", Hostname: "localhost"})
	if err == nil || s.ID == "" || len(b.List("owner")) != 1 {
		t.Fatal("post-launch save failure lost access", s, err)
	}
	_ = b.Close()
	restore()
	var saved document
	if err := store.ReadSessions(&saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Sessions) != 1 || saved.Sessions[0].ID != target.SessionID || saved.Sessions[0].RecoveryKey != target.RecoveryKey {
		t.Fatal("original provider cannot be recovered")
	}
}
func TestLocalCreationFailedInitialSaveStartsNothing(t *testing.T) {
	b, store, _ := localFixture(t)
	var calls atomic.Int32
	b.launch = func(terminal.LaunchRequest, string) (process, error) { calls.Add(1); return fake(), nil }
	file := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := b.Create("owner", CreateRequest{Transport: "local", Hostname: "localhost"})
	if err == nil || calls.Load() != 0 || len(b.List("owner")) != 0 {
		t.Fatal("unbanked job started", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStopPostDispatchSaveFailureReconcilesOriginalWithoutRetry(t *testing.T) {
	b, store, calls := localFixture(t)
	s := localSession(t, b)
	request := localRequest(t, s)
	var restore func()
	b.stopLocal = func(context.Context, terminal.LocalOwnerTarget, string) error {
		calls.Add(1)
		restore = blockSessionSave(t, store)
		return nil
	}
	if _, err := b.StopLocal(context.Background(), "owner", s.ID, request); err == nil {
		t.Fatal("failed result save reported success")
	}
	_ = b.Close()
	restore()
	var launches atomic.Int32
	next, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { launches.Add(1); return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	next.localOwnerScope = "fixture-scope"
	for _, e := range next.entries {
		e.value.LocalOwnerAvailable = true
	}
	next.stopLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { calls.Add(1); return nil }
	next.inspectLocal = func(context.Context, terminal.LocalOwnerTarget, string) error { return nil }
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	result, err := next.StopLocal(context.Background(), "owner", s.ID, request)
	if err != nil || result.Outcome != "stopped" || calls.Load() != 1 || launches.Load() != 0 {
		t.Fatal("reconstruction retried or retargeted", result, err)
	}
}

func TestLocalStopStaleSelectionCannotStopReconnectedOwner(t *testing.T) {
	b, _, calls := localFixture(t)
	s := localSession(t, b)
	request := localRequest(t, s)
	b.mu.Lock()
	target := *b.entries[s.ID].value.LocalOwner
	target.OwnerCreation++
	b.entries[s.ID].value.LocalOwner = &target
	b.mu.Unlock()
	if _, err := b.StopLocal(context.Background(), "owner", s.ID, request); !errors.Is(err, ErrRecoveryConflict) || calls.Load() != 0 {
		t.Fatal("stale selection stopped new owner", err)
	}
}

func TestLocalOwnerForeignHostAndFailedReattachRetainAccess(t *testing.T) {
	b, _, calls := localFixture(t)
	s := localSession(t, b)
	request := localRequest(t, s)
	var launches atomic.Int32
	original := b.launch
	b.launch = func(r terminal.LaunchRequest, password string) (process, error) {
		launches.Add(1)
		return original(r, password)
	}
	b.localOwnerScope = "different-host-account"
	if _, err := b.Reconnect("owner", s.ID, ""); err == nil || launches.Load() != 0 {
		t.Fatal("foreign host launched replacement", err)
	}
	if err := b.Delete("owner", s.ID); !errors.Is(err, ErrLocalStopRequired) {
		t.Fatal("foreign job access record discarded", err)
	}
	b.localOwnerScope = "fixture-scope"
	b.launch = func(terminal.LaunchRequest, string) (process, error) {
		return nil, errors.New("attachment outcome unconfirmed")
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err == nil {
		t.Fatal("failed attachment accepted")
	}
	if _, err := b.StopLocal(context.Background(), "owner", s.ID, request); err == nil || calls.Load() != 0 {
		t.Fatal("old target authorized stop after uncertain reattachment", err)
	}
	if _, err := b.Get("owner", s.ID); err != nil {
		t.Fatal("failed attachment lost access", err)
	}
}
