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

//go:embed rig-recovery.mts
var rigRecovery []byte

//go:embed rig-enrollment.mts
var rigEnrollment []byte

//go:embed rig-attachment.mts
var rigAttachment []byte

//go:embed rig-stop.mts
var rigStop []byte

//go:embed codex-owner-plan.mts
var codexOwnerPlan []byte

//go:embed codex-launch-policy.mts
var codexLaunchPolicy []byte

//go:embed codex-launch-wrapper.mts
var codexLaunchWrapper []byte

//go:embed rig-create-intent.mts
var rigCreateIntent []byte

//go:embed rig-native-create.mts
var rigNativeCreate []byte

// Serialize launches and spec creation across simultaneous browser requests.
var mutation sync.Mutex

// ControlEnvironment follows the configured Rig endpoint instead of an
// unrelated host-selection preference, without changing the caller's routing.
func ControlEnvironment() []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited)+1)
	for _, entry := range inherited {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "OPENRIG_HOST_SELECTED") {
			env = append(env, entry)
		}
	}
	return append(env, "OPENRIG_HOST_SELECTED=local")
}

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
	if err := os.WriteFile(filepath.Join(directory, "rig-recovery.mts"), rigRecovery, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "rig-enrollment.mts"), rigEnrollment, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "rig-attachment.mts"), rigAttachment, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "rig-stop.mts"), rigStop, 0600); err != nil {
		return nil, err
	}
	for name, source := range map[string][]byte{
		"codex-owner-plan.mts": codexOwnerPlan, "codex-launch-policy.mts": codexLaunchPolicy,
		"codex-launch-wrapper.mts": codexLaunchWrapper, "rig-create-intent.mts": rigCreateIntent,
		"rig-native-create.mts": rigNativeCreate,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), source, 0600); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, "node", script, action, directory)
	if action == "native-create" {
		configureNativeCreateCancellation(cmd)
	}
	cmd.Env = ControlEnvironment()
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
