package session

// OpenRigSession returns the owner's dashboard attachment, separate from the
// terminal grid. Closing a viewer never closes the attachment or agent teams.
func (b *Broker) OpenRigSession(owner string) *Session {
	value, ok := b.OpenRigViews(owner)["dashboard"]
	if !ok {
		return nil
	}
	return &value
}

func (b *Broker) OpenRigViews(owner string) map[string]Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make(map[string]Session)
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner != owner || e.value.Companion != "openrig" {
			continue
		}
		value := e.value.clone()
		if e.run == nil {
			value.State = "disconnected"
		}
		view := value.OpenRigView
		if view == "" {
			view = "dashboard"
		}
		result[view] = value
	}
	return result
}

// EnsureOpenRig only attaches a dashboard client. The fixed argv deliberately
// does not run setup, start a daemon, launch a team, or open a desktop terminal.
func (b *Broker) EnsureOpenRig(owner, executable string) (Session, error) {
	return b.EnsureOpenRigView(owner, "dashboard", "OpenRig dashboard", []string{executable, "tui"})
}

// EnsureOpenRigView reuses one PTY per owner and view. argv comes from trusted
// server handlers, never directly from the request.
func (b *Broker) EnsureOpenRigView(owner, view, title string, argv []string) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	var selected *entry
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner && e.value.Companion == "openrig" && (e.value.OpenRigView == view || (view == "dashboard" && e.value.OpenRigView == "")) {
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
		selected = &entry{value: Session{ID: id, Kind: "terminal", Owner: owner, Transport: "exec", Hostname: "localhost", Cols: 120, Rows: 36, Title: title, OpenRigView: view, Workspace: "openrig", Companion: "openrig", CreatedAt: now(), UpdatedAt: now()}}
		b.entries[id] = selected
		b.order = append(b.order, id)
	}
	oldValue := selected.value.clone()
	launched := false
	if selected.run == nil {
		selected.value.ExecArgv = append([]string(nil), argv...)
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
