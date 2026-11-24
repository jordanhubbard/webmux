//go:build darwin || linux

package recovery

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A pipe-protocol fixture, not native Codex qualification or a coding agent.
func TestCodexSelectionChild(t *testing.T) {
	if os.Getenv("WEBMUX_CODEX_SELECTION_FIXTURE") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg != "--selection-observer-fd" || index+1 == len(os.Args) {
			continue
		}
		fd, err := strconv.Atoi(os.Args[index+1])
		if err != nil {
			os.Exit(2)
		}
		file := os.NewFile(uintptr(fd), "private-selection")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var observation codexSelectionObservation
			if json.Unmarshal(scanner.Bytes(), &observation) != nil {
				os.Exit(3)
			}
			if observation.PID == 0 {
				observation.PID = os.Getpid()
			}
			if json.NewEncoder(file).Encode(observation) != nil {
				os.Exit(4)
			}
		}
		os.Exit(0)
	}
	os.Exit(5)
}

func codexSelectionBinary(t *testing.T, supported bool) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "codex")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }
	help := "--other-option"
	if supported {
		help = "--selection-observer-fd FD"
	}
	script := "#!/bin/sh\nif [ \"$1\" = --help ]; then echo " + quote(help) + "; exit 0; fi\nexec " + quote(os.Args[0]) + " -test.run=^TestCodexSelectionChild$ -- \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return binary
}

func TestCodexSelectionPrivatePipeBindsProcessAndRefusesReplay(t *testing.T) {
	binary := codexSelectionBinary(t, true)
	o := newCodexSelectionObserver(binary, t.TempDir(), nil)
	if o == nil {
		t.Fatal("supported private observation unavailable")
	}
	defer o.close()
	input, writer := io.Pipe()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "WEBMUX_CODEX_SELECTION_FIXTURE=1")
	cmd.Stdin = input
	unrelated, err := os.CreateTemp(t.TempDir(), "other-launch-channel")
	if err != nil {
		t.Fatal(err)
	}
	defer unrelated.Close()
	cmd.ExtraFiles = []*os.File{unrelated}
	o.attach(cmd)
	if len(cmd.ExtraFiles) != 2 || cmd.Args[1] != "--selection-observer-fd" || cmd.Args[2] != "4" {
		t.Fatal("missing isolated inherited descriptor")
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close(); _ = input.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	o.start(cmd.Process.Pid)
	read := func() codexSelectionObservation {
		t.Helper()
		select {
		case observation := <-o.observations:
			return observation
		case <-time.After(3 * time.Second):
			t.Fatal("missing observation")
			return codexSelectionObservation{}
		}
	}
	id := "01a1c268-92c8-7393-bfc3-e387e33d52b9"
	observation := codexSelectionObservation{Schema: "codex.tui.selection.v1", Sequence: 1, ThreadID: &id}
	if err := json.NewEncoder(writer).Encode(observation); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.ThreadID == nil || *got.ThreadID != id || got.PID != cmd.Process.Pid {
		t.Fatal(got)
	}
	// A duplicated sequence poisons this channel. It cannot maintain confidence
	// or redirect recovery to a stale conversation after an in-process switch.
	if err := json.NewEncoder(writer).Encode(observation); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.ThreadID != nil || got.Schema != "" {
		t.Fatal("replay retained selection", got)
	}
	o.close()
}

func TestCodexSelectionDoesNotInjectIntoUnsupportedOrCallerOwnedChannels(t *testing.T) {
	if got := newCodexSelectionObserver(codexSelectionBinary(t, false), t.TempDir(), nil); got != nil {
		got.close()
		t.Fatal("unsupported CLI was modified")
	}
	if got := newCodexSelectionObserver(codexSelectionBinary(t, true), t.TempDir(), []string{"--selection-observer-fd=5"}); got != nil {
		got.close()
		t.Fatal("caller's observer channel replaced")
	}
	o := newCodexSelectionObserver(codexSelectionBinary(t, true), t.TempDir(), nil)
	if o == nil {
		t.Fatal("missing supported observer")
	}
	// Failed process launch must not leave a blocking reader or an open pipe.
	o.close()
	o.close()
}
