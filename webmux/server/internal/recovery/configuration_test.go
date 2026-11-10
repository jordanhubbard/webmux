package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func environmentMap(environment []string) map[string]string {
	result := map[string]string{}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		result[key] = value
	}
	return result
}

func TestNativeRootsCaptureAbsenceRelativePathsAndNoCredentials(t *testing.T) {
	t.Setenv("CODEX_HOME", "relative-state")
	t.Setenv("UNRELATED_API_KEY", "synthetic-value")
	c := captureNativeConfiguration("codex")
	if c.Roots["CODEX_HOME"] != "relative-state" {
		t.Fatal("changed the native root's literal identity")
	}
	if _, exists := c.Roots["UNRELATED_API_KEY"]; exists {
		t.Fatal("captured arbitrary environment")
	}
	environment, err := nativeEnvironment("codex", c, []string{"CODEX_HOME=/different", "PATH=/native", "HOME=/different-home"})
	if err != nil || environmentMap(environment)["CODEX_HOME"] != "relative-state" {
		t.Fatalf("original root not restored: %v", err)
	}
	unset := &NativeConfiguration{Roots: map[string]string{}}
	environment, err = nativeEnvironment("cursor", unset, []string{"CURSOR_CONFIG_DIR=/another-terminal", "XDG_CONFIG_HOME=/different", "PATH=/native"})
	if err != nil || !reflect.DeepEqual(environmentMap(environment), map[string]string{"PATH": "/native"}) {
		t.Fatalf("inherited another terminal's roots: %v %v", environment, err)
	}
}

func TestNativeConfigurationRejectsEnvironmentInjectionAndPreservesClone(t *testing.T) {
	for _, roots := range []map[string]string{
		{"PATH": "/replacement"}, {"LD_PRELOAD": "/replacement"}, {"CODEX_HOME": "/root\ncommand"},
	} {
		if (&NativeConfiguration{Roots: roots}).Validate("codex") == nil {
			t.Fatalf("accepted invalid roots: %v", roots)
		}
	}
	for _, root := range []string{"/remote/linux", `C:\Users\remote\state`, `\\server\share\state`} {
		if (&NativeConfiguration{Roots: map[string]string{"CODEX_HOME": root}}).Validate("codex") != nil {
			t.Fatal("controller rejected execution host's absolute path syntax")
		}
	}
	state := &State{Activity: &Activity{Configuration: &NativeConfiguration{Roots: map[string]string{"CODEX_HOME": "/original"}}}}
	clone := state.Clone()
	clone.Activity.Configuration.Roots["CODEX_HOME"] = "/changed"
	if state.Activity.Configuration.Roots["CODEX_HOME"] != "/original" {
		t.Fatal("shared mutable configuration between owner snapshots")
	}
}

func TestInlineOpenCodePolicyRequiresManualRestoreWithoutRecordingContent(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"permission":{"edit":"deny"}}`)
	c := captureNativeConfiguration("opencode")
	if c.ManualReason == "" {
		t.Fatal("inline settings would be silently discarded")
	}
	encoded, _ := json.Marshal(c)
	if strings.Contains(string(encoded), "edit") || strings.Contains(string(encoded), "deny") {
		t.Fatal("recorded inline configuration content")
	}
	if _, err := nativeEnvironment("opencode", c, nil); err == nil {
		t.Fatal("continued without original inline policy")
	}
	for _, command := range []string{`OPENCODE_CONFIG_CONTENT='{"permission":{"edit":"deny"}}' opencode`, `claude --settings '{"env":{"SETTING":"value"}}'`} {
		if safe, replayable := SafeCommand(command); replayable || safe == command {
			t.Fatal("stored opaque inline configuration in recovery command metadata")
		}
	}
	if args := safeArgv([]string{"claude", "--settings", `{"env":{"SETTING":"value"}}`}); len(args) != 2 || args[1] != "[credential arguments omitted]" {
		t.Fatal("stored opaque inline configuration in recovery argv")
	}
}

// A real subprocess checks the isolation boundary without replacing an installed
// CLI or changing the parent's environment during concurrent recovery requests.
func TestNativePreflightConfigurationSubprocess(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_CONFIGURATION_CHILD") == "1" {
		var request NativeResume
		if json.NewDecoder(os.Stdin).Decode(&request) != nil {
			os.Exit(2)
		}
		root := os.Getenv("CODEX_HOME")
		if _, err := os.Stat(filepath.Join(root, "native-fixture")); err != nil {
			os.Exit(3)
		}
		_ = json.NewEncoder(os.Stdout).Encode(nativePreflightResult{Argv: []string{"codex", "resume", root}})
		os.Exit(0)
	}
	original, selected := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", original)
	t.Setenv("WEBMUX_TEST_CONFIGURATION_CHILD", "1")
	if err := os.WriteFile(filepath.Join(selected, "native-fixture"), []byte("native state"), 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args, err := preflightWithCommand(NativeResume{Activity: Activity{CLI: "codex", Configuration: &NativeConfiguration{Roots: map[string]string{"CODEX_HOME": selected}}}}, []string{self, "-test.run=^TestNativePreflightConfigurationSubprocess$"})
	if err != nil || len(args) != 3 || args[2] != selected {
		t.Fatalf("child used another native root: %v %v", args, err)
	}
	if os.Getenv("CODEX_HOME") != original {
		t.Fatal("preflight changed parent's configuration")
	}
	relative := filepath.Join(selected, "relative-state")
	if err := os.Mkdir(relative, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relative, "native-fixture"), []byte("native state"), 0600); err != nil {
		t.Fatal(err)
	}
	args, err = preflightWithCommand(NativeResume{Activity: Activity{CLI: "codex", Cwd: selected, Configuration: &NativeConfiguration{Roots: map[string]string{"CODEX_HOME": "relative-state"}}}}, []string{self, "-test.run=^TestNativePreflightConfigurationSubprocess$"})
	if err != nil || len(args) != 3 || args[2] != "relative-state" {
		t.Fatalf("relative native root lost its original cwd or identity: %v %v", args, err)
	}
}
