package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func managedActionFixture(t *testing.T) (*Broker, *storage.Store, Session, RecoveryRequest) {
	t.Helper()
	b, store, _ := fixture(t)
	enableLogging(t, store)
	identity := RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:seat", Host: "localhost", Runtime: "claude", Workspace: t.TempDir(), NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeStateFile: filepath.Join(t.TempDir(), "state.jsonl")}
	s, err := b.EnsureRigTerminal("owner", identity.Target, "coder", []string{"attach"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	input := previewAction(t, b, s.ID, "managed")
	input.Confirm = true
	return b, store, s, input
}

func TestManagedRecoveryConsumesBeforeAttachmentAndNeverDuplicates(t *testing.T) {
	b, store, s, input := managedActionFixture(t)
	actions := 0
	plan := func(_ context.Context, owner string, identity RigIdentity) (func(context.Context) ([]string, error), error) {
		if owner != "owner" || identity.NativeSessionID != s.Rig.NativeSessionID {
			t.Fatal("wrong owner or conversation")
		}
		return func(context.Context) ([]string, error) {
			data, err := os.ReadFile(filepath.Join(store.Home, "data/sessions/sessions.yaml"))
			if err != nil || !strings.Contains(string(data), "outcome: requested") {
				t.Fatal("external start preceded durable intent", err)
			}
			actions++
			return []string{"fresh", "attach"}, nil
		}, nil
	}
	for i := 0; i < 2; i++ {
		result, err := b.RecoverManaged(context.Background(), "owner", s.ID, input, plan)
		if err != nil || result.Outcome != "consumed" {
			t.Fatal(result, err)
		}
	}
	if actions != 1 {
		t.Fatal("duplicate native start", actions)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.RecoverManaged(context.Background(), "owner", s.ID, input, plan); err != nil || actions != 1 {
		t.Fatal("consumed receipt lost across restart", actions, err)
	}
}

func TestManagedRecoveryUncertainRequestSurvivesRestartAndBlocksNewStarts(t *testing.T) {
	b, store, s, input := managedActionFixture(t)
	actions := 0
	plan := func(context.Context, string, RigIdentity) (func(context.Context) ([]string, error), error) {
		return func(context.Context) ([]string, error) {
			actions++
			return nil, errors.New("connection lost after start")
		}, nil
	}
	if _, err := b.RecoverManaged(context.Background(), "owner", s.ID, input, plan); err == nil {
		t.Fatal("uncertain start accepted")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.RecoverManaged(context.Background(), "owner", s.ID, input, plan); err == nil {
		t.Fatal("uncertain request retried")
	}
	fresh := previewAction(t, restored, s.ID, "managed")
	fresh.Confirm = true
	if _, err := restored.RecoverManaged(context.Background(), "owner", s.ID, fresh, plan); err == nil || actions != 1 {
		t.Fatal("uncertain start repeated under a new ID", actions, err)
	}
}

func TestManagedRecoveryRejectsUnapprovedForeignStaleOrChangedInputsBeforePlanning(t *testing.T) {
	b, _, s, input := managedActionFixture(t)
	plan := func(context.Context, string, RigIdentity) (func(context.Context) ([]string, error), error) {
		t.Fatal("invalid request reached provider")
		return nil, nil
	}
	if _, err := b.RecoverManaged(context.Background(), "other", s.ID, input, plan); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RecoveryRequest){
		func(v *RecoveryRequest) { v.Confirm = false }, func(v *RecoveryRequest) { v.Context = "unapproved output" }, func(v *RecoveryRequest) { v.Revision++ }, func(v *RecoveryRequest) { v.Instance = "other-profile" }, func(v *RecoveryRequest) { v.Kind = "native" },
	} {
		altered := input
		mutate(&altered)
		if _, err := b.RecoverManaged(context.Background(), "owner", s.ID, altered, plan); err == nil {
			t.Fatal("invalid input accepted", altered)
		}
	}
}

func TestManagedRecoveryRechecksRevisionAfterReadOnlyPlan(t *testing.T) {
	b, _, s, input := managedActionFixture(t)
	plan := func(context.Context, string, RigIdentity) (func(context.Context) ([]string, error), error) {
		b.mu.Lock()
		b.entries[s.ID].value.Recovery.Revision++
		b.mu.Unlock()
		return func(context.Context) ([]string, error) { t.Fatal("stale plan started a process"); return nil, nil }, nil
	}
	if _, err := b.RecoverManaged(context.Background(), "owner", s.ID, input, plan); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal(err)
	}
}

func TestManagedRecoveryDoesNotStartWhenIntentCannotBeSaved(t *testing.T) {
	b, store, s, input := managedActionFixture(t)
	path := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	retained := path + ".retained"
	if err := os.Rename(path, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		if err := os.Rename(retained, path); err != nil {
			t.Error(err)
		}
	}()
	plan := func(context.Context, string, RigIdentity) (func(context.Context) ([]string, error), error) {
		return func(context.Context) ([]string, error) {
			t.Fatal("external start preceded durable intent")
			return nil, nil
		}, nil
	}
	if _, err := b.RecoverManaged(context.Background(), "owner", s.ID, input, plan); err == nil {
		t.Fatal("failed durable save was ignored")
	}
	view, err := b.Recovery("owner", s.ID)
	if err != nil || view.State.Intent != nil || view.State.Revision != input.Revision {
		t.Fatal("failed intent changed recovery state", view, err)
	}
}
