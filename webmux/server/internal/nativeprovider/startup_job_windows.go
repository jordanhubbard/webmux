package nativeprovider

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// These observations describe only the failing bootstrap's current membership
// and immediate job. Nested parent jobs may impose additional restrictions.
// Querying them neither changes limits nor establishes the cause of failure.
func ownerProcessFailurePhase() string {
	var inJob int32
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	ok, _, _ := proc.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return "bootstrap-owner-start"
	}
	if inJob == 0 {
		return "bootstrap-owner-start-outside-job"
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	return ownerJobFailurePhase(err == nil, limits.BasicLimitInformation.LimitFlags)
}

func ownerJobFailurePhase(readable bool, flags uint32) string {
	if !readable {
		return "bootstrap-owner-start-immediate-job-unreadable"
	}
	if flags&(windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK|windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK) != 0 {
		return "bootstrap-owner-start-immediate-job-breakaway"
	}
	return "bootstrap-owner-start-immediate-job-no-breakaway"
}
