package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

var ErrLocalStopRequired = errors.New("retain this local terminal until its original stop is confirmed; hide the window to keep work running")

type LocalStopIntent struct {
	ID      string                    `yaml:"id"`
	Target  terminal.LocalOwnerTarget `yaml:"target"`
	Outcome string                    `yaml:"outcome"`
}

type LocalLifecycle struct {
	Contract    string `json:"contract"`
	State       string `json:"state"`
	Selection   string `json:"selection,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

type LocalStopRequest struct {
	ID        string `json:"id"`
	Selection string `json:"selection"`
	Mode      string `json:"mode"`
	Confirm   bool   `json:"confirm"`
}

type LocalStopResult struct {
	Outcome string  `json:"outcome"`
	Message string  `json:"message,omitempty"`
	Session Session `json:"session"`
}

func ordinaryLocal(s Session) bool {
	return s.Transport == "local" && s.AgentID == "" && s.AgentRole == "" && s.AgentSessionName == "" && s.Rig == nil && s.Companion == "" && s.OpenRigView == ""
}

func localSelection(t terminal.LocalOwnerTarget) string {
	data, _ := json.Marshal(t)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func localLifecycle(s Session) *LocalLifecycle {
	if !s.LocalOwnerManaged || !ordinaryLocal(s) {
		return nil
	}
	v := &LocalLifecycle{Contract: "webmux-local-provider-lifecycle/v1", State: "unavailable"}
	if !s.LocalOwnerAvailable {
		return v
	}
	if s.LocalOwner != nil {
		v.State, v.Selection = "running", localSelection(*s.LocalOwner)
	}
	if s.LocalStop != nil {
		v.State, v.Selection, v.OperationID = s.LocalStop.Outcome, localSelection(s.LocalStop.Target), s.LocalStop.ID
	}
	return v
}

var localStopID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// StopLocal freezes a server-observed owner before any dispatch. Ordinary
// repeated requests only inspect; a deliberate continuation can address only
// that same original target. No path launches or reattaches a provider.
func (b *Broker) StopLocal(ctx context.Context, owner, id string, request LocalStopRequest) (LocalStopResult, error) {
	if err := ctx.Err(); err != nil {
		return LocalStopResult{}, err
	}
	if !localStopID.MatchString(request.ID) || len(request.Selection) != 64 || (request.Mode != "stop" && request.Mode != "inspect" && request.Mode != "continue") || (request.Mode != "inspect" && !request.Confirm) {
		return LocalStopResult{}, invalid("review and confirm the original local terminal before stopping it")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return LocalStopResult{}, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		b.mu.Unlock()
		return LocalStopResult{}, err
	}
	if !ordinaryLocal(e.value) || !e.value.LocalOwnerManaged || !e.value.LocalOwnerAvailable || e.value.LocalOwner == nil || b.stopLocal == nil || b.inspectLocal == nil {
		b.mu.Unlock()
		return LocalStopResult{}, invalid("this session's execution host or lifecycle manager must handle stop; retain its window for recovery")
	}
	target := *e.value.LocalOwner
	if target.Session != id || localSelection(target) != request.Selection {
		b.mu.Unlock()
		return LocalStopResult{}, ErrRecoveryConflict
	}
	dispatch := false
	prepared := false
	if intent := e.value.LocalStop; intent != nil {
		if intent.ID != request.ID || intent.Target != target {
			b.mu.Unlock()
			return LocalStopResult{}, ErrRecoveryConflict
		}
		if intent.Outcome == "stopped" {
			result := LocalStopResult{Outcome: "stopped", Session: e.value.clone()}
			b.mu.Unlock()
			return result, nil
		}
		// A still-running handler owns dispatch, even if another browser
		// explicitly asks to continue at the same instant.
		if request.Mode == "continue" && intent.Outcome == "uncertain" && !e.localStopBusy {
			dispatch = true
		}
	} else {
		if request.Mode != "stop" {
			b.mu.Unlock()
			return LocalStopResult{}, ErrRecoveryConflict
		}
		old := e.value
		e.value.LocalStop = &LocalStopIntent{ID: request.ID, Target: target, Outcome: "requested"}
		e.value.Persistent = false
		if err := b.persistLocked(); err != nil {
			e.value = old
			b.mu.Unlock()
			return LocalStopResult{}, err
		}
		if e.run != nil {
			e.run.stop()
		}
		dispatch = true
		prepared = true
	}
	if e.localStopBusy {
		result := LocalStopResult{Outcome: "uncertain", Message: "The original stop is still being checked.", Session: e.value.clone()}
		b.mu.Unlock()
		return result, nil
	}
	if dispatch && !prepared {
		old := *e.value.LocalStop
		e.value.LocalStop.Outcome = "requested"
		if err := b.persistLocked(); err != nil {
			*e.value.LocalStop = old
			b.mu.Unlock()
			return LocalStopResult{}, err
		}
	}
	e.localStopBusy = true
	key := e.value.RecoveryKey
	operation := b.inspectLocal
	if dispatch {
		operation = b.stopLocal
	}
	b.mu.Unlock()
	err = operation(ctx, target, key)
	b.mu.Lock()
	defer b.mu.Unlock()
	e.localStopBusy = false
	if b.entries[id] != e || e.value.LocalOwner == nil || *e.value.LocalOwner != target || e.value.LocalStop == nil || e.value.LocalStop.ID != request.ID || e.value.LocalStop.Target != target {
		return LocalStopResult{}, ErrRecoveryConflict
	}
	outcome, message := "stopped", ""
	if err != nil {
		outcome, message = "uncertain", "The original stop is not confirmed. Keep this session and inspect it before continuing the original stop."
	}
	e.value.LocalStop.Outcome = outcome
	if err := b.persistLocked(); err != nil {
		return LocalStopResult{}, err
	}
	return LocalStopResult{Outcome: outcome, Message: message, Session: e.value.clone()}, nil
}
