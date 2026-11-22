//go:build !windows

package recovery

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The caller has authenticated the owning session. Query only its canonical
// provider pane, without persisting directory history or running a shell.
func coordinationProviderDirectory(parent context.Context, session string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	// The trailing colon selects the exact session's current window/pane.
	// Without it tmux can return an empty pane format with exit status zero.
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", "=webmux-"+session+":", "#{pane_current_path}")
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(out, 4097))
	_ = out.Close()
	waitErr := cmd.Wait()
	directory := strings.TrimSuffix(string(data), "\n")
	if readErr != nil || waitErr != nil || len(data) > 4096 || !filepath.IsAbs(directory) || !utf8.ValidString(directory) || strings.ContainsFunc(directory, unicode.IsControl) {
		return "", errors.New("live provider directory is unavailable")
	}
	return directory, nil
}
