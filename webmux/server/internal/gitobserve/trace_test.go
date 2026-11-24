package gitobserve

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestCommandFixture(t *testing.T) {
	if os.Getenv("WEBMUX_GITOBSERVE_FIXTURE") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "ok":
		os.Stdout.WriteString("private-output")
		os.Exit(0)
	case "exit":
		os.Exit(7)
	case "wait":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(8)
}

func TestTracePreservesCommandsAndCancellation(t *testing.T) {
	for _, mode := range []string{"ok", "exit", "wait", "missing"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var events []Event
			ctx = WithTrace(ctx, func(e Event) { events = append(events, e) })
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandFixture$", "--", mode)
			command.Env = append(os.Environ(), "WEBMUX_GITOBSERVE_FIXTURE=1")
			var output bytes.Buffer
			command.Stdout = &output
			if mode == "wait" {
				cancel()
			}
			if mode == "missing" {
				command = exec.CommandContext(ctx, "webmux-missing-git-diagnostic-fixture")
			}
			err := Run(ctx, "fixture", time.Now(), command)
			if len(events) != 1 || events[0].Phase != "fixture" {
				t.Fatal(events)
			}
			e := events[0]
			switch mode {
			case "ok":
				if err != nil || e.Outcome != "ok" || e.ExitCode != 0 || output.String() != "private-output" {
					t.Fatal(e, err)
				}
			case "exit":
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 7 || e.ExitCode != 7 || e.Outcome != "command-error" {
					t.Fatal(e, err)
				}
			case "wait":
				if !errors.Is(err, context.Canceled) || e.Outcome != "canceled" {
					t.Fatal(e, err)
				}
			case "missing":
				if err == nil || e.Outcome != "start-error" {
					t.Fatal(e, err)
				}
			}
		})
	}
}

func TestExpiredDeadlineStaysExpired(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	var event Event
	ctx = WithTrace(ctx, func(e Event) { event = e })
	err := Run(ctx, "expired", time.Now(), exec.CommandContext(ctx, os.Args[0]))
	if !errors.Is(err, context.DeadlineExceeded) || !event.HasDeadline || event.Remaining >= 0 || event.Outcome != "deadline" {
		t.Fatal(event, err)
	}
}
