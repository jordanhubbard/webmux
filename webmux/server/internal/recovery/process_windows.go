//go:build windows

package recovery

import (
	"os"
	"os/exec"
)

func cliSignals() []os.Signal      { return []os.Signal{os.Interrupt} }
func privateProcess(cmd *exec.Cmd) {}
func stopPrivateProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
