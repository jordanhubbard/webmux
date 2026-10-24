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
	return b.ensureRigView(owner, view, title, argv, false, nil)
}

// EnsureRigTerminal attaches a verified managed tmux seat in the ordinary grid.
// Closing this PTY detaches its viewer and leaves the agent running.
func (b *Broker) EnsureRigTerminal(owner, target, title string, argv []string, identity ...RigIdentity) (Session, error) {
	return b.EnsureRigTerminalAt(owner, target, title, argv, nil, nil, identity...)
}

// EnsureRigTerminalAt places a viewer in a requested free cell. Seat identity
// and provider lifecycle are independent of the viewer's grid position.
func (b *Broker) EnsureRigTerminalAt(owner, target, title string, argv []string, row, col *int, identity ...RigIdentity) (Session, error) {
	var rig *RigIdentity
	if len(identity) == 1 && identity[0].Target == target {
		value := identity[0]
		rig = &value
	}
	return b.ensureRigViewAt(owner, "managed:"+target, title, argv, true, rig, row, col)
}

func (b *Broker) ensureRigView(owner, view, title string, argv []string, grid bool, rig *RigIdentity) (Session, error) {
	return b.ensureRigViewAt(owner, view, title, argv, grid, rig, nil, nil)
}

func (b *Broker) ensureRigViewAt(owner, view, title string, argv []string, grid bool, rig *RigIdentity, requestedRow, requestedCol *int) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	var selected *entry
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner && (e.value.Companion == "openrig" || grid) && (e.value.OpenRigView == view || (view == "dashboard" && e.value.OpenRigView == "")) {
			selected = e
			break
		}
	}
	created := selected == nil
	if (requestedRow == nil) != (requestedCol == nil) {
		return Session{}, invalid("Terminal grid row and col must be provided together")
	}
	if grid && requestedRow != nil {
		limits, err := loadLimits(b.store)
		if err != nil {
			return Session{}, err
		}
		if err := limits.validate(*requestedRow, *requestedCol); err != nil {
			return Session{}, err
		}
		for _, item := range b.ownerEntriesLocked(owner) {
			if item != selected && !item.value.Minimized && item.value.Workspace == "" && item.value.Row == *requestedRow && item.value.Col == *requestedCol {
				return Session{}, invalid("Terminal grid cell is occupied")
			}
		}
	}
	if created {
		id, err := uuid()
		if err != nil {
			return Session{}, err
		}
		selected = &entry{value: Session{ID: id, Kind: "terminal", Owner: owner, Transport: "exec", Hostname: "localhost", Cols: 120, Rows: 36, Title: title, OpenRigView: view, Workspace: "openrig", Companion: "openrig", CreatedAt: now(), UpdatedAt: now()}}
		if grid {
			limits, err := loadLimits(b.store)
			if err != nil {
				return Session{}, err
			}
			row, col, err := limits.next(b.ownerEntriesLocked(owner), requestedRow, requestedCol)
			if err != nil {
				return Session{}, err
			}
			selected.value.Row, selected.value.Col = row, col
			selected.value.Workspace, selected.value.Companion = "", ""
			selected.value.Persistent = true
		}
		b.entries[id] = selected
		b.order = append(b.order, id)
	}
	oldValue := selected.value.clone()
	restoring := grid && selected.value.Minimized
	if restoring {
		limits, err := loadLimits(b.store)
		if err != nil {
			return Session{}, err
		}
		visible := []*entry{}
		for _, item := range b.ownerEntriesLocked(owner) {
			if !item.value.Minimized {
				visible = append(visible, item)
			}
		}
		row, col, err := limits.next(visible, requestedRow, requestedCol)
		if err != nil {
			return Session{}, err
		}
		selected.value.Row, selected.value.Col = row, col
		selected.value.Minimized = false
	}
	if grid && !created && !restoring && requestedRow != nil {
		selected.value.Row, selected.value.Col = *requestedRow, *requestedCol
	}
	launched := false
	if selected.run == nil {
		selected.value.ExecArgv = append([]string(nil), argv...)
		if err := b.startLocked(selected, "", ""); err != nil {
			if !created {
				selected.value = oldValue
			}
			return Session{}, err
		}
		launched = true
	}
	if rig != nil {
		value := *rig
		selected.value.Rig = &value
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
