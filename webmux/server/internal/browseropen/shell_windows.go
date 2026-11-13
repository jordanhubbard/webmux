//go:build windows

package browseropen

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

func installOpeners(binary string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	var aliases []string
	for _, name := range []string{"codex", "claude", "opencode", "agent", "cursor-agent"} {
		// Leave script-only installations on the shell's normal PATH. They
		// have no qualified direct-executable recovery contract.
		if _, err := recovery.OriginalCLI(name); err == nil {
			aliases = append(aliases, name)
		}
	}
	// Availability belongs to the cache identity too: a later shell without a
	// native CLI must not inherit an old alias that shadows its script shim.
	digest := sha256.New()
	_, _ = digest.Write(data)
	_, _ = digest.Write([]byte("\x00" + strings.Join(aliases, "\x00")))
	id := fmt.Sprintf("%x", digest.Sum(nil))[:16]
	dir := filepath.Join(cache, "webmux", "browser-open", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "webmux-open.exe")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.WriteFile(path, data, 0700)
		if err != nil {
			return "", err
		}
	}
	// Keep aliases beside the immutable opener image, rather than linking to
	// the installed executable that a later application update may replace.
	for _, name := range aliases {
		alias := filepath.Join(dir, name+".exe")
		if _, err := os.Stat(alias); err == nil {
			continue
		}
		if err := os.Link(path, alias); err != nil {
			if _, exists := os.Stat(alias); exists == nil {
				continue
			}
			if err := os.WriteFile(alias, data, 0700); err != nil {
				return "", err
			}
		}
	}
	return dir, nil
}

func Shell() error {
	env, err := ShellEnvironment()
	if err != nil {
		return err
	}
	return runShell(env, os.Stdin, os.Stdout, os.Stderr)
}

func runShell(env []string, input, output, diagnostic *os.File) error {
	// The waiter owns console input until a deliberate recovery choice. Do not
	// let cmd consume queued input or interpret a POSIX replay status as code.
	values := make(map[string]string)
	clean := make([]string, 0, len(env))
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		key = strings.ToUpper(key)
		values[key] = value
		if key != "WEBMUX_RECOVERY_WAIT" {
			clean = append(clean, item)
		}
	}
	if values["WEBMUX_RECOVERY_WAIT"] == "1" {
		helper := values["WEBMUX_RECOVERY_HELPER"]
		if !filepath.IsAbs(helper) {
			return errors.New("Windows recovery helper is unavailable; retained logs remain available")
		}
		wait := exec.Command(helper, "--recovery-wait")
		wait.Env = clean
		wait.Stdin, wait.Stdout, wait.Stderr = input, output, diagnostic
		if err := wait.Run(); err != nil {
			return fmt.Errorf("Windows recovery worker stopped; retained logs remain available: %w", err)
		}
	}
	if values["WEBMUX_RECOVERY_SESSION"] != "" && values["WEBMUX_RECOVERY_KEY"] != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		event := exec.Command(values["WEBMUX_RECOVERY_HELPER"], "--recovery-event", "shell", cwd)
		event.Env = clean
		event.Stdout, event.Stderr = output, diagnostic
		if err := event.Run(); err != nil {
			return fmt.Errorf("Windows shell recovery initialization failed: %w", err)
		}
	}
	shell := values["COMSPEC"]
	if shell == "" {
		shell = "cmd.exe"
	}
	cmd := exec.Command(shell)
	cmd.Env = clean
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, diagnostic
	return cmd.Run()
}

func Execute(path string, args, env []string) error {
	cmd := exec.Command(path, args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
