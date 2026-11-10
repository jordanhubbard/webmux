package session

import (
	"context"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// ManagedResolver checks current access and provider inventory, and returns
// only an attachment command for the original host-qualified identity.
type ManagedResolver func(context.Context, string, RigIdentity) ([]string, error)

// RestoreManaged queries lifecycle owners outside the broker mutex. Ordinary
// Restore deliberately leaves these viewers disconnected until this check.
// An unavailable coordinator must not prevent the WebMux server from starting.
func (b *Broker) RestoreManaged(ctx context.Context, resolve ManagedResolver) error {
	if resolve == nil {
		return invalid("A lifecycle manager resolver is required")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	var pending []Session
	for _, id := range b.order {
		e := b.entries[id]
		if e.run == nil && e.value.Persistent && e.value.Rig != nil {
			pending = append(pending, e.value.clone())
		}
	}
	b.mu.Unlock()
	for _, saved := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		argv, err := resolve(ctx, saved.Owner, *saved.Rig)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return cancelErr
		}
		if err == nil {
			_, err = b.ReconnectManaged(saved.Owner, saved.ID, *saved.Rig, argv)
		}
		if err == nil {
			continue
		}
		b.logger.Warn("managed session reattachment unavailable", "session_id", saved.ID, "error", err)
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return ErrClosed
		}
		e := b.entries[saved.ID]
		if e == nil || e.value.Owner != saved.Owner || e.run != nil || e.value.Rig == nil || *e.value.Rig != *saved.Rig {
			b.mu.Unlock()
			continue
		}
		if e.value.Recovery == nil {
			e.value.Recovery = &recovery.State{Version: 1}
		}
		message := recovery.PlainText(err.Error())
		if len(message) > 1024 {
			message = strings.ToValidUTF8(message[:1024], "�")
		}
		e.value.State = "disconnected"
		e.value.Recovery.Pending = true
		e.value.Recovery.Disposition = "managed-unavailable"
		e.value.Recovery.ManagerMessage = message
		e.value.Recovery.Revision++
		err = b.persistLocked()
		b.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}
