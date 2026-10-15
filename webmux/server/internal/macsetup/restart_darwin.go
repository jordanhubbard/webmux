package macsetup

import (
	"os"
	"syscall"
)

// Replace restarts this executable after HTTP shutdown and session detach.
// Keeping the process under its existing launchd job preserves service ownership.
func Replace() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
