package recovery

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// The versioned records arrive on a private inherited descriptor, never the PTY.
// PID and sequence bind the stream to this exact launched viewer. Native state
// files separately verify the account and invocation directory before promotion.
type codexSelectionObservation struct {
	Schema   string  `json:"schema"`
	PID      int     `json:"pid"`
	Sequence uint64  `json:"sequence"`
	ThreadID *string `json:"thread_id"`
}

func (o codexSelectionObservation) valid(pid int, previous uint64) bool {
	return o.Schema == "codex.tui.selection.v1" && o.PID == pid && o.Sequence > previous &&
		(o.ThreadID == nil || ValidNativeID("codex", *o.ThreadID))
}

type codexSelectionObserver struct {
	attach       func(*exec.Cmd)
	start        func(int)
	close        func()
	observations <-chan codexSelectionObservation
}

func codexSelectionEligible(binary, cwd string, args []string) bool {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		if name == "--selection-observer-fd" {
			// An explicitly supplied observation channel belongs to its caller.
			return false
		}
	}
	help, err := cliHelpAt(cwd, binary, "--help")
	if err != nil {
		return false
	}
	for _, field := range strings.Fields(help) {
		if field == "--selection-observer-fd" {
			return true
		}
	}
	return false
}

func observeCodexSelection(event *Event, observation codexSelectionObservation, originalCwd string) {
	// Any unsupported, absent, cross-host, or no-longer-settled selection clears
	// current confidence. Keep the previous ID solely as a manual recovery choice.
	event.NativeEvidence = "unavailable"
	if observation.ThreadID == nil || !ValidNativeID("codex", *observation.ThreadID) {
		return
	}
	id := *observation.ThreadID
	file := codexStateFile(id, originalCwd, "")
	if file == "" {
		return
	}
	event.SessionID, event.StateFile, event.NativeEvidence = id, file, "native-selection"
}

func decodeCodexSelection(data []byte, pid int, previous uint64) (codexSelectionObservation, bool) {
	var observation codexSelectionObservation
	if len(data) > 512 || json.Unmarshal(data, &observation) != nil || !observation.valid(pid, previous) {
		return codexSelectionObservation{}, false
	}
	return observation, true
}
