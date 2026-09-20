package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func enableLogging(t *testing.T, store *storage.Store) {
	t.Helper()
	if err := storage.UpdateConfig(store, "app.yaml", func(doc *config.Document) error {
		doc.App["session_logging"] = config.Object{"enabled": true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func logContains(t *testing.T, path, text string) {
	t.Helper()
	eventually(t, func() bool { data, err := os.ReadFile(path); return err == nil && strings.Contains(string(data), text) })
}
func logContents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTranscriptManualPauseResumeAndDeletion(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "ready\n")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return text != "" })
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	if event(t, v, "status")["transcript_enabled"] != false {
		t.Fatal("logging default changed")
	}
	event(t, v, "output")
	if err := b.ToggleTranscript("foreign", s.ID, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign transcript toggle accepted")
	}
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	started := event(t, v, "transcript_status")
	if started["transcript_enabled"] != true {
		t.Fatal("manual logging did not start")
	}
	path := started["transcript_file"].(string)
	p.output(t, "recorded λ😀\n")
	event(t, v, "output")
	logContains(t, path, "recorded λ😀")
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	paused := event(t, v, "transcript_status")
	if paused["transcript_enabled"] != false || paused["transcript_file"] != path {
		t.Fatal("pause changed file")
	}
	if !strings.Contains(logContents(t, path), "reason=manual_pause") {
		t.Fatal("pause returned before drain")
	}
	p.output(t, "not-recorded\n")
	event(t, v, "output")
	if strings.Contains(logContents(t, path), "not-recorded") {
		t.Fatal("paused output recorded")
	}
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	resumed := event(t, v, "transcript_status")
	if resumed["transcript_enabled"] != true || resumed["transcript_file"] != path {
		t.Fatal("resume changed file")
	}
	p.output(t, "after-resume\n")
	event(t, v, "output")
	if err := b.Delete("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	logContains(t, path, "reason=deleted")
	data := logContents(t, path)
	if !strings.Contains(data, "[webmux transcript resumed ") || !strings.Contains(data, "after-resume") || strings.Contains(data, "not-recorded") {
		t.Fatalf("transcript: %s", data)
	}
	if runtime.GOOS != "windows" {
		for file, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
			info, err := os.Stat(file)
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("permissions: %s %v", file, err)
			}
		}
	}
}

func TestTranscriptAutomaticStartReconnectAndShutdown(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	s := create(t, b, "owner")
	first := <-launched
	b.mu.Lock()
	firstPath := b.entries[s.ID].run.log.path
	b.mu.Unlock()
	first.output(t, "first-launch\n")
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	second := <-launched
	b.mu.Lock()
	secondPath := b.entries[s.ID].run.log.path
	b.mu.Unlock()
	if firstPath == secondPath {
		t.Fatal("reconnect reused previous launch file")
	}
	second.output(t, "second-launch\n")
	logContains(t, secondPath, "second-launch")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logContents(t, firstPath), "reason=reconnect") || !strings.Contains(logContents(t, secondPath), "reason=shutdown") {
		t.Fatal("shutdown did not drain transcript files")
	}
}

type testLogSink struct {
	buffer  bytes.Buffer
	fail    bool
	release <-chan struct{}
	once    sync.Once
}

func (s *testLogSink) Write(data []byte) (int, error) {
	if s.release != nil {
		s.once.Do(func() { <-s.release })
	}
	if s.fail {
		return 0, errors.New("fixture write failure")
	}
	return s.buffer.Write(data)
}
func (s *testLogSink) Close() error { return nil }

func TestTranscriptFailureDoesNotStopTerminal(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	b.openLog = func(string, string, bool) (transcriptSink, error) { return &testLogSink{fail: true}, nil }
	s := create(t, b, "owner")
	p := <-launched
	eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.entries[s.ID].run.log.failureReported })
	p.output(t, "terminal-still-running\n")
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.Contains(text, "terminal-still-running")
	})
	if value, _ := b.Get("owner", s.ID); value.State != "connected" {
		t.Fatal("logging failure stopped terminal")
	}
}

func TestTranscriptBackpressureIsBounded(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	b.openLog = func(string, string, bool) (transcriptSink, error) { return &testLogSink{release: release}, nil }
	s := create(t, b, "owner")
	p := <-launched
	for range 300 {
		p.output(t, "output\n")
	}
	eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.entries[s.ID].run.log.failureReported })
	if value, _ := b.Get("owner", s.ID); value.State != "connected" {
		t.Fatal("slow storage blocked terminal")
	}
	unblock()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTranscriptResumeRejectsSubstitutedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires Windows developer privileges")
	}
	b, store, _ := fixture(t)
	enableLogging(t, store)
	s := create(t, b, "owner")
	b.mu.Lock()
	path := b.entries[s.ID].run.log.path
	b.mu.Unlock()
	if err := b.ToggleTranscript("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "unrelated.log")
	if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := b.ToggleTranscript("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	active := b.entries[s.ID].run.log.active()
	b.mu.Unlock()
	if active || logContents(t, outside) != "unchanged" {
		t.Fatal("resume followed substituted symlink")
	}
}

func TestTranscriptReconnectDoesNotWaitForPausedWriter(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	opens := 0
	b.openLog = func(string, string, bool) (transcriptSink, error) {
		opens++
		if opens == 1 {
			return &testLogSink{release: release, fail: true}, nil
		}
		return &testLogSink{}, nil
	}
	s := create(t, b, "owner")
	b.mu.Lock()
	old := b.entries[s.ID].run.log
	b.mu.Unlock()
	paused := make(chan error, 1)
	go func() { paused <- b.ToggleTranscript("owner", s.ID, "") }()
	eventually(t, func() bool { return !old.active() })
	reconnected := make(chan error, 1)
	go func() { _, err := b.Reconnect("owner", s.ID, ""); reconnected <- err }()
	select {
	case err := <-reconnected:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disk drain blocked reconnect")
	}
	b.mu.Lock()
	current := b.entries[s.ID].run.log
	b.mu.Unlock()
	if current == old {
		t.Fatal("new launch retained old log")
	}
	unblock()
	select {
	case err := <-paused:
		if err == nil {
			t.Fatal("stale pause succeeded on new launch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pause did not finish")
	}
	if !current.active() {
		t.Fatal("old write failure disabled new launch logging")
	}
}

func TestTranscriptStopsOnExitAndRejectsDisconnectedToggle(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	s := create(t, b, "owner")
	p := <-launched
	b.mu.Lock()
	path := b.entries[s.ID].run.log.path
	b.mu.Unlock()
	p.exit()
	eventually(t, func() bool { value, _ := b.Get("owner", s.ID); return value.State == "disconnected" })
	logContains(t, path, "reason=process_exit")
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	event(t, v, "status")
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	status := event(t, v, "transcript_status")
	if status["transcript_enabled"] != false || status["message"] != "Cannot log a disconnected session" {
		t.Fatalf("disconnected toggle: %v", status)
	}
}
