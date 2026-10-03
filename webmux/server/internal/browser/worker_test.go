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
	if err := json.NewDecoder(receive).Decode(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Error != "" || len(frame.Image) == 0 {
		t.Fatalf("browser did not start: %s", frame.Error)
	}
	profiles, _ := filepath.Glob(filepath.Join(root, "webmux-browser-*"))
	if len(profiles) != 1 {
		t.Fatalf("expected one private profile, got %v", profiles)
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
