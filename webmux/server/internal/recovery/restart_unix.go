//go:build !windows

package recovery

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Restart is deliberately limited to a single-pane WebMux-owned provider.
// The selection changes when its tmux server, session, window or root process
// changes. The final comparison and respawn run in one tmux command queue.
const restartFormat = "#{pid}|#{start_time}|#{session_id}|#{window_id}|#{pane_id}|#{pane_pid}"

var restartIdentity = regexp.MustCompile(`^[0-9]+\|[0-9]+\|\$[0-9]+\|@[0-9]+\|%[0-9]+\|[0-9]+$`)

func liveRestartTarget(ctx context.Context, id string) (string, string, error) {
	if !nativeUUID.MatchString(id) {
		return "", "", errors.New("invalid provider identity")
	}
	data, err := exec.CommandContext(ctx, "tmux", "list-panes", "-s", "-t", "=webmux-"+id, "-F", restartFormat).Output()
	target := strings.TrimSpace(string(data))
	if err != nil || !restartIdentity.MatchString(target) {
		return "", "", errors.New("restart requires one verified WebMux terminal pane")
	}
	return target, fmt.Sprintf("%x", sha256.Sum256([]byte(id+"|"+target))), nil
}

func liveRestartSelection(ctx context.Context, id string) (string, error) {
	_, selection, err := liveRestartTarget(ctx, id)
	return selection, err
}

func restartLiveProvider(ctx context.Context, request RPCRequest) (RPCResponse, error) {
	helper, err := os.Executable()
	if err != nil {
		return RPCResponse{}, err
	}
	return restartLiveProviderWithHelper(ctx, request, helper)
}

func restartLiveProviderWithHelper(ctx context.Context, request RPCRequest, helper string) (RPCResponse, error) {
	a := request.Action
	if a == nil || !nativeUUID.MatchString(a.ID) || !a.Confirm || a.Kind != "restart" || a.Context != "" {
		return RPCResponse{}, errors.New("explicit terminal restart confirmation is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	outcome := ""
	_, err := updateHostState(request.SessionID, request.Key, false, func(state *HostState) error {
		// A successful receipt is inspect-only on retry, even after further work.
		for _, consumed := range state.Consumed {
			if consumed == a.ID {
				outcome = "consumed"
				return nil
			}
		}
		if state.Control != nil && state.Control.Mode == "waiting" {
			return errors.New("the provider is already waiting; refresh recovery")
		}
		target, selection, err := liveRestartTarget(ctx, request.SessionID)
		if err != nil {
			return err
		}
		if selection != a.Instance {
			return errors.New("terminal changed; refresh the restart preview")
		}
		cache, err := HostCacheRoot()
		if err != nil {
			return err
		}
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
		fields := strings.Split(target, "|")
		pane := fields[4]
		// Start a fresh PTY with the same scoped identity. Old terminal-driver input
		// cannot cross this boundary. No old command or native conversation is run.
		args := []string{"respawn-pane", "-k", "-t", pane}
		for _, env := range []string{
			"WEBMUX_RECOVERY_SESSION=" + request.SessionID, "WEBMUX_RECOVERY_KEY=" + request.Key,
			"WEBMUX_RECOVERY_CACHE_HOME=" + cache, "WEBMUX_RECOVERY_WAIT=1", "WEBMUX_RECOVERY_SEED=",
			"WEBMUX_COORDINATION_FILE=", "WEBMUX_COORDINATION_EXPECTED_SESSION=" + request.SessionID,
			"WEBMUX_COORDINATION_CLI=" + helper, "WEBMUX_COORDINATION_CACHE_HOME=" + os.Getenv("WEBMUX_COORDINATION_CACHE_HOME"),
		} {
			args = append(args, "-e", env)
		}
		args = append(args, "--", helper, "--browser-shell")
		for i := range args {
			args[i] = quote(args[i])
		}
		command := strings.Join(args, " ")
		condition := "#{==:" + restartFormat + "," + target + "}"
		// A changed target emits a marker so it cannot be mistaken for a restart.
		dispatch := exec.CommandContext(ctx, "tmux", "source-file", "-")
		dispatch.Stdin = strings.NewReader("if-shell -F -t " + quote(pane) + " " + quote(condition) + " " + quote(command) + " " + quote("display-message -p 'restart-target-changed'") + "\n")
		output, err := dispatch.CombinedOutput()
		if err != nil {
			outcome = "uncertain"
			return errors.New("terminal restart was not confirmed; refresh before continuing")
		}
		if strings.TrimSpace(string(output)) != "" {
			return errors.New("terminal changed; refresh the restart preview")
		}
		state.Control = &Event{Kind: "control", Mode: "waiting", Instance: "restart-pending", Helper: helper, Ready: false}
		state.Replay = nil
		state.Consumed = append(state.Consumed, a.ID)
		if len(state.Consumed) > 32 {
			state.Consumed = state.Consumed[len(state.Consumed)-32:]
		}
		outcome = "consumed"
		return nil
	})
	if err != nil {
		if outcome == "" {
			outcome = "rejected"
		} else {
			outcome = "uncertain"
		}
	}
	return RPCResponse{Outcome: outcome}, err
}
