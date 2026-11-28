//go:build !windows

package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func processID() int { return os.Getpid() }

func currentProcessStarted() string { return processStartTicks(os.Getpid()) }

func currentBootID() string {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func observeProcess(pid int) ProcessObservation {
	observed := ProcessObservation{BootID: currentBootID()}
	if pid <= 0 {
		return observed
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return observed
	}
	observed.Alive = true
	observed.StartedAt = processStartTicks(pid)
	return observed
}

// processStartTicks reads field 22 of /proc/pid/stat. The command name is
// truncated at the last closing parenthesis so a name containing spaces cannot
// shift the start-time field.
func processStartTicks(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return ""
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
