package browser

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorkerProfileCleanup(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 to run installed Chromium")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// A real Snap browser must see this path too, so keep this test's private
	// home below the actual account home rather than an isolated /tmp mount.
	root, err := os.MkdirTemp(home, "webmux-browser-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", t.TempDir())
	in, send := io.Pipe()
	receive, out := io.Pipe()
	done := make(chan error, 1)
	go func() { err := Worker(in, out); _ = out.Close(); done <- err }()
	t.Cleanup(func() { _ = send.Close(); _ = receive.Close() })
	go func() { _ = json.NewEncoder(send).Encode(Request{Action: "frame"}) }()
	var frame Frame
	decoder := json.NewDecoder(receive)
	if err := decoder.Decode(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Error != "" || len(frame.Image) == 0 {
		t.Fatalf("browser did not start: %s", frame.Error)
	}
	profiles, _ := filepath.Glob(filepath.Join(root, "webmux-browser-*"))
	if len(profiles) != 1 {
		t.Fatalf("expected one private profile, got %v", profiles)
	}
	first := frame.Target
	go func() { _ = json.NewEncoder(send).Encode(Request{Action: "new_tab"}) }()
	if err := decoder.Decode(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Error != "" || frame.Target == first || len(frame.Tabs) != 2 {
		t.Fatalf("new tab failed: %+v", frame)
	}
	second := frame.Target
	for _, req := range []Request{{Action: "tab", Target: first}, {Action: "close_tab", Target: first}, {Action: "frame"}} {
		go func() { _ = json.NewEncoder(send).Encode(req) }()
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		if frame.Error != "" {
			t.Fatalf("tab lifecycle failed: %s", frame.Error)
		}
	}
	if frame.Target != second || len(frame.Tabs) != 1 {
		t.Fatalf("closing the root tab lost the retained tab: %+v", frame)
	}
	_ = send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after pipe closed")
	}
	profiles, _ = filepath.Glob(filepath.Join(root, "webmux-browser-*"))
	if len(profiles) != 0 {
		t.Fatalf("credential profile leaked: %v", profiles)
	}
}

func TestModifierCombinationsSurviveWorkerProtocol(t *testing.T) {
	for modifiers := int64(0); modifiers < 16; modifiers++ {
		encoded, err := json.Marshal(Request{Action: "key", Modifiers: modifiers})
		if err != nil {
			t.Fatal(err)
		}
		var decoded Request
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Modifiers != modifiers {
			t.Fatalf("modifiers changed: %d -> %d", modifiers, decoded.Modifiers)
		}
	}
}

func TestTemporaryProfileUsesSharedPrivateHome(t *testing.T) {
	home, isolated := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TMPDIR", isolated)
	t.Setenv("TMP", isolated)
	t.Setenv("TEMP", isolated)
	profile, err := temporaryProfile()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)
	if filepath.Dir(profile) != home {
		t.Fatal("profile and transfer files must use the shared account home")
	}
	info, err := os.Stat(profile)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Fatalf("private profile permissions: %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(isolated)
	if err != nil || len(entries) != 0 {
		t.Fatal("profile used the browser-isolated temporary directory")
	}
}

func TestWorkerRefusesUnavailableSharedProfile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-marker")
	if err := os.WriteFile(home, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var output bytes.Buffer
	if err := Worker(strings.NewReader("{\"action\":\"frame\"}\n"), &output); err == nil {
		t.Fatal("worker silently fell back to an isolated temporary profile")
	}
	var frame Frame
	if err := json.Unmarshal(output.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(frame.Error, "home directory") || strings.Contains(frame.Error, "private-marker") {
		t.Fatalf("missing or private-path-bearing profile diagnostic: %s", frame.Error)
	}
}
