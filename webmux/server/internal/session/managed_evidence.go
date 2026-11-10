package session

import (
	"path/filepath"
	"regexp"
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
func clearRigRecoveryReference(value *RigIdentity) {
	value.ManagerRigID, value.ManagerNodeID, value.ManagerLogicalID = "", "", ""
	value.ManagerSnapshotID, value.ManagerSnapshotHash, value.ManagerPolicyHash, value.ManagerRecordIdentity = "", "", "", ""
}

func copyRigRecoveryReference(value *RigIdentity, from RigIdentity) {
	value.ManagerRigID, value.ManagerNodeID, value.ManagerLogicalID = from.ManagerRigID, from.ManagerNodeID, from.ManagerLogicalID
	value.ManagerSnapshotID, value.ManagerSnapshotHash, value.ManagerPolicyHash, value.ManagerRecordIdentity = from.ManagerSnapshotID, from.ManagerSnapshotHash, from.ManagerPolicyHash, from.ManagerRecordIdentity
}

var rigRecoveryULID = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
var rigRecoveryHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validRigRecoveryReference(value RigIdentity) bool {
	return (value.Provider == "" || value.Provider == "rig") && value.Host == "localhost" && value.Runtime == "claude-code" &&
		value.NativeSource == "rig-snapshot" && filepath.IsAbs(value.Workspace) && filepath.IsAbs(value.NativeStateFile) && recovery.ValidNativeID("claude", value.NativeSessionID) && rigRecoveryULID.MatchString(value.ManagerRigID) && rigRecoveryULID.MatchString(value.ManagerNodeID) &&
		rigRecoveryULID.MatchString(value.ManagerSnapshotID) && value.ManagerLogicalID != "" && len(value.ManagerLogicalID) <= 256 && !strings.ContainsFunc(value.ManagerLogicalID, unicode.IsControl) &&
		rigRecoveryHash.MatchString(value.ManagerSnapshotHash) && rigRecoveryHash.MatchString(value.ManagerPolicyHash) && rigRecoveryHash.MatchString(value.ManagerRecordIdentity)
}

func (b *Broker) managedEvidenceLocked(e *entry, value RigIdentity) RigIdentity {
	id, file := value.NativeSessionID, value.NativeStateFile
	candidate := value
	clearRigRecoveryReference(&value)
	value.NativeSessionID, value.NativeStateFile, value.NativeSource, value.NativeObservedAt = "", "", "", ""
	old := e.value.Rig
	if old != nil && sameManagedDestination(*old, value) {
		value.NativeSessionID, value.NativeStateFile = old.NativeSessionID, old.NativeStateFile
		value.NativeSource, value.NativeObservedAt = old.NativeSource, old.NativeObservedAt
		copyRigRecoveryReference(&value, *old)
		if validRigRecoveryReference(*old) && value.ManagerProfile == "" {
			value.ManagerProfile = old.ManagerProfile
		}
	}
	if e.run == nil || !e.run.log.active() || value.Provider != "agent-deck" && !validRigRecoveryReference(candidate) {
		return value
	}
	if !recovery.ValidNativeID(managedCLI(value.Runtime), id) || len(file) > 4096 || strings.ContainsFunc(file, unicode.IsControl) {
		return value
	}
	unchanged := value.NativeSessionID == id && value.NativeStateFile == file && value.NativeSource != "" && value.ManagerSnapshotHash == candidate.ManagerSnapshotHash && value.ManagerPolicyHash == candidate.ManagerPolicyHash && value.ManagerRecordIdentity == candidate.ManagerRecordIdentity
	if validRigRecoveryReference(candidate) {
		copyRigRecoveryReference(&value, candidate)
	}
	if unchanged {
		return value
	}
	value.NativeSessionID, value.NativeStateFile = id, file
	value.NativeSource, value.NativeObservedAt = "agent-deck-inventory", now()
	if validRigRecoveryReference(candidate) {
		value.NativeSource = "rig-snapshot"
	}
	return value
}
