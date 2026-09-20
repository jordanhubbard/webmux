package desktop

import "errors"

var ErrForbidden = errors.New("Forbidden")

// Connection is an owner-authorized snapshot and transport lifetime. State
// updates from a closed/deleted connection cannot overwrite another viewer.
type Connection struct {
	Session  Session
	Password string
	Done     <-chan struct{}
	broker   *Broker
	entry    *entry
}

func (b *Broker) Connect(owner, id string) (*Connection, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	e := b.entries[id]
	if e == nil {
		return nil, ErrNotFound
	}
	if e.value.Owner != owner {
		return nil, ErrForbidden
	}
	c := &Connection{Session: e.value.clone(), Password: e.password, Done: e.done, broker: b, entry: e}
	if e.connections == nil {
		e.connections = map[*Connection]string{}
	}
	e.connections[c] = "connecting"
	return c, nil
}
func (c *Connection) SetState(state string) { c.update(state, false) }
func (c *Connection) Finish(state string)   { c.update(state, true); c.Password = "" }
func (c *Connection) update(state string, finish bool) {
	b := c.broker
	b.mu.Lock()
	defer b.mu.Unlock()
	e := c.entry
	if b.closed || b.entries[e.value.ID] != e {
		return
	}
	if _, exists := e.connections[c]; !exists {
		return
	}
	switch state {
	case "connecting", "connected", "disconnected", "error":
	default:
		return
	}
	if finish {
		delete(e.connections, c)
	} else {
		e.connections[c] = state
	}
	for _, active := range e.connections {
		if active == "connected" {
			state = "connected"
			break
		}
		if active == "connecting" {
			state = "connecting"
		}
	}
	e.value.State = state
	e.value.UpdatedAt = now()
}
