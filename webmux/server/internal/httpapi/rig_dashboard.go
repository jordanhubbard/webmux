package httpapi

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

var healthyRigDaemonPort = regexp.MustCompile(`(?m)^Daemon running on port ([0-9]{1,5})(?: \(pid [0-9]+\))?\r?$`)

// A dashboard is a read-only client. Modern Rig's TUI otherwise falls back to
// its default port, even when CLI discovery found a different local daemon.
func rigDashboardArgv(parent context.Context, executable string) []string {
	argv := []string{executable, "tui"}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "daemon", "status")
	command.Env = openrigcoord.ControlEnvironment()
	command.WaitDelay = 250 * time.Millisecond
	output, err := command.Output()
	if err != nil || len(output) > 4096 {
		return argv
	}
	matches := healthyRigDaemonPort.FindAllSubmatch(output, -1)
	if len(matches) != 1 {
		return argv
	}
	port, err := strconv.Atoi(string(matches[0][1]))
	if err != nil || port < 1024 || port > 65535 {
		return argv
	}
	return append([]string{"/usr/bin/env", "OPENRIG_URL=http://127.0.0.1:" + strconv.Itoa(port)}, argv...)
}
