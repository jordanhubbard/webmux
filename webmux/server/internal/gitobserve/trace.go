// Package gitobserve provides opt-in, value-free timing of local Git commands.
// Callers retain their existing command, environment, output bounds and deadline.
package gitobserve

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// Event deliberately contains no argv, paths, environment, output or error text.
// Prepare includes executable lookup and command setup. Start includes OS
// process creation; Wait includes Git work and scheduling.
// These timings alone cannot distinguish CPU scheduling from Git execution.
type Event struct {
	ExecutableResolution string        `json:"executable_resolution,omitempty"`
	Prepare              time.Duration `json:"prepare_ns"`
	Phase                string        `json:"phase"`
	Start                time.Duration `json:"start_ns"`
	Wait                 time.Duration `json:"wait_ns"`
	Remaining            time.Duration `json:"remaining_ns"`
	HasDeadline          bool          `json:"has_deadline"`
	Outcome              string        `json:"outcome"`
	ExitCode             int           `json:"exit_code"`
}

type traceKey struct{}

// WithTrace scopes a synchronous observer to an explicit qualification context.
// The observer must be concurrency safe and must not block command execution.
func WithTrace(ctx context.Context, observe func(Event)) context.Context {
	return context.WithValue(ctx, traceKey{}, observe)
}

// Run is equivalent to command.Run, with timing only when explicitly requested.
// Phase must be a fixed label selected by the caller, never repository data.
// Capture preparedAt immediately before exec.CommandContext, which may look up
// the Git executable synchronously before process creation starts.
func Run(ctx context.Context, phase string, preparedAt time.Time, command *exec.Cmd, preparation ...Preparation) error {
	observe, _ := ctx.Value(traceKey{}).(func(Event))
	if observe == nil {
		return command.Run()
	}
	event := Event{Phase: phase, Prepare: time.Since(preparedAt), ExitCode: -1}
	if len(preparation) != 0 {
		event.ExecutableResolution = preparation[0].resolution
	}
	if deadline, ok := ctx.Deadline(); ok {
		event.HasDeadline = true
		event.Remaining = deadline.Sub(preparedAt)
	}
	started := time.Now()
	err := command.Start()
	event.Start = time.Since(started)
	if err == nil {
		started = time.Now()
		err = command.Wait()
		event.Wait = time.Since(started)
	}
	if command.ProcessState != nil {
		event.ExitCode = command.ProcessState.ExitCode()
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		event.Outcome = "deadline"
	case errors.Is(ctx.Err(), context.Canceled):
		event.Outcome = "canceled"
	case err == nil:
		event.Outcome = "ok"
	case command.Process == nil:
		event.Outcome = "start-error"
	default:
		event.Outcome = "command-error"
	}
	observe(event)
	return err
}
