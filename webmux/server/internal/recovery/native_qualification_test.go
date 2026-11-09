package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This is an opt-in check against real conversations created by
// scripts/qualify-recovery-native.py, using the same host account/environment.
// It reads native metadata but submits no further model turns.
func TestQualifiedNativeConversationPreflight(t *testing.T) {
	manifest := os.Getenv("WEBMUX_QUALIFY_NATIVE_MANIFEST")
	if manifest == "" {
		t.Skip("set WEBMUX_QUALIFY_NATIVE_MANIFEST to an explicit native qualification manifest")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var results []struct {
		CLI       string `json:"cli"`
		SessionID string `json:"native_session_id"`
		Project   string `json:"project"`
		Evidence  string `json:"evidence"`
		Passed    bool   `json:"conversational_resume"`
	}
	if err := json.Unmarshal(data, &results); err != nil || len(results) == 0 {
		t.Fatalf("invalid qualification manifest: %v", err)
	}
	for _, result := range results {
		t.Run(result.CLI+"/"+result.SessionID, func(t *testing.T) {
			if !result.Passed {
				t.Fatal("native conversational qualification did not pass")
			}
			cwd := result.Project
			if cwd == "" {
				cwd = filepath.Join(result.Evidence, "project")
			}
			cwd, err := filepath.Abs(cwd)
			if err != nil {
				t.Fatal(err)
			}
			activity := Activity{CLI: result.CLI, Cwd: cwd, NativeSessionID: result.SessionID}
			args, err := prepareNativeResume(NativeResume{Activity: activity})
			if err != nil || len(args) == 0 {
				t.Fatalf("qualified native conversation rejected: %v", err)
			}
			activity.Cwd = t.TempDir()
			if _, err := prepareNativeResume(NativeResume{Activity: activity}); err == nil {
				t.Fatal("same native ID accepted in a different project")
			}
		})
	}
}
