package session

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type fakeProcess struct {
	reader     *io.PipeReader
	writer     *io.PipeWriter
	done       chan struct{}
	once       sync.Once
	input      chan string
	delayClose bool
}

func fake() *fakeProcess {
	r, w := io.Pipe()
	return &fakeProcess{reader: r, writer: w, done: make(chan struct{}), input: make(chan string, 10)}
}
func (p *fakeProcess) Read(data []byte) (int, error) { return p.reader.Read(data) }
func (p *fakeProcess) Write(data []byte) (int, error) {
	select {
	case <-p.done:
		return 0, os.ErrClosed
	case p.input <- string(data):
		return len(data), nil
	}
}
func (p *fakeProcess) Resize(int, int) error { return nil }
func (p *fakeProcess) Wait() terminal.Exit   { <-p.done; return terminal.Exit{Code: 7} }
func (p *fakeProcess) exit()                 { p.once.Do(func() { _ = p.writer.Close(); close(p.done) }) }
func (p *fakeProcess) Close() error {
	if !p.delayClose {
		p.exit()
	}
	return nil
}
func (p *fakeProcess) output(t *testing.T, value string) {
	t.Helper()
	if _, err := io.WriteString(p.writer, value); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (*Broker, *storage.Store, chan *fakeProcess) {
	t.Helper()
	defaults := t.TempDir()
	for name, value := range map[string]string{"app.yaml": "app:\n  terminal_grid: {max_cols: 3, max_rows: 4}\n", "layout.yaml": "layout:\n  columns: 3\n  tiles: []\n  custom: keep\n"} {
		if err := os.WriteFile(filepath.Join(defaults, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := storage.Open(t.TempDir(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	launched := make(chan *fakeProcess, 100)
	b, err := newBroker(store, slog.New(slog.NewTextHandler(io.Discard, nil)), func(terminal.LaunchRequest, string) (process, error) { p := fake(); launched <- p; return p, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, store, launched
}
func create(t *testing.T, b *Broker, owner string) Session {
	t.Helper()
	s, err := b.Create(owner, CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("session transition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSessionLifecycleAndStaleGeneration(t *testing.T) {
	b, store, launched := fixture(t)
	s := create(t, b, "owner")
	first := <-launched
	first.delayClose = true // Let old output/exit arrive after a successful reconnect.
	first.output(t, "first λ\n")
	eventually(t, func() bool { value, _ := b.Get("owner", s.ID); return value.State == "connected" })
	if _, err := b.Get("other", s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	if err := b.Input("other", s.ID, "secret"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign terminal accepted input")
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	second := <-launched
	first.output(t, "obsolete output\n")
	first.exit()
	second.output(t, "new output\n")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return strings.Contains(text, "new output") })
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(text, "obsolete") || strings.Contains(text, "first") {
		t.Fatalf("old output retained: %q", text)
	}
	value, _ := b.Get("owner", s.ID)
	if value.State != "connected" {
		t.Fatalf("old exit changed new state: %s", value.State)
	}
	if err := b.Input("owner", s.ID, "hello\r"); err != nil {
		t.Fatal(err)
	}
	if input := <-second.input; input != "hello\r" {
		t.Fatalf("input changed: %q", input)
	}
	second.exit()
	eventually(t, func() bool { value, _ := b.Get("owner", s.ID); return value.State == "disconnected" })
	var saved document
	if err := store.ReadSessions(&saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Sessions) != 1 || saved.Sessions[0].State != "disconnected" {
		t.Fatalf("saved sessions: %+v", saved)
	}
	if err := b.Delete("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	if len(b.List("owner")) != 0 {
		t.Fatal("deleted session retained")
	}
}

func TestConcurrentGridAllocationAndOwnerCompaction(t *testing.T) {
	b, store, _ := fixture(t)
	var workers sync.WaitGroup
	failures := make(chan error, 12)
	for range 12 {
		workers.Go(func() {
			_, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice"})
			failures <- err
		})
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	items := b.List("owner")
	positions := map[[2]int]bool{}
	for _, s := range items {
		position := [2]int{s.Row, s.Col}
		if positions[position] {
			t.Fatalf("duplicate grid position: %v", position)
		}
		positions[position] = true
	}
	if _, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice"}); err == nil || err.Error() != "Terminal grid is full" {
		t.Fatalf("full grid: %v", err)
	}
	other := create(t, b, "other")
	if other.Row != 0 || other.Col != 0 {
		t.Fatal("owners share a grid")
	}
	if err := b.Delete("owner", items[1].ID); err != nil {
		t.Fatal(err)
	}
	if value, _ := b.Get("other", other.ID); value.Row != 0 || value.Col != 0 {
		t.Fatal("delete moved another owner's session")
	}
	row0 := 0
	for _, s := range b.List("owner") {
		if s.Row == 0 {
			if s.Col > 1 {
				t.Fatal("gap after deletion")
			}
			row0++
		}
	}
	if row0 != 2 {
		t.Fatalf("row size: %d", row0)
	}
	var layout map[string]any
	if err := store.ReadConfig("layout.yaml", &layout); err != nil {
		t.Fatal(err)
	}
	body := layout["layout"].(map[string]any)
	if body["custom"] != "keep" || len(body["tiles"].([]any)) != 12 {
		t.Fatalf("layout: %+v", layout)
	}
}

func TestSessionRestartAndPasswordNotPersisted(t *testing.T) {
	b, store, launched := fixture(t)
	s, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Password: "fixture-secret"})
	if err != nil {
		t.Fatal(err)
	}
	(<-launched).output(t, "ready")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.Home, "data/sessions/sessions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("fixture-secret")) || bytes.Contains(data, []byte("password")) {
		t.Fatal("password persisted")
	}
	var recovered terminal.LaunchRequest
	next, err := newBroker(store, slog.Default(), func(request terminal.LaunchRequest, password string) (process, error) {
		recovered = request
		if password != "" {
			t.Error("password survived restart")
		}
		return fake(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	value, err := next.Get("owner", s.ID)
	if err != nil || value.State != "connecting" || recovered.Hostname != "localhost" {
		t.Fatalf("recovery: %+v, %v", value, err)
	}
}

func TestSplitUTF8AndScrollbackBound(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "\xf0\x9f")
	p.output(t, "\x98\x80\n")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return text == "😀\n" })
	p.output(t, strings.Repeat("日本語😀\n", 15000))
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return len(text) > 1000 })
	text, _ := b.Scrollback("owner", s.ID)
	if !utf8.ValidString(text) || strings.ContainsRune(text, utf8.RuneError) || len(text) > 4*65536 {
		t.Fatal("invalid or unbounded scrollback")
	}
}

func TestCorruptSessionsFailClosed(t *testing.T) {
	b, store, _ := fixture(t)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	bad := []byte("sessions: [broken")
	if err := os.WriteFile(file, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(store, nil); err == nil {
		t.Fatal("corrupt saved sessions accepted")
	}
	actual, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(actual, bad) {
		t.Fatal("corrupt data was overwritten")
	}
}

func TestFailedPersistenceRollsBackAndClosesProcess(t *testing.T) {
	b, store, launched := fixture(t)
	file := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice"})
	if err == nil {
		t.Fatal("creation succeeded without persisted state")
	}
	p := <-launched
	eventually(t, func() bool {
		select {
		case <-p.done:
			return true
		default:
			return false
		}
	})
	if len(b.List("owner")) != 0 {
		t.Fatal("failed creation left an orphan session")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	s := create(t, b, "owner")
	backup := file + ".saved"
	if err := os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(file); _ = os.Rename(backup, file) }()
	title := "changed"
	if _, err := b.Patch("owner", s.ID, Patch{Title: &title}); err == nil {
		t.Fatal("patch succeeded without persistence")
	}
	if value, _ := b.Get("owner", s.ID); value.Title != s.Title {
		t.Fatal("failed patch changed memory")
	}
	if err := b.Delete("owner", s.ID); err == nil {
		t.Fatal("delete succeeded without persistence")
	}
	if _, err := b.Get("owner", s.ID); err != nil {
		t.Fatal("failed deletion removed session")
	}
}
