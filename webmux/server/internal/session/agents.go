package session

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/agent"
	"github.com/jordanhubbard/webmux/server/internal/config"
)

func (b *Broker) Agents() *agent.Service { return b.agents }

func (b *Broker) startPolicyLocked() {
	if b.policyStarted {
		return
	}
	b.policyStarted = true
	b.workers.Add(1)
	go func() {
		defer b.workers.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-b.policyStop:
				return
			case <-ticker.C:
				if err := b.EnforceAgentAccess(); err != nil && !errors.Is(err, ErrClosed) {
					b.logger.Warn("enforce agent access policy", "error", err)
				}
			}
		}
	}()
}

// EnforceAgentAccess also removes saved sessions for definitions that disappear.
// Malformed config blocks new access without destroying recoverable saved state.
func (b *Broker) EnforceAgentAccess() error {
	b.agentMu.Lock()
	defer b.agentMu.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	var sessions []Session
	for _, id := range b.order {
		if e := b.entries[id]; e.value.Agent() {
			sessions = append(sessions, e.value.clone())
		}
	}
	b.mu.Unlock()
	for _, s := range sessions {
		_, err := b.agents.Access(s.AgentID)
		var denied *agent.AccessError
		if errors.As(err, &denied) && denied.Status != 500 {
			if err := b.deleteSession(s.Owner, s.ID, 1008, denied.Message); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

func (b *Broker) markAttachReadyLocked(e *entry) {
	if e.value.AgentRole == "attach" && e.run != nil && e.value.State != "error" {
		changed := e.value.State != "connected"
		e.value.State = "connected"
		if changed {
			e.value.UpdatedAt = now()
		}
		b.broadcastLocked(e, Event{"type": "status", "session_id": e.value.ID, "state": "connected"})
	}
}

func interactiveArgv() ([]string, error) {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			var err error
			shell, err = exec.LookPath("cmd.exe")
			if err != nil {
				return nil, errors.New("Windows command shell (cmd.exe) was not found")
			}
		}
		return []string{shell}, nil
	}
	shell := config.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell, "-l"}, nil
}

func (b *Broker) EnsureAgentAttach(owner, id, name string, cols, rows int) (Session, bool, error) {
	argv, err := b.agents.AttachArgv(id, name)
	if err != nil {
		return Session{}, false, err
	}
	return b.ensureAgent(owner, id, "attach", name, cols, rows, argv, "")
}
func (b *Broker) EnsureAgentScratch(owner, id string, cols, rows int, cwd string) (Session, bool, error) {
	argv, err := interactiveArgv()
	if err != nil {
		return Session{}, false, err
	}
	return b.ensureAgent(owner, id, "scratch", "", cols, rows, argv, cwd)
}

// Serialize agent selection so simultaneous attach requests cannot create two
// processes for one owner's pane. Process termination stays outside the state
// mutex, allowing ConPTY output to drain during relaunch.
func (b *Broker) ensureAgent(owner, id, role, name string, cols, rows int, argv []string, cwd string) (Session, bool, error) {
	if cols < 1 || cols > 500 || rows < 1 || rows > 200 {
		return Session{}, false, invalid("invalid terminal size")
	}
	b.agentMu.Lock()
	defer b.agentMu.Unlock()
	c, err := b.agents.Access(id)
	if err != nil {
		return Session{}, false, err
	}
	definition, _ := c.Find(id)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Session{}, false, ErrClosed
	}
	b.startPolicyLocked()
	var selected *entry
	var duplicates []Session
	for _, key := range b.order {
		e := b.entries[key]
		if e.value.Owner == owner && e.value.AgentID == id && e.value.AgentRole == role {
			if selected == nil || role == "attach" && selected.value.AgentSessionName != name && e.value.AgentSessionName == name {
				selected = e
			}
		}
	}
	if role == "attach" && selected != nil {
		for _, key := range b.order {
			e := b.entries[key]
			if e != selected && e.value.Owner == owner && e.value.AgentID == id && e.value.AgentRole == role {
				duplicates = append(duplicates, e.value.clone())
			}
		}
	}
	b.mu.Unlock()
	for _, duplicate := range duplicates {
		if err := b.Delete(owner, duplicate.ID); err != nil && !errors.Is(err, ErrNotFound) {
			return Session{}, false, err
		}
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Session{}, false, ErrClosed
	}
	if selected != nil && b.entries[selected.value.ID] != selected {
		selected = nil
	}
	created := selected == nil
	if created {
		key, err := uuid()
		if err != nil {
			b.mu.Unlock()
			return Session{}, false, err
		}
		value := Session{ID: key, Kind: "terminal", Owner: owner, Transport: "exec", Hostname: id + ".local", Port: 22, Username: id, Cols: cols, Rows: rows, Title: name, State: "connecting", CreatedAt: now(), UpdatedAt: now(), Workspace: definition.Workspace, AgentID: id, AgentRole: role, AgentSessionName: name, ExecArgv: slices.Clone(argv), ExecCwd: cwd}
		if role == "scratch" {
			value.Username = "shell"
			value.Hostname = "local.shell"
			value.Title = "Scratch shell"
			value.Col = 1
		}
		selected = &entry{value: value}
		b.entries[key] = selected
		b.order = append(b.order, key)
	}
	oldValue := selected.value.clone()
	relaunch := selected.run == nil || selected.value.State == "disconnected" || selected.value.State == "error" || role == "attach" && (selected.value.AgentSessionName != name || !slices.Equal(selected.value.ExecArgv, argv))
	resize := selected.value.Cols != cols || selected.value.Rows != rows || role == "scratch"
	selected.value.Cols, selected.value.Rows = cols, rows
	selected.value.Workspace = definition.Workspace
	selected.value.ExecArgv = slices.Clone(argv)
	selected.value.ExecCwd = cwd
	selected.value.AgentSessionName = name
	selected.value.UpdatedAt = now()
	if role == "attach" {
		selected.value.Title = name
	}
	var previous *run
	if relaunch {
		previous = selected.run
		selected.run = nil
		if previous != nil {
			previous.stop()
			previous.log.stop("reconnect")
		}
	}
	b.mu.Unlock()
	if previous != nil {
		_ = previous.process.Close()
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Session{}, false, ErrClosed
	}
	if b.entries[selected.value.ID] != selected {
		b.mu.Unlock()
		return Session{}, false, ErrNotFound
	}
	// Recheck policy after a possibly slow process close.
	if _, err := b.agents.Access(id); err != nil {
		if created {
			delete(b.entries, selected.value.ID)
			b.order = slices.DeleteFunc(b.order, func(key string) bool { return key == selected.value.ID })
		} else {
			selected.value = oldValue
			if relaunch {
				selected.value.State = "disconnected"
			}
		}
		b.mu.Unlock()
		return Session{}, false, err
	}
	var launchErr error
	if relaunch && selected.run == nil {
		launchErr = b.startLocked(selected, "", "")
	}
	b.markAttachReadyLocked(selected)
	if err := b.persistLocked(); err != nil {
		current := selected.run
		if created {
			delete(b.entries, selected.value.ID)
			b.order = slices.DeleteFunc(b.order, func(key string) bool { return key == selected.value.ID })
		} else {
			selected.value = oldValue
			if relaunch {
				selected.value.State = "disconnected"
			}
		}
		if relaunch && current != nil {
			selected.run = nil
			current.stop()
			current.log.stop("create_failed")
		}
		b.mu.Unlock()
		if relaunch && current != nil {
			_ = current.process.Close()
		}
		return Session{}, false, err
	}
	value := selected.value.clone()
	current := selected.run
	if created {
		b.audit(map[string]any{"type": "session_created", "session_id": value.ID, "hostname": value.Hostname, "username": value.Username})
	}
	b.mu.Unlock()
	if !relaunch && resize && current != nil {
		if err := current.process.Resize(cols, rows); err != nil {
			return Session{}, created, err
		}
	}
	// Creation preserves the existing error-state response; a failed replacement
	// is reported as a failed attach/scratch operation.
	if !created && launchErr != nil {
		return value, false, launchErr
	}
	return value, created, nil
}
