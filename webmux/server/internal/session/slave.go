package session

// ResetForSlave is a startup-only operation, before accepting HTTP requests.
// Slave mode owns the terminal list, including agent panes and other owners.
// Deletion uses the normal persistence, process, viewer and audit lifecycle.
func (b *Broker) ResetForSlave(host string, port int) (Session, error) {
	if host == "" || port < 0 || port > 65535 {
		return Session{}, invalid("invalid slave destination")
	}
	b.agentMu.Lock()
	defer b.agentMu.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Session{}, ErrClosed
	}
	old := make([]Session, 0, len(b.order))
	for _, id := range b.order {
		old = append(old, b.entries[id].value.clone())
	}
	b.mu.Unlock()
	for _, value := range old {
		if err := b.Delete(value.Owner, value.ID); err != nil {
			return Session{}, err
		}
	}
	zero := 0
	return b.Create("system", CreateRequest{Hostname: host, Port: port, Username: "console", Transport: "exec", Row: &zero, Col: &zero})
}
