package session

import (
	"time"

	"github.com/jordanhubbard/webmux/server/internal/agent"
)

type agentActivityKey struct{ id, name string }
type pendingAgentActivity struct {
	update agent.StatusUpdate
	due    time.Time
}

func (b *Broker) recordAgentActivityLocked(e *entry, r *run, input bool) {
	if e.value.AgentRole != "attach" || e.value.AgentID == "" || e.value.AgentSessionName == "" {
		return
	}
	instant := time.Now()
	if !input && instant.Before(r.replayUntil) {
		return
	}
	if b.activity == nil {
		b.activity = map[agentActivityKey]pendingAgentActivity{}
		b.activityWake = make(chan struct{}, 1)
		b.workers.Add(1)
		go b.writeAgentActivity()
	}
	key := agentActivityKey{e.value.AgentID, e.value.AgentSessionName}
	pending := b.activity[key]
	pending.update.Status = "working"
	pending.update.Source = "webmux"
	pending.due = instant.Add(200 * time.Millisecond)
	stamp := instant.UTC().Format("2006-01-02T15:04:05.000Z")
	if input {
		if pending.update.LastInputAt == nil || stamp >= *pending.update.LastInputAt {
			pending.update.LastInputAt = &stamp
		}
	} else if pending.update.LastOutputAt == nil || stamp >= *pending.update.LastOutputAt {
		pending.update.LastOutputAt = &stamp
		live := "live"
		pending.update.LastOutputSource = &live
	}
	b.activity[key] = pending
	select {
	case b.activityWake <- struct{}{}:
	default:
	}
}

// A single writer coalesces each agent/name for 200 ms and serializes disk
// updates outside the session mutex. Shutdown flushes every pending update.
func (b *Broker) writeAgentActivity() {
	defer b.workers.Done()
	for {
		b.mu.Lock()
		closing := b.closed
		now := time.Now()
		ready := map[agentActivityKey]pendingAgentActivity{}
		var next time.Time
		for key, pending := range b.activity {
			if closing || !pending.due.After(now) {
				ready[key] = pending
				delete(b.activity, key)
			} else if next.IsZero() || pending.due.Before(next) {
				next = pending.due
			}
		}
		b.mu.Unlock()
		for key, pending := range ready {
			if err := b.agents.RecordStatus(key.id, key.name, pending.update); err != nil {
				b.logger.Warn("record agent activity", "agent_id", key.id, "error", err)
			}
		}
		if closing {
			return
		}
		if len(ready) > 0 {
			continue
		}
		var timer *time.Timer
		var due <-chan time.Time
		if !next.IsZero() {
			timer = time.NewTimer(time.Until(next))
			due = timer.C
		}
		select {
		case <-b.policyStop:
		case <-b.activityWake:
		case <-due:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
