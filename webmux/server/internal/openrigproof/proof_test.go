package openrigproof

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOwnerScopeAndMaterialization(t *testing.T) {
	home := t.TempDir()
	first, err := Prepare(home, "../../owner")
	if err != nil {
		t.Fatal(err)
	}
	second := Locate(home, "other")
	if first.Team == Locate(t.TempDir(), "../../owner").Team {
		t.Fatal("different installations share a test team")
	}
	if first.Directory == second.Directory || first.Team == second.Team || !strings.HasPrefix(first.Directory, home+string(os.PathSeparator)) {
		t.Fatal("owner scope escaped or collided")
	}
	if info, err := os.Stat(first.Directory); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0700) {
		t.Fatalf("private directory: %v %v", info, err)
	}
	if data, err := os.ReadFile(first.Runner); err != nil || string(data) != string(runner) {
		t.Fatalf("runner materialization: %v", err)
	}
	if Status(home, "../../owner") != nil {
		t.Fatal("fabricated result before explicit run")
	}
	if err := os.WriteFile(filepath.Join(first.Directory, "status.json"), []byte(`{"phase":"passed","run_id":"new"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if Status(home, "../../owner")["run_id"] != "new" || Status(home, "other") != nil {
		t.Fatal("status crossed owner boundary")
	}
	if err := Starting(first); err != nil {
		t.Fatal(err)
	}
	state := Status(home, "../../owner")
	if state["phase"] != "preparing" || state["run_id"] != "" {
		t.Fatalf("old PASS survived a fresh start: %+v", state)
	}
}
