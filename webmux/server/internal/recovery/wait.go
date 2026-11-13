package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

type preparedAction struct {
	CLI           string
	Configuration *NativeConfiguration
	Kind          string
	Cwd           string
	Command       string
	Argv          []string
}

func cachedActivity(state HostState) Activity {
	var activity Activity
	if event := state.Command; event != nil {
		activity = Activity{Platform: event.Platform, ShellInstance: event.Instance, Cwd: event.Cwd, Command: event.Command, Replayable: !event.Redacted, Running: state.Complete == nil}
	}
	if event := state.CLI; event != nil {
		// Windows native aliases can be the first reliable observation: cmd
		// has no command hook to provide this identity on their behalf.
		activity.Platform, activity.ShellInstance = event.Platform, event.Instance
		activity.Running = state.Complete == nil || state.Complete.Instance != event.Instance
		activity.CLI, activity.CLIVersion, activity.NativeSessionID, activity.NativeStateFile = event.CLI, event.Version, event.SessionID, event.StateFile
		activity.NativeEvidence = event.NativeEvidence
		activity.Argv = append([]string(nil), event.Argv...)
		activity.Configuration = event.Configuration.Clone()
		activity.Cwd = event.Cwd
	}
	return activity
}

func preflightAction(state HostState, action RecoveryAction) (preparedAction, error) {
	activity := cachedActivity(state)
	switch action.Kind {
	case "shell":
		if action.Context != "" {
			return preparedAction{}, errors.New("shell recovery does not accept transcript handoff")
		}
		return preparedAction{Kind: "shell"}, nil
	case "native":
		if action.Context != "" && !action.Confirm {
			return preparedAction{}, errors.New("transcript handoff requires explicit confirmation")
		}
		args, err := prepareNativeResume(NativeResume{Activity: activity, Context: action.Context})
		return preparedAction{Kind: "native", Cwd: activity.Cwd, Argv: args, CLI: activity.CLI, Configuration: activity.Configuration.Clone()}, err
	case "replay":
		if !action.Confirm || !activity.Replayable || activity.Command == "" || action.Context != "" {
			return preparedAction{}, errors.New("command replay requires an unredacted command and explicit confirmation")
		}
		info, err := os.Stat(activity.Cwd)
		if err != nil || !info.IsDir() {
			return preparedAction{}, errors.New("the original command directory is unavailable")
		}
		return preparedAction{Kind: "replay", Cwd: activity.Cwd, Command: activity.Command}, nil
	default:
		return preparedAction{}, errors.New("unknown recovery action")
	}
}

// The dedicated worker is the only execution authority. Its serial accept loop
// can consume one intent, then closes before a child is started. A shell-ready
// frame is never sufficient authority to inject an action into a running TTY.
func serveWaiting(ctx context.Context, listener *net.UnixListener, id, key, instance string, prepare func(HostState, RecoveryAction) (preparedAction, error)) (preparedAction, error) {
	for {
		if err := ctx.Err(); err != nil {
			return preparedAction{}, err
		}
		_ = listener.SetDeadline(time.Now().Add(200 * time.Millisecond))
		connection, err := listener.AcceptUnix()
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				continue
			}
			return preparedAction{}, err
		}
		// Slow/incomplete local clients cannot hold the worker indefinitely.
		_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
		var request RPCRequest
		var prepared preparedAction
		err = readWorkerRPC(connection, &request)
		if err == nil {
			prepared, err = acceptWaitingAction(request, id, key, instance, prepare)
		}
		response := RPCResponse{Protocol: 1, Outcome: "consumed"}
		if err != nil {
			response.Outcome, response.Error = "rejected", err.Error()
		}
		// Consumption is already durable. A lost response must never cause replay.
		_ = json.NewEncoder(connection).Encode(response)
		_ = connection.Close()
		if err == nil {
			return prepared, nil
		}
	}
}

func acceptWaitingAction(request RPCRequest, id, key, instance string, prepare func(HostState, RecoveryAction) (preparedAction, error)) (preparedAction, error) {
	if request.Protocol != 1 || request.Operation != "action" || request.SessionID != id || request.Key != key || request.Action == nil {
		return preparedAction{}, errors.New("recovery worker authentication failed")
	}
	action := *request.Action
	if !nativeUUID.MatchString(action.ID) || action.Instance != instance {
		return preparedAction{}, errors.New("stale or invalid recovery action")
	}
	state, err := updateHostState(id, key, false, nil)
	if err != nil {
		return preparedAction{}, err
	}
	valid := func(state *HostState) error {
		if state.Revision != action.Revision || state.Control == nil || state.Control.Mode != "waiting" || state.Control.Instance != instance {
			return errors.New("recovery state changed; refresh the preview before continuing")
		}
		for _, consumed := range state.Consumed {
			if consumed == action.ID {
				return errors.New("this recovery action was already consumed")
			}
		}
		return nil
	}
	if err := valid(&state); err != nil {
		return preparedAction{}, err
	}
	prepared, err := prepare(state, action)
	if err != nil {
		return preparedAction{}, err
	}
	_, err = updateHostState(id, key, false, func(state *HostState) error {
		if err := valid(state); err != nil {
			return err
		}
		state.Consumed = append(state.Consumed, action.ID)
		if len(state.Consumed) > 64 {
			state.Consumed = state.Consumed[len(state.Consumed)-64:]
		}
		state.Control = &Event{Kind: "control", Platform: runtime.GOOS, Mode: "consumed", Instance: instance}
		if prepared.Kind == "replay" {
			state.Replay = &ReplayIntent{Instance: instance, Cwd: prepared.Cwd, Command: prepared.Command}
		}
		if prepared.Kind == "shell" {
			state.Complete = &Event{Kind: "complete", Instance: cachedActivity(*state).ShellInstance}
		}
		return nil
	})
	return prepared, err
}

// ReadReplay consumes the approved command before returning any bytes. Only the
// startup shell which owns this waiting worker can evaluate it, before readline
// or an application accepts terminal input. A lost response is not retried.
func ReadReplay() (string, error) {
	id, key, instance := os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY"), os.Getenv("WEBMUX_RECOVERY_INSTANCE")
	var intent *ReplayIntent
	_, err := updateHostState(id, key, false, func(state *HostState) error {
		if state.Replay == nil || state.Replay.Instance != instance || state.Control == nil || state.Control.Mode != "consumed" || state.Control.Instance != instance {
			return errors.New("no approved command belongs to this startup shell")
		}
		intent, state.Replay = state.Replay, nil
		return nil
	})
	if err != nil {
		return "", err
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
	return "cd -- " + quote(intent.Cwd) + " && { \"$WEBMUX_RECOVERY_HELPER\" --recovery-event command \"$PWD\" " + quote(intent.Command) + "; eval " + quote(intent.Command) + "; }", nil
}

type ExitStatusError int

func (e ExitStatusError) Error() string { return "recovery helper exited" }
func (e ExitStatusError) ExitCode() int { return int(e) }

// ShellWait is appended after user profiles and before capture hooks/readline.
// It never turns terminal output or arbitrary OSC packets into shell commands.
func ShellWait() string {
	return `
if [ "${WEBMUX_RECOVERY_WAIT:-}" = 1 ]; then
  unset WEBMUX_RECOVERY_WAIT
  "$WEBMUX_RECOVERY_HELPER" --recovery-wait
  _webmux_wait_status=$?
  if [ "$_webmux_wait_status" = 10 ]; then
    if _webmux_approved_replay=$("$WEBMUX_RECOVERY_HELPER" --recovery-read-command); then
      eval "$_webmux_approved_replay"
    fi
    unset _webmux_approved_replay
  elif [ "$_webmux_wait_status" != 0 ]; then
    printf '\nWebMux recovery worker stopped; previous logs remain available.\n' >&2
  fi
  unset _webmux_wait_status
fi
`
}
