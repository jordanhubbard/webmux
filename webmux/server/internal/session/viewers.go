package session

// Event uses the existing browser WebSocket message field names. Events become
// immutable before entering a viewer queue.
type Event map[string]any

// Viewer is a bounded subscription. The broker never waits for network writes.
// On overflow the client reconnects and receives fresh state and scrollback.
type Viewer struct {
	ID     string
	events chan Event
	done   chan struct{}
	code   int
	reason string
}

func (v *Viewer) Events() <-chan Event  { return v.events }
func (v *Viewer) Done() <-chan struct{} { return v.done }

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
	select {
	case <-v.done:
		return
	default:
	}
	select {
	case v.events <- event:
	default:
		v.finish(1013, "Viewer is too slow")
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
	// Agent access policy is handled by the forthcoming agent integration.
	// Until then, never expose a restored agent PTY through the generic route.
	if e.value.Agent() {
		return nil, invalid("Agent session unavailable")
	}
	viewerID, err := uuid()
	if err != nil {
		return nil, err
	}
	v := &Viewer{ID: viewerID, events: make(chan Event, 128), done: make(chan struct{})}
	if e.viewers == nil {
		e.viewers = map[string]*Viewer{}
	}
	e.viewers[v.ID] = v
	if e.focus == "" {
		e.focus = v.ID
	}
	b.broadcastLocked(e, b.presenceLocked(e, "viewer_join", v.ID))
	v.send(Event{"type": "status", "session_id": id, "state": e.value.State, "viewer_id": v.ID, "transcript_enabled": false})
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
