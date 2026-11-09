//go:build windows

package recovery

import (
	"golang.org/x/sys/windows"
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

func lockStateFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}
func unlockStateFile(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
