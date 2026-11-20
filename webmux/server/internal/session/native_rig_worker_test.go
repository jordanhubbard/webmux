package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func awaitNativeWorker(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("native recovery worker did not reach barrier")
	}
}

func TestNativeRecoveryWorkerBanksBeforePendingAndDoesNotReplayWhileBusy(t *testing.T) {
	b, store, value, input := nativeRigFixture(t)
	preparing, allowPrepare, executing := make(chan struct{}), make(chan struct{}), make(chan struct{})
	workerContext := make(chan context.Context, 1)
	var calls atomic.Int32
	operation := func(ctx context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		calls.Add(1)
		if prepare {
			close(preparing)
			select {
			case <-allowPrepare:
			case <-ctx.Done():
				return NativeRigOperationResult{}, ctx.Err()
			}
			return NativeRigOperationResult{State: "prepared"}, nil
		}
		data, err := os.ReadFile(filepath.Join(store.Home, "data/sessions/sessions.yaml"))
		if err != nil || !strings.Contains(string(data), "outcome: requested") || !strings.Contains(string(data), "managed_request:") {
			t.Error("provider entered before durable original request", err)
		}
		workerContext <- ctx
		close(executing)
		<-ctx.Done()
		return NativeRigOperationResult{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		result, err := b.BeginRigNativeRecovery(ctx, "owner", value.ID, input, operation)
		if err != nil || result.Outcome != "uncertain" {
			t.Error(result, err)
		}
	}()
	awaitNativeWorker(t, preparing)
	select {
	case <-returned:
		t.Fatal("pending acknowledged before preparation and persistence")
	default:
	}
	close(allowPrepare)
	awaitNativeWorker(t, returned)
	awaitNativeWorker(t, executing)
	cancel() // Losing the HTTP connection must not cancel the banked work.
	if work := <-workerContext; work.Err() != nil {
		t.Fatal("HTTP cancellation killed banked work", work.Err())
	}
	for _, inspect := range []bool{false, true} {
		duplicate := input
		duplicate.Inspect = inspect
		result, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, duplicate, operation)
		if err != nil || result.Outcome != "uncertain" {
			t.Fatal(result, err)
		}
	}
	changed := input
	changed.Revision++
	if _, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, changed, operation); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("changed request accepted", err)
	}
	if _, err := b.BeginRigNativeRecovery(context.Background(), "foreign", value.ID, input, operation); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner accepted", err)
	}
	previewDone := make(chan struct{})
	go func() {
		defer close(previewDone)
		if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) {
			t.Error("pending request was recaptured")
			return RigIdentity{}, nil
		}); err != nil {
			t.Error(err)
		}
	}()
	awaitNativeWorker(t, previewDone)
	if calls.Load() != 2 {
		t.Fatal("duplicate invoked native provider", calls.Load())
	}
	closed := make(chan struct{})
	go func() { defer close(closed); _ = b.Close() }()
	awaitNativeWorker(t, closed) // Close must cancel and drain the owned worker.
}

func TestNativeRecoveryWorkerRequiresFreshExplicitConfirmation(t *testing.T) {
	b, _, value, input := nativeRigFixture(t)
	operation := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if prepare {
			return NativeRigOperationResult{State: "prepared"}, nil
		}
		return NativeRigOperationResult{State: "confirmed", Argv: []string{"stale-cached-attachment"}}, nil
	}
	if _, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, input, operation); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	job := b.nativeRecoveryJobs[value.ID]
	b.mu.Unlock()
	awaitNativeWorker(t, job.done)
	view, err := b.Recovery("owner", value.ID)
	if err != nil || view.ManagedOriginalRequest == nil || view.State.Intent.Outcome != "uncertain" {
		t.Fatal("background worker consumed original", view, err)
	}
	checks := 0
	refuse := func(_ context.Context, _ string, _ RigIdentity, request RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		checks++
		if prepare || !request.Inspect || request.ID != input.ID {
			t.Error("check did not preserve original request")
		}
		return NativeRigOperationResult{State: "uncertain", Code: "native_original_changed"}, nil
	}
	result, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, *view.ManagedOriginalRequest, refuse)
	if err != nil || result.Outcome != "uncertain" || checks != 1 {
		t.Fatal("cached confirmation bypassed live refusal", result, err, checks)
	}
	confirm := func(_ context.Context, _ string, _ RigIdentity, request RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		checks++
		return NativeRigOperationResult{State: "confirmed", Argv: []string{"fresh-original-attachment"}}, nil
	}
	result, err = b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, *view.ManagedOriginalRequest, confirm)
	if err != nil || result.Outcome != "consumed" || checks != 2 {
		t.Fatal(result, err, checks)
	}
}

func TestNativeRecoveryWorkerPreparationFailureNeverDispatchesOrAcknowledges(t *testing.T) {
	b, _, value, input := nativeRigFixture(t)
	want := errors.New("native preparation refused")
	operation := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if !prepare {
			t.Error("failed preparation dispatched")
		}
		return NativeRigOperationResult{}, want
	}
	result, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, input, operation)
	if !errors.Is(err, want) || result.Outcome != "" {
		t.Fatal("preparation failure acknowledged as pending", result, err)
	}
	view, err := b.Recovery("owner", value.ID)
	if err != nil || view.ManagedOriginalRequest != nil {
		t.Fatal("unbanked request became recovery authority", view, err)
	}
}

func TestNativeRecoveryWorkerPersistenceFailureNeverDispatchesOrAcknowledges(t *testing.T) {
	b, store, value, input := nativeRigFixture(t)
	path := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	operation := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if !prepare {
			t.Error("failed persistence dispatched")
			return NativeRigOperationResult{}, nil
		}
		if err := os.Rename(path, path+".retained"); err != nil {
			return NativeRigOperationResult{}, err
		}
		if err := os.Mkdir(path, 0700); err != nil {
			return NativeRigOperationResult{}, err
		}
		return NativeRigOperationResult{State: "prepared"}, nil
	}
	result, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, input, operation)
	if err == nil || result.Outcome != "" {
		t.Fatal("failed persistence acknowledged", result, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".retained", path); err != nil {
		t.Fatal(err)
	}
	view, err := b.Recovery("owner", value.ID)
	if err != nil || view.State.Intent != nil || view.State.Revision != input.Revision {
		t.Fatal("persistence failure changed recovery authority", view, err)
	}
}

func TestNativeRecoveryWorkerKeepsOriginalDeadlineAndRefusesClosedBroker(t *testing.T) {
	b, _, value, input := nativeRigFixture(t)
	deadline := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	operation := func(work context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		actual, ok := work.Deadline()
		if !ok || !actual.Equal(deadline) {
			t.Error("worker extended original request deadline", actual, deadline)
		}
		if prepare {
			return NativeRigOperationResult{State: "prepared"}, nil
		}
		<-work.Done()
		return NativeRigOperationResult{}, work.Err()
	}
	if _, err := b.BeginRigNativeRecovery(ctx, "owner", value.ID, input, operation); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.BeginRigNativeRecovery(context.Background(), "owner", value.ID, input, operation); !errors.Is(err, ErrClosed) {
		t.Fatal("closed broker accepted work", err)
	}
}

func TestNativeRecoveryPreviewDoesNotWaitForBusyLogOperation(t *testing.T) {
	b, _, value, _ := nativeRigFixture(t)
	b.mu.Lock()
	e := b.entries[value.ID]
	b.mu.Unlock()
	e.logOperation.Lock()
	defer e.logOperation.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) {
			t.Error("preview captured while operation was busy")
			return RigIdentity{}, nil
		}); err != nil {
			t.Error(err)
		}
	}()
	awaitNativeWorker(t, done)
}
