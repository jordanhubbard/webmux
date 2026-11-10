package session

import (
	"context"
)

// ManagedEvidenceCapture reads native lifecycle-owner metadata; it accepts no
// browser commands or transcript contents. Snapshot creation, when needed, is
// serialized with logging changes and never occurs while logging is paused.
type ManagedEvidenceCapture func(context.Context, RigIdentity) (RigIdentity, error)

func (b *Broker) CaptureManagedRecovery(ctx context.Context, owner, id string, capture ManagedEvidenceCapture) error {
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	e.logOperation.Lock()
	defer e.logOperation.Unlock()
	b.mu.Lock()
	if b.closed || b.entries[id] != e {
		b.mu.Unlock()
		return ErrClosed
	}
	if e.value.Rig == nil || e.run == nil || !e.run.log.active() || capture == nil {
		b.mu.Unlock()
		return nil
	}
	before := *e.value.Rig
	b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	observed, err := capture(ctx, before)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !sameManagedDestination(before, observed) || !validRigRecoveryReference(observed) {
		return invalid("The lifecycle manager did not verify the original recovery destination.")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.entries[id] != e || e.value.Rig == nil || *e.value.Rig != before || e.run == nil || !e.run.log.active() {
		return ErrRecoveryConflict
	}
	old := e.value.clone()
	updated := b.managedEvidenceLocked(e, observed)
	if updated == before {
		return nil
	}
	e.value.Rig = &updated
	if e.value.Recovery != nil {
		e.value.Recovery.Revision++
	}
	if err := b.persistLocked(); err != nil {
		e.value = old
		return err
	}
	return nil
}
