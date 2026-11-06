package browser

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkerProfileCleanup(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 to run installed Chromium")
	}
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
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
