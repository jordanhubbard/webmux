package recovery

import (
	"context"
	"testing"
	"time"
)

func TestWaitingSocketRPCConsumesExactlyOneIntent(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := serveWaiting(ctx, listener, testHostSession, testHostKey, "current-worker", preflightAction)
		_ = listener.Close()
		finished <- err
	}()
	response := HostRPC(ctx, request)
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatal(response)
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
