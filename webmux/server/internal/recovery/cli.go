package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"
)

func originalCLI(name string) (string, error) {
	search := os.Getenv("WEBMUX_BROWSER_ORIGINAL_PATH")
	if search == "" {
		search = os.Getenv("PATH")
	}
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	for _, dir := range filepath.SplitList(search) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil && resolved != self {
			opener, openerErr := filepath.EvalSymlinks(filepath.Join(dir, "webmux-open"))
			if openerErr == nil && opener == resolved {
				continue
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s is not installed on this execution host or is not available on its original PATH", name)
}

func cliOutput(binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = os.Environ()
	data, err := cmd.Output()
	if len(data) > 64<<10 {
		return "", errors.New("coding CLI returned too much metadata")
	}
	return strings.TrimSpace(string(data)), err
}

func newNativeUUID() (string, error) {
	hex, err := NewInstance()
	if err != nil {
		return "", err
	}
	return hex[:8] + "-" + hex[8:12] + "-4" + hex[13:16] + "-a" + hex[17:20] + "-" + hex[20:], nil
}

// RunCLI keeps the native CLI attached directly to the same TTY. Capturing its
// stdout in a pipe would change interactive behavior and terminal capabilities.
func RunCLI(name string, args []string) error {
	binary, err := originalCLI(name)
	if err != nil {
		return err
	}
	cli := CLIName(name)
	if cli == "" {
		return errors.New("unknown coding CLI")
	}
	if os.Getenv("WEBMUX_RECOVERY_INSTANCE") == "" || commandOnly(args) {
		return executeCLI(binary, args)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	version, versionErr := cliOutput(binary, "--version")
	if versionErr != nil {
		version = "unknown"
	}
	id := SelectedNativeID(cli, args)
	launchArgs := append([]string(nil), args...)
	if id == "" && cli == "claude" && !hasSessionSelection(cli, args) {
		help, err := cliOutput(binary, "--help")
		if err == nil && strings.Contains(help, "--session-id") {
			id, err = newNativeUUID()
			if err == nil {
				launchArgs = append([]string{"--session-id", id}, launchArgs...)
			}
		}
	}
	if id == "" && cli == "cursor" && !hasSessionSelection(cli, args) {
		created, err := cliOutput(binary, "create-chat")
		if err == nil && ValidNativeID(cli, created) {
			id = created
			launchArgs = append([]string{"--resume", id}, launchArgs...)
		}
	}
	if id == "" && cli == "opencode" && !hasSessionSelection(cli, args) {
		if created, err := createOpenCodeSession(binary, cwd); err == nil {
			id = created
			launchArgs = append([]string{"--session", id}, launchArgs...)
		}
	}
	event := Event{Kind: "cli", Helper: os.Getenv("WEBMUX_RECOVERY_HELPER"), Platform: runtime.GOOS, Instance: os.Getenv("WEBMUX_RECOVERY_INSTANCE"), Cwd: cwd, CLI: cli, Version: version, SessionID: id, Argv: safeArgv(append([]string{name}, args...))}
	cmd := exec.Command(binary, launchArgs...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	completed := make(chan error, 1)
	go func() { completed <- cmd.Wait() }()
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, cliSignals()...)
	defer signal.Stop(signals)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	emit := func() {
		packet, err := Packet(event)
		if err == nil {
			_ = SendPacket(packet, os.Stdout)
		}
	}
	emit()
	for {
		select {
		case err := <-completed:
			emit()
			packet, packetErr := Packet(Event{Kind: "complete", Instance: event.Instance, Cwd: cwd})
			if packetErr == nil {
				_ = SendPacket(packet, os.Stdout)
			}
			return err
		case sig := <-signals:
			// The terminal sends Ctrl-C to the complete foreground process group.
			// Forwarding it again turns a single interrupt into a double interrupt.
			if sig != os.Interrupt || !term.IsTerminal(int(os.Stdin.Fd())) {
				_ = cmd.Process.Signal(sig)
			}
		case <-ticker.C:
			if cli == "codex" && event.SessionID == "" {
				if id, file := codexSessionForProcess(cmd.Process.Pid, cwd); id != "" {
					event.SessionID, event.StateFile = id, file
					ticker.Reset(5 * time.Second)
				}
			}
			emit()
		}
	}
}

func executeCLI(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func hasContinue(args []string) bool {
	for _, arg := range args {
		if arg == "--continue" || arg == "--last" || arg == "-c" {
			return true
		}
	}
	return false
}

func hasSessionSelection(cli string, args []string) bool {
	if hasContinue(args) {
		return true
	}
	for _, arg := range args {
		key, _, _ := strings.Cut(arg, "=")
		if key == "--resume" || key == "--session" || key == "--session-id" || cli == "claude" && key == "-r" || cli == "opencode" && key == "-s" {
			return true
		}
	}
	return false
}
