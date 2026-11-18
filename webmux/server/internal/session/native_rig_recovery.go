package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

type NativeRigOperationResult struct {
	State string
	Code  string
	Argv  []string
}

// Prepare banks the native request without dispatch; execute reuses it. The
// server supplies the saved identity and the browser supplies no launch fields.
type NativeRigOperation func(context.Context, string, RigIdentity, RecoveryRequest, bool) (NativeRigOperationResult, error)

func originalManagedRequest(state *recovery.State) (RecoveryRequest, bool) {
	var value RecoveryRequest
	if state == nil || state.Intent == nil || state.Intent.ManagedRequest == "" || len(state.Intent.ManagedRequest) > 4096 {
		return value, false
	}
	data := []byte(state.Intent.ManagedRequest)
	if json.Unmarshal(data, &value) != nil || value.Inspect || value.ID != state.Intent.ID || value.Kind != "managed" || !value.Confirm || value.Context != "" || value.HostRevision != 0 {
		return value, false
	}
	return value, fmt.Sprintf("%x", sha256.Sum256(data)) == state.Intent.Hash
}

// RecoverRigNative retains one exact operation through controller loss. A
// normal duplicate is read-only; only an explicit Check may complete it.
func (b *Broker) RecoverRigNative(ctx context.Context, owner, id string, input RecoveryRequest, operation NativeRigOperation) (RecoveryResult, error) {
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
	if input.Kind != "managed" || !input.Confirm || input.Context != "" || input.HostRevision != 0 || !regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`).MatchString(input.ID) || operation == nil || e.value.Rig == nil {
		b.mu.Unlock()
		return RecoveryResult{}, invalid("Original Rig recovery requires the recorded request and explicit approval.")
	}
	saved := *e.value.Rig
	if _, ok := NativeRigReference(saved); !ok {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	state := e.value.Recovery
	canonical := input
	canonical.Inspect = false
	data, _ := json.Marshal(canonical)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	completing := false
	if state != nil && state.Intent != nil {
		if state.Intent.ID == input.ID {
			original, ok := originalManagedRequest(state)
			if !ok || state.Intent.Hash != hash {
				b.mu.Unlock()
				return RecoveryResult{}, ErrRecoveryConflict
			}
			if state.Intent.Outcome == "consumed" {
				b.mu.Unlock()
				return RecoveryResult{Outcome: "consumed"}, nil
			}
			if original.Instance != managedInstance(saved) {
				b.mu.Unlock()
				return RecoveryResult{}, ErrRecoveryConflict
			}
			if state.Intent.Outcome != "requested" && state.Intent.Outcome != "uncertain" {
				b.mu.Unlock()
				return RecoveryResult{}, ErrRecoveryConflict
			}
			if !input.Inspect {
				b.mu.Unlock()
				return RecoveryResult{Outcome: "uncertain", Code: "original_operation_pending"}, nil
			}
			completing = true
		} else if state.Intent.Outcome == "requested" || state.Intent.Outcome == "uncertain" {
			b.mu.Unlock()
			return RecoveryResult{}, invalid("The original Rig recovery is unresolved. Check that original request before choosing another operation.")
		}
	}
	if !completing && (input.Inspect || state == nil || state.Revision != input.Revision || managedInstance(saved) != input.Instance) {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	revision := state.Revision
	b.mu.Unlock()
	var survivorArgv []string
	if !completing {
		prepared, err := operation(ctx, owner, saved, canonical, true)
		if err != nil {
			return RecoveryResult{}, err
		}
		if prepared.State == "surviving" && len(prepared.Argv) > 0 {
			survivorArgv = prepared.Argv
		} else if prepared.State != "prepared" || len(prepared.Argv) != 0 {
			return RecoveryResult{}, invalid("Rig did not bank the original recovery request.")
		}
	}
	b.mu.Lock()
	if b.closed || b.entries[id] != e || e.value.Rig == nil || *e.value.Rig != saved || e.value.Recovery == nil || e.value.Recovery.Revision != revision {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	if !completing {
		previous := e.value.Recovery.Clone()
		state = e.value.Recovery
		state.Intent = &recovery.Intent{ID: input.ID, Hash: hash, ManagedRequest: string(data), Outcome: "requested"}
		state.Disposition = "requested"
		if len(survivorArgv) > 0 {
			state.Intent.Outcome = "consumed"
			state.Pending = false
			state.Disposition = "reattached"
		}
		state.Revision++
		if err := b.persistLocked(); err != nil {
			e.value.Recovery = previous
			b.mu.Unlock()
			return RecoveryResult{}, err
		}
	}
	b.mu.Unlock()
	if len(survivorArgv) > 0 {
		if _, err := b.ReconnectManaged(owner, id, saved, survivorArgv); err != nil {
			return RecoveryResult{}, invalid("The original Rig conversation survives, but its viewer could not attach. Reconnect the viewer; no Resume was dispatched.")
		}
		return RecoveryResult{Outcome: "consumed"}, nil
	}
	result, operationErr := operation(ctx, owner, saved, input, false)
	b.mu.Lock()
	if b.closed || b.entries[id] != e || e.value.Rig == nil || *e.value.Rig != saved {
		b.mu.Unlock()
		return RecoveryResult{}, ErrClosed
	}
	state = e.value.Recovery
	if state == nil || state.Intent == nil || state.Intent.ID != input.ID || state.Intent.Hash != hash {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	if operationErr != nil || result.State != "confirmed" || len(result.Argv) == 0 {
		state.Intent.Outcome = "uncertain"
		state.Disposition = "execution-uncertain"
		state.Revision++
		err := b.persistLocked()
		b.mu.Unlock()
		if err != nil {
			return RecoveryResult{}, err
		}
		code := result.Code
		if code == "" {
			code = "native_recovery_unconfirmed"
		}
		return RecoveryResult{Outcome: "uncertain", Code: code}, nil
	}
	state.Intent.Outcome = "consumed"
	state.Pending = false
	state.Disposition = "continuing"
	state.Revision++
	err = b.persistLocked()
	b.mu.Unlock()
	if err != nil {
		return RecoveryResult{}, err
	}
	if _, err = b.ReconnectManaged(owner, id, saved, result.Argv); err != nil {
		return RecoveryResult{}, invalid("Rig confirmed recovery, but its viewer could not attach. Reconnect the viewer; the original recovery is already consumed.")
	}
	return RecoveryResult{Outcome: "consumed"}, nil
}
