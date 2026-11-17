package rigobservation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReportRejectsAmbiguousOrExecutableIdentity(t *testing.T) {
	base := Seat{RigID: "rig-1", RigName: "project", LogicalID: "agents.worker", CanonicalSessionName: "canonical-1", TmuxTarget: "safe-target"}
	for _, name := range []string{"duplicate-seat", "duplicate-session", "command", "control", "oversized", "version", "unavailable-with-seats"} {
		t.Run(name, func(t *testing.T) {
			r := Report{Contract: Contract, Source: Source, State: "ok", Seats: []Seat{base}}
			switch name {
			case "duplicate-seat":
				r.Seats = append(r.Seats, base)
			case "duplicate-session":
				b := base
				b.LogicalID = "other"
				r.Seats = append(r.Seats, b)
			case "command":
				r.Seats[0].TmuxTarget = "$(touch /tmp/unsafe)"
			case "control":
				r.Seats[0].RigName = "project\nforged"
			case "oversized":
				r.Seats[0].RigName = strings.Repeat("x", 257)
			case "version":
				r.Contract = "unknown/v2"
			case "unavailable-with-seats":
				r.State = "unavailable"
			}
			if Validate(&r) == nil {
				t.Fatal("accepted invalid report", name)
			}
		})
	}
	changed := base
	changed.CanonicalSessionName = "replacement"
	changed.TmuxTarget = "new-target"
	if changed.Key() != base.Key() {
		t.Fatal("attachment change altered stable seat identity")
	}
}

func TestObserverIsLocalBoundedAndReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig supports Unix hosts")
	}
	directory := t.TempDir()
	log := filepath.Join(directory, "calls")
	executable := filepath.Join(directory, "rig")
	script := `#!/bin/sh
printf '%s\n' "$*" "$OPENRIG_HOST_SELECTED" "${OPENRIG_URL-unset}" > "$RIG_OBSERVATION_LOG"
printf '%s\n' '{"entries":[{"rigId":"r","rigName":"project","logicalId":"agents.worker","canonicalSessionName":"seat","tmuxAttachCommand":"tmux attach -t safe-target"}]}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("RIG_OBSERVATION_LOG", log)
	t.Setenv("OPENRIG_HOST_SELECTED", "other-host")
	t.Setenv("OPENRIG_URL", "http://other-host")
	r := Observe(context.Background())
	if r.State != "ok" || len(r.Seats) != 1 || r.Seats[0].TmuxTarget != "safe-target" {
		t.Fatalf("invalid observation: %+v", r)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "ps --nodes -A --no-cleanup --json --fields ") || !strings.HasSuffix(string(calls), "\nlocal\nunset\n") {
		t.Fatal(string(calls))
	}
	// A peer cannot turn arbitrary CLI output into an executable attach request.
	if err := os.WriteFile(executable, []byte(strings.ReplaceAll(script, "tmux attach -t safe-target", "tmux attach -t safe; touch /tmp/unsafe")), 0700); err != nil {
		t.Fatal(err)
	}
	r = Observe(context.Background())
	if r.State != "ok" || r.Seats[0].TmuxTarget != "" {
		t.Fatal(r)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nwhile :; do printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r = Observe(context.Background())
	if r.State != "unavailable" || len(r.Seats) != 0 || time.Since(start) > 4*time.Second {
		t.Fatal("unbounded observer", r)
	}
	data, _ := json.Marshal(r)
	if len(data) > MaxBytes {
		t.Fatal("oversized report")
	}
}

// Called by the real published-Rig browser fixture after it enrolls actual
// tmux seats. Ordinary unit runs do not start or borrow a developer daemon.
func TestPublishedRigObservation(t *testing.T) {
	expected := os.Getenv("WEBMUX_TEST_RIG_TARGET")
	if expected == "" {
		t.Skip("requires the isolated published-Rig browser fixture")
	}
	first := Observe(context.Background())
	if first.State != "ok" {
		t.Fatalf("published CLI observation failed: %+v", first)
	}
	key := ""
	for _, seat := range first.Seats {
		if seat.CanonicalSessionName == expected {
			key = seat.Key()
			if seat.TmuxTarget == "" || seat.RigID == "" || seat.LogicalID == "" {
				t.Fatal("canonical seat is not attachable", seat)
			}
		}
	}
	if key == "" {
		t.Fatalf("actual enrolled seat %q absent: %+v", expected, first)
	}
	second := Observe(context.Background())
	if second.State != "ok" || len(second.Seats) != len(first.Seats) {
		t.Fatal("observation altered the topology")
	}
	for _, seat := range second.Seats {
		if seat.CanonicalSessionName == expected {
			if seat.Key() != key {
				t.Fatal("unstable canonical identity")
			}
			return
		}
	}
	t.Fatal("second observation lost the actual seat")
}
