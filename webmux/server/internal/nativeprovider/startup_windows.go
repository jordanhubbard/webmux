package nativeprovider

import (
	"errors"
	"fmt"
	"syscall"
)

// These exact labels, not arbitrary child error strings, may cross the private
// bootstrap pipe into controller diagnostics. Never include paths or payloads.
const (
	startupConfig       = "native provider startup failed at config"
	startupRoot         = "native provider startup failed at root"
	startupOwnership    = "native provider startup failed at ownership"
	startupSocket       = "native provider startup failed at socket"
	startupImage        = "native provider startup failed at image"
	startupScreen       = "native provider startup failed at screen"
	startupRecord       = "native provider startup failed at record"
	startupTerminal     = "native provider startup failed at terminal"
	startupOwnerProcess = "native provider startup failed at owner_process"
	startupOwnerReply   = "native provider startup failed at owner_reply"
	startupBootstrap    = "native provider startup failed at bootstrap_process"
	startupReply        = "native provider startup failed at bootstrap_reply"
)

func knownStartupFailure(value string) bool {
	switch value {
	case startupConfig, startupRoot, startupOwnership, startupSocket, startupImage,
		startupScreen, startupRecord, startupTerminal, startupOwnerProcess,
		startupOwnerReply, startupBootstrap, startupReply:
		return true
	}
	return false
}

func startupFailureFrame(phase string, err error) frame {
	if !knownStartupFailure(phase) {
		phase = startupConfig
	}
	var code syscall.Errno
	_ = errors.As(err, &code)
	return frame{Kind: "ready", Error: phase, Code: int(uint32(code))}
}

func startupReplyError(reply frame) error {
	if !knownStartupFailure(reply.Error) {
		return nil
	}
	if reply.Code != 0 && reply.Code > 0 && uint64(reply.Code) <= 1<<32-1 {
		return fmt.Errorf("%s (Windows error %d)", reply.Error, reply.Code)
	}
	return errors.New(reply.Error)
}
