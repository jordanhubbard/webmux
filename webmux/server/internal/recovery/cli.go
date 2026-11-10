package recovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	return cliOutputAt("", binary, args...)
}

func cliOutputAt(cwd, binary string, args ...string) (string, error) {
	return cliTextAt(cwd, binary, false, args...)
}

func cliHelpAt(cwd, binary string, args ...string) (string, error) {
	return cliTextAt(cwd, binary, true, args...)
}

type boundedCLIOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	exceeded bool
}

func (b *boundedCLIOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := (64 << 10) - b.buffer.Len()
	if len(data) > remaining {
		b.exceeded = true
	}
	_, _ = b.buffer.Write(data[:min(len(data), remaining)])
	return len(data), nil
}

func cliTextAt(cwd, binary string, includeStderr bool, args ...string) (string, error) {
	return cliTextContext(context.Background(), cwd, binary, includeStderr, args...)
}

func cliTextContext(parent context.Context, cwd, binary string, includeStderr bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = cmd.Environ()
	var output boundedCLIOutput
	cmd.Stdout = &output
	if includeStderr {
		// Several native CLIs, including OpenCode, print successful help to
		// stderr. Identity-producing commands must still read stdout alone.
		cmd.Stderr = &output
	}
	err := cmd.Run()
	if output.exceeded {
		return "", errors.New("coding CLI returned too much metadata")
	}
	return strings.TrimSpace(output.buffer.String()), err
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
	instance := os.Getenv("WEBMUX_RECOVERY_INSTANCE")
	if instance == "" || os.Getenv("WEBMUX_RECOVERY_NATIVE_PARENT") == instance || commandOnly(cli, args) {
		return executeCLI(binary, args)
	}
	// Other products also install an executable named agent. Never inject
	// Cursor's selectors into an unrelated CLI, even when its flags overlap.
	if cli == "cursor" && !isCursorCLI(binary) {
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
		help, err := cliHelpAt(cwd, binary, "--help")
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
	if cli == "codex" && id != "" {
		event.StateFile = codexStateFile(id, cwd, "")
	}
	event.Configuration = captureNativeConfiguration(cli)
	event.NativeEvidence = "unavailable"
	if id != "" {
		event.NativeEvidence = "launch-selector"
	}
	var observations <-chan claudeSessionObservation
	var claudeHookTranscript string
	var observer *claudeObserver
	if cli == "claude" {
		observer = newClaudeObserver(version, args)
		if observer != nil {
			defer observer.close()
			launchArgs = append([]string{"--settings", observer.settings}, launchArgs...)
			observations = observer.observations
		}
	}
	cmd := exec.Command(binary, launchArgs...)
	cmd.Dir = cwd
	// Commands launched by a coding CLI do not own its terminal's foreground
	// recovery identity. A new provider's distinct instance remains independent.
	cmd.Env = append(cmd.Environ(), "WEBMUX_RECOVERY_NATIVE_PARENT="+instance)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if observer != nil {
		observer.start(cmd.Process.Pid)
	}
	completed := make(chan error, 1)
	go func() { completed <- cmd.Wait() }()
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, cliSignals()...)
	defer signal.Stop(signals)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	emit := func() {
		_ = Emit(event, os.Stdout)
	}
	emit()
	for {
		select {
		case err := <-completed:
			// Native hooks can finish just before the CLI exits. Preserve any
			// already authenticated observations queued before completion.
			for {
				select {
				case observation := <-observations:
					observeClaudeSession(&event, observation, cwd)
				default:
					emit()
					_ = Emit(Event{Kind: "complete", Instance: event.Instance, Cwd: cwd}, os.Stdout)
					return err
				}
			}
		case sig := <-signals:
			// The terminal sends Ctrl-C to the complete foreground process group.
			// Forwarding it again turns a single interrupt into a double interrupt.
			if sig != os.Interrupt || !term.IsTerminal(int(os.Stdin.Fd())) {
				_ = cmd.Process.Signal(sig)
			}
		case observation := <-observations:
			observeClaudeSession(&event, observation, cwd)
			claudeHookTranscript = observation.Transcript
			emit()
		case <-ticker.C:
			if cli == "claude" && event.SessionID != "" {
				if event.NativeEvidence == "native-hook" {
					event.StateFile = claudeObservedStateFile(event.SessionID, cwd, claudeHookTranscript)
				} else {
					event.StateFile = claudeStateFile(event.SessionID, cwd)
				}
			}
			if cli == "cursor" && event.SessionID != "" {
				event.StateFile = cursorStateFile(event.SessionID, cwd)
			}
			if cli == "codex" {
				currentID, currentFile := codexSessionForProcess(cmd.Process.Pid, cwd)
				observeCodexProcess(&event, currentID, currentFile)
			}
			if event.StateFile != "" {
				ticker.Reset(5 * time.Second)
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
		if cli == "cursor" && (key == "ls" || key == "resume" || key == "--worktree" || key == "-w" || key == "--workspace") || cli == "claude" && (key == "--from-pr" || key == "--teleport" || key == "--cloud" || key == "--environment" || key == "--fork-session" || key == "--worktree" || key == "-w" || key == "--desktop" || key == "--bg" || key == "--background") || cli == "opencode" && (key == "--fork" || key == "pr") {
			return true
		}
		if key == "--resume" || key == "--session" || key == "--session-id" || cli == "claude" && key == "-r" || cli == "opencode" && key == "-s" {
			return true
		}
	}
	return false
}

func isCursorCLI(binary string) bool {
	return isCursorCLIContext(context.Background(), binary)
}

func isCursorCLIContext(ctx context.Context, binary string) bool {
	help, err := cliTextContext(ctx, "", binary, true, "--help")
	return err == nil && strings.Contains(help, "Start the Cursor Agent") && strings.Contains(help, "create-chat") && strings.Contains(help, "--resume")
}

// Prefer the unambiguous alias, but support installations that expose only
// agent after verifying its native help. A shared basename is not identity.
func cursorCLI() (string, string, error) {
	return cursorCLIContext(context.Background())
}

func cursorCLIContext(ctx context.Context) (string, string, error) {
	for _, name := range []string{"cursor-agent", "agent"} {
		if binary, err := originalCLI(name); err == nil && isCursorCLIContext(ctx, binary) {
			return name, binary, nil
		}
	}
	return "", "", errors.New("a supported Cursor Agent is unavailable on this execution host")
}
