package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/agent"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type process interface {
	io.ReadWriteCloser
	Resize(int, int) error
	Wait() terminal.Exit
}
type launchFunc func(terminal.LaunchRequest, string) (process, error)
type run struct {
	process     process
	initial     string
	timer       *time.Timer
	first       bool
	input       chan string
	stopped     chan struct{}
	stopOnce    sync.Once
	inputBytes  atomic.Int64
	generation  uint64
	log         *transcriptLog
	replayUntil time.Time
}

func (r *run) stop() {
	r.stopOnce.Do(func() { close(r.stopped) })
	if r.timer != nil {
		r.timer.Stop()
	}
}

type entry struct {
	value        Session
	run          *run
	scrollback   string
	viewers      map[string]*Viewer
	focus        string
	generation   uint64
	logOperation sync.Mutex
}

// Broker owns sessions and PTYs. Viewers do not own process lifetime. All map,
// state and persistence transitions share one lock; PTY reads/writes/close do
// not hold it. A run pointer is a generation token: old output and exit events
// can never mutate a reconnected or deleted session.
type Broker struct {
	mu            sync.Mutex
	store         *storage.Store
	logger        *slog.Logger
	launch        launchFunc
	entries       map[string]*entry
	order         []string
	closed        bool
	workers       sync.WaitGroup
	openLog       func(string, string, bool) (transcriptSink, error)
	agents        *agent.Service
	agentMu       sync.Mutex
	policyStop    chan struct{}
	policyStarted bool
	activity      map[agentActivityKey]pendingAgentActivity
	activityWake  chan struct{}
}

func New(store *storage.Store, logger *slog.Logger) (*Broker, error) {
	launcher := terminal.Launcher{Store: store}
	return newBroker(store, logger, func(request terminal.LaunchRequest, password string) (process, error) {
		return launcher.Launch(request, password)
	})
}
func newBroker(store *storage.Store, logger *slog.Logger, launch launchFunc) (*Broker, error) {
	if logger == nil {
		logger = slog.Default()
	}
	b := &Broker{store: store, logger: logger, launch: launch, entries: map[string]*entry{}, openLog: openTranscript, agents: agent.New(store, logger), policyStop: make(chan struct{})}
	var saved document
	if err := store.ReadSessions(&saved); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load terminal sessions: %w", err)
	}
	for _, s := range saved.Sessions {
		if s.ID == "" || b.entries[s.ID] != nil {
			return nil, errors.New("invalid or duplicate persisted session ID")
		}
		b.entries[s.ID] = &entry{value: s}
		b.order = append(b.order, s.ID)
	}
	return b, nil
}

// Restore is separate from construction so startup can validate listeners and
// TLS before reconnecting persisted sessions. Agent recovery is owned by the
// agent service and must never be inferred from a saved exec argv alone.
func (b *Broker) Restore() error {
	if err := b.EnforceAgentAccess(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	b.startPolicyLocked()
	for _, id := range b.order {
		e := b.entries[id]
		if e.run != nil {
			continue
		}
		if e.value.Persistent && e.value.Hostname != "" && !e.value.Agent() {
			_ = b.startLocked(e, "", "")
		}
	}
	if len(b.order) > 0 {
		return b.persistLocked()
	}
	return nil
}

func (b *Broker) ownedLocked(owner, id string) (*entry, error) {
	e := b.entries[id]
	if e == nil || e.value.Owner != owner {
		return nil, ErrNotFound
	}
	return e, nil
}
func (b *Broker) ownerEntriesLocked(owner string) []*entry {
	result := []*entry{}
	for _, id := range b.order {
		e := b.entries[id]
		if e.value.Owner == owner && !e.value.Agent() {
			result = append(result, e)
		}
	}
	return result
}
func (b *Broker) List(owner string) []Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := []Session{}
	for _, e := range b.ownerEntriesLocked(owner) {
		result = append(result, e.value.clone())
	}
	return result
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
func (b *Broker) Scrollback(owner, id string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return "", err
	}
	return e.scrollback, nil
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
func uuid() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = bytes[6]&15 | 64
	bytes[8] = bytes[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:]), nil
}

func (b *Broker) Create(owner string, request CreateRequest) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	if request.Username == "" {
		return Session{}, invalid("username is required")
	}
	id, err := uuid()
	if err != nil {
		return Session{}, err
	}
	if request.Port == 0 {
		request.Port = 22
	}
	if request.Cols == 0 {
		request.Cols = 80
	}
	if request.Rows == 0 {
		request.Rows = 24
	}
	if request.Transport == "" {
		request.Transport = "ssh"
	}
	if request.HostID != "" {
		var hosts struct {
			Hosts []struct {
				ID, Hostname string
				Port         int
				Mosh         bool `yaml:"mosh_allowed"`
			} `yaml:"hosts"`
		}
		if b.store.ReadConfig("hosts.yaml", &hosts) == nil {
			for _, host := range hosts.Hosts {
				if host.ID == request.HostID {
					request.Hostname, request.Port = host.Hostname, host.Port
					if request.Transport == "ssh" && host.Mosh {
						var app config.Document
						if b.store.ReadConfig("app.yaml", &app) == nil && config.AsObject(app.App["transport"])["prefer_mosh"] == true {
							request.Transport = "mosh"
						}
					}
					break
				}
			}
		}
	}
	limits, err := loadLimits(b.store)
	if err != nil {
		return Session{}, err
	}
	row, col, err := limits.next(b.ownerEntriesLocked(owner), request.Row, request.Col)
	if err != nil {
		return Session{}, err
	}
	title := request.Username + "@" + request.Hostname
	if request.Transport == "exec" {
		title = fmt.Sprintf("%s:%d", request.Hostname, request.Port)
	}
	s := Session{ID: id, Kind: "terminal", Owner: owner, Transport: request.Transport, HostID: request.HostID, Hostname: request.Hostname, Port: request.Port, Username: request.Username, KeyID: request.KeyID, ExecCommand: request.ExecCommand, Cols: request.Cols, Rows: request.Rows, Row: row, Col: col, State: "connecting", CreatedAt: now(), UpdatedAt: now(), Title: title, Persistent: true}
	e := &entry{value: s}
	b.entries[id] = e
	b.order = append(b.order, id)
	initial := request.InitialCommand
	if initial == "" {
		initial = templateCommands[request.TemplateID]
	}
	// A failed launch is a created session in state=error, matching the browser's
	// reconnect flow. The transient password is never copied into session state.
	_ = b.startLocked(e, request.Password, initial)
	if err := b.persistLocked(); err != nil {
		delete(b.entries, id)
		b.order = b.order[:len(b.order)-1]
		if e.run != nil {
			r := e.run
			e.run = nil
			r.stop()
			r.log.stop("create_failed")
			go r.process.Close()
		}
		return Session{}, err
	}
	b.audit(map[string]any{"type": "session_created", "session_id": id, "hostname": s.Hostname, "username": s.Username})
	return e.value.clone(), nil
}

var templateCommands = map[string]string{"claude-cli": "claude", "htop": "htop", "python-repl": "python3", "nano-repl": "nano --repl", "ssh-agent": `eval $(ssh-agent -s) && ssh-add ~/.ssh/id_rsa && echo "SSH agent ready"`}

func (b *Broker) startLocked(e *entry, password, initial string) error {
	e.generation++
	e.value.State = "connecting"
	e.value.UpdatedAt = now()
	e.scrollback = ""
	p, err := b.launch(e.value.launchRequest(), password)
	if err != nil {
		e.value.State = "error"
		e.value.UpdatedAt = now()
		b.logger.Warn("terminal launch failed", "session_id", e.value.ID, "error", err)
		return err
	}
	r := &run{process: p, initial: initial, first: true, input: make(chan string, 256), stopped: make(chan struct{}), generation: e.generation, replayUntil: time.Now().Add(1500 * time.Millisecond)}
	e.run = r
	if b.loggingEnabledLocked() {
		b.openTranscriptLocked(e, r)
	}
	b.broadcastLocked(e, b.transcriptEventLocked(e))
	b.workers.Add(2)
	go b.read(e.value.ID, r)
	go b.writeInput(r)
	return nil
}

func (b *Broker) Reconnect(owner, id, password string) (Session, error) {
	b.agentMu.Lock()
	defer b.agentMu.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Session{}, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		b.mu.Unlock()
		return Session{}, err
	}
	if e.value.Agent() {
		if _, err := b.agents.Access(e.value.AgentID); err != nil {
			b.mu.Unlock()
			return Session{}, err
		}
	}
	old := e.run
	e.run = nil
	if old != nil {
		old.stop()
		old.log.stop("reconnect")
	}
	// Invalidate under the lock, but terminate outside it: ConPTY close may
	// drain output which needs this same lock to finish.
	b.mu.Unlock()
	if old != nil {
		_ = old.process.Close()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	current, err := b.ownedLocked(owner, id)
	if err != nil {
		return Session{}, err
	}
	if current != e || e.run != nil {
		return e.value.clone(), nil
	}
	if e.value.Agent() {
		if _, err := b.agents.Access(e.value.AgentID); err != nil {
			return Session{}, err
		}
	}
	err = b.startLocked(e, password, "")
	if err == nil {
		b.markAttachReadyLocked(e)
	}
	if persistErr := b.persistLocked(); persistErr != nil {
		return Session{}, persistErr
	}
	return e.value.clone(), err
}

func (b *Broker) Patch(owner, id string, patch Patch) (Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Session{}, ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return Session{}, err
	}
	old := e.value
	switch {
	case patch.Minimized != nil:
		e.value.Minimized = *patch.Minimized
	case patch.Title != nil:
		title := config.TrimSpace(*patch.Title)
		if len(utf16.Encode([]rune(title))) < 1 || len(utf16.Encode([]rune(title))) > 128 {
			return Session{}, invalid("title must be 1-128 characters")
		}
		e.value.Title = title
	default:
		if patch.Row == nil || patch.Col == nil {
			return Session{}, invalid("row and col are required")
		}
		if e.value.Agent() {
			return Session{}, invalid("Agent workspace sessions cannot be moved")
		}
		limits, err := loadLimits(b.store)
		if err != nil {
			return Session{}, err
		}
		if err = limits.validate(*patch.Row, *patch.Col); err != nil {
			return Session{}, err
		}
		e.value.Row, e.value.Col = *patch.Row, *patch.Col
	}
	e.value.UpdatedAt = now()
	if err := b.persistLocked(); err != nil {
		e.value = old
		return Session{}, err
	}
	return e.value.clone(), nil
}

func (b *Broker) Delete(owner, id string) error {
	return b.deleteSession(owner, id, 1000, "Session deleted")
}
func (b *Broker) deleteSession(owner, id string, code int, reason string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	oldOrder := append([]string{}, b.order...)
	oldPositions := map[*entry][2]int{}
	delete(b.entries, id)
	b.order = slices.DeleteFunc(b.order, func(value string) bool { return value == id })
	if !e.value.Agent() {
		items := b.ownerEntriesLocked(owner)
		for _, item := range items {
			oldPositions[item] = [2]int{item.value.Row, item.value.Col}
		}
		compact(items)
	}
	if err := b.persistLocked(); err != nil {
		b.entries[id] = e
		b.order = oldOrder
		for item, pos := range oldPositions {
			item.value.Row, item.value.Col = pos[0], pos[1]
		}
		// A failed disk write must not keep a revoked agent process accessible.
		// Retain its saved record for a later cleanup retry, but revoke viewers
		// and the process immediately.
		if code == 1008 {
			r := e.run
			e.run = nil
			b.closeViewersLocked(e, code, reason)
			e.value.State = "disconnected"
			e.value.UpdatedAt = now()
			if r != nil {
				r.stop()
				r.log.stop("deleted")
			}
			b.mu.Unlock()
			if r != nil {
				_ = r.process.Close()
			}
			return err
		}
		b.mu.Unlock()
		return err
	}
	r := e.run
	e.run = nil
	b.closeViewersLocked(e, code, reason)
	if r != nil {
		r.stop()
		r.log.stop("deleted")
	}
	b.audit(map[string]any{"type": "session_deleted", "session_id": id})
	b.mu.Unlock()
	if r != nil {
		_ = r.process.Close()
	}
	return nil
}

func (b *Broker) Input(owner, id, data string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	var current *run
	if err == nil && e.run != nil {
		current = e.run
	}
	b.mu.Unlock()
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	select {
	case <-current.stopped:
		return ErrClosed
	default:
	}
	size := int64(len(data))
	if current.inputBytes.Add(size) > 2<<20 {
		current.inputBytes.Add(-size)
		return ErrInputBusy
	}
	select {
	case current.input <- data:
		b.mu.Lock()
		if e := b.entries[id]; !b.closed && e != nil && e.run == current {
			b.recordAgentActivityLocked(e, current, true)
		}
		b.mu.Unlock()
		return nil
	case <-current.stopped:
		current.inputBytes.Add(-size)
		return ErrClosed
	default:
		current.inputBytes.Add(-size)
		return ErrInputBusy
	}
}

var ErrInputBusy = errors.New("terminal input queue is full")

func (b *Broker) writeInput(r *run) {
	defer b.workers.Done()
	for {
		select {
		case <-r.stopped:
			return
		default:
		}
		select {
		case <-r.stopped:
			return
		case data := <-r.input:
			r.inputBytes.Add(-int64(len(data)))
			if _, err := r.process.Write([]byte(data)); err != nil {
				return
			}
		}
	}
}
func (b *Broker) Resize(owner, id string, cols, rows int) error {
	if cols < 1 || cols > 500 || rows < 1 || rows > 200 {
		return invalid("invalid terminal size")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	e, err := b.ownedLocked(owner, id)
	var p process
	if err == nil {
		e.value.Cols, e.value.Rows = cols, rows
		e.value.UpdatedAt = now()
		if e.run != nil {
			p = e.run.process
		}
	}
	b.mu.Unlock()
	if err != nil {
		return err
	}
	if p != nil {
		return p.Resize(cols, rows)
	}
	return nil
}

func (b *Broker) read(id string, r *run) {
	defer b.workers.Done()
	defer r.process.Close()
	buffer := make([]byte, 32*1024)
	pending := []byte{}
	for {
		n, err := r.process.Read(buffer)
		pending = append(pending, buffer[:n]...)
		var output strings.Builder
		for len(pending) > 0 && (err != nil || utf8.FullRune(pending)) {
			value, size := utf8.DecodeRune(pending)
			output.WriteRune(value)
			pending = pending[size:]
		}
		if output.Len() > 0 {
			b.output(id, r, output.String())
		}
		if err != nil {
			break
		}
	}
	exit := r.process.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[id]
	if b.closed || e == nil || e.run != r {
		return
	}
	e.run = nil
	r.stop()
	r.log.stop("process_exit")
	e.value.State = "disconnected"
	e.value.UpdatedAt = now()
	b.broadcastLocked(e, Event{"type": "status", "session_id": id, "state": "disconnected", "message": fmt.Sprintf("Process exited with code %d", exit.Code), "transcript_enabled": false})
	if err := b.persistLocked(); err != nil {
		b.logger.Error("save terminal exit", "error", err)
	}
	b.audit(map[string]any{"type": "session_exited", "session_id": id, "exit_code": exit.Code})
}
func (b *Broker) output(id string, r *run, data string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[id]
	if b.closed || e == nil || e.run != r {
		return
	}
	if r.log != nil {
		if err := r.log.append(data); err != nil {
			b.transcriptErrorLocked(id, r, r.log, err)
		}
	}
	if r.first {
		r.first = false
		e.value.State = "connected"
		e.value.UpdatedAt = now()
		b.broadcastLocked(e, Event{"type": "status", "session_id": id, "state": "connected"})
		if r.initial != "" {
			r.timer = time.AfterFunc(800*time.Millisecond, func() {
				b.mu.Lock()
				e := b.entries[id]
				valid := !b.closed && e != nil && e.run == r
				b.mu.Unlock()
				if valid {
					_, _ = r.process.Write([]byte(r.initial + "\r"))
				}
			})
		}
		if err := b.persistLocked(); err != nil {
			b.logger.Error("save terminal connection", "error", err)
		}
	}
	e.scrollback = trimScrollback(e.scrollback + data)
	b.broadcastLocked(e, Event{"type": "output", "session_id": id, "data": data})
	b.recordAgentActivityLocked(e, r, false)
}
func trimScrollback(value string) string {
	// JS bounds this buffer in UTF-16 code units. Keep the same bound without
	// cutting a surrogate pair or producing invalid UTF-8 in JSON output.
	units := 0
	start := len(value)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(value[:start])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > 64*1024 {
			break
		}
		units += width
		start -= size
	}
	if start == 0 {
		return value
	}
	value = value[start:]
	if newline := strings.IndexByte(value, '\n'); newline >= 0 && len(utf16.Encode([]rune(value[:newline]))) < 4096 {
		value = value[newline+1:]
	}
	return value
}

func (b *Broker) persistLocked() error {
	sessions := make([]Session, 0, len(b.order))
	tiles := []map[string]any{}
	for _, id := range b.order {
		s := b.entries[id].value
		sessions = append(sessions, s)
		if !s.Agent() {
			tiles = append(tiles, map[string]any{"session_id": s.ID, "row": s.Row, "col": s.Col})
		}
	}
	if err := b.store.WriteSessions(document{sessions}); err != nil {
		return err
	}
	// Layout is a derived view. A missing layout file must not lose sessions;
	// every later session persistence repairs tiles without dropping UI metadata.
	err := storage.UpdateConfig[config.Object](b.store, "layout.yaml", func(doc *config.Object) error {
		layout, ok := (*doc)["layout"].(map[string]any)
		if !ok {
			return errors.New("layout is missing")
		}
		layout["tiles"] = tiles
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		b.logger.Warn("update session layout", "error", err)
	}
	return nil
}
func (b *Broker) audit(event map[string]any) {
	if err := b.store.AppendEvent(event); err != nil {
		b.logger.Warn("write session audit event", "error", err)
	}
}
func (b *Broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.workers.Wait()
		return nil
	}
	b.closed = true
	close(b.policyStop)
	var processes []process
	for _, e := range b.entries {
		b.closeViewersLocked(e, 1001, "Server shutting down")
		if e.run != nil {
			e.run.stop()
			e.run.log.stop("shutdown")
			processes = append(processes, e.run.process)
			e.run = nil
		}
		if e.value.State == "connected" || e.value.State == "connecting" {
			e.value.State = "disconnected"
			e.value.UpdatedAt = now()
		}
	}
	err := b.persistLocked()
	b.mu.Unlock()
	for _, p := range processes {
		_ = p.Close()
	}
	b.workers.Wait()
	return err
}
