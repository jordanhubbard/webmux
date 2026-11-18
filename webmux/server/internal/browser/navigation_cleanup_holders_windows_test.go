//go:build windows

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The fixed-layout Restart Manager records remain private. No application or
// service name, PID, timestamp or image path is serialized. Only the single
// refused owned file is registered; no shutdown/restart API is loaded or called.
type cleanupRMProcess struct {
	PID     uint32
	Started windows.Filetime
}
type cleanupRMInfo struct {
	Process         cleanupRMProcess
	Application     [256]uint16
	Service         [64]uint16
	ApplicationType uint32
	Status          uint32
	Session         uint32
	Restartable     int32
}

func queryFirstNavigationRemovalHolders(removal error, profile string, owned map[uint32]uint64) *firstNavigationRemovalHolders {
	var pathError *os.PathError
	var code syscall.Errno
	if !errors.As(removal, &pathError) || !errors.As(removal, &code) || (code != 32 && code != 33) {
		return nil
	}
	r := &firstNavigationRemovalHolders{State: "resource_unavailable"}
	rel, err := filepath.Rel(profile, pathError.Path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		r.State = "outside_file_scope"
		return r
	}
	info, err := os.Lstat(pathError.Path)
	if err != nil {
		return r
	}
	if !info.Mode().IsRegular() {
		r.State = "not_regular_file"
		return r
	}
	file, err := windows.UTF16PtrFromString(pathError.Path)
	if err != nil {
		return r
	}
	library := windows.NewLazySystemDLL("rstrtmgr.dll")
	start := library.NewProc("RmStartSession")
	register := library.NewProc("RmRegisterResources")
	list := library.NewProc("RmGetList")
	end := library.NewProc("RmEndSession")
	for _, proc := range []*windows.LazyProc{start, register, list, end} {
		if proc.Find() != nil {
			r.State = "api_unavailable"
			return r
		}
	}
	var session uint32
	var key [33]uint16
	result, _, _ := start.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&key[0])))
	if result != 0 {
		r.State = "start_failed"
		r.OSCode = uint32(result)
		return r
	}
	defer end.Call(uintptr(session))
	names := [1]*uint16{file}
	result, _, _ = register.Call(uintptr(session), 1, uintptr(unsafe.Pointer(&names[0])), 0, 0, 0, 0)
	if result != 0 {
		r.State = "register_failed"
		r.OSCode = uint32(result)
		return r
	}
	var rows [32]cleanupRMInfo
	needed, count, reasons := uint32(0), uint32(len(rows)), uint32(0)
	result, _, _ = list.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&rows[0])), uintptr(unsafe.Pointer(&reasons)))
	if result != 0 || count > uint32(len(rows)) {
		r.State = "list_failed"
		r.OSCode = uint32(result)
		if result == 234 || count > uint32(len(rows)) {
			r.State = "process_limit"
		}
		return r
	}
	r.State = "queried"
	r.Reported = int(count)
	fixture := os.Getenv("WEBMUX_BROWSER_EXECUTABLE")
	for _, row := range rows[:count] {
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, row.Process.PID)
		if err != nil {
			r.Unqueryable++
			continue
		}
		var created, exited, kernel, user windows.Filetime
		err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user)
		var image [1024]uint16
		size := uint32(len(image))
		if err == nil {
			err = windows.QueryFullProcessImageName(process, 0, &image[0], &size)
		}
		windows.CloseHandle(process)
		if err != nil || created != row.Process.Started {
			r.Unqueryable++
			continue
		}
		// This map was captured from the fixture browser CDP set before close,
		// after validating image and process creation ordering. Keep it private.
		if expected, ok := owned[row.Process.PID]; ok && expected == uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) {
			r.OwnedObservedProcesses++
		}
		if row.Process.PID == uint32(os.Getpid()) {
			r.CurrentProcess++
		} else if fixture != "" && strings.EqualFold(windows.UTF16ToString(image[:size]), fixture) {
			r.FixtureImage++
		} else {
			r.OtherImage++
		}
	}
	return r
}
