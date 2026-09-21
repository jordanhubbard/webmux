//go:build !windows

package terminal

import (
	"github.com/creack/pty"
	"os"
	"os/exec"
)

func childBlockInput() error {
	// Canonical line handling can discard an overlong unterminated line rather
	// than block the writer. Disable it and echo before asserting backpressure.
	command := exec.Command("stty", "-icanon", "-echo")
	command.Stdin = os.Stdin
	return command.Run()
}

func childSize() (int, int, error) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		return 0, 0, err
	}
	return int(size.Cols), int(size.Rows), nil
}
