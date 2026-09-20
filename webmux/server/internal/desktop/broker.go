// Package desktop owns VNC/RDP session records and transient credentials.
package desktop

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type Kind string

const (
	VNC Kind = "vnc"
	RDP Kind = "rdp"
)

var ErrNotFound = errors.New("Session not found")
var ErrClosed = errors.New("desktop broker is shutting down")

type Session struct {
	ID          string  `json:"id" yaml:"id"`
	Kind        Kind    `json:"kind" yaml:"kind"`
	Owner       string  `json:"owner" yaml:"owner"`
	HostID      string  `json:"host_id" yaml:"host_id"`
	Hostname    string  `json:"hostname" yaml:"hostname"`
	VNCPort     int     `json:"vnc_port,omitempty" yaml:"vnc_port,omitempty"`
	RDPPort     int     `json:"rdp_port,omitempty" yaml:"rdp_port,omitempty"`
	RDPUsername *string `json:"rdp_username,omitempty" yaml:"rdp_username,omitempty"`
	RDPDomain   *string `json:"rdp_domain,omitempty" yaml:"rdp_domain,omitempty"`
	Row         float64 `json:"row" yaml:"row"`
	Col         float64 `json:"col" yaml:"col"`
	State       string  `json:"state" yaml:"state"`
	CreatedAt   string  `json:"created_at" yaml:"created_at"`
	UpdatedAt   string  `json:"updated_at" yaml:"updated_at"`
	Title       string  `json:"title" yaml:"title"`
	Persistent  bool    `json:"persistent" yaml:"persistent"`
}

func (s Session) clone() Session {
	if s.RDPUsername != nil {
		v := *s.RDPUsername
		s.RDPUsername = &v
	}
	if s.RDPDomain != nil {
		v := *s.RDPDomain
		s.RDPDomain = &v
	}
	return s
}

type CreateRequest struct {
	HostID      string   `json:"host_id"`
	Hostname    string   `json:"hostname"`
	VNCPort     int      `json:"vnc_port"`
	VNCPassword string   `json:"vnc_password"`
	RDPPort     int      `json:"rdp_port"`
	RDPUsername string   `json:"rdp_username"`
	RDPPassword string   `json:"rdp_password"`
	RDPDomain   string   `json:"rdp_domain"`
	Row         *float64 `json:"row"`
	Col         *float64 `json:"col"`
}
type entry struct {
	value       Session
	password    string
	done        chan struct{}
	connections map[*Connection]string
}
type Broker struct {
	mu      sync.Mutex
	store   *storage.Store
	kind    Kind
	entries map[string]*entry
	order   []string
	closed  bool
}

func New(store *storage.Store, kind Kind) (*Broker, error) {
	if kind != VNC && kind != RDP {
		return nil, errors.New("invalid desktop kind")
	}
	b := &Broker{store: store, kind: kind, entries: map[string]*entry{}}
	var doc map[string][]Session
	if err := store.ReadDesktopSessions(string(kind), &doc); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load %s sessions: %w", kind, err)
	}
	for _, s := range doc[string(kind)+"_sessions"] {
		if s.ID == "" || b.entries[s.ID] != nil || s.Kind != kind {
			return nil, errors.New("invalid or duplicate persisted desktop session")
		}
		if !validPosition(s.Row, s.Col) {
			return nil, errors.New("invalid persisted desktop position")
		}
		b.entries[s.ID] = &entry{value: s, done: make(chan struct{})}
		b.order = append(b.order, s.ID)
	}
	return b, nil
}
func now() string                  { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
func invalid(message string) error { return &config.ValidationError{Message: message} }
func validPosition(row, col float64) bool {
	return row >= 0 && col >= 0 && !math.IsInf(row, 0) && !math.IsInf(col, 0) && !math.IsNaN(row) && !math.IsNaN(col)
}
func (b *Broker) persistLocked() error {
	items := make([]Session, 0, len(b.order))
	for _, id := range b.order {
		items = append(items, b.entries[id].value)
	}
	return b.store.WriteDesktopSessions(string(b.kind), map[string][]Session{string(b.kind) + "_sessions": items})
}
func (b *Broker) ownedLocked(owner, id string) (*entry, error) {
	e := b.entries[id]
	if e == nil || e.value.Owner != owner {
		return nil, ErrNotFound
	}
	return e, nil
}
func (b *Broker) ownerEntriesLocked(owner string) []*entry {
	items := []*entry{}
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner {
			items = append(items, e)
		}
	}
	return items
}

// Restore resets saved desktops without opening network connections. Credentials
// are intentionally absent after restart and never read from saved records.
func (b *Broker) Restore() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	previous := map[*entry]Session{}
	for _, e := range b.entries {
		previous[e] = e.value
		e.value.State = "disconnected"
		e.value.UpdatedAt = now()
	}
	if len(previous) == 0 {
		return nil
	}
	if err := b.persistLocked(); err != nil {
		for e, value := range previous {
			e.value = value
		}
		return err
	}
	return nil
}
func (b *Broker) List(owner string) []Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := []Session{}
	for _, e := range b.ownerEntriesLocked(owner) {
		items = append(items, e.value.clone())
	}
	return items
}
func (b *Broker) Get(owner, id string) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return Session{}, err
	}
	return e.value.clone(), nil
}

func (b *Broker) Create(owner string, request CreateRequest) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	if request.Hostname == "" && request.HostID == "" {
		return Session{}, invalid("hostname or host_id is required")
	}
	port, password := request.VNCPort, request.VNCPassword
	if b.kind == RDP {
		port, password = request.RDPPort, request.RDPPassword
	}
	if port == 0 {
		port = 5900
		if b.kind == RDP {
			port = 3389
		}
	}
	hostname := request.Hostname
	if request.HostID != "" {
		var hosts struct {
			Hosts []struct {
				ID       string `yaml:"id"`
				Hostname string `yaml:"hostname"`
				VNCPort  int    `yaml:"vnc_port"`
				RDPPort  int    `yaml:"rdp_port"`
			} `yaml:"hosts"`
		}
		if b.store.ReadConfig("hosts.yaml", &hosts) == nil {
			for _, h := range hosts.Hosts {
				if h.ID == request.HostID {
					hostname = h.Hostname
					override := h.VNCPort
					if b.kind == RDP {
						override = h.RDPPort
					}
					if override != 0 {
						port = override
					}
					break
				}
			}
		}
	}
	if port < 1 || port > 65535 {
		return Session{}, invalid("invalid desktop port")
	}
	row, col := float64(0), float64(0)
	if request.Row != nil && request.Col != nil {
		row, col = *request.Row, *request.Col
	} else {
		for i, e := range b.ownerEntriesLocked(owner) {
			if i == 0 || e.value.Row > row {
				row, col = e.value.Row, e.value.Col+1
			} else if e.value.Row == row {
				col = math.Max(col, e.value.Col+1)
			}
		}
	}
	if !validPosition(row, col) {
		return Session{}, invalid("row and col must be non-negative numbers")
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Session{}, err
	}
	idBytes[6] = (idBytes[6] & 15) | 64
	idBytes[8] = (idBytes[8] & 63) | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", idBytes[:4], idBytes[4:6], idBytes[6:8], idBytes[8:10], idBytes[10:])
	s := Session{ID: id, Kind: b.kind, Owner: owner, HostID: request.HostID, Hostname: hostname, Row: row, Col: col, State: "connecting", CreatedAt: now(), UpdatedAt: now(), Title: fmt.Sprintf("%s://%s:%d", b.kind, hostname, port), Persistent: true}
	if b.kind == VNC {
		s.VNCPort = port
	} else {
		s.RDPPort = port
		s.RDPUsername = &request.RDPUsername
		s.RDPDomain = &request.RDPDomain
	}
	b.entries[id] = &entry{value: s, password: password, done: make(chan struct{})}
	b.order = append(b.order, id)
	if err := b.persistLocked(); err != nil {
		delete(b.entries, id)
		b.order = b.order[:len(b.order)-1]
		return Session{}, err
	}
	return s.clone(), nil
}
func (b *Broker) Move(owner, id string, row, col float64) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return Session{}, err
	}
	if !validPosition(row, col) {
		return Session{}, invalid("row and col must be non-negative numbers")
	}
	old := e.value
	e.value.Row, e.value.Col = row, col
	e.value.UpdatedAt = now()
	if err := b.persistLocked(); err != nil {
		e.value = old
		return Session{}, err
	}
	return e.value.clone(), nil
}
func (b *Broker) SetState(owner, id, state string) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return Session{}, err
	}
	switch state {
	case "connecting", "connected", "disconnected", "error":
	default:
		return Session{}, invalid("invalid desktop state")
	}
	e.value.State = state
	e.value.UpdatedAt = now()
	return e.value.clone(), nil
}

// Credentials returns transient data only to the session's owner. The lifetime
// channel lets transport handlers stop when a session is deleted or shut down.
func (b *Broker) Credentials(owner, id string) (string, <-chan struct{}, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", nil, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return "", nil, err
	}
	return e.password, e.done, nil
}
func compact(items []*entry) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].value, items[j].value
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		return a.Col < b.Col
	})
	row, col := float64(-1), float64(0)
	previous := float64(-1)
	for _, e := range items {
		oldRow := e.value.Row
		if oldRow != previous {
			row++
			col = 0
			previous = oldRow
		}
		e.value.Row, e.value.Col = row, col
		col++
	}
}
func (b *Broker) Delete(owner, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return err
	}
	oldOrder := slices.Clone(b.order)
	positions := map[*entry][2]float64{}
	for _, item := range b.ownerEntriesLocked(owner) {
		positions[item] = [2]float64{item.value.Row, item.value.Col}
	}
	delete(b.entries, id)
	b.order = slices.DeleteFunc(b.order, func(key string) bool { return key == id })
	compact(b.ownerEntriesLocked(owner))
	if err := b.persistLocked(); err != nil {
		b.entries[id] = e
		b.order = oldOrder
		for item, pos := range positions {
			item.value.Row, item.value.Col = pos[0], pos[1]
		}
		return err
	}
	e.password = ""
	close(e.done)
	return nil
}
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for _, e := range b.entries {
		e.password = ""
		close(e.done)
	}
	return b.persistLocked()
}
