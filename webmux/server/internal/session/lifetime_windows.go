//go:build windows

package session

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func processID() int { return os.Getpid() }

func currentProcessStarted() string { return observeProcess(os.Getpid()).StartedAt }

func currentBootID() string { return "" }

func observeProcess(pid int) ProcessObservation {
	observed := ProcessObservation{BootID: currentBootID()}
	if pid <= 0 {
		return observed
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied still proves a process exists. Treat that as alive so
		// a second server does not resume over work it cannot inspect.
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			observed.Alive = true
		}
		return observed
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err == nil {
		observed.StartedAt = fmt.Sprintf("%d.%d", creation.HighDateTime, creation.LowDateTime)
	}
	observed.Alive = true
	return observed
}
