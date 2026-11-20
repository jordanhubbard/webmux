//go:build windows

package recovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == CodexFilesHelper {
		if RunCodexFilesHelper() != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--codex-file-worker-fixture" {
		switch os.Args[2] {
		case "timeout":
			time.Sleep(time.Minute)
		case "overflow":
			fmt.Print(strings.Repeat(" ", (64<<10)+1))
			fmt.Print(`{}`)
		case "partial":
			fmt.Print(`{"version":1,`)
		case "mismatch":
			fmt.Print(`{"version":1,"identity":{"pid":1,"created":1},"files":["unexpected.jsonl"]}`)
		}
		os.Exit(0)
	}
	if len(os.Args) >= 2 && os.Args[1] == "--codex-file-tree" {
		child := exec.Command(os.Args[0], append([]string{"--codex-file-holder"}, os.Args[2:]...)...)
		stdin, err := child.StdinPipe()
		if err != nil {
			os.Exit(2)
		}
		stdout, err := child.StdoutPipe()
		if err != nil {
			os.Exit(2)
		}
		if child.Start() != nil {
			os.Exit(2)
		}
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		if strings.TrimSpace(line) != "ready" {
			child.Process.Kill()
			child.Wait()
			os.Exit(2)
		}
		fmt.Println("ready")
		io.Copy(io.Discard, os.Stdin)
		stdin.Close()
		if child.Wait() != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if len(os.Args) >= 2 && (os.Args[1] == "--codex-file-holder" || os.Args[1] == "--codex-file-holder-many") {
		var files []*os.File
		paths := os.Args[2:]
		if os.Args[1] == "--codex-file-holder-many" && len(paths) == 1 {
			path := paths[0]
			for len(paths) <= codexHandleLimit {
				paths = append(paths, path)
			}
		}
		for _, path := range paths {
			f, err := os.Open(path)
			if err != nil {
				os.Exit(2)
			}
			files = append(files, f)
		}
		fmt.Println("ready")
		io.Copy(io.Discard, os.Stdin)
		for _, f := range files {
			f.Close()
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func codexHolder(t *testing.T, paths ...string) (*exec.Cmd, func()) {
	t.Helper()
	return codexHolderMode(t, "--codex-file-holder", paths...)
}

func codexHolderMode(t *testing.T, mode string, paths ...string) (*exec.Cmd, func()) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, append([]string{mode}, paths...)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Error(err)
		}
	})
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if strings.TrimSpace(line) != "ready" {
			t.Fatal("holder not ready", line)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("holder startup timeout")
	}
	return cmd, func() { stdin.Close() }
}

func codexFixture(t *testing.T, root, project, id string) string {
	t.Helper()
	dir := filepath.Join(root, "sessions", "dated")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-"+id+".jsonl")
	data, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]string{"id": id, "cwd": project}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWindowsCodexOwnedJournal(t *testing.T) {
	root, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", root)
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	file := codexFixture(t, root, project, id)
	// A newer, closed journal in the same directory must never be selected.
	codexFixture(t, root, project, "01a1c383-71b0-7631-99d9-e0e5eb52a7a8")
	cmd, _ := codexHolder(t, file)
	observe, closer := newCodexProcessObserver(cmd.Process.Pid)
	if closer == nil {
		t.Fatal("could not pin fixture")
	}
	defer closer.Close()
	pin := closer.(*codexProcessPin)
	files, scanErr := windowsCodexFiles(pin.identity)
	if scanErr != nil {
		t.Fatalf("direct metadata scan: %v", scanErr)
	}
	t.Logf("direct metadata paths: %q; helper paths: %q", files, pin.files())
	got, path := observe(project)
	if got != id || !sameDirectory(filepath.Dir(path), filepath.Dir(file)) {
		t.Fatalf("owned journal = %q %q, want %q", got, path, id)
	}
	if got, _ := observe(t.TempDir()); got != "" {
		t.Fatal("wrong project accepted", got)
	}
}

func TestWindowsCodexRejectsAmbiguousOrForeignProfile(t *testing.T) {
	root, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", root)
	first := codexFixture(t, root, project, "01a1c268-92c8-7393-bfc3-e387e33d52b9")
	second := codexFixture(t, root, project, "01a1c383-71b0-7631-99d9-e0e5eb52a7a8")
	foreign := codexFixture(t, t.TempDir(), project, "01a1c383-71b0-7631-99d9-e0e5eb52a7a8")
	for name, paths := range map[string][]string{"ambiguous": {first, second}, "foreign-profile": {foreign}} {
		t.Run(name, func(t *testing.T) {
			cmd, _ := codexHolder(t, paths...)
			observe, closer := newCodexProcessObserver(cmd.Process.Pid)
			if closer == nil {
				t.Fatal("could not pin fixture")
			}
			defer closer.Close()
			if id, _ := observe(project); id != "" {
				t.Fatal("unowned/ambiguous identity", id)
			}
		})
	}
}

func TestWindowsCodexRejectsStaleCreationAndExitedProcess(t *testing.T) {
	root, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", root)
	file := codexFixture(t, root, project, "01a1c268-92c8-7393-bfc3-e387e33d52b9")
	cmd, stop := codexHolder(t, file)
	pin, err := pinCodexProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	defer pin.Close()
	stale := pin.identity
	stale.Created++
	if files, err := windowsCodexFiles(stale); err == nil || len(files) != 0 {
		t.Fatal("stale lifetime accepted", files, err)
	}
	stop()
	// The test owns this exact process and waits for its signaled handle; no PID
	// lookup or process-name kill is used for cleanup.
	if _, err := windows.WaitForSingleObject(pin.handle, 5000); err != nil {
		t.Fatal(err)
	}
	if files := pin.files(); len(files) != 0 {
		t.Fatal("exited owner accepted", files)
	}

}

func TestWindowsCodexMetadataDuplicateCannotReadOrWriteData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-fixture")
	if err := os.WriteFile(path, []byte("canary"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := codexHandles(windows.CurrentProcess())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Value == file.Fd() {
			duplicate, err := duplicateCodexMetadataHandle(windows.CurrentProcess(), row)
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(duplicate)
			var n uint32
			if err := windows.ReadFile(duplicate, make([]byte, 1), &n, nil); err != windows.ERROR_ACCESS_DENIED {
				t.Fatalf("metadata duplicate read: %v", err)
			}
			if err := windows.WriteFile(duplicate, []byte("X"), &n, nil); err != windows.ERROR_ACCESS_DENIED {
				t.Fatalf("metadata duplicate write: %v", err)
			}
			if got, err := codexHandlePath(windows.CurrentProcess(), row); err != nil || got == "" {
				t.Fatal("metadata query failed", got, err)
			}
			return
		}
	}
	t.Fatal("fixture handle not found")
}

func TestWindowsCodexWorkerDeadlineAndInvalidResults(t *testing.T) {
	pin, err := pinCodexProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer pin.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timeout", "overflow", "partial", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			started := time.Now()
			if files := pin.filesCommand(ctx, executable, "--codex-file-worker-fixture", mode); len(files) != 0 {
				t.Fatal("invalid metadata accepted", files)
			}
			if time.Since(started) > 3*time.Second {
				t.Fatal("worker deadline was not enforced")
			}
			if !processAlive(pin.handle) {
				t.Fatal("worker cleanup killed target")
			}
		})
	}
}

func TestWindowsCodexDescendantAndHandleBound(t *testing.T) {
	root, project := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", root)
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	file := codexFixture(t, root, project, id)
	t.Run("descendant", func(t *testing.T) {
		cmd, _ := codexHolderMode(t, "--codex-file-tree", file)
		observe, closer := newCodexProcessObserver(cmd.Process.Pid)
		if closer == nil {
			t.Fatal("could not pin tree")
		}
		defer closer.Close()
		if got, _ := observe(project); got != id {
			t.Fatal("descendant ownership unavailable", got)
		}
	})
	t.Run("handle-bound", func(t *testing.T) {
		cmd, _ := codexHolderMode(t, "--codex-file-holder-many", file)
		pin, err := pinCodexProcess(cmd.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		defer pin.Close()
		if files, err := windowsCodexFiles(pin.identity); err == nil || len(files) > 0 {
			t.Fatal("partial handle scan accepted", files, err)
		}
	})
}

func TestWindowsCodexInvalidFileHandleFailsClosed(t *testing.T) {
	if kind, err := codexFileType(windows.InvalidHandle); err == nil {
		t.Fatal("invalid file handle accepted", kind)
	}
}
