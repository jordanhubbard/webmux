package recovery

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitingSocketRPCConsumesExactlyOneIntent(t *testing.T) {
	testWaitingSocketRPC(t, 0)
}

func TestWaitingSocketRPCAllowsSlowPreparationWithinProductionBudget(t *testing.T) {
	// Real native validation may take longer than the old fixture's five
	// seconds while remaining inside the production 25-second RPC budget.
	testWaitingSocketRPC(t, 6*time.Second)
}

func testWaitingSocketRPC(t *testing.T, delay time.Duration) {
	t.Helper()
	request := waitingFixture(t)
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	state, err := updateHostState(testHostSession, testHostKey, false, func(state *HostState) error { state.Control.Endpoint = endpoint; return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.Action.Revision = state.Revision
	ctx, cancel := context.WithTimeout(context.Background(), recoveryWorkerTimeout+5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	var phase atomic.Value
	phase.Store("waiting for request")
	started := time.Now()
	go func() {
		_, err := serveWaiting(ctx, listener, testHostSession, testHostKey, "current-worker", func(state HostState, action RecoveryAction) (preparedAction, error) {
			phase.Store("preparing action")
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return preparedAction{}, ctx.Err()
				}
			}
			prepared, err := preflightAction(state, action)
			phase.Store("prepared; awaiting durable consumption and reply")
			return prepared, err
		})
		phase.Store("worker finished")
		_ = listener.Close()
		finished <- err
	}()
	response := HostRPC(ctx, request)
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatalf("socket response after %s, phase=%s: %+v", time.Since(started), phase.Load(), response)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	// The original worker no longer exists. A reconnect still sees consumption
	// and cannot submit the request to a replacement actor or a shell prompt.
	response = HostRPC(ctx, request)
	if response.Error != "" || response.Outcome != "consumed" || len(hostSnapshot(t).Consumed) != 1 {
		t.Fatal(response)
	}
}

func TestWaitingSocketCancellationKeepsConsumedReceiptAfterLostReply(t *testing.T) {
	request := waitingFixture(t)
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	state, err := updateHostState(testHostSession, testHostKey, false, func(s *HostState) error { s.Control.Endpoint = endpoint; return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.Action.Revision = state.Revision
	workerContext, cancelWorker := context.WithTimeout(context.Background(), recoveryWorkerTimeout+5*time.Second)
	defer cancelWorker()
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := serveWaiting(workerContext, listener, testHostSession, testHostKey, "current-worker", func(state HostState, action RecoveryAction) (preparedAction, error) {
			close(entered)
			select {
			case <-release:
				return preflightAction(state, action)
			case <-workerContext.Done():
				return preparedAction{}, workerContext.Err()
			}
		})
		_ = listener.Close()
		finished <- err
	}()
	clientContext, cancelClient := context.WithCancel(workerContext)
	defer cancelClient()
	response := make(chan RPCResponse, 1)
	go func() { response <- HostRPC(clientContext, request) }()
	select {
	case <-entered:
	case <-workerContext.Done():
		t.Fatal("worker did not enter preparation")
	}
	cancelClient()
	select {
	case reply := <-response:
		if reply.Outcome != "uncertain" || !strings.Contains(reply.Error, "canceled") {
			t.Fatalf("cancelled request lost uncertainty: %+v", reply)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not wake the established socket read")
	}
	close(release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-workerContext.Done():
		t.Fatal("worker did not finish its approved action")
	}
	// The approved intent may commit after its caller stops waiting. Its lost
	// reply must not let a reconnect execute it again.
	reply := HostRPC(context.Background(), request)
	if reply.Error != "" || reply.Outcome != "consumed" || len(hostSnapshot(t).Consumed) != 1 {
		t.Fatalf("lost reply permitted another execution: %+v", reply)
	}
}
