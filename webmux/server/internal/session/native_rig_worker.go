package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// A job is an in-memory owner of bounded work, not recovery authority. The
// persisted original request and native owner's keyed operation remain the
// authority after a controller restart. All mutable fields use Broker.mu.
type nativeRecoveryJob struct {
	hash          string
	banked        chan struct{}
	done          chan struct{}
	finished      bool
	readyForCheck bool
	cancel        context.CancelFunc
	result        RecoveryResult
	err           error
}

func (b *Broker) nativeRecoveryJobActiveLocked(id string) bool {
	job := b.nativeRecoveryJobs[id]
	return job != nil && !job.finished
}

// BeginRigNativeRecovery returns pending only after RecoverRigNative has
// durably banked the exact approved request. The worker can then finish owner
// preparation independently of response loss, within the existing 45s budget.
// Background confirmation never attaches a viewer: an explicit Check must
// reverify the live original, rather than trust cached native argv or identity.
func (b *Broker) BeginRigNativeRecovery(ctx context.Context, owner, id string, input RecoveryRequest, operation NativeRigOperation) (RecoveryResult, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryResult{}, err
	}
	canonical := input
	canonical.Inspect = false
	data, _ := json.Marshal(canonical)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	if b.closed {
		b.mu.Unlock()
		return RecoveryResult{}, ErrClosed
	}
	if input.Kind != "managed" || !input.Confirm || input.Context != "" || input.HostRevision != 0 ||
		!regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[0-9a-f]{12}$`).MatchString(input.ID) || operation == nil || e.value.Rig == nil {
		b.mu.Unlock()
		return RecoveryResult{}, invalid("Original Rig recovery requires the recorded request and explicit approval.")
	}
	job := b.nativeRecoveryJobs[id]
	if job != nil && !job.finished {
		if job.hash != hash {
			b.mu.Unlock()
			return RecoveryResult{}, ErrRecoveryConflict
		}
		b.mu.Unlock()
		return waitNativeRecovery(ctx, job)
	}
	// Ordinary duplicates remain read-only. Once preparation has settled,
	// explicit completion uses the existing full live-original checks again.
	if job != nil && job.hash == hash && (!input.Inspect || job.readyForCheck) {
		b.mu.Unlock()
		return b.RecoverRigNative(ctx, owner, id, input, operation)
	}
	if nativeRigPending(e.value.Recovery) && (e.value.Recovery.Intent == nil || e.value.Recovery.Intent.Hash != hash) {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	deadline := time.Now().Add(45 * time.Second)
	if original, ok := ctx.Deadline(); ok && original.Before(deadline) {
		deadline = original
	}
	work, cancel := context.WithDeadline(context.WithoutCancel(ctx), deadline)
	job = &nativeRecoveryJob{hash: hash, banked: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	if b.nativeRecoveryJobs == nil {
		b.nativeRecoveryJobs = make(map[string]*nativeRecoveryJob)
	}
	b.nativeRecoveryJobs[id] = job
	b.workers.Add(1) // Close sets closed and cancels jobs under this same lock.
	b.mu.Unlock()
	go func() {
		defer b.workers.Done()
		defer cancel()
		wrapped := func(work context.Context, owner string, saved RigIdentity, request RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
			if prepare {
				return operation(work, owner, saved, request, true)
			}
			// This callback is reached only after the original request has
			// been persisted. No provider preparation starts before that.
			b.mu.Lock()
			if err := work.Err(); err != nil {
				b.mu.Unlock()
				return NativeRigOperationResult{}, err
			}
			close(job.banked)
			b.mu.Unlock()
			result, err := operation(work, owner, saved, request, false)
			b.mu.Lock()
			// A typed native result has completed preparation. A later Check
			// can reverify it directly, even when that result was uncertain.
			job.readyForCheck = err == nil
			b.mu.Unlock()
			if result.State == "confirmed" && err == nil {
				// Keep pending UI/consent until a subsequent explicit Check
				// verifies the current provider and returns consumed itself.
				return NativeRigOperationResult{State: "uncertain", Code: "original_confirmation_pending"}, nil
			}
			return result, err
		}
		result, err := b.RecoverRigNative(work, owner, id, input, wrapped)
		b.mu.Lock()
		job.result, job.err, job.finished = result, err, true
		close(job.done)
		if b.entries[id] != e && b.nativeRecoveryJobs[id] == job {
			delete(b.nativeRecoveryJobs, id)
		}
		b.mu.Unlock()
	}()
	return waitNativeRecovery(ctx, job)
}

func waitNativeRecovery(ctx context.Context, job *nativeRecoveryJob) (RecoveryResult, error) {
	select {
	case <-job.banked:
		return RecoveryResult{Outcome: "uncertain", Code: "original_operation_pending"}, nil
	case <-job.done:
		return job.result, job.err
	case <-ctx.Done():
		// Work already banked belongs to the broker, not this connection.
		// Its independent deadline and Broker.Close still bound its life.
		return RecoveryResult{}, ctx.Err()
	}
}
