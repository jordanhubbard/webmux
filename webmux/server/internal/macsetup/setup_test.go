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
	case "missing":
		os.Stdout.WriteString(`[{"name":"Desktop","state":"missing"},{"name":"Documents","state":"missing"},{"name":"Downloads","state":"missing"}]`)
	case "oversized":
		os.Stdout.WriteString(string(bytes.Repeat([]byte("x"), 8192)))
	case "ssh":
		os.Stdout.WriteString("Desktop allowed\nDocuments missing\nDownloads denied\n")
	case "progress":
		os.Stdout.WriteString(`[{"name":"Desktop","state":"allowed"}]` + "\n")
		var ack [1]byte
		if _, err := os.Stdin.Read(ack[:]); err != nil {
			os.Exit(1)
		}
		os.Stdout.WriteString(`[{"name":"Desktop","state":"allowed"},{"name":"Documents","state":"denied"},{"name":"Downloads","state":"allowed"}]` + "\n")
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
		{"missing", "local", "unavailable"}, {"oversized", "local", "error"}, {"invalid", "local", "error"}, {"failure", "ssh", "unavailable"}, {"hang", "local", "waiting"},
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
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected one update per folder: %s", out.Bytes())
	}
	if err := json.Unmarshal(lines[len(lines)-1], &folders); err != nil {
		t.Fatal(err)
	}
	if len(folders) != 3 || folders[1].State != "missing" || folders[2].State != "allowed" {
		t.Fatalf("%+v", folders)
	}
}

func TestProgressBeforeHelperExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProbeProcess$")
	cmd.Env = append(os.Environ(), "WEBMUX_TEST_PROBE=progress", "GORACE=atexit_sleep_ms=0")
	ack, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ack.Close()
	var updates [][]Folder
	result := runProbeProgress(ctx, cmd, "local", func(folders []Folder) {
		if len(updates) == 0 {
			// The child cannot finish until its first update reaches this callback.
			if _, err := ack.Write([]byte("x")); err != nil {
				t.Error(err)
			}
		}
		updates = append(updates, folders)
	})
	if result.State != "denied" || len(updates) < 1 || updates[0][0].State != "allowed" || len(result.Folders) != 3 {
		t.Fatalf("result=%+v updates=%+v", result, updates)
	}
}
