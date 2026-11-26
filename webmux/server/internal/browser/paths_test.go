package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrowserDataStaysOutOfHomeRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profile, err := temporaryProfile()
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("c", 64)
	saved, lock, err := savedProfile(key)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, p := range []string{profile, saved} {
		if filepath.Dir(p) == home || !strings.HasPrefix(p, home+string(filepath.Separator)) {
			t.Fatalf("browser storage escaped its private subtree: %s", p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Fatalf("public browser data: %s", p)
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "webmux-") {
			t.Fatalf("home root polluted: %s", entry.Name())
		}
	}
}

func TestLegacyProfileRequiresMigrationWithoutLosingData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	key := strings.Repeat("d", 64)
	legacy := filepath.Join(home, "webmux-browser-profiles", key)
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(legacy, "cookie-marker")
	if err := os.WriteFile(marker, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, lock, err := savedProfile(key); err == nil {
		lock.Close()
		t.Fatal("silently replaced the existing login profile")
	}
	if value, err := os.ReadFile(marker); err != nil || string(value) != "retained" {
		t.Fatal("legacy data changed", err)
	}
	root, err := browserDirectory("profiles")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(legacy, filepath.Join(root, key)); err != nil {
		t.Fatal(err)
	}
	profile, lock, err := savedProfile(key)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if value, err := os.ReadFile(filepath.Join(profile, "cookie-marker")); err != nil || string(value) != "retained" {
		t.Fatal("migrated data lost", err)
	}
}

func TestBrowserDataRejectsLinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink boundary")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := browserDataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := temporaryProfile(); err == nil {
		t.Fatal("followed linked browser root")
	}
}
