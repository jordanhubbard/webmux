package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
)

// Windows has no qualified shell command capture/evaluation contract yet.
// Reject before durable consumption, keeping native and shell-choice actions
// available without silently interpreting a POSIX replay in cmd.exe.
func prepareWaitingAction(state HostState, action RecoveryAction) (preparedAction, error) {
	if runtime.GOOS == "windows" && action.Kind == "replay" {
		return preparedAction{}, errors.New("command replay is not supported by this Windows recovery worker; reconnect manually")
	}
	return preflightAction(state, action)
}

func WaitForRecovery(ctx context.Context) error {
	id, key, instance := os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY"), os.Getenv("WEBMUX_RECOVERY_INSTANCE")
	if instance == "" {
		return errors.New("recovery startup identity is missing")
	}
	if _, err := updateHostState(id, key, false, nil); err != nil {
		return err
	}
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		return err
	}
	defer cleanup()
	restore, err := discardWaitingInput(os.Stdin)
	if err != nil {
		return err
	}
	defer func() {
		if restore != nil {
			_ = restore()
		}
	}()
	control := Event{Kind: "control", Mode: "waiting", Endpoint: endpoint, Instance: instance, Helper: os.Getenv("WEBMUX_RECOVERY_HELPER"), Platform: runtime.GOOS}
	_, err = updateHostState(id, key, false, func(state *HostState) error { state.Control = &control; return nil })
	if err != nil {
		return err
	}
	_ = Emit(control, os.Stdout)
	fmt.Fprint(os.Stdout, "\r\n[WebMux] Previous work was interrupted. Open Recover work to review and continue.\r\n")
	prepared, err := serveWaiting(ctx, listener, id, key, instance, prepareWaitingAction)
	_ = listener.Close()
	restoreErr := restore()
	restore = nil
	if err != nil || restoreErr != nil {
		return errors.Join(err, restoreErr)
	}
	switch prepared.Kind {
	case "replay":
		return ExitStatusError(10)
	case "native":
		if err := os.Chdir(prepared.Cwd); err != nil {
			return err
		}
		if err := RunConfiguredCLI(prepared.Argv, prepared.CLI, prepared.Configuration); err != nil {
			fmt.Fprintln(os.Stderr, "\r\n[WebMux] Native recovery exited:", err)
		}
	}
	return nil
}
