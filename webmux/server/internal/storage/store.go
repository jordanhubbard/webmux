// Package storage preserves WebMux's on-disk YAML and JSONL formats.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// Store serializes read/modify/write operations within one server process.
// Running multiple servers against the same home is not supported.
type Store struct {
	Home    string
	mu      sync.Mutex
	eventMu sync.Mutex
}

func Open(home, defaults string) (*Store, error) {
	for _, dir := range []string{"config", "config/tls", "data/sessions", "data/events", "logs"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0700); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(defaults)
	if err != nil {
		return nil, fmt.Errorf("read configuration defaults: %w", err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		dest := filepath.Join(home, "config", entry.Name())
		// O_EXCL also preserves a symlink, including a dangling one. Never replace
		// an operator's configuration just because its target is unavailable.
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		data, readErr := os.ReadFile(filepath.Join(defaults, entry.Name()))
		if readErr == nil {
			_, readErr = f.Write(data)
		}
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			_ = os.Remove(dest)
			return nil, errors.Join(readErr, closeErr)
		}
	}
	return &Store{Home: home}, nil
}

// ConfigPath takes internal, fixed filenames, never request-supplied paths.
func (s *Store) ConfigPath(name string) string { return filepath.Join(s.Home, "config", name) }

func (s *Store) ReadConfig(name string, out any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readYAML(s.ConfigPath(name), out)
}

// InspectConfig holds the configuration lock through an operation derived from
// its contents. The callback must not call other Store methods. Upload cleanup
// uses this to prevent a key reference being added between inspection and deletion.
func InspectConfig[T any](s *Store, name string, inspect func(*T) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value T
	if err := readYAML(s.ConfigPath(name), &value); err != nil {
		return err
	}
	return inspect(&value)
}

func (s *Store) WriteConfig(name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeYAML(s.ConfigPath(name), value)
}

// ReadSessions and WriteSessions use the legacy terminal session document.
// The filename is fixed; session IDs never become filesystem paths.
func (s *Store) ReadSessions(out any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readYAML(filepath.Join(s.Home, "data", "sessions", "sessions.yaml"), out)
}

func (s *Store) WriteSessions(value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeYAML(filepath.Join(s.Home, "data", "sessions", "sessions.yaml"), value)
}

func desktopFilename(kind string) (string, error) {
	switch kind {
	case "vnc":
		return "vnc-sessions.yaml", nil
	case "rdp":
		return "rdp-sessions.yaml", nil
	default:
		return "", errors.New("invalid desktop kind")
	}
}
func (s *Store) ReadDesktopSessions(kind string, out any) error {
	name, err := desktopFilename(kind)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readYAML(filepath.Join(s.Home, "data", "sessions", name), out)
}
func (s *Store) WriteDesktopSessions(kind string, value any) error {
	name, err := desktopFilename(kind)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeYAML(filepath.Join(s.Home, "data", "sessions", name), value)
}

// UpdateConfig keeps the read, validation, mutation and atomic replacement in
// one critical section, preventing concurrent bootstrap or account updates
// from overwriting each other. Returning an error aborts the write.
func UpdateConfig[T any](s *Store, name string, change func(*T) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value T
	if err := readYAML(s.ConfigPath(name), &value); err != nil {
		return err
	}
	if err := change(&value); err != nil {
		return err
	}
	return writeYAML(s.ConfigPath(name), value)
}

func readYAML(name string, out any) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("multiple YAML documents are not supported")
	}
	return nil
}

func writeYAML(name string, value any) error {
	// Updating a linked config updates its target instead of removing the link.
	resolved, err := filepath.EvalSymlinks(name)
	if errors.Is(err, os.ErrNotExist) {
		// A missing file may be created, but a dangling operator-owned symlink
		// must never be silently replaced by a regular file.
		if _, linkErr := os.Lstat(name); errors.Is(linkErr, os.ErrNotExist) {
			parent, parentErr := filepath.EvalSymlinks(filepath.Dir(name))
			if parentErr != nil {
				return parentErr
			}
			resolved, err = filepath.Join(parent, filepath.Base(name)), nil
		}
	}
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(resolved), ".webmux-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), resolved)
}

// AppendEvent writes one complete JSON line per event. No passwords, password
// hashes, tokens or tickets should be supplied as fields.
func (s *Store) AppendEvent(event map[string]any) error {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	now := time.Now().UTC()
	record := make(map[string]any, len(event)+1)
	for key, value := range event {
		record[key] = value
	}
	record["ts"] = now.Format("2006-01-02T15:04:05.000Z")
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	name := filepath.Join(s.Home, "data", "events", "events-"+now.Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(line, '\n'))
	return errors.Join(writeErr, f.Close())
}
