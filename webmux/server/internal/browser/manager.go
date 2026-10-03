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
	mu      sync.Mutex
	cmd     *exec.Cmd
	in      io.WriteCloser
	lines   *bufio.Scanner
	done    chan struct{}
	owner   string
	stopped chan struct{}
	once    sync.Once
}

type Manager struct {
	mu      sync.Mutex
	clients map[string]*client
	closed  bool
}

func command(s session.Session, store *storage.Store) (*exec.Cmd, error) {
	if s.Transport == "local" || s.BrowserLocal {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		return exec.Command(binary, "--browser-worker"), nil
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
	args = append(args, "--", s.Hostname, "webmux --browser-worker")
	return exec.Command("ssh", args...), nil
}

func (m *Manager) Start(s session.Session, store *storage.Store) error {
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
	if m.clients[s.ID] != nil {
		return nil
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
	c.lines.Buffer(make([]byte, 4096), 8<<20)
	m.clients[s.ID] = c
	go func() { _ = cmd.Wait(); _ = in.Close(); close(c.done) }()
	return nil
}

func (m *Manager) Call(ctx context.Context, id string, req Request) (Frame, error) {
	m.mu.Lock()
	c := m.clients[id]
	m.mu.Unlock()
	if c == nil {
		return Frame{}, errors.New("Browser has ended; reopen it to start a new browser")
	}
	// Serialize input and frame responses across viewers of the same session.
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.stopped:
		return Frame{}, errors.New("Browser has stopped; end and reopen it")
	default:
	}
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
