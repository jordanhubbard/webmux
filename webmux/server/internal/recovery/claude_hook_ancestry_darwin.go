package recovery

import (
	"time"

	"golang.org/x/sys/unix"
)

// The peer PID is authenticated by the Unix socket. Read its live ancestor
// chain from the kernel; authorizing a hook must not depend on starting ps
// through the caller's PATH or enumerating unrelated processes.
func nativeProcessDescendant(pid, root int) bool {
	if pid < 1 || root < 1 {
		return false
	}
	deadline := time.Now().Add(400 * time.Millisecond)
	for n := 0; n < 64 && pid > 1; n++ {
		if !time.Now().Before(deadline) {
			return false
		}
		if pid == root {
			return true
		}
		info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil || int(info.Proc.P_pid) != pid || !time.Now().Before(deadline) {
			return false
		}
		parent := int(info.Eproc.Ppid)
		if parent < 1 || parent == pid {
			return false
		}
		pid = parent
	}
	return pid == root && time.Now().Before(deadline)
}
