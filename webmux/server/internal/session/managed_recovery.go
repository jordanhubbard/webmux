package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// ManagedRecoveryPlanner performs read-only validation, then returns a provider-
// owned operation. Execute must recheck the destination and ownership before
// starting anything. It returns only fresh viewer attachment arguments.
type ManagedRecoveryPlanner func(context.Context, string, RigIdentity) (func(context.Context) ([]string, error), error)

func managedInstance(identity RigIdentity) string {
	data, _ := json.Marshal(identity)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Managed recovery initially supports exact local Claude conversations owned by
// Agent Deck. Other provider/runtime combinations retain manual recovery.
func managedNativeSupported(identity RigIdentity) bool {
	return identity.Provider == "agent-deck" && identity.Host == "localhost" && strings.HasPrefix(identity.Target, "agent-deck:local:") &&
		managedCLI(identity.Runtime) == "claude" && identity.NativeSource == "agent-deck-inventory" &&
		recovery.ValidNativeID("claude", identity.NativeSessionID) && filepath.IsAbs(identity.NativeStateFile) && identity.Workspace != ""
}

// RecoverManaged is deliberately independent of a viewer PTY: after a host or
// process loss that viewer may already be gone. Uncertain requests are never
// retried, including after reconstruction of the broker from disk.
func (b *Broker) RecoverManaged(ctx context.Context, owner, id string, input RecoveryRequest, plan ManagedRecoveryPlanner) (RecoveryResult, error) {
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	b.mu.Unlock()
	if err != nil {
		return RecoveryResult{}, err
	}
	e.logOperation.Lock()
	defer e.logOperation.Unlock()
	b.mu.Lock()
	if b.closed || b.entries[id] != e {
		b.mu.Unlock()
		return RecoveryResult{}, ErrClosed
	}
	if input.Kind != "managed" || !input.Confirm || input.Context != "" || input.HostRevision != 0 || !regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(input.ID) {
		b.mu.Unlock()
		return RecoveryResult{}, invalid("Managed native recovery requires explicit approval and does not accept transcript contents.")
	}
	data, _ := json.Marshal(input)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	state := e.value.Recovery
	if state != nil && state.Intent != nil {
		if state.Intent.ID == input.ID {
			if state.Intent.Hash == hash && state.Intent.Outcome == "consumed" {
				b.mu.Unlock()
				return RecoveryResult{Outcome: "consumed"}, nil
			}
			b.mu.Unlock()
			return RecoveryResult{}, ErrRecoveryConflict
		}
		if state.Intent.Outcome == "requested" || state.Intent.Outcome == "uncertain" {
			b.mu.Unlock()
			return RecoveryResult{}, invalid("The previous managed recovery may have started. Inspect the lifecycle manager and reconnect; another start will not be attempted.")
		}
	}
	if state == nil || state.Revision != input.Revision || e.value.Rig == nil || !managedNativeSupported(*e.value.Rig) || managedInstance(*e.value.Rig) != input.Instance || plan == nil {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	saved := *e.value.Rig
	b.mu.Unlock()
	execute, err := plan(ctx, owner, saved)
	if err != nil {
		return RecoveryResult{}, err
	}
	if execute == nil {
		return RecoveryResult{}, invalid("The lifecycle manager did not return a recovery operation")
	}
	b.mu.Lock()
	if b.closed || b.entries[id] != e || e.value.Rig == nil || *e.value.Rig != saved || e.value.Recovery == nil || e.value.Recovery.Revision != input.Revision {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	previous := e.value.Recovery.Clone()
	state = e.value.Recovery
	state.Intent = &recovery.Intent{ID: input.ID, Hash: hash, Outcome: "requested"}
	state.Disposition = "requested"
	state.Revision++
	if err := b.persistLocked(); err != nil {
		e.value.Recovery = previous
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	b.mu.Unlock()
	argv, operationErr := execute(ctx)
	b.mu.Lock()
	if b.closed || b.entries[id] != e || e.value.Rig == nil || *e.value.Rig != saved {
		b.mu.Unlock()
		return RecoveryResult{}, ErrClosed
	}
	state = e.value.Recovery
	if operationErr != nil || len(argv) == 0 {
		state.Intent.Outcome = "uncertain"
		state.Disposition = "execution-uncertain"
		state.Revision++
		persistErr := b.persistLocked()
		b.mu.Unlock()
		if persistErr != nil {
			return RecoveryResult{}, persistErr
		}
		return RecoveryResult{}, invalid("The lifecycle manager could not confirm the recovery result. Inspect it before reconnecting; no start will be retried. Retained logs are preserved.")
	}
	state.Intent.Outcome = "consumed"
	state.Pending, state.Disposition = false, "continuing"
	state.Revision++
	err = b.persistLocked()
	b.mu.Unlock()
	if err != nil {
		return RecoveryResult{}, err
	}
	if _, err := b.ReconnectManaged(owner, id, saved, argv); err != nil {
		return RecoveryResult{}, invalid("The lifecycle manager completed recovery but the viewer could not attach. Reconnect the viewer; the recovery request is already consumed.")
	}
	return RecoveryResult{Outcome: "consumed"}, nil
}
