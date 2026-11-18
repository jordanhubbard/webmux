//go:build linux

package openrigcoord

import (
	"context"
	"encoding/json"
	"github.com/jordanhubbard/webmux/server/internal/nativeobserve"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeCreatePinsControllerHelperBeforeNode(t *testing.T) {
	home, bins := t.TempDir(), t.TempDir()
	node := "#!/bin/sh\nset -e\ncat \"$3/.native-codex-observer.json\"\n"
	if err := os.WriteFile(filepath.Join(bins, "node"), []byte(node), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	foreign := filepath.Join(t.TempDir(), "foreign")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := Run(ctx, home, "owner", "native-create", `{"directory":"`+foreign+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	var reference nativeobserve.Reference
	if err = json.Unmarshal(out, &reference); err != nil {
		t.Fatal(err)
	}
	if reference.Contract != nativeobserve.Contract || !filepath.IsAbs(reference.Executable) || len(reference.ExecutableSHA256) != 64 {
		t.Fatalf("unbound helper: %+v", reference)
	}
	if _, err = os.Stat(foreign); !os.IsNotExist(err) {
		t.Fatal("untrusted input selected the helper namespace")
	}
	files, err := filepath.Glob(filepath.Join(home, "data", "coordination", "*", nativeobserve.ReferenceFile))
	if err != nil || len(files) != 1 {
		t.Fatalf("helper reference missing: %v %v", files, err)
	}
	info, err := os.Lstat(files[0])
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe helper reference: %v %v", info, err)
	}
	if strings.Contains(string(out), foreign) {
		t.Fatal("input path entered helper reference")
	}
	resumed, err := Run(ctx, home, "owner", "native-resume", `{"directory":"`+foreign+`"}`)
	if err != nil || string(resumed) != string(out) {
		t.Fatalf("resume did not receive the same pinned owner helper: %v", err)
	}
}
