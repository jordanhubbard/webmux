package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/browseropen"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// Recovery uses the execution host's account. Mosh has its own terminal screen
// protocol, so metadata/control uses this separate authenticated SSH pipe.
// Passwords are not persisted or silently replayed for background requests.
func (l Launcher) Recovery(ctx context.Context, launch LaunchRequest, local bool, request recovery.RPCRequest) recovery.RPCResponse {
	request.Protocol, request.SessionID, request.Key = 1, launch.SessionID, launch.RecoveryKey
	if local {
		return recovery.HostRPC(ctx, request)
	}
	response := recovery.RPCResponse{Protocol: 1}
	args, err := recoverySSHArgs(launch, SSHControlPath(l.ControlDir, launch.SessionID))
	if err != nil {
		response.Error = err.Error()
		return response
	}
	if launch.KeyID != "" {
		var document struct {
			Keys []struct {
				ID   string `yaml:"id"`
				Path string `yaml:"private_key_path"`
			} `yaml:"keys"`
		}
		if err := l.Store.ReadConfig("keys.yaml", &document); err != nil {
			response.Error = "Cannot load the execution host's SSH key configuration"
			return response
		}
		path := ""
		for _, key := range document.Keys {
			if key.ID == launch.KeyID {
				path = key.Path
				break
			}
		}
		if path == "" {
			response.Error = "The execution host's configured SSH key is unavailable"
			return response
		}
		args = append([]string{"-i", path}, args...)
	}
	return remoteRecoveryRPC(ctx, args, request)
}

func remoteRecoveryRPC(ctx context.Context, args []string, request recovery.RPCRequest) recovery.RPCResponse {
	response := recovery.RPCResponse{Protocol: 1}
	data, err := json.Marshal(request)
	if err != nil {
		response.Error = "Cannot encode the recovery control request"
		return response
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ssh", args...)
	command.Stdin = bytes.NewReader(data)
	output, err := command.StdoutPipe()
	if err == nil {
		err = command.Start()
	}
	if err != nil {
		response.Error = "Cannot open the execution host's authenticated recovery channel"
		return response
	}
	// Bound output before JSON decoding, and close the pipe before Wait on an
	// invalid/oversized response so an untrusted host cannot fill memory or hang.
	data, readErr := io.ReadAll(io.LimitReader(output, (256<<10)+1))
	_ = output.Close()
	waitErr := command.Wait()
	if readErr != nil || waitErr != nil || len(data) > 256<<10 || json.Unmarshal(data, &response) != nil || response.Protocol != 1 {
		return recovery.RPCResponse{Protocol: 1, Error: "The execution host's recovery helper is unavailable or could not authenticate. Reconnect or recover manually; retained logs remain available."}
	}
	return response
}

func recoveryLaunchEvidence() (*recovery.Activity, error) {
	encoded := os.Getenv("WEBMUX_RECOVERY_SEED")
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 64<<10 {
		return nil, errors.New("recovery launch evidence exceeds its limit")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid recovery launch evidence")
	}
	var activity recovery.Activity
	if json.Unmarshal(data, &activity) != nil {
		return nil, errors.New("invalid recovery launch evidence")
	}
	return &activity, nil
}

func prepareRemoteRecovery(value *browserConnect) bool {
	key := os.Getenv("WEBMUX_RECOVERY_KEY")
	if key == "" {
		return false
	}
	args, err := recoverySSHArgs(value.Request, value.ControlPath)
	if err != nil {
		return false
	}
	if value.KeyPath != "" {
		args = append([]string{"-i", value.KeyPath}, args...)
	}
	probeArgs := append([]string(nil), args...)
	probeArgs[len(probeArgs)-1] = "webmux --recovery-capabilities"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	output, err := exec.CommandContext(ctx, "ssh", probeArgs...).Output()
	cancel()
	if err != nil || strings.TrimSpace(string(output)) != "1" {
		return false
	}
	seed, err := recoveryLaunchEvidence()
	if err != nil {
		return false
	}
	recording := false
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	response := remoteRecoveryRPC(ctx, args, recovery.RPCRequest{Protocol: 1, SessionID: value.Request.SessionID, Key: key, Operation: "ensure", Recording: &recording, Seed: seed})
	cancel()
	if response.Error != "" {
		fmt.Fprintln(os.Stderr, "[WebMux] Work recovery is unavailable on this host:", response.Error)
		return false
	}
	for _, event := range response.Events {
		if event.Kind == "provider" && recovery.Verify(event, key) {
			if packet, err := recovery.SignedPacket(event, key); err == nil {
				fmt.Fprint(os.Stdout, packet)
			}
		}
	}
	if value.Request.Transport == "mosh" {
		value.Args = append(value.Args, "webmux", "--session-attach", value.Request.SessionID)
	} else {
		value.Args = append(value.Args, "webmux --session-attach "+shellQuote(value.Request.SessionID))
	}
	return true
}

// SessionShell creates or reattaches a stable provider, then replaces itself
// with the client. The provider's scoped shell owns recovery.
func SessionShell() error {
	if os.Getenv("WEBMUX_RECOVERY_SEED_UNAVAILABLE") == "1" {
		fmt.Fprintln(os.Stderr, "[WebMux] Saved recovery evidence is too large to send. Retained logs remain available in Recover work.")
	}
	id, key := os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY")
	seed, err := recoveryLaunchEvidence()
	if err != nil {
		return err
	}
	if err := recovery.InitializeHostEvidence(id, key, false, seed); err != nil {
		return err
	}
	provider, err := recovery.EnsureProvider(context.Background(), id, key)
	if err != nil {
		return err
	}
	if packet, err := recovery.SignedPacket(provider, key); err == nil {
		fmt.Fprint(os.Stdout, packet)
	}
	return AttachSession(id)
}

func AttachSession(id string) error {
	if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(id) {
		return errors.New("invalid provider session identity")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return errors.New("tmux is unavailable on the execution host")
	}
	var env []string
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if name != "TMUX" && name != "TMUX_PANE" {
			env = append(env, item)
		}
	}
	target := "webmux-" + id
	args := []string{tmux, "attach-session", "-t", target, ";", "set-option", "-t", target, "mouse", "on"}
	return browseropen.Execute(tmux, args, env)
}

func recoverySSHArgs(request LaunchRequest, controlPath string) ([]string, error) {
	if request.Transport != "ssh" && request.Transport != "mosh" && request.Transport != "" {
		return nil, errors.New("this transport's lifecycle provider owns recovery")
	}
	if !hostname.MatchString(request.Hostname) || request.Username != "" && !username.MatchString(request.Username) {
		return nil, errors.New("invalid execution-host identity")
	}
	port := request.Port
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("invalid execution-host SSH port")
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "-p", strconv.Itoa(port)}
	if controlPath != "" {
		args = append(args, "-o", "ControlPath="+controlPath)
	}
	if request.Username != "" {
		args = append(args, "-l", request.Username)
	}
	return append(args, "--", request.Hostname, "webmux --recovery-rpc"), nil
}
