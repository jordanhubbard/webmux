//go:build !windows

package agentdeckattach

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func check(ctx context.Context, binary string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "version")
	// The version child must not consume PTY input or perform color detection.
	// The actual attach inherits the original environment unchanged.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TERM=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "TERM=dumb")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 200 * time.Millisecond
	var output versionOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("cannot verify Agent Deck version; check the coordinator and reconnect this viewer: %w", err)
	}
	if !supported(output.buffer.String()) {
		return fmt.Errorf("WebMux requires Agent Deck %s or newer. Update the coordinator, then reconnect this viewer", Required)
	}
	return nil
}

// Run executes in the PTY child, outside the broker's lock. Input stays with the
// PTY while the bounded, noninteractive version check runs; it is never read by
// that check. Exec preserves the attachment PID, arguments, cwd and environment.
func Run(ctx context.Context, argv []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	if !Match(argv) {
		return errors.New("invalid Agent Deck attach invocation")
	}
	binary, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	if err := check(ctx, binary); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return syscall.Exec(binary, argv, os.Environ())
}
