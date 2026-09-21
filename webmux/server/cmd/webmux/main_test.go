package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestSlavePortNumericFormsAndInvalidValues(t *testing.T) {
	for raw, want := range map[string]int{"": 0, " ": 0, "0": 0, "22": 22, " 3389 ": 3389, "0x16": 22, "2.2e1": 22} {
		got, err := parseSlavePort(raw)
		if err != nil || got != want {
			t.Fatal(raw, got, err)
		}
	}
	for _, raw := range []string{"NaN", "Infinity", "-1", "65536", "1.5", "garbage"} {
		if _, err := parseSlavePort(raw); err == nil {
			t.Fatal("invalid port accepted", raw)
		}
	}
}

func TestExplicitAssetOverrideAndInvalidDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config.defaults"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.defaults", "app.yaml"), []byte("app: {name: custom}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "web"), 0700); err != nil {
		t.Fatal(err)
	}
	defaults, web, err := runtimeAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	defer defaults.Close()
	data, err := fs.ReadFile(defaults.FS(), "app.yaml")
	if err != nil || string(data) != "app: {name: custom}\n" {
		t.Fatal(string(data), err)
	}
	if web.directory != filepath.Join(root, "web") {
		t.Fatal(web.directory)
	}
	if err := os.Remove(filepath.Join(root, "web")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtimeAssets(root); err == nil {
		t.Fatal("invalid explicit asset directory accepted")
	}
}
