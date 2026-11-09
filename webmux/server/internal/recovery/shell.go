package recovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func NewInstance() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

// ShellEvent runs on the terminal's execution host, not on the browser host.
func ShellEvent(args []string, out io.Writer) error {
	if len(args) < 2 || len(args) > 3 {
		return errors.New("invalid shell recovery event")
	}
	e := Event{Kind: args[0], Cwd: args[1], Helper: os.Getenv("WEBMUX_RECOVERY_HELPER"), Platform: runtime.GOOS, Instance: os.Getenv("WEBMUX_RECOVERY_INSTANCE")}
	if len(args) == 3 {
		e.Command = args[2]
	}
	if e.Instance == "" {
		return nil
	}
	packet, err := Packet(e)
	if err != nil {
		return err
	}
	return SendPacket(packet, out)
}

// tmux consumes unknown OSC sequences. Deliver metadata to its attached client
// TTYs, as the browser opener does, without storing it in tmux pane history.
func SendPacket(packet string, out io.Writer) error {
	if pane := os.Getenv("TMUX_PANE"); pane != "" && os.Getenv("TMUX") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		session, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#{session_id}").Output()
		if err != nil {
			return nil
		}
		clients, err := exec.CommandContext(ctx, "tmux", "list-clients", "-t", strings.TrimSpace(string(session)), "-F", "#{client_tty}").Output()
		if err != nil {
			return nil
		}
		for _, tty := range strings.Fields(string(clients)) {
			file, err := os.OpenFile(tty, os.O_WRONLY, 0)
			if err != nil {
				continue
			}
			_, _ = io.WriteString(file, packet)
			_ = file.Close()
		}
		return nil
	}
	_, err := fmt.Fprint(out, packet)
	return err
}

// ShellHooks are scoped to the shell started by WebMux. User startup files and
// existing preexec/prompt hooks are retained. Existing DEBUG traps take priority.
func ShellHooks(shell string) string {
	switch shell {
	case "bash":
		return `
_webmux_begin_prompt() {
  local _webmux_exit_status=$?
  WEBMUX_IN_PROMPT=1
  "$WEBMUX_RECOVERY_HELPER" --recovery-event complete "$PWD"
  return "$_webmux_exit_status"
}
_webmux_end_prompt() {
  local _webmux_exit_status=$?
  "$WEBMUX_RECOVERY_HELPER" --recovery-event shell "$PWD"
  WEBMUX_IN_PROMPT=0
  return "$_webmux_exit_status"
}
_webmux_preexec() {
  case "$1" in _webmux_*|WEBMUX_IN_PROMPT=*) return;; esac
  if [ "${WEBMUX_IN_PROMPT:-1}" = 0 ] && [ "${WEBMUX_CAPTURE_BUSY:-0}" = 0 ]; then
    WEBMUX_CAPTURE_BUSY=1
    "$WEBMUX_RECOVERY_HELPER" --recovery-event command "$PWD" "$1"
    WEBMUX_CAPTURE_BUSY=0
  fi
}
if [ -z "$(trap -p DEBUG)" ]; then
  trap '_webmux_preexec "$BASH_COMMAND"' DEBUG
fi
if [[ $(declare -p PROMPT_COMMAND 2>/dev/null) == "declare -a "* ]]; then
  PROMPT_COMMAND=(_webmux_begin_prompt "${PROMPT_COMMAND[@]}" _webmux_end_prompt)
else
  PROMPT_COMMAND="_webmux_begin_prompt${PROMPT_COMMAND:+; $PROMPT_COMMAND}; _webmux_end_prompt"
fi
`
	case "zsh":
		return `
_webmux_preexec() { "$WEBMUX_RECOVERY_HELPER" --recovery-event command "$PWD" "$1"; }
_webmux_prompt() {
  local _webmux_exit_status=$?
  "$WEBMUX_RECOVERY_HELPER" --recovery-event complete "$PWD"
  "$WEBMUX_RECOVERY_HELPER" --recovery-event shell "$PWD"
  return "$_webmux_exit_status"
}
autoload -Uz add-zsh-hook
add-zsh-hook -d preexec _webmux_preexec
add-zsh-hook -d precmd _webmux_prompt
add-zsh-hook preexec _webmux_preexec
add-zsh-hook precmd _webmux_prompt
`
	}
	return ""
}
