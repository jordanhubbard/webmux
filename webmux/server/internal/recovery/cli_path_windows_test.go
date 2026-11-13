//go:build windows

package recovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOriginalCLIWindowsNativeExecutable(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "codex.exe")
	if err := os.WriteFile(want, []byte("native executable fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", dir)
	for _, name := range []string{"codex", "codex.exe"} {
		got, err := OriginalCLI(name)
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestOriginalCLIWindowsSkipsShellWrappersAndHelperCopies(t *testing.T) {
	wrappers, helpers, native := t.TempDir(), t.TempDir(), t.TempDir()
	for _, file := range []string{filepath.Join(wrappers, "codex.cmd"), filepath.Join(wrappers, "codex.bat"), filepath.Join(helpers, "codex.exe"), filepath.Join(helpers, "webmux-open.exe"), filepath.Join(native, "codex.exe")} {
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATHEXT", ".CMD;.BAT;.EXE")
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", wrappers+string(os.PathListSeparator)+helpers+string(os.PathListSeparator)+native)
	if got, err := OriginalCLI("codex"); err != nil || got != filepath.Join(native, "codex.exe") {
		t.Fatalf("selected wrapper/helper: %q, %v", got, err)
	}
	for _, name := range []string{"codex.cmd", "codex.bat"} {
		if got, err := OriginalCLI(name); err == nil {
			t.Fatalf("accepted shell wrapper %q", got)
		}
	}
}

func TestOriginalCLIWindowsSkipsSelfAndRelativePATH(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", "."+string(os.PathListSeparator)+filepath.Dir(self))
	if got, err := OriginalCLI(filepath.Base(self)); err == nil {
		t.Fatalf("resolved helper back to itself: %q", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", ".")
	if got, err := OriginalCLI("codex"); err == nil {
		t.Fatalf("selected executable from current project: %q", got)
	}
}
