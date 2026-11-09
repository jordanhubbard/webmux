//go:build !windows

package recovery

import (
	"os"
	"os/exec"
	"syscall"
)

func cliSignals() []os.Signal      { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }
func privateProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func stopPrivateProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
