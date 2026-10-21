package session

// OpenRigSession returns the owner's dashboard attachment, separate from the
// terminal grid. Closing a viewer never closes the attachment or agent teams.
func (b *Broker) OpenRigSession(owner string) *Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner && e.value.Companion == "openrig" {
			value := e.value.clone()
			if e.run == nil {
				value.State = "disconnected"
			}
			return &value
		}
	}
	return nil
}

// EnsureOpenRig only attaches a dashboard client. The fixed argv deliberately
// does not run setup, start a daemon, launch a team, or open a desktop terminal.
func (b *Broker) EnsureOpenRig(owner, executable string) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	var selected *entry
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner && e.value.Companion == "openrig" {
			selected = e
			break
		}
	}
	created := selected == nil
	if created {
		id, err := uuid()
		if err != nil {
			return Session{}, err
		}
		selected = &entry{value: Session{ID: id, Kind: "terminal", Owner: owner, Transport: "exec", Hostname: "localhost", Cols: 120, Rows: 36, Title: "OpenRig dashboard", Workspace: "openrig", Companion: "openrig", CreatedAt: now(), UpdatedAt: now()}}
		b.entries[id] = selected
		b.order = append(b.order, id)
	}
	oldValue := selected.value.clone()
	launched := false
	if selected.run == nil {
		selected.value.ExecArgv = []string{executable, "tui", "--shared"}
		if err := b.startLocked(selected, "", ""); err != nil {
			return Session{}, err
		}
		launched = true
	}
	if err := b.persistLocked(); err != nil {
		if launched {
			r := selected.run
			selected.run = nil
			r.stop()
			r.log.stop("create_failed")
			go r.process.Close()
		}
		if created {
			delete(b.entries, selected.value.ID)
			b.order = b.order[:len(b.order)-1]
		} else {
			selected.value = oldValue
		}
		return Session{}, err
	}
	return selected.value.clone(), nil
}
