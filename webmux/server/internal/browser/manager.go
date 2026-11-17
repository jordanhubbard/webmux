package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type client struct {
	control      controllerLease
	profile      Profile
	initialized  bool
	resetPending bool
	mu           sync.Mutex
	cmd          *exec.Cmd
	in           io.WriteCloser
	lines        *bufio.Scanner
	done         chan struct{}
	owner        string
	stopped      chan struct{}
	once         sync.Once
}

type Manager struct {
	mu      sync.Mutex
	clients map[string]*client
	closed  bool
}

func command(s session.Session, store *storage.Store) (*exec.Cmd, error) {
	return WorkerCommand(context.Background(), s, store, "--browser-worker")
}

// WorkerCommand uses only fixed worker modes, never user URLs or shell text.
func WorkerCommand(ctx context.Context, s session.Session, store *storage.Store, mode string) (*exec.Cmd, error) {
	if mode != "--browser-worker" && mode != "--auth-callback" && mode != "--browser-profile-remove" {
		return nil, errors.New("Unsupported worker mode")
	}
	if s.Transport == "local" || s.BrowserLocal {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		return exec.CommandContext(ctx, binary, mode), nil
	}
	if s.Transport != "ssh" && s.Transport != "mosh" && s.Transport != "" {
		return nil, errors.New("Browser companion requires a Local, SSH, or Mosh terminal; exec commands do not identify the shell host")
	}
	port := s.Port
	if port == 0 {
		port = 22
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-p", strconv.Itoa(port)}
	if s.KeyID != "" {
		var keys struct {
			Keys []struct {
				ID   string `yaml:"id"`
				Path string `yaml:"private_key_path"`
			} `yaml:"keys"`
		}
		if err := store.ReadConfig("keys.yaml", &keys); err != nil {
			return nil, err
		}
		found := false
		for _, key := range keys.Keys {
			if key.ID == s.KeyID {
				args = append(args, "-i", key.Path)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("Terminal SSH key is no longer configured")
		}
	}
	if s.Username != "" {
		args = append(args, "-l", s.Username)
	}
	// Session hostnames and usernames have already passed terminal validation.
	args = append(args, "--", s.Hostname, "webmux "+mode)
	return exec.CommandContext(ctx, "ssh", args...), nil
}

func (m *Manager) Start(s session.Session, store *storage.Store) error {
	return m.StartProfile(s, store, nil)
}

func (m *Manager) StartProfile(s session.Session, store *storage.Store, profile *Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("Server is shutting down")
	}
	if m.clients == nil {
		m.clients = make(map[string]*client)
	}
	for id, c := range m.clients {
		select {
		case <-c.done:
			c.close()
			delete(m.clients, id)
		default:
		}
	}
	if existing := m.clients[s.ID]; existing != nil {
		if profile != nil && profile.Key != existing.profile.Key {
			return errors.New("End the current browser before changing profiles")
		}
		return nil
	}
	if profile != nil && profile.Key != "" {
		for _, existing := range m.clients {
			if existing.profile.Key == profile.Key {
				return errors.New("Saved browser profile is already open; end that browser first")
			}
		}
	}
	owned := 0
	for _, c := range m.clients {
		if c.owner == s.Owner {
			owned++
		}
	}
	if owned >= 4 || len(m.clients) >= 16 {
		return errors.New("Browser limit reached; end an existing browser first")
	}
	cmd, err := command(s, store)
	if err != nil {
		return err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return err
	}
	// Never relay stderr: browser diagnostics can contain authentication URLs.
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return errors.New("Cannot launch browser worker; check the WebMux binary and SSH installation")
	}
	c := &client{cmd: cmd, in: in, lines: bufio.NewScanner(out), done: make(chan struct{}), owner: s.Owner, stopped: make(chan struct{})}
	if profile != nil {
		c.profile = *profile
	}
	c.lines.Buffer(make([]byte, 4096), MaxMessageBytes)
	m.clients[s.ID] = c
	go func() { _ = cmd.Wait(); _ = in.Close(); close(c.done) }()
	return nil
}

// Call is an internal worker operation. HTTP viewers use CallControlled.
func (m *Manager) Call(ctx context.Context, id string, req Request) (Frame, error) {
	return m.call(ctx, id, req, false)
}
func (m *Manager) CallControlled(ctx context.Context, id string, req Request) (Frame, error) {
	return m.call(ctx, id, req, true)
}
func (m *Manager) call(ctx context.Context, id string, req Request, guard bool) (Frame, error) {
	m.mu.Lock()
	c := m.clients[id]
	m.mu.Unlock()
	if c == nil {
		return Frame{}, errors.New("Browser has ended; reopen it to start a new browser")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.stopped:
		return Frame{}, errors.New("Browser has stopped; end and reopen it")
	default:
	}
	// Check at the actual worker boundary, after waiting for earlier requests.
	if guard && req.Action != "frame" {
		if err := c.control.authorize(req.Viewer, req.ControlToken); err != nil {
			return Frame{}, err
		}
	}
	viewer, token := req.Viewer, req.ControlToken
	req.Viewer, req.ControlToken = "", ""
	req.ProfileID = nil
	req.ProfileKey = ""
	req.Force = false
	if c.resetPending && c.initialized {
		frame, err := c.exchange(ctx, Request{Action: "release_input"})
		if err != nil {
			return frame, err
		}
		if frame.Error != "" {
			c.close()
			return Frame{}, errors.New("Cannot release browser input; end and reopen it")
		}
		c.resetPending = false
	}
	wire := req
	configuring := !c.initialized && c.profile.Key != ""
	if configuring {
		wire = Request{Action: "configure", ProfileKey: c.profile.Key}
	}
	frame, err := c.exchange(ctx, wire)
	if err != nil {
		return frame, err
	}
	if frame.Protocol != 0 && frame.Protocol != Protocol {
		c.close()
		return Frame{}, errors.New("Update WebMux on the terminal host; browser profiles and controller leases require a matching worker")
	}
	if frame.Protocol == Protocol && frame.ProfileKey != c.profile.Key {
		c.close()
		return Frame{}, errors.New("Update WebMux on the terminal host before using saved browser profiles")
	}
	c.initialized = frame.Protocol == Protocol
	if configuring {
		c.resetPending = false
	}
	if configuring && req.Action != "frame" && c.initialized {
		frame, err = c.exchange(ctx, req)
		if err != nil {
			return frame, err
		}
	}
	frame.ProfileKey = ""
	frame.ProfileID = c.profile.ID
	frame.ProfileName = c.profile.Name
	if guard {
		control, _ := c.control.control(viewer, token, false, false, false)
		frame.Control = &control
	}
	return frame, nil
}
func (c *client) exchange(ctx context.Context, req Request) (Frame, error) {
	result := make(chan Frame, 1)
	failure := make(chan error, 1)
	go func() {
		if err := json.NewEncoder(c.in).Encode(req); err != nil {
			failure <- err
			return
		}
		if !c.lines.Scan() {
			failure <- io.EOF
			return
		}
		var frame Frame
		if err := json.Unmarshal(c.lines.Bytes(), &frame); err != nil {
			failure <- err
			return
		}
		result <- frame
	}()
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	select {
	case frame := <-result:
		return frame, nil
	case <-failure:
		c.close()
		return Frame{}, errors.New("Browser worker stopped. On SSH hosts, install matching WebMux and Chrome/Chromium, and configure non-interactive SSH key/agent access. Reopen to retry.")
	case <-timer.C:
		c.close()
		return Frame{}, errors.New("Browser worker timed out; end and reopen the browser to retry")
	case <-ctx.Done():
		c.close()
		return Frame{}, ctx.Err()
	}
}
func (m *Manager) ProfileInUse(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		if c.profile.Key == key {
			select {
			case <-c.done:
			default:
				return true
			}
		}
	}
	return false
}
func (m *Manager) EndControlled(id, viewer, token string) error {
	m.mu.Lock()
	c := m.clients[id]
	m.mu.Unlock()
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.control.authorize(viewer, token); err != nil {
		return err
	}
	m.mu.Lock()
	if m.clients[id] == c {
		delete(m.clients, id)
	}
	m.mu.Unlock()
	c.close()
	select {
	case <-c.done:
		return nil
	case <-time.After(22 * time.Second):
		return errors.New("Browser is still stopping; wait before reopening its saved profile")
	}
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.stopped)
		_ = c.in.Close() // EOF lets the worker close Chrome and delete its profile.
		go func() {
			select {
			case <-c.done:
			case <-time.After(20 * time.Second):
				_ = c.cmd.Process.Kill()
			}
		}()
	})
}

func (m *Manager) End(id string) {
	m.mu.Lock()
	c := m.clients[id]
	delete(m.clients, id)
	m.mu.Unlock()
	if c != nil {
		c.close()
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	clients := m.clients
	m.clients = nil
	m.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
	for _, c := range clients {
		<-c.done
	}
	return nil
}
