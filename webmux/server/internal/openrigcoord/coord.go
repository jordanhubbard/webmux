// Package openrigcoord connects WebMux's terminal grid to a single Rig daemon.
package openrigcoord

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed runner.mts
var runner []byte

// Serialize launches and spec creation across simultaneous browser requests.
var mutation sync.Mutex

func Run(ctx context.Context, home, owner, action, input string) ([]byte, error) {
	mutation.Lock()
	defer mutation.Unlock()
	scope := sha256.Sum256([]byte(owner))
	directory := filepath.Join(home, "data", "coordination", hex.EncodeToString(scope[:12]))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	script := filepath.Join(directory, "runner.mts")
	if err := os.WriteFile(script, runner, 0600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "node", script, action, directory)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return output, nil
}
