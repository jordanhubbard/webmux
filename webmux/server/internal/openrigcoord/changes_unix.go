//go:build !windows

package openrigcoord

import (
	"os"
	"os/exec"
	"syscall"
)

// A canceled wait also owns any CLI child currently checking daemon readiness.
func configureChangeProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
