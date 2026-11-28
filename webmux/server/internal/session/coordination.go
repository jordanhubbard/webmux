package session

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

// Provisioning may sync a ledger and fsync access files. Serialize its snapshots
// separately so it never blocks reads of an already running terminal. Take a
// fresh snapshot after acquiring this lock, not before waiting behind another
// provisioner: otherwise an older inventory could withdraw a newer interface.
// The caller holds no broker lock and revalidates the generation before launch.
func (b *Broker) provisionCoordinationLaunch(e *entry, generation uint64, provision func([]Session, Session) (*terminal.CoordinationAccess, error)) (*terminal.CoordinationAccess, error) {
	b.provisionMu.Lock()
	defer b.provisionMu.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	if b.entries[e.value.ID] != e || e.generation != generation {
		b.mu.Unlock()
		return nil, ErrRecoveryConflict
	}
	all, current := b.coordinationSnapshotLocked(), e.value.clone()
	b.mu.Unlock()
	return provision(all, current)
}

// InspectCoordinationRepository binds a fixed, read-only SSH/mosh RPC to the
// broker's current live session and authenticated directory observation. Network
// and Git work never hold the broker lock. A response from a replaced run,
// changed destination, or changed observation cannot populate current context.
func (b *Broker) InspectCoordinationRepository(ctx context.Context, snapshot Session) (*coordination.Binding, error) {
	b.mu.Lock()
	e := b.entries[snapshot.ID]
	if !b.coordinationRepositoryCurrent(e, snapshot) {
		b.mu.Unlock()
		return nil, coordination.ErrRepository
	}
	r := e.run
	controller, ok := r.process.(recoveryController)
	b.mu.Unlock()
	if !ok {
		return nil, coordination.ErrRepository
	}
	response := controller.RecoveryRPC(ctx, recovery.RPCRequest{Operation: "coordination-project", CoordinationDirectory: snapshot.CoordinationActivity.Directory})
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[snapshot.ID] != e || e.run != r || !b.coordinationRepositoryCurrent(e, snapshot) || ctx.Err() != nil {
		return nil, coordination.ErrRepository
	}
	if response.Protocol != 1 || response.Error != "" || response.CoordinationProject == nil {
		return nil, errors.New("execution-host repository verification unavailable")
	}
	// The execution-host account is trusted for its Git observation, just as
	// the SSH transport is trusted for its signed shell events. Ledger input
	// validation still applies; peers and agents cannot supply this response.
	project := *response.CoordinationProject
	if project.Source != "verified-git" {
		return nil, coordination.ErrRepository
	}
	return &project, nil
}

// Caller holds b.mu. Snapshot fields originate only with this broker.
func (b *Broker) coordinationRepositoryCurrent(e *entry, snapshot Session) bool {
	if b.closed || e == nil || e.run == nil || !snapshot.CoordinationRemote() || !e.value.CoordinationRemote() ||
		e.value.State != "connected" || e.run.generation != snapshot.CoordinationRun || snapshot.CoordinationActivity == nil || snapshot.CoordinationActivity.Directory == "" || e.value.CoordinationActivity == nil {
		return false
	}
	v := e.value
	return v.Owner == snapshot.Owner && v.CreatedAt == snapshot.CreatedAt && v.Transport == snapshot.Transport && v.Hostname == snapshot.Hostname &&
		v.Username == snapshot.Username && v.Port == snapshot.Port && v.KeyID == snapshot.KeyID && v.RecoveryKey == snapshot.RecoveryKey &&
		*v.CoordinationActivity == *snapshot.CoordinationActivity
}

// ObserveCoordinationDirectory restores missing live routing context through the
// current provider's authenticated, read-only probe. Retained history and caller
// supplied paths cannot establish this observation. Provider work never holds
// the broker lock, and a newer signed observation always wins the race.
func (b *Broker) ObserveCoordinationDirectory(ctx context.Context, snapshot Session) (Session, error) {
	b.mu.Lock()
	e := b.entries[snapshot.ID]
	if !b.coordinationDirectoryObservationCurrent(e, snapshot) {
		b.mu.Unlock()
		return Session{}, coordination.ErrRepository
	}
	r := e.run
	controller, ok := r.process.(recoveryController)
	b.mu.Unlock()
	if !ok {
		return Session{}, coordination.ErrRepository
	}
	response := controller.RecoveryRPC(ctx, recovery.RPCRequest{Operation: "coordination-observe"})
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[snapshot.ID] != e || e.run != r || !b.coordinationDirectoryObservationCurrent(e, snapshot) || ctx.Err() != nil {
		return Session{}, coordination.ErrRepository
	}
	directory := response.CoordinationDirectory
	if response.Protocol != 1 || response.Error != "" || len(directory) > 4096 || !utf8.ValidString(directory) || strings.ContainsFunc(directory, unicode.IsControl) ||
		!coordinationObservedDirectory(recovery.Event{Platform: response.CoordinationPlatform, Cwd: directory}) {
		return Session{}, errors.New("live provider directory observation unavailable")
	}
	observation := CoordinationObservation{Directory: directory}
	if e.value.CoordinationActivity != nil {
		observation = *e.value.CoordinationActivity
		observation.Directory = directory
	}
	e.value.CoordinationActivity = &observation
	value := e.value.clone()
	value.CoordinationRun, value.CoordinationLive = r.generation, true
	return value, nil
}

// Caller holds b.mu. Only a current ordinary local run with no directory may
// be probed; an RPC response cannot overwrite routing learned in the meantime.
func (b *Broker) coordinationDirectoryObservationCurrent(e *entry, snapshot Session) bool {
	eligible := func(s Session) bool {
		return s.Transport == "local" && s.Rig == nil && !s.Agent() && s.Companion == "" && s.OpenRigView == "" &&
			s.ExecCwd == "" && (s.CoordinationActivity == nil || s.CoordinationActivity.Directory == "")
	}
	if b.closed || e == nil || e.run == nil || !snapshot.CoordinationLive || snapshot.CoordinationRun == 0 ||
		e.run.generation != snapshot.CoordinationRun || e.value.State != "connected" || !eligible(e.value) || !eligible(snapshot) {
		return false
	}
	v := e.value
	if v.Owner != snapshot.Owner || v.CreatedAt != snapshot.CreatedAt || v.RecoveryKey == "" || v.RecoveryKey != snapshot.RecoveryKey {
		return false
	}
	if v.CoordinationActivity == nil || snapshot.CoordinationActivity == nil {
		return v.CoordinationActivity == nil && snapshot.CoordinationActivity == nil
	}
	return *v.CoordinationActivity == *snapshot.CoordinationActivity
}

// WaitCoordinationLaunch waits for this caller's provider, not another session.
// Admission still rechecks the full incarnation in WithCoordinationSnapshot.
func (b *Broker) WaitCoordinationLaunch(ctx context.Context, owner, id string) error {
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return ErrClosed
		}
		e, err := b.ownedLocked(owner, id)
		if err != nil {
			b.mu.Unlock()
			return err
		}
		done := e.launchDone
		live := e.run != nil && (e.value.State == "connected" || e.value.State == "connecting")
		b.mu.Unlock()
		if done == nil {
			if !live {
				return ErrNotFound
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		}
	}
}
