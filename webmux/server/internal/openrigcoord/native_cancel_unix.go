//go:build darwin || linux

package openrigcoord

import (
	"os/exec"
	"syscall"
	"time"
)

// Signal only our helper, not the separately owned native manager/provider.
// Its policy resolver gets time to reap its private detached app-server.
func configureNativeCreateCancellation(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
}
