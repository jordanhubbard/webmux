package nativeprovider

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These exact labels, not arbitrary child error strings, may cross the private
// bootstrap pipe into controller diagnostics. Never include paths or payloads.
const (
	startupConfig              = "native provider startup failed at config"
	startupRoot                = "native provider startup failed at root"
	startupOwnership           = "native provider startup failed at ownership"
	startupSocket              = "native provider startup failed at socket"
	startupImage               = "native provider startup failed at image"
	startupScreen              = "native provider startup failed at screen"
	startupRecord              = "native provider startup failed at record"
	startupTerminal            = "native provider startup failed at terminal"
	startupOwnerProcess        = "native provider startup failed at owner_process"
	startupOwnerNoJob          = "native provider startup failed at owner_process; bootstrap outside jobs"
	startupOwnerJobNoBreakaway = "native provider startup failed at owner_process; bootstrap immediate job disallows breakaway"
	startupOwnerJobBreakaway   = "native provider startup failed at owner_process; bootstrap immediate job allows breakaway"
	startupOwnerJobUnknown     = "native provider startup failed at owner_process; bootstrap in job with unreadable limits"
	startupOwnerReply          = "native provider startup failed at owner_reply"
	startupBootstrap           = "native provider startup failed at bootstrap_process"
	startupReply               = "native provider startup failed at bootstrap_reply"
)

func knownStartupFailure(value string) bool {
	switch value {
	case startupConfig, startupRoot, startupOwnership, startupSocket, startupImage,
		startupScreen, startupRecord, startupTerminal, startupOwnerProcess,
		startupOwnerReply, startupBootstrap, startupReply, startupOwnerNoJob,
		startupOwnerJobNoBreakaway, startupOwnerJobBreakaway, startupOwnerJobUnknown:
		return true
	}
	return false
}

// Observe only the current bootstrap's job membership and immediate job flags.
// Nested parent jobs can impose further restrictions; these labels describe
// observations, not the cause of CreateProcess failure. Never change job limits.
func ownerProcessFailurePhase() string {
	var inJob int32
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	ok, _, _ := proc.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return startupOwnerProcess
	}
	if inJob == 0 {
		return startupOwnerNoJob
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	return ownerJobFailurePhase(err == nil, limits.BasicLimitInformation.LimitFlags)
}

func ownerJobFailurePhase(readable bool, flags uint32) string {
	if !readable {
		return startupOwnerJobUnknown
	}
	if flags&(windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK|windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK) != 0 {
		return startupOwnerJobBreakaway
	}
	return startupOwnerJobNoBreakaway
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
