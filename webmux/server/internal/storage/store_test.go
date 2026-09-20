package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestUpdatesAreSerializedAndAbortWithoutWriting(t *testing.T) {
	defaults := t.TempDir()
	if err := os.WriteFile(filepath.Join(defaults, "counter.yaml"), []byte("count: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(t.TempDir(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	type counter struct {
		Count int `yaml:"count"`
	}
	var workers sync.WaitGroup
	for range 50 {
		workers.Go(func() {
			if err := UpdateConfig(s, "counter.yaml", func(c *counter) error { c.Count++; return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	wantErr := errors.New("abort")
	if err := UpdateConfig(s, "counter.yaml", func(c *counter) error { c.Count = -1; return wantErr }); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	var got counter
	if err := s.ReadConfig("counter.yaml", &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 50 {
		t.Fatalf("lost updates: %d", got.Count)
	}
	entries, err := filepath.Glob(filepath.Join(s.Home, "config", ".webmux-*.tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(s.ConfigPath("counter.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe permissions: %o", info.Mode().Perm())
		}
	}
}

func TestExistingConfigAndSymlinkSurvive(t *testing.T) {
	defaults := t.TempDir()
	if err := os.WriteFile(filepath.Join(defaults, "auth.yaml"), []byte("value: default\n"), 0600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "auth.yaml")
	if err := os.WriteFile(target, []byte("value: existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "config", "auth.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	s, err := Open(home, defaults)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]string
	if err := s.ReadConfig("auth.yaml", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["value"] != "existing" {
		t.Fatal("default overwrote existing configuration")
	}
	if err := UpdateConfig(s, "auth.yaml", func(c *map[string]string) error { (*c)["value"] = "updated"; return nil }); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("symlink replaced: %q %v", got, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(home, defaults); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatalf("dangling config link replaced: %v", err)
	}
	if err := s.ReadConfig("auth.yaml", &cfg); err == nil {
		t.Fatal("missing target did not fail")
	}
}

func TestMultipleYAMLDocumentsRejected(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("auth: local\n---\nauth: none\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]string
	if err := readYAML(file, &cfg); err == nil {
		t.Fatal("ambiguous configuration accepted")
	}
}
