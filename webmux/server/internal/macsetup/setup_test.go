package macsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Reexec this test executable to exercise real process cancellation and pipe EOF.
func TestProbeProcess(t *testing.T) {
	switch os.Getenv("WEBMUX_TEST_PROBE") {
	case "valid":
		os.Stdout.WriteString(`[{"name":"Desktop","state":"allowed"},{"name":"Documents","state":"missing"},{"name":"Downloads","state":"denied"}]`)
	case "ssh":
		os.Stdout.WriteString("Desktop allowed\nDocuments missing\nDownloads denied\n")
	case "hang":
		time.Sleep(time.Minute)
	case "invalid":
		os.Stdout.WriteString("not a probe result")
	case "failure":
		os.Exit(255)
	default:
		return
	}
	os.Exit(0)
}
func TestProbeResultsAndTimeout(t *testing.T) {
	for _, tc := range []struct{ mode, transport, state string }{
		{"valid", "local", "denied"}, {"ssh", "ssh", "denied"},
		{"invalid", "local", "error"}, {"failure", "ssh", "unavailable"}, {"hang", "local", "waiting"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			duration := 3 * time.Second
			if tc.mode == "hang" {
				duration = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProbeProcess$")
			cmd.Env = append(os.Environ(), "WEBMUX_TEST_PROBE="+tc.mode, "GORACE=atexit_sleep_ms=0")
			cmd.WaitDelay = time.Second
			start := time.Now()
			got := runProbe(ctx, cmd, tc.transport)
			if got.State != tc.state || got.CheckedAt == "" {
				t.Fatalf("unexpected result: %+v", got)
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("helper did not terminate promptly")
			}
		})
	}
}
func TestWorkerDiscardsFilenames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"Desktop", "Downloads"} {
		if err := os.Mkdir(filepath.Join(home, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "Downloads", "private-filename"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ProbeWorker(&out); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("private")) {
		t.Fatal("directory contents leaked")
	}
	var folders []Folder
	if err := json.Unmarshal(out.Bytes(), &folders); err != nil {
		t.Fatal(err)
	}
	if len(folders) != 3 || folders[1].State != "missing" || folders[2].State != "allowed" {
		t.Fatalf("%+v", folders)
	}
}
