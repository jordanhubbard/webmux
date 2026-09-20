//go:build !windows

package terminal

import (
	"github.com/creack/pty"
	"os"
)

func childSize() (int, int, error) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		return 0, 0, err
	}
	return int(size.Cols), int(size.Rows), nil
}
