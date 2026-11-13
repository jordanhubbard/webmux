package recovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitingSocketFramesRejectIncompleteOversizedAndMultipleValues(t *testing.T) {
	for _, message := range []string{
		`{"protocol":1}`,
		"{invalid}\n",
		"{\"protocol\":1}{\"protocol\":1}\n",
		strings.Repeat(" ", maxRPCBytes) + "{}\n",
	} {
		var request RPCRequest
		if err := readWorkerRPC(strings.NewReader(message), &request); err == nil {
			t.Fatal("accepted an incomplete, oversized or ambiguous worker frame")
		}
	}
}

func TestWaitingWorkerReadsCompleteRequestWithoutWriteEOF(t *testing.T) {
	request := waitingFixture(t)
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := serveWaiting(ctx, listener, testHostSession, testHostKey, "current-worker", preflightAction)
		finished <- err
	}()
	c, err := net.Dial("unix", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = c.Close()
		cancel()
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("worker fixture did not stop")
		}
	}()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(c).Encode(request); err != nil {
		t.Fatal(err)
	}
	// Keep the write side open: a complete newline-delimited request must not
	// depend on delivery of an AF_UNIX half-close before the worker acts.
	var response RPCResponse
	if err := json.NewDecoder(c).Decode(&response); err != nil {
		t.Fatalf("complete request waited for EOF: %v", err)
	}
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatal(response)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if len(hostSnapshot(t).Consumed) != 1 {
		t.Fatal("request not consumed exactly once")
	}
}

func TestWaitingClientReadsCompleteReplyWithoutReadEOF(t *testing.T) {
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	release := make(chan struct{})
	defer close(release)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var request RPCRequest
		if readRPC(c, &request) != nil {
			return
		}
		_ = json.NewEncoder(c).Encode(RPCResponse{Protocol: 1, Outcome: "consumed"})
		<-release // Simulate a delayed read EOF after a complete legacy reply.
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := callWaitingWorker(ctx, endpoint, RPCRequest{Protocol: 1})
	if err != nil || response.Outcome != "consumed" {
		t.Fatalf("complete reply waited for EOF: %+v %v", response, err)
	}
}

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
