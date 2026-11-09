package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"time"

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
