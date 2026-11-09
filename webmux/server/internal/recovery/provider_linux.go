//go:build linux

package recovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// A daemonized tmux server otherwise remains in WebMux's service cgroup.
// KillMode=mixed correctly cleans up the service, but would also kill its jobs.
// Move provider creation into an independent scope owned by this same account.
func scopedProviderCommand(ctx context.Context, tmux, id string, environment []string) *exec.Cmd {
	runner, err := exec.LookPath("systemd-run")
	if err != nil {
		return nil
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	bus := filepath.Join(runtimeDir, "bus")
	if info, err := os.Stat(bus); err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	instance, err := NewInstance()
	if err != nil {
		return nil
	}
	command := exec.CommandContext(ctx, runner, "--user", "--scope", "--collect", "--quiet", "--unit=webmux-provider-"+id+"-"+instance[:8], "--", tmux, "start-server", ";", "source-file", "-")
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		if name != "XDG_RUNTIME_DIR" && name != "DBUS_SESSION_BUS_ADDRESS" {
			command.Env = append(command.Env, item)
		}
	}
	command.Env = append(command.Env, "XDG_RUNTIME_DIR="+runtimeDir, "DBUS_SESSION_BUS_ADDRESS=unix:path="+bus)
	return command
}

func providerServiceWarning(ctx context.Context, tmux, target string, environment []string) string {
	group, err := os.ReadFile("/proc/self/cgroup")
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(group)), ".service") {
		return ""
	}
	probe := exec.CommandContext(ctx, tmux, "display-message", "-p", "-t", target, "#{pid}")
	probe.Env = environment
	output, err := probe.Output()
	if err != nil {
		return "The host could not verify whether running jobs will survive a service restart."
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || pid <= 1 {
		return "The host could not verify whether running jobs will survive a service restart."
	}
	provider, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "The host could not verify whether running jobs will survive a service restart."
	}
	if strings.TrimSpace(string(provider)) == strings.TrimSpace(string(group)) {
		return "Running jobs on this host may end during a server restart. Any retained logs and recorded activity remain available for recovery."
	}
	return ""
}
