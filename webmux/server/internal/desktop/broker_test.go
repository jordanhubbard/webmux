package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func fixture(t *testing.T, kind Kind) (*Broker, *storage.Store) {
	t.Helper()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(store, kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, store
}
func create(t *testing.T, b *Broker, owner string) Session {
	t.Helper()
	s, err := b.Create(owner, CreateRequest{Hostname: "fixture.invalid", VNCPassword: "transient-vnc", RDPPassword: "transient-rdp"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDesktopLifecycleCredentialsAndRecovery(t *testing.T) {
	for _, kind := range []Kind{VNC, RDP} {
		t.Run(string(kind), func(t *testing.T) {
			b, store := fixture(t, kind)
			s := create(t, b, "owner")
			if s.Kind != kind || s.State != "connecting" || !s.Persistent || s.Row != 0 || s.Col != 0 {
				t.Fatal(s)
			}
			if _, err := b.Get("other", s.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			if _, _, err := b.Credentials("other", s.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			secret, done, err := b.Credentials("owner", s.ID)
			if err != nil || secret != "transient-"+string(kind) {
				t.Fatal("missing transient credential", err)
			}
			encoded, err := json.Marshal(s)
			if err != nil || bytes.Contains(encoded, []byte("password")) || bytes.Contains(encoded, []byte("transient")) {
				t.Fatal("credentials in API response", err)
			}
			file := filepath.Join(store.Home, "data", "sessions", string(kind)+"-sessions.yaml")
			data, err := os.ReadFile(file)
			if err != nil || bytes.Contains(data, []byte("password")) || bytes.Contains(data, []byte("transient")) {
				t.Fatal("credentials persisted", err)
			}
			if _, err := b.SetState("owner", s.ID, "connected"); err != nil {
				t.Fatal(err)
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			default:
				t.Fatal("shutdown did not release transport lifetime")
			}
			next, err := New(store, kind)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			if err := next.Restore(); err != nil {
				t.Fatal(err)
			}
			restored, err := next.Get("owner", s.ID)
			if err != nil || restored.State != "disconnected" {
				t.Fatal(restored, err)
			}
			secret, deleted, err := next.Credentials("owner", s.ID)
			if err != nil || secret != "" {
				t.Fatal("credential survived restart", err)
			}
			if _, err := next.SetState("owner", s.ID, "connecting"); err != nil {
				t.Fatal(err)
			}
			if err := next.Delete("owner", s.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-deleted:
			default:
				t.Fatal("deletion did not release transport lifetime")
			}
			if len(next.List("owner")) != 0 {
				t.Fatal("deleted session retained")
			}
		})
	}
}

func TestDesktopConcurrentPositionsAndCompaction(t *testing.T) {
	b, _ := fixture(t, VNC)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			if _, err := b.Create("owner", CreateRequest{Hostname: "fixture.invalid"}); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	items := b.List("owner")
	if len(items) != 16 {
		t.Fatal(len(items))
	}
	seen := map[float64]bool{}
	for _, s := range items {
		if seen[s.Col] || s.Row != 0 {
			t.Fatal("duplicate grid position", s)
		}
		seen[s.Col] = true
	}
	other := create(t, b, "other")
	if other.Col != 0 {
		t.Fatal("shared owner grid")
	}
	if _, err := b.Move("owner", items[15].ID, 9.5, 8.5); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete("owner", items[0].ID); err != nil {
		t.Fatal(err)
	}
	moved, err := b.Get("owner", items[15].ID)
	if err != nil || moved.Row != 1 || moved.Col != 0 {
		t.Fatal(moved, err)
	}
	unchanged, _ := b.Get("other", other.ID)
	if unchanged.Row != 0 || unchanged.Col != 0 {
		t.Fatal(unchanged)
	}
}

func TestDesktopHostDefaultsAndExplicitPosition(t *testing.T) {
	for _, kind := range []Kind{VNC, RDP} {
		t.Run(string(kind), func(t *testing.T) {
			b, store := fixture(t, kind)
			if err := store.WriteConfig("hosts.yaml", map[string]any{"hosts": []any{map[string]any{"id": "saved", "hostname": "catalog.invalid", "vnc_port": 5905, "rdp_port": 3395}}}); err != nil {
				t.Fatal(err)
			}
			row, col := 2.5, 3.5
			s, err := b.Create("owner", CreateRequest{HostID: "saved", Hostname: "ignored.invalid", VNCPort: 5901, RDPPort: 3391, RDPUsername: "user", RDPDomain: "domain", Row: &row, Col: &col})
			if err != nil || s.Hostname != "catalog.invalid" || s.Row != row || s.Col != col {
				t.Fatal(s, err)
			}
			if kind == VNC && (s.VNCPort != 5905 || s.RDPUsername != nil) {
				t.Fatal(s)
			}
			if kind == RDP && (s.RDPPort != 3395 || s.RDPUsername == nil || *s.RDPUsername != "user") {
				t.Fatal(s)
			}
			if s.RDPUsername != nil {
				*s.RDPUsername = "mutated"
				copy, _ := b.Get("owner", s.ID)
				if *copy.RDPUsername != "user" {
					t.Fatal("API copy mutated broker")
				}
			}
			next := create(t, b, "owner")
			if next.Row != 2.5 || next.Col != 4.5 {
				t.Fatal(next)
			}
		})
	}
}

func TestDesktopPersistenceFailuresRollback(t *testing.T) {
	b, store := fixture(t, RDP)
	file := filepath.Join(store.Home, "data", "sessions", "rdp-sessions.yaml")
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create("owner", CreateRequest{Hostname: "fixture.invalid"}); err == nil {
		t.Fatal("create accepted failed write")
	}
	if len(b.List("owner")) != 0 {
		t.Fatal("failed create retained")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	s := create(t, b, "owner")
	_, done, _ := b.Credentials("owner", s.ID)
	backup := file + ".saved"
	if err := os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(file); _ = os.Rename(backup, file) }()
	if _, err := b.Move("owner", s.ID, 3, 4); err == nil {
		t.Fatal("move accepted failed write")
	}
	if err := b.Delete("owner", s.ID); err == nil {
		t.Fatal("delete accepted failed write")
	}
	actual, err := b.Get("owner", s.ID)
	if err != nil || actual.Row != s.Row || actual.Col != s.Col {
		t.Fatal(actual, err)
	}
	select {
	case <-done:
		t.Fatal("failed deletion closed live transport")
	default:
	}
}

func TestCorruptDesktopRecordsFailClosed(t *testing.T) {
	b, store := fixture(t, VNC)
	s := create(t, b, "owner")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteDesktopSessions("vnc", map[string][]Session{"vnc_sessions": {s, s}}); err != nil {
		t.Fatal(err)
	}
	if _, err := New(store, VNC); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	if err := store.WriteDesktopSessions("../other", nil); err == nil {
		t.Fatal("unrecognized persistence path accepted")
	}
}
