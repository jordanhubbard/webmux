package nativeprovider

import (
	"errors"
	"fmt"
	"syscall"
)

// Startup diagnostics cross a private child pipe into a browser-visible error.
// Only fixed phase and OS-code names may cross that boundary, never child error
// strings, argv, environment, recovery keys or filesystem paths.
func startupCode(err error) uint32 {
	var code syscall.Errno
	if errors.As(err, &code) {
		return uint32(code)
	}
	return 0
}

func startupDiagnostic(phase string, code, exit uint32) string {
	switch phase {
	case "bootstrap-config", "bootstrap-start", "bootstrap-pipe", "bootstrap-owner-start", "bootstrap-owner-reply", "bootstrap-owner-wait", "bootstrap-owner-exit",
		"bootstrap-owner-start-outside-job", "bootstrap-owner-start-immediate-job-unreadable", "bootstrap-owner-start-immediate-job-breakaway", "bootstrap-owner-start-immediate-job-no-breakaway",
		"bootstrap-reply", "startup-probe", "record-lookup", "record-probe", "owner-config", "owner-root", "owner-lock", "owner-temp",
		"owner-secure-temp", "owner-listen", "owner-identity", "owner-image", "owner-screen", "owner-record", "owner-terminal":
	default:
		phase = "unavailable"
	}
	// The owner-exit ready frame carries a process status, not a Win32 error.
	// In particular, process exit 2 must not become ERROR_FILE_NOT_FOUND.
	if phase == "bootstrap-owner-exit" {
		exit, code = code, 0
	}
	names := map[uint32]string{
		2: "file_not_found", 3: "path_not_found", 5: "access_denied", 8: "not_enough_memory", 14: "out_of_memory",
		32: "sharing_violation", 33: "lock_violation", 50: "not_supported", 87: "invalid_parameter", 109: "broken_pipe",
		193: "bad_executable_format", 225: "virus_infected", 226: "virus_deleted", 740: "elevation_required", 1314: "privilege_not_held",
		10013: "socket_access_denied", 10022: "socket_invalid_parameter", 10043: "protocol_not_supported",
		10047: "address_family_not_supported", 10048: "address_in_use", 10049: "address_not_available",
		10050: "network_down", 10054: "connection_reset", 10060: "connection_timed_out", 10061: "connection_refused",
		0x80070005: "hresult_access_denied", 0x80070057: "hresult_invalid_parameter",
		0xc0000005: "access_violation", 0xc000001d: "illegal_instruction", 0xc00000fd: "stack_overflow",
		0xc0000142: "dll_initialization_failed", 0xc0000409: "fail_fast",
	}
	codeName := names[code]
	if codeName == "" {
		codeName = "unavailable"
	}
	exitName := "unavailable"
	switch exit {
	case 0:
		exitName = "none"
		if phase == "bootstrap-owner-exit" {
			exitName = "exit_code_0"
		}
	case 1:
		exitName = "exit_failure"
	case 2:
		exitName = "exit_code_2"
	case 0xc0000005:
		exitName = "access_violation"
	case 0xc000001d:
		exitName = "illegal_instruction"
	case 0xc00000fd:
		exitName = "stack_overflow"
	case 0xc0000142:
		exitName = "dll_initialization_failed"
	case 0xc0000409:
		exitName = "fail_fast"
	}
	return fmt.Sprintf("phase=%s os=%s exit=%s", phase, codeName, exitName)
}
