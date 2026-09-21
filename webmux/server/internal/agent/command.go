package agent

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type runner func(context.Context, string, ...string) (string, string, error)

type boundedOutput struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *boundedOutput) Len() int       { return b.buffer.Len() }
func (b *boundedOutput) String() string { return b.buffer.String() }

func (b *boundedOutput) Write(data []byte) (int, error) {
	remaining := (1 << 20) - b.Len()
	if len(data) > remaining {
		n, _ := b.buffer.Write(data[:remaining])
		b.exceeded = true
		b.cancel()
		return n, errors.New("tmux output exceeds 1 MiB")
	}
	return b.buffer.Write(data)
}
func runCommand(ctx context.Context, command string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.WaitDelay = time.Second
	stdout, stderr := &boundedOutput{cancel: cancel}, &boundedOutput{cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.exceeded || stderr.exceeded {
		err = errors.New("tmux output exceeds 1 MiB")
	}
	if ctx.Err() != nil && !stdout.exceeded && !stderr.exceeded {
		err = ctx.Err()
	}
	return stdout.String(), stderr.String(), err
}
