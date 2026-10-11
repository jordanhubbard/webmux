package browseropen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Open writes to the controlling terminal, not stdout (CLIs often capture their
// browser command's stdout). Bypass tmux's OSC filtering and scrollback by writing
// to the attached client's tty. No OAuth URL is passed to a shell command.
func Open(args []string) error {
	if len(args) != 1 || !ValidURL(args[0]) {
		return errors.New("webmux-open expects one HTTP or HTTPS URL")
	}
	ttys := []string{"/dev/tty"}
	if runtime.GOOS == "windows" {
		ttys = []string{"CONOUT$"}
	}
	if pane := os.Getenv("TMUX_PANE"); pane != "" && os.Getenv("TMUX") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		session, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#{session_id}").Output()
		if err != nil {
			return errors.New("Cannot locate the terminal's tmux session")
		}
		clients, err := exec.CommandContext(ctx, "tmux", "list-clients", "-t", strings.TrimSpace(string(session)), "-F", "#{client_tty}").Output()
		if err != nil {
			return errors.New("Cannot locate the terminal's tmux client")
		}
		ttys = strings.Fields(string(clients))
	}
	if len(ttys) == 0 {
		return errors.New("No terminal client is attached; reconnect WebMux and retry")
	}
	delivered := false
	for _, tty := range ttys {
		f, err := os.OpenFile(tty, os.O_WRONLY, 0)
		if err != nil {
			continue
		}
		_, err = f.WriteString(Packet(args[0]))
		_ = f.Close()
		if err == nil {
			delivered = true
		}
	}
	if !delivered {
		return errors.New("Cannot reach the WebMux terminal; no system browser was opened")
	}
	return nil
}

// ShellEnvironment scopes browser selection to this shell and its descendants.
// No login files, default browser associations, or global tmux settings change.
func ShellEnvironment() ([]string, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return nil, err
	}
	dir, err := installOpeners(binary)
	if err != nil {
		return nil, err
	}
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "BROWSER", "GH_BROWSER", "PATH", "WEBMUX_BROWSER_ORIGINAL_PATH":
			continue
		}
		env = append(env, item)
	}
	opener := filepath.Join(dir, "webmux-open")
	if runtime.GOOS == "windows" {
		opener += ".exe"
	}
	return append(env, "BROWSER="+opener, "GH_BROWSER="+opener, "WEBMUX_BROWSER_BIN="+dir, "WEBMUX_BROWSER_ORIGINAL_PATH="+os.Getenv("PATH"), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH")), nil
}

// Handle also catches PATH-based open/xdg-open invocations. Non-URL invocations
// retain the platform's normal behavior (opening files, folders, or named apps).
func Handle(name string, args []string) error {
	if name == "webmux-open" || len(args) == 1 && ValidURL(args[0]) {
		return Open(args)
	}
	for _, dir := range filepath.SplitList(os.Getenv("WEBMUX_BROWSER_ORIGINAL_PATH")) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		resolved, _ := filepath.EvalSymlinks(candidate)
		self, _ := os.Executable()
		self, _ = filepath.EvalSymlinks(self)
		if resolved == self {
			continue
		}
		cmd := exec.Command(candidate, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("%s is unavailable for this non-URL request", name)
}
