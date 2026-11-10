package session

import (
	"encoding/json"
	"io"
	"strings"
	"unicode"
)

// Rig process records contain bounded identity evidence, never argv, executable
// contents, settings contents or credentials. Older checkpoint-only references
// remain readable but do not establish original process ownership.
type rigProcessReference struct {
	PanePID              int    `json:"pane_pid"`
	NativePID            int    `json:"native_pid"`
	StartedAt            string `json:"started_at"`
	ExecutionUID         *int   `json:"execution_uid"`
	NativeSessionID      string `json:"native_session_id"`
	NativeIdentitySource string `json:"native_identity_source"`
	PermissionMode       string `json:"permission_mode"`
	ExecutableHash       string `json:"executable_hash"`
	SettingsHash         string `json:"settings_hash"`
	BootID               string `json:"boot_id"`
}

func validRigProcessReference(value, nativeID string) bool {
	if value == "" {
		return true
	}
	if len(value) > 4096 {
		return false
	}
	var record rigProcessReference
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF {
		return false
	}
	return record.PanePID > 0 && int64(record.PanePID) < 1<<31 && record.NativePID > 0 && int64(record.NativePID) < 1<<31 && record.ExecutionUID != nil && *record.ExecutionUID >= 0 && record.NativeSessionID == nativeID &&
		(record.NativeIdentitySource == "process-argv" || record.NativeIdentitySource == "manager-hook") &&
		(record.PermissionMode == "plan" || record.PermissionMode == "acceptEdits") &&
		strings.HasSuffix(record.StartedAt, " UTC") && len(record.StartedAt) <= 128 && !strings.ContainsFunc(record.StartedAt, unicode.IsControl) &&
		rigRecoveryHash.MatchString(record.ExecutableHash) && rigRecoveryHash.MatchString(record.SettingsHash) && rigRecoveryHash.MatchString(record.BootID)
}
