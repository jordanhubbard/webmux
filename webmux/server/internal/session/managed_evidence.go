package session

import (
	"strings"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

func managedCLI(runtime string) string {
	switch runtime {
	case "claude", "claude-code":
		return "claude"
	case "codex", "opencode", "cursor":
		return runtime
	}
	return ""
}

func sameManagedDestination(a, b RigIdentity) bool {
	profile := func(value string) string {
		if value == "" {
			return "default"
		}
		return value
	}
	return a.Provider == b.Provider && a.Target == b.Target && a.Host == b.Host && a.Workspace == b.Workspace && a.Runtime == b.Runtime && profile(a.ManagerProfile) == profile(b.ManagerProfile)
}

// The coordinator owns these references. They are evidence, never executable
// commands, and do not establish which conversation is active after a switch.
func (b *Broker) managedEvidenceLocked(e *entry, value RigIdentity) RigIdentity {
	id, file := value.NativeSessionID, value.NativeStateFile
	value.NativeSessionID, value.NativeStateFile, value.NativeSource, value.NativeObservedAt = "", "", "", ""
	old := e.value.Rig
	if old != nil && sameManagedDestination(*old, value) {
		value.NativeSessionID, value.NativeStateFile = old.NativeSessionID, old.NativeStateFile
		value.NativeSource, value.NativeObservedAt = old.NativeSource, old.NativeObservedAt
	}
	if e.run == nil || !e.run.log.active() || value.Provider != "agent-deck" {
		return value
	}
	if !recovery.ValidNativeID(managedCLI(value.Runtime), id) || len(file) > 4096 || strings.ContainsFunc(file, unicode.IsControl) {
		return value
	}
	if value.NativeSessionID == id && value.NativeStateFile == file && value.NativeSource != "" {
		return value
	}
	value.NativeSessionID, value.NativeStateFile = id, file
	value.NativeSource, value.NativeObservedAt = "agent-deck-inventory", now()
	return value
}
