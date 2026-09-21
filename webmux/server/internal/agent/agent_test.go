package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func fixture(t *testing.T) *Service {
	t.Helper()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	writeConfig(t, s, true, true)
	if err := store.WriteConfig("auth.yaml", map[string]any{"auth": map[string]any{"mode": "none", "users": []any{}}}); err != nil {
		t.Fatal(err)
	}
	return s
}
func writeConfig(t *testing.T, s *Service, enabled, guard bool) {
	t.Helper()
	if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"agents": map[string]any{"enabled": enabled, "disable_in_multi_user_mode": guard, "definitions": []any{map[string]any{"id": "alpha", "tmux_socket": "fixture-alpha", "workspace": "Agents"}, map[string]any{"id": "beta", "tmux_socket": "fixture-beta"}}}, "default_term": map[string]any{"font_family": "invalid; font"}}}); err != nil {
		t.Fatal(err)
	}
}
func TestAccessPolicy(t *testing.T) {
	s := fixture(t)
	if c, err := s.Access("alpha"); err != nil || len(c.Definitions) != 2 {
		t.Fatalf("independent agent normalization: %+v %v", c, err)
	}
	check := func(id string, want int) {
		t.Helper()
		_, err := s.Access(id)
		var access *AccessError
		if !errors.As(err, &access) || access.Status != want {
			t.Fatalf("access %q: %v, want %d", id, err, want)
		}
	}
	check("missing", 404)
	writeConfig(t, s, false, true)
	check("", 404)
	writeConfig(t, s, true, true)
	if err := s.store.WriteConfig("auth.yaml", map[string]any{"auth": map[string]any{"mode": "local", "users": []any{map[string]any{"username": "one", "password_hash": "fixture"}, map[string]any{"username": "two", "password_hash": "fixture"}}}}); err != nil {
		t.Fatal(err)
	}
	check("alpha", 403)
	writeConfig(t, s, true, false)
	if _, err := s.Access("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.store.ConfigPath("auth.yaml"), []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, s, true, true)
	check("alpha", 403)
	if err := os.WriteFile(s.store.ConfigPath("app.yaml"), []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	check("", 500)
}

func TestDiscoveryAndCommandArguments(t *testing.T) {
	s := fixture(t)
	calls := 0
	s.run = func(_ context.Context, command string, args ...string) (string, string, error) {
		calls++
		if command != "tmux" || !reflect.DeepEqual(args, []string{"-L", "fixture-alpha", "list-sessions", "-F", "#S\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}"}) {
			t.Fatalf("command %q %q", command, args)
		}
		return "alpha-task-2026-09-20-12-00-00\t2\t1\t1789905600\t1789905600\nalpha-task-2026-09-19-12-00-00\t1\t0\t1789819200\t1789819200\n", "", nil
	}
	sessions, err := s.List(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].DisplayName != "task (2)" || sessions[1].DisplayName != "task (1)" || sessions[0].Windows != 2 || sessions[0].Status != "working" || sessions[1].Status != "stale" {
		t.Fatalf("%+v", sessions)
	}
	found, err := s.HasSession(context.Background(), "alpha", sessions[0].Name)
	if !found || err != nil || calls != 2 {
		t.Fatal(found, err, calls)
	}
	name := "literal name; $(echo fixture)"
	argv, err := s.AttachArgv("alpha", name)
	if err != nil || !reflect.DeepEqual(argv, []string{"tmux", "-L", "fixture-alpha", "attach-session", "-t", name}) {
		t.Fatal(argv, err)
	}
	cwd := t.TempDir()
	s.run = func(_ context.Context, _ string, args ...string) (string, string, error) {
		if !reflect.DeepEqual(args, []string{"-L", "fixture-alpha", "display-message", "-p", "-t", name, "#{pane_current_path}"}) {
			t.Fatal(args)
		}
		return cwd + "\n", "", nil
	}
	if got := s.PaneCurrentPath(context.Background(), "alpha", name); got != cwd {
		t.Fatal(got)
	}
	absolute := filepath.Join(cwd, "socket")
	if got := socketArgs(Definition{TMUXSocket: absolute}); !reflect.DeepEqual(got, []string{"-S", absolute}) {
		t.Fatal(got)
	}
}

func TestDiscoveryFailuresAndCombinedNames(t *testing.T) {
	s := fixture(t)
	for _, message := range []string{"no server running on fixture", "error connecting to fixture (No such file or directory)"} {
		s.run = func(context.Context, string, ...string) (string, string, error) {
			return "", message, errors.New("exit status 1")
		}
		items, err := s.List(context.Background(), "alpha")
		if err != nil || items == nil || len(items) != 0 {
			t.Fatal(items, err)
		}
	}
	s.run = func(context.Context, string, ...string) (string, string, error) { return "", "", exec.ErrNotFound }
	if _, err := s.List(context.Background(), "alpha"); err == nil || err.Error() != "tmux is not installed" {
		t.Fatal(err)
	}
	s.run = func(_ context.Context, _ string, args ...string) (string, string, error) {
		if args[1] == "fixture-alpha" {
			return "alpha-task\t1\t0\t100\t0", "", nil
		}
		return "beta-task\t1\t0\t50\t0", "", nil
	}
	items, err := s.ListAll(context.Background())
	if err != nil || len(items) != 2 || items[0].DisplayName != "task (2)" || items[1].DisplayName != "task (1)" {
		t.Fatal(items, err)
	}
	s.run = func(_ context.Context, _ string, args ...string) (string, string, error) {
		if args[1] == "fixture-alpha" {
			return "", "", errors.New("unavailable")
		}
		return "beta-task\t1\t0\t50\t0", "", nil
	}
	items, err = s.ListAll(context.Background())
	if err != nil || len(items) != 1 || items[0].DisplayName != "task" {
		t.Fatal(items, err)
	}
}

func TestStatusInferenceBoundaries(t *testing.T) {
	s := fixture(t)
	now := s.now()
	iso := func(d time.Duration) string { return now.Add(d).Format(timeFormat) }
	for _, tc := range []struct {
		name, output   string
		meta           metadata
		status, source string
	}{
		{"absent", "", nil, "unknown", "none"},
		{"recent boundary", iso(-5 * time.Minute), nil, "working", "tmux"},
		{"past recent", iso(-5*time.Minute - time.Millisecond), nil, "unknown", "tmux"},
		{"stale boundary", iso(-24 * time.Hour), nil, "stale", "tmux"},
		{"explicit unknown", iso(-25 * time.Hour), metadata{"status": "unknown"}, "unknown", "tmux"},
		{"explicit working", iso(-25 * time.Hour), metadata{"status": "working"}, "working", "webmux"},
		{"waiting slop", iso(1500 * time.Millisecond), metadata{"status": "waiting", "updated_at": iso(0)}, "waiting", "hook"},
		{"after waiting slop", iso(1501 * time.Millisecond), metadata{"status": "waiting", "updated_at": iso(0)}, "working", "tmux"},
		{"replay ignored", "", metadata{"source": "webmux", "last_output_at": iso(0)}, "unknown", "none"},
		{"live output", "", metadata{"source": "webmux", "last_output_at": iso(0), "last_output_source": "live"}, "working", "tmux"},
		{"ready output", "", metadata{"last_ready_at": iso(0), "status": "waiting", "updated_at": iso(0)}, "waiting", "hook"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := Session{LastOutputAt: tc.output}
			s.mergeStatus(&item, tc.meta)
			if item.Status != tc.status || item.StatusSource != tc.source {
				t.Fatalf("%+v", item)
			}
		})
	}
}

func TestStatusAtomicUpdatesAndUnknownFields(t *testing.T) {
	s := fixture(t)
	name := "../task/λ😀"
	root, err := s.statusRoot("alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	original := metadata{"agent_id": "alpha", "name": name, "extension": map[string]any{"keep": true}, "last_output_source": "live", "last_output_at": "old"}
	data, _ := json.Marshal(original)
	if err := root.WriteFile(statusFilename(name), data, 0600); err != nil {
		t.Fatal(err)
	}
	stamp := s.now().Format(timeFormat)
	if err := s.RecordStatus("alpha", name, StatusUpdate{Status: "working", Source: "webmux", LastInputAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	got := s.readStatus("alpha", name)
	if got.text("last_output_source") != "live" || got["extension"] == nil || got.text("updated_at") != stamp {
		t.Fatal(got)
	}
	if err := s.RecordStatus("alpha", name, StatusUpdate{Status: "waiting", Source: "hook", LastOutputAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	got = s.readStatus("alpha", name)
	if _, ok := got["last_output_source"]; ok {
		t.Fatal("obsolete live marker preserved")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			if err := s.RecordStatus("alpha", name, StatusUpdate{Status: "working", Source: "webmux", LastInputAt: &stamp}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got = s.readStatus("alpha", name); got.text("last_output_at") != stamp || got["extension"] == nil {
		t.Fatal(got)
	}
	dir := filepath.Join(s.store.Home, "data", "agent-status", "alpha")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	info, err := root.Stat(statusFilename(name))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err := s.RecordStatus("../outside", name, StatusUpdate{}); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if s.readStatus("alpha", "other") != nil {
		t.Fatal("missing metadata")
	}
	if err := root.WriteFile(statusFilename(name), []byte(`{"agent_id":"beta"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if s.readStatus("alpha", name) != nil {
		t.Fatal("mismatched metadata accepted")
	}
}

func TestStatusSymlinkCannotEscapeRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	s := fixture(t)
	root, err := s.statusRoot("alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := filepath.Join(t.TempDir(), "outside.json")
	original := []byte(`{"extension":"outside"}`)
	if err := os.WriteFile(outside, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink(outside, statusFilename("task")); err != nil {
		t.Fatal(err)
	}
	if s.readStatus("alpha", "task") != nil {
		t.Fatal("read outside root")
	}
	if err := s.RecordStatus("alpha", "task", StatusUpdate{Status: "working", Source: "webmux"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(outside)
	if err != nil || string(after) != string(original) {
		t.Fatal("outside file modified", err)
	}
}

func TestCommandCancellationAndOutputLimit(t *testing.T) {
	cancelled := false
	output := boundedOutput{cancel: func() { cancelled = true }}
	// os/exec uses io.Copy: an accidentally promoted bytes.Buffer.ReadFrom
	// would bypass Write and silently remove the bound.
	n, err := io.Copy(&output, io.LimitReader(strings.NewReader(strings.Repeat("a", (1<<20)+10)), (1<<20)+10))
	if n != 1<<20 || err == nil || !cancelled || !output.exceeded || output.Len() != 1<<20 {
		t.Fatal(n, err, cancelled)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = runCommand(ctx, os.Args[0])
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
