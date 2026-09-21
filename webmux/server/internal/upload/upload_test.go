package upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	if err := store.WriteConfig("keys.yaml", map[string]any{"keys": []any{}}); err != nil {
		t.Fatal(err)
	}
	return New(store)
}

func TestUploadNamesBytesAndPermissions(t *testing.T) {
	s := fixture(t)
	payload := []byte{0, 255, 13, 10, 128}
	for _, tc := range []struct{ name, suffix string }{{"test.pem", "-test.pem"}, {"../../etc/passwd", "-passwd"}, {"unsafe file.pem", ".pem"}, {"", ".bin"}} {
		got, err := s.Store(context.Background(), tc.name, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(got.Name, tc.suffix) || got.Size != int64(len(payload)) || filepath.Dir(got.Path) != s.directory {
			t.Fatal(got)
		}
		data, err := os.ReadFile(got.Path)
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatal(data, err)
		}
		info, err := os.Stat(got.Path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatal(info.Mode())
		}
	}
}

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { p[0] = 1; return 1, io.ErrUnexpectedEOF }

func TestUploadFailureCleanupAndCancelledGate(t *testing.T) {
	s := fixture(t)
	s.maxFile = 8
	if _, err := s.Store(context.Background(), "large", strings.NewReader("123456789")); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if _, err := s.Store(context.Background(), "broken", brokenReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Store(ctx, "cancelled", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	// A cancelled acquisition must not leave the serialization slot occupied.
	live, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := s.Store(live, "valid", strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store(live, "closed", strings.NewReader("x")); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
}

func TestConcurrentQuotaCannotBeOversubscribed(t *testing.T) {
	s := fixture(t)
	s.quota, s.maxFile = 10, 8
	var workers sync.WaitGroup
	errorsOut := make(chan error, 2)
	for range 2 {
		workers.Go(func() {
			_, err := s.Store(context.Background(), "same", strings.NewReader("12345678"))
			errorsOut <- err
		})
	}
	workers.Wait()
	close(errorsOut)
	success, rejected := 0, 0
	for err := range errorsOut {
		if err == nil {
			success++
		} else if errors.Is(err, ErrQuota) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatal(success, rejected)
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if used, err := usage(root); err != nil || used != 8 {
		t.Fatal(used, err)
	}
}

func TestPurgePreservesReferencesAndFailsClosed(t *testing.T) {
	s := fixture(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	paths := map[string]string{}
	for _, name := range []string{"referenced", "old", "boundary", "new"} {
		value, err := s.Store(context.Background(), name, strings.NewReader(name))
		if err != nil {
			t.Fatal(err)
		}
		paths[name] = value.Path
		age := MaxAge + time.Hour
		if name == "new" {
			age = time.Hour
		}
		if name == "boundary" {
			age = MaxAge - time.Second
		}
		if err := os.Chtimes(value.Path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.store.WriteConfig("keys.yaml", map[string]any{"keys": []any{map[string]string{"private_key_path": paths["referenced"]}}}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Purge(context.Background())
	if err != nil || result.Deleted != 1 || result.FreedBytes != 3 {
		t.Fatal(result, err)
	}
	for _, name := range []string{"referenced", "boundary", "new"} {
		if _, err := os.Stat(paths[name]); err != nil {
			t.Fatal(name, err)
		}
	}
	if err := os.WriteFile(s.store.ConfigPath("keys.yaml"), []byte("keys: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(2 * MaxAge) }
	if _, err := s.Purge(context.Background()); err == nil {
		t.Fatal("malformed catalog allowed purge")
	}
	for _, name := range []string{"referenced", "boundary", "new"} {
		if _, err := os.Stat(paths[name]); err != nil {
			t.Fatal(name, err)
		}
	}
}

func TestNearQuotaPurgesOldUnreferencedFiles(t *testing.T) {
	s := fixture(t)
	s.quota, s.maxFile = 10, 8
	value, err := s.Store(context.Background(), "old", strings.NewReader("12345678"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-MaxAge - time.Hour)
	if err := os.Chtimes(value.Path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store(context.Background(), "new", strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(value.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestPurgePreservesAliasReferencesAndIgnoresOutsideSymlinks(t *testing.T) {
	s := fixture(t)
	value, err := s.Store(context.Background(), "key", strings.NewReader("key bytes"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-MaxAge - time.Hour)
	if err := os.Chtimes(value.Path, old, old); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "key-alias")
	if err := os.Symlink(value.Path, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := s.store.WriteConfig("keys.yaml", map[string]any{"keys": []any{map[string]string{"private_key_path": alias}}}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outside, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.directory, "outside-link")); err != nil {
		t.Fatal(err)
	}
	result, err := s.Purge(context.Background())
	if err != nil || result.Deleted != 0 {
		t.Fatal(result, err)
	}
	if data, err := os.ReadFile(value.Path); err != nil || string(data) != "key bytes" {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatal(err)
	}
}
