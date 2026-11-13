//go:build !windows

package recovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOriginalCLIUnixSkipsNonExecutableAndOpenerAliases(t *testing.T) {
	plain, helpers, native := t.TempDir(), t.TempDir(), t.TempDir()
	for _, file := range []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(plain, "codex"), 0600},
		{filepath.Join(helpers, "webmux-open"), 0700},
		{filepath.Join(native, "codex"), 0700},
	} {
		if err := os.WriteFile(file.path, []byte("fixture"), file.mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("webmux-open", filepath.Join(helpers, "codex")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", plain+string(os.PathListSeparator)+helpers+string(os.PathListSeparator)+native)
	got, err := originalCLI("codex")
	if err != nil || got != filepath.Join(native, "codex") {
		t.Fatalf("selected a non-executable or shim: %q, %v", got, err)
	}
}
