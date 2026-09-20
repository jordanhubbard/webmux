package session

import "sync"

// Event uses the existing browser WebSocket message field names. Events become
// immutable before entering a viewer queue.
type Event map[string]any

// Viewer is a bounded subscription. The broker never waits for network writes.
// On overflow the client reconnects and receives fresh state and scrollback.
type Viewer struct {
	ID     string
	mu     sync.Mutex
	queue  [128]Event
	head   int
	count  int
	ready  chan struct{}
	done   chan struct{}
	code   int
	reason string
}

func (v *Viewer) Ready() <-chan struct{} { return v.ready }
func (v *Viewer) Done() <-chan struct{}  { return v.done }

// Take removes one event after Ready signals. A single consumer owns this queue.
func (v *Viewer) Take() Event {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.count == 0 {
		return nil
	}
	event := v.queue[v.head]
	v.queue[v.head] = nil
	v.head = (v.head + 1) % len(v.queue)
	v.count--
	if v.count > 0 {
		v.ready <- struct{}{}
	}
	return event
}

// CloseReason is safe to read after Done closes.
func (v *Viewer) CloseReason() (int, string) { <-v.done; return v.code, v.reason }
func (v *Viewer) finish(code int, reason string) {
	select {
	case <-v.done:
		return
	default:
		v.code, v.reason = code, reason
		close(v.done)
	}
}
func (v *Viewer) send(event Event) {
	v.mu.Lock()
	defer v.mu.Unlock()
	select {
	case <-v.done:
		return
	default:
	}
	// PTY reads have arbitrary chunk boundaries. Combine only adjacent output
	// for the same session, preserving all bytes and control-event ordering.
	// Never wait to form a batch; an idle consumer is notified immediately.
	if event["type"] == "output" && v.count > 0 {
		last := (v.head + v.count - 1) % len(v.queue)
		previous := v.queue[last]
		if previous["type"] == "output" && previous["session_id"] == event["session_id"] {
			before, okBefore := previous["data"].(string)
			after, okAfter := event["data"].(string)
			if okBefore && okAfter && len(before)+len(after) <= 64*1024 {
				v.queue[last] = Event{"type": "output", "session_id": event["session_id"], "data": before + after}
				return
			}
		}
	}
	if v.count == len(v.queue) {
		v.finish(1013, "Viewer is too slow")
		return
	}
	v.queue[(v.head+v.count)%len(v.queue)] = event
	v.count++
	if v.count == 1 {
		v.ready <- struct{}{}
	}
}

func (b *Broker) Join(owner, id string) (*Viewer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return nil, err
	}
	if e.value.Agent() {
		if _, err := b.agents.Access(e.value.AgentID); err != nil {
			return nil, err
		}
	}
	viewerID, err := uuid()
	if err != nil {
		return nil, err
	}
	v := &Viewer{ID: viewerID, ready: make(chan struct{}, 1), done: make(chan struct{})}
	if e.viewers == nil {
		e.viewers = map[string]*Viewer{}
	}
	e.viewers[v.ID] = v
	if e.focus == "" {
		e.focus = v.ID
	}
	b.broadcastLocked(e, b.presenceLocked(e, "viewer_join", v.ID))
	v.send(Event{"type": "status", "session_id": id, "state": e.value.State, "viewer_id": v.ID, "transcript_enabled": e.run != nil && e.run.log.active()})
	if e.scrollback != "" {
		v.send(Event{"type": "output", "session_id": id, "data": e.scrollback})
	}
	return v, nil
}
func (b *Broker) Leave(owner, id, viewerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return
	}
	v := e.viewers[viewerID]
	if v == nil {
		return
	}
	delete(e.viewers, viewerID)
	v.finish(1000, "Viewer left")
	// Match the UI contract: focus is released, not silently transferred. The
	// next focus request (or newly joining viewer) chooses the next owner.
	if e.focus == viewerID {
		e.focus = ""
	}
	b.broadcastLocked(e, b.presenceLocked(e, "viewer_leave", viewerID))
}
func (b *Broker) Focus(owner, id, viewerID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return err
	}
	v := e.viewers[viewerID]
	if v == nil {
		return ErrNotFound
	}
	select {
	case <-v.done:
		return ErrClosed
	default:
	}
	e.focus = viewerID
	b.broadcastLocked(e, b.presenceLocked(e, "focus", ""))
	return nil
}
func (b *Broker) presenceLocked(e *entry, kind, viewerID string) Event {
	event := Event{"type": kind, "session_id": e.value.ID, "viewer_count": len(e.viewers)}
	if viewerID != "" {
		event["viewer_id"] = viewerID
	}
	if e.focus != "" {
		event["focus_owner"] = e.focus
	}
	return event
}
func (b *Broker) broadcastLocked(e *entry, event Event) {
	for _, v := range e.viewers {
		v.send(event)
	}
}
func (b *Broker) closeViewersLocked(e *entry, code int, reason string) {
	for _, v := range e.viewers {
		v.finish(code, reason)
	}
	e.viewers = nil
	e.focus = ""
}
