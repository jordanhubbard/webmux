package terminal

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// A private remote stream-local socket avoids TCP exposure even when the SSH
// server's GatewayPorts policy forces wildcard reverse TCP listeners. The only
// TCP destination is the source controller's existing loopback agent listener.
func coordinationForwardArgs(request LaunchRequest, controlPath, endpoint, socket, operation string) ([]string, error) {
	if controlPath == "" || (operation != "forward" && operation != "cancel") || !strings.HasPrefix(socket, "/") || len(socket) > 100 || strings.ContainsAny(socket, ":\x00\r\n") {
		return nil, errors.New("private SSH coordination forwarding is unavailable")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("coordination forwarding requires the private source endpoint")
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		return nil, errors.New("invalid source coordination endpoint")
	}
	if port, err := strconv.Atoi(u.Port()); err != nil || port < 1 || port > 65535 || u.RawPath != "" {
		return nil, errors.New("invalid source coordination endpoint port")
	}
	args, err := recoverySSHArgs(request, controlPath)
	if err != nil {
		return nil, err
	}
	args = args[:len(args)-3] // No remote shell command is part of -O forwarding.
	return append(args, "-O", operation, "-R", socket+":"+u.Host, "--", request.Hostname), nil
}

func prepareRemoteCoordination(value *browserConnect, rpcArgs []string) (bool, error) {
	if os.Getenv("WEBMUX_COORDINATION_FILE") == "" {
		return false, nil
	}
	if value.ControlPath == "" {
		return false, errors.New("this execution host has no private SSH control connection")
	}
	access, err := coordinationclient.Load()
	if err != nil {
		return false, errors.New("source session capability is unavailable")
	}
	if access.Subject.Session != value.Request.SessionID || access.Socket != "" {
		return false, errors.New("source session capability identity does not match")
	}
	channel, err := recovery.NewInstance()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request := recovery.RPCRequest{Protocol: 1, SessionID: value.Request.SessionID, Key: os.Getenv("WEBMUX_RECOVERY_KEY"), Operation: "coordination-prepare", CoordinationChannel: channel}
	response := remoteRecoveryRPC(ctx, rpcArgs, request)
	if response.Error != "" || response.CoordinationSocket == "" {
		return false, errors.New("execution-host helper does not support private coordination bootstrap")
	}
	args, err := coordinationForwardArgs(value.Request, value.ControlPath, access.Endpoint, response.CoordinationSocket, "forward")
	if err != nil {
		return false, err
	}
	if err := exec.CommandContext(ctx, "ssh", args...).Run(); err != nil {
		return false, errors.New("SSH stream-local forwarding was refused")
	}
	request.Operation, request.CoordinationAccess = "coordination-install", &access
	installed := remoteRecoveryRPC(ctx, rpcArgs, request)
	if installed.Error == "" && installed.Outcome == "coordination-installed" {
		return true, nil
	}
	// The installation reply can be lost after atomic publication. Retain this
	// private forward until its owning run closes, so an uncertain reply cannot
	// strand an already installed locator. A refused probe never replaced the
	// previous locator; an unreferenced forward grants no capability by itself.
	return false, errors.New("forwarded session identity or coordination access could not be verified")
}

func closeCoordinationMaster(request LaunchRequest, controlPath string) {
	if controlPath == "" {
		return
	}
	args, err := recoverySSHArgs(request, controlPath)
	if err != nil {
		return
	}
	args = append(args[:len(args)-3], "-O", "exit", "--", request.Hostname)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// -O cannot create a connection or fall back to another authentication path.
	// The per-backend/session control path cannot address an unrelated master.
	_ = exec.CommandContext(ctx, "ssh", args...).Run()
}
