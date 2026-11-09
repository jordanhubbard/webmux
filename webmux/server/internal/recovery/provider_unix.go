//go:build !windows

package recovery

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// EnsureProvider probes before creation. The state lock serializes concurrent
// reconnects for this terminal, including requests arriving through SSH/Mosh.
// Source-file stdin carries session environment, so the authentication key is
// neither a public command-line argument nor bytes typed into a terminal.
func EnsureProvider(ctx context.Context, id, key string) (Event, error) {
	helper, err := os.Executable()
	if err != nil {
		return Event{}, err
	}
	return ensureProvider(ctx, id, key, helper)
}

func ensureProvider(ctx context.Context, id, key, helper string) (Event, error) {
	if strings.ContainsFunc(helper, unicode.IsControl) || !filepath.IsAbs(helper) {
		return Event{}, errors.New("invalid execution-host helper path")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return Event{}, errors.New("durable recovery requires tmux on this execution host")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	environment := []string{}
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if name != "TMUX" && name != "TMUX_PANE" {
			environment = append(environment, item)
		}
	}
	session := "webmux-" + id
	provider := Event{Kind: "provider", Mode: "ready", Platform: runtime.GOOS, Helper: helper}
	_, err = updateHostState(id, key, false, func(state *HostState) error {
		probe := exec.CommandContext(ctx, tmux, "has-session", "-t", "="+session)
		probe.Env = environment
		if probe.Run() == nil {
			provider.Survived = true
			if state.Control != nil && state.Control.Mode == "waiting" {
				provider.Mode = "waiting"
			}
			state.Provider = &provider
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
		args := []string{"new-session", "-d", "-s", quote(session), "-e", quote("WEBMUX_RECOVERY_SESSION=" + id), "-e", quote("WEBMUX_RECOVERY_KEY=" + key)}
		// Explicit environment overrides prevent a shared tmux server from
		// accidentally passing another terminal's key or wait policy to a pane.
		wait := "0"
		if state.Command != nil || state.CLI != nil {
			name := filepath.Base(os.Getenv("SHELL"))
			if name == "bash" || name == "zsh" {
				wait, provider.Mode = "1", "waiting"
			} else {
				provider.Mode = "unsupported-shell"
			}
		}
		args = append(args, "-e", quote("WEBMUX_RECOVERY_WAIT="+wait), "-e", quote("WEBMUX_RECOVERY_SEED="))
		if shell := os.Getenv("SHELL"); shell != "" && !strings.ContainsFunc(shell, unicode.IsControl) {
			args = append(args, "-e", quote("SHELL="+shell))
		}
		args = append(args, "--", quote(helper), "--browser-shell")
		command := exec.CommandContext(ctx, tmux, "start-server", ";", "source-file", "-")
		command.Env = environment
		command.Stdin = strings.NewReader(strings.Join(args, " ") + "\n")
		if err := command.Run(); err != nil {
			return errors.New("unable to create the session-owned tmux provider")
		}
		state.Control, state.Replay = nil, nil
		state.Provider = &provider
		return nil
	})
	return provider, err
}
