package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// Restart replaces an explicitly selected local tmux provider, never an agent,
// managed seat or arbitrary PID supplied by the browser.
func (b *Broker) restartTerminal(owner, id string, input RecoveryRequest) (RecoveryResult, error) {
	if !localStopID.MatchString(input.ID) || !input.Confirm || input.Inspect || input.Context != "" || len(input.Instance) != 64 {
		return RecoveryResult{}, invalid("review and confirm the terminal restart")
	}
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	b.mu.Unlock()
	if err != nil {
		return RecoveryResult{}, err
	}
	e.logOperation.Lock()
	defer e.logOperation.Unlock()
	b.mu.Lock()
	if b.closed || b.entries[id] != e || e.run == nil {
		b.mu.Unlock()
		return RecoveryResult{}, ErrClosed
	}
	r := e.run
	controller, ok := r.process.(recoveryController)
	if !ok || !ordinaryLocal(e.value) || e.value.LocalOwnerManaged || e.value.LocalStop != nil {
		b.mu.Unlock()
		return RecoveryResult{}, invalid("this provider's lifecycle manager owns restart")
	}
	state := e.value.Recovery
	if state == nil && input.Revision == 0 {
		state = &recovery.State{Version: 1}
		e.value.Recovery = state
	}
	data, _ := json.Marshal(input)
	digest := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", digest)
	if state != nil && state.Intent != nil && state.Intent.ID == input.ID {
		if state.Intent.Hash != hash {
			b.mu.Unlock()
			return RecoveryResult{}, ErrRecoveryConflict
		}
		if state.Intent.Outcome == "consumed" {
			b.mu.Unlock()
			return RecoveryResult{Outcome: "consumed"}, nil
		}
		// Lost acknowledgements never authorize a second destructive dispatch.
		b.mu.Unlock()
		return RecoveryResult{Outcome: "uncertain"}, nil
	}
	if r.restarting.Load() || state == nil || state.Revision != input.Revision || r.recoveryRevision != input.HostRevision || r.recoveryControl != nil && r.recoveryControl.Mode != "active" {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	previous := state.Clone()
	state.Intent = &recovery.Intent{ID: input.ID, Hash: hash, Outcome: "requested"}
	state.AutoResume = false
	state.Disposition = "restart-requested"
	state.Revision++
	if err := b.persistLocked(); err != nil {
		e.value.Recovery = previous
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	if r.recoveryControl != nil {
		r.restartOldInstance = r.recoveryControl.Instance
	}
	r.restarting.Store(true)
	r.inputEpoch.Add(1)
	r.clipboardRead = nil
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// Serialize with a PTY write already in flight. New input is refused and old
	// queued packets are invalidated by the epoch even if the writer took one.
	response := recovery.RPCResponse{Error: "terminal input is still busy; refresh recovery", Outcome: "rejected"}
	select {
	case r.inputGate <- struct{}{}:
		response = controller.RecoveryRPC(ctx, recovery.RPCRequest{Operation: "restart", Action: &recovery.RecoveryAction{ID: input.ID, Instance: input.Instance, Kind: "restart", Confirm: true}})
		<-r.inputGate
	case <-ctx.Done():
	case <-r.stopped:
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[id] != e || e.run != r {
		return RecoveryResult{}, ErrClosed
	}
	outcome := response.Outcome
	if outcome != "consumed" && outcome != "rejected" {
		outcome = "uncertain"
	}
	state = e.value.Recovery
	state.Intent.Outcome = outcome
	state.Revision++
	state.Pending = true
	if outcome == "consumed" {
		state.Disposition = "waiting"
		r.recoveryControl = &recovery.Event{Kind: "control", Mode: "waiting", Instance: "restart-pending"}
		r.recoveryReady = false
		e.shellReady = false
	} else if outcome == "rejected" {
		state.Disposition = previous.Disposition
		state.Pending = previous.Pending
		r.restarting.Store(false)
	} else {
		state.Disposition = "execution-uncertain"
	}
	if err := b.persistLocked(); err != nil {
		return RecoveryResult{}, err
	}
	if response.Error != "" {
		return RecoveryResult{}, invalid(response.Error)
	}
	return RecoveryResult{Outcome: outcome}, nil
}
