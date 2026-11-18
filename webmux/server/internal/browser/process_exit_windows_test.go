//go:build windows

package browser

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const browserExitChild = "WEBMUX_PRIVATE_BROWSER_EXIT_CHILD"

func TestBrowserProcessExitHeldFileChild(t *testing.T) {
	if os.Getenv(browserExitChild) != "1" {
		t.Skip("private exit-wait control child")
	}
	var path string
	if json.NewDecoder(io.LimitReader(os.Stdin, 8192)).Decode(&path) != nil {
		os.Exit(2)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		os.Exit(2)
	}
	file, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		os.Exit(2)
	}
	if json.NewEncoder(os.Stdout).Encode(true) != nil {
		os.Exit(2)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	windows.CloseHandle(file)
	os.Exit(0)
}

func TestBrowserProcessExitWaitsForOwnedChildAndPreservesSibling(t *testing.T) {
	start := func(profile string) (browserExitIdentity, func()) {
		t.Helper()
		path := filepath.Join(profile, "synthetic-held-file")
		if os.WriteFile(path, []byte("controlled child fixture"), 0600) != nil {
			t.Fatal("fixture file unavailable")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal("fixture executable unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestBrowserProcessExitHeldFileChild$")
		cmd.Env = append(os.Environ(), browserExitChild+"=1")
		input, err := cmd.StdinPipe()
		if err != nil {
			cancel()
			t.Fatal("fixture input unavailable")
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			t.Fatal("fixture output unavailable")
		}
		if cmd.Start() != nil {
			cancel()
			t.Fatal("fixture child unavailable")
		}
		var once sync.Once
		release := func() { once.Do(func() { input.Close(); _ = cmd.Wait(); cancel() }) }
		t.Cleanup(release)
		if json.NewEncoder(input).Encode(path) != nil {
			t.Fatal("fixture request failed")
		}
		var ready bool
		if json.NewDecoder(io.LimitReader(output, 1024)).Decode(&ready) != nil || !ready {
			t.Fatal("fixture handle not ready")
		}
		identity, err := processExitIdentity(uint32(cmd.Process.Pid))
		if err != nil {
			t.Fatal("fixture identity unavailable")
		}
		return identity, release
	}
	selectedRoot, siblingRoot := t.TempDir(), t.TempDir()
	selected, releaseSelected := start(selectedRoot)
	sibling, releaseSibling := start(siblingRoot)
	defer releaseSibling()
	wrong := selected
	wrong.created++
	if wait, err := retainBrowserExitHandles([]browserExitIdentity{wrong}); err == nil || wait != nil {
		t.Fatal("PID creation mismatch accepted")
	}
	wrong = selected
	wrong.image += "-unrelated"
	if wait, err := retainBrowserExitHandles([]browserExitIdentity{wrong}); err == nil || wait != nil {
		t.Fatal("image mismatch accepted")
	}
	if wait, err := retainBrowserExitHandles([]browserExitIdentity{selected, selected}); err == nil || wait != nil {
		t.Fatal("duplicate process identity accepted")
	}
	if wait, err := retainBrowserExitHandles(make([]browserExitIdentity, 65)); err == nil || wait != nil {
		t.Fatal("oversized process set accepted")
	}
	if err := os.RemoveAll(selectedRoot); err == nil {
		t.Fatal("live child did not refuse removal")
	}
	wait, err := retainBrowserExitHandles([]browserExitIdentity{selected})
	if err != nil {
		t.Fatal("cannot retain selected process")
	}
	short, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = wait(short)
	stop()
	if err == nil {
		t.Fatal("live child was reported exited")
	}
	if strings.Contains(err.Error(), selected.image) || strings.Contains(err.Error(), selectedRoot) {
		t.Fatal("private process identity leaked")
	}
	if err := os.RemoveAll(selectedRoot); err == nil {
		t.Fatal("timeout closed the child's source file handle")
	}
	wait, err = retainBrowserExitHandles([]browserExitIdentity{selected})
	if err != nil {
		t.Fatal("cannot retain selected process after timeout")
	}
	releaseSelected()
	bounded, finish := context.WithTimeout(context.Background(), 2*time.Second)
	err = wait(bounded)
	finish()
	if err != nil {
		t.Fatal("selected process exit not observed")
	}
	if err := os.RemoveAll(selectedRoot); err != nil {
		t.Fatal("profile removal failed after owned process exit")
	}
	// The same executable in an unselected process must still own its file.
	siblingHandle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, sibling.pid)
	if err != nil {
		t.Fatal("unrelated sibling disappeared")
	}
	defer windows.CloseHandle(siblingHandle)
	status, err := windows.WaitForSingleObject(siblingHandle, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("unrelated sibling did not survive")
	}
	if err := os.RemoveAll(siblingRoot); err == nil {
		t.Fatal("unrelated sibling file handle was closed")
	}
	releaseSibling()
	if err := os.RemoveAll(siblingRoot); err != nil {
		t.Fatal("sibling fixture cleanup failed after its own release")
	}
}
