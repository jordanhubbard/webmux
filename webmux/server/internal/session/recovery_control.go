package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

type recoveryController interface {
	RecoveryRPC(context.Context, recovery.RPCRequest) recovery.RPCResponse
}

var ErrRecoveryConflict = errors.New("recovery state changed; refresh the preview before continuing")

func (b *Broker) startRecoveryPollingLocked(e *entry, r *run) {
	if b.closed || r.recoveryPolling || e.value.Transport == "exec" || e.value.Agent() || e.value.Rig != nil {
		return
	}
	if _, ok := r.process.(recoveryController); !ok {
		return
	}
	r.recoveryPolling = true
	b.workers.Add(1)
	go func() {
		defer b.workers.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			b.syncRecovery(e, r)
			select {
			case <-r.stopped:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (b *Broker) syncRecovery(e *entry, r *run) {
	e.logOperation.Lock()
	b.mu.Lock()
	if b.closed || b.entries[e.value.ID] != e || e.run != r {
		b.mu.Unlock()
		e.logOperation.Unlock()
		return
	}
	controller, ok := r.process.(recoveryController)
	if !ok {
		b.mu.Unlock()
		e.logOperation.Unlock()
		return
	}
	recording := r.log.active()
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	response := controller.RecoveryRPC(ctx, recovery.RPCRequest{Operation: "policy", Recording: &recording})
	cancel()
	b.mu.Lock()
	var automatic *RecoveryRequest
	owner, id := e.value.Owner, e.value.ID
	if !b.closed && b.entries[id] == e && e.run == r {
		if response.Error != "" {
			r.recoveryError = response.Error
		} else {
			r.recoveryError = ""
			b.applyRecoverySnapshotLocked(e, r, response)
			state, control := e.value.Recovery, r.recoveryControl
			if recording && response.Recording && state != nil && state.AutoResume && state.Activity != nil && state.Activity.Running && control != nil && control.Mode == "waiting" && r.autoAttempted != control.Instance && state.Activity.CanAutoResumeNative() && (state.Intent == nil || state.Intent.Outcome != "uncertain" && state.Intent.Outcome != "requested") {
				r.autoAttempted = control.Instance
				if actionID, err := uuid(); err == nil {
					automatic = &RecoveryRequest{ID: actionID, Revision: state.Revision, HostRevision: r.recoveryRevision, Instance: control.Instance, Kind: "native"}
				}
			}
		}
	}
	b.mu.Unlock()
	e.logOperation.Unlock()
	if automatic != nil {
		_, _ = b.Recover(owner, id, *automatic)
	}
}

func (b *Broker) applyRecoverySnapshotLocked(e *entry, r *run, response recovery.RPCResponse) {
	for _, event := range response.Events {
		if !recovery.Verify(event, e.value.RecoveryKey) {
			r.recoveryError = "The execution host returned unauthenticated recovery evidence."
			return
		}
	}
	r.recoveryReady = true
	if response.Revision == r.recoveryRevision {
		return
	}
	r.recoveryRevision = response.Revision
	var command, native, complete, shell, provider, control *recovery.Event
	for _, event := range response.Events {
		copy := event
		switch event.Kind {
		case "command":
			command = &copy
		case "cli":
			native = &copy
		case "complete":
			complete = &copy
		case "shell":
			shell = &copy
		case "provider":
			provider = &copy
		case "control":
			control = &copy
		}
	}
	r.recoveryControl = control
	if provider != nil {
		r.recoveryWarning = provider.Warning
	}
	if control != nil {
		e.shellHelper, e.shellPlatform = control.Helper, control.Platform
		e.shellReady = control.Ready && control.Mode != "waiting"
	}
	if provider != nil && e.shellHelper == "" {
		e.shellHelper, e.shellPlatform = provider.Helper, provider.Platform
	}
	if shell != nil {
		e.shellInstance = shell.Instance
	}
	if e.value.Recovery == nil {
		e.value.Recovery = &recovery.State{Version: 1}
	}
	state := e.value.Recovery
	previous := state.Clone()
	if command != nil || native != nil {
		a := &recovery.Activity{Running: true, ObservedAt: now()}
		if command != nil {
			a.Platform, a.ShellInstance, a.Cwd = command.Platform, command.Instance, command.Cwd
			a.Command, a.Replayable = recovery.SafeCommand(command.Command)
			a.Replayable = a.Replayable && !command.Redacted
			a.Source = "shell-integration"
		}
		if native != nil {
			a.CLI, a.CLIVersion, a.NativeSessionID, a.NativeStateFile = native.CLI, native.Version, native.SessionID, native.StateFile
			a.NativeEvidence = native.NativeEvidence
			a.Configuration = native.Configuration.Clone()
			a.Cwd, a.ShellInstance, a.Platform = native.Cwd, native.Instance, native.Platform
			a.Argv, a.Source = append([]string(nil), native.Argv...), "cli-native-state"
		}
		if complete != nil && complete.Instance == a.ShellInstance {
			a.Running = false
		}
		if state.Activity != nil {
			observed := a.ObservedAt
			a.ObservedAt = state.Activity.ObservedAt
			if !reflect.DeepEqual(a, state.Activity) {
				a.ObservedAt = observed
			}
		}
		state.Activity = a
	}
	if control != nil && control.Mode == "waiting" {
		state.Pending = true
		if state.Disposition != "execution-uncertain" && state.Disposition != "requested" {
			state.Disposition = "waiting"
		}
		e.shellReady = false
		b.restoreRecoveryHistoryLocked(e, r)
	} else if control != nil && control.Mode == "consumed" {
		state.Pending, state.Disposition = false, "continuing"
	} else if provider != nil && provider.Survived {
		state.Pending, state.Disposition = false, "reattached"
	}
	if state.Activity != nil && control != nil && state.Activity.ShellInstance == control.Instance && control.Mode == "consumed" {
		state.Disposition = "resumed"
	}
	if control != nil && control.Mode == "active" && state.Intent != nil && state.Intent.Outcome == "consumed" {
		state.Pending, state.Disposition = false, "resumed"
	}
	if reflect.DeepEqual(previous, state) {
		return
	}
	state.Revision++
	if err := b.persistLocked(); err != nil {
		b.logger.Warn("save host recovery snapshot", "error", err)
	}
	b.broadcastLocked(e, Event{"type": "recovery_status", "session_id": e.value.ID, "recovery": state.Clone()})
}

type RecoveryRequest struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Revision     uint64 `json:"revision"`
	HostRevision uint64 `json:"host_revision"`
	Instance     string `json:"instance"`
	Confirm      bool   `json:"confirm"`
	Context      string `json:"context,omitempty"`
}

type RecoveryResult struct {
	Outcome string `json:"outcome"`
}

func (b *Broker) Recover(owner, id string, input RecoveryRequest) (RecoveryResult, error) {
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
	if !ok || e.value.Rig != nil || e.value.Agent() || e.value.Transport == "exec" {
		b.mu.Unlock()
		return RecoveryResult{}, &config.ValidationError{Message: "This session's lifecycle manager owns recovery."}
	}
	if !regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(input.ID) || len(input.Context) > 16<<10 {
		b.mu.Unlock()
		return RecoveryResult{}, &config.ValidationError{Message: "Invalid recovery action or oversized context."}
	}
	if input.Kind != "native" && input.Kind != "shell" && input.Kind != "replay" || input.Kind == "replay" && !input.Confirm || input.Context != "" && (!input.Confirm || input.Kind != "native") {
		b.mu.Unlock()
		return RecoveryResult{}, &config.ValidationError{Message: "Command replay and transcript handoff require explicit approval."}
	}
	state := e.value.Recovery
	data, _ := json.Marshal(input)
	digest := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", digest)
	retry := state != nil && state.Intent != nil && state.Intent.ID == input.ID
	if retry {
		if state.Intent.Hash != hash {
			b.mu.Unlock()
			return RecoveryResult{}, ErrRecoveryConflict
		}
		if state.Intent.Outcome == "consumed" {
			b.mu.Unlock()
			return RecoveryResult{Outcome: "consumed"}, nil
		}
	}
	if !retry && (!r.recoveryReady || state == nil || state.Revision != input.Revision || r.recoveryRevision != input.HostRevision || r.recoveryControl == nil || r.recoveryControl.Mode != "waiting" || r.recoveryControl.Instance != input.Instance) {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	if state == nil {
		b.mu.Unlock()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	previous := state.Clone()
	state.Intent = &recovery.Intent{ID: input.ID, Hash: hash, Outcome: "requested"}
	state.Disposition = "requested"
	state.Revision++
	if err := b.persistLocked(); err != nil {
		e.value.Recovery = previous
		b.mu.Unlock()
		return RecoveryResult{}, err
	}
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response := controller.RecoveryRPC(ctx, recovery.RPCRequest{Operation: "action", Action: &recovery.RecoveryAction{ID: input.ID, Instance: input.Instance, Revision: input.HostRevision, Kind: input.Kind, Confirm: input.Confirm, Context: input.Context}})
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.entries[id] != e || e.run != r {
		return RecoveryResult{}, ErrClosed
	}
	state = e.value.Recovery
	if response.Error != "" {
		outcome := response.Outcome
		if outcome != "rejected" {
			outcome = "uncertain"
		}
		state.Intent.Outcome = outcome
		if outcome == "uncertain" {
			state.Disposition = "execution-uncertain"
		} else {
			state.Disposition = "waiting"
		}
		state.Revision++
		if err := b.persistLocked(); err != nil {
			return RecoveryResult{}, err
		}
		return RecoveryResult{}, &config.ValidationError{Message: response.Error}
	}
	if response.Outcome != "consumed" {
		state.Intent.Outcome = "uncertain"
		state.Disposition = "execution-uncertain"
		state.Revision++
		_ = b.persistLocked()
		return RecoveryResult{}, ErrRecoveryConflict
	}
	state.Intent.Outcome = "consumed"
	state.Pending, state.Disposition = false, "continuing"
	state.Revision++
	if r.recoveryControl != nil {
		copy := *r.recoveryControl
		copy.Mode = "consumed"
		r.recoveryControl = &copy
	}
	if err := b.persistLocked(); err != nil {
		return RecoveryResult{}, err
	}
	return RecoveryResult{Outcome: "consumed"}, nil
}

func (b *Broker) SetAutoRecovery(owner, id string, enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return err
	}
	if enabled && (e.value.Agent() || e.value.Rig != nil || e.value.Transport == "exec") {
		return &config.ValidationError{Message: "This session's lifecycle manager owns recovery."}
	}
	previous := e.value.Recovery.Clone()
	if e.value.Recovery == nil {
		e.value.Recovery = &recovery.State{Version: 1}
	}
	e.value.Recovery.AutoResume = enabled
	e.value.Recovery.Revision++
	if err := b.persistLocked(); err != nil {
		e.value.Recovery = previous
		return err
	}
	return nil
}
