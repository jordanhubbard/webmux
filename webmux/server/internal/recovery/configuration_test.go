package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func environmentMap(environment []string) map[string]string {
	result := map[string]string{}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		result[key] = value
	}
	return result
}

func TestNativeConfigurationCapturesUserProfile(t *testing.T) {
	original := t.TempDir()
	t.Setenv("USERPROFILE", original)
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	t.Setenv("OPENCODE_PERMISSION", "")
	for _, cli := range []string{"codex", "claude", "cursor", "opencode"} {
		c := captureNativeConfiguration(cli)
		if c.Roots["USERPROFILE"] != original {
			t.Fatalf("%s lost the execution host's Windows home", cli)
		}
		environment, err := nativeEnvironment(cli, c, []string{"USERPROFILE=/different-home", "PATH=/native"})
		if err != nil || environmentMap(environment)["USERPROFILE"] != original {
			t.Fatalf("%s inherited another terminal's Windows home: %v %v", cli, environment, err)
		}
	}
}

func TestNativeConfigurationProfileVersionCompatibility(t *testing.T) {
	t.Setenv("USERPROFILE", "/recorded-profile")
	captured := captureNativeConfiguration("codex")
	if captured.Version != 1 || captured.Clone().Version != captured.Version {
		t.Fatal("lost root snapshot version")
	}
	for _, format := range []string{"json", "yaml"} {
		var roundTrip NativeConfiguration
		if format == "json" {
			data, err := json.Marshal(captured)
			if err != nil || json.Unmarshal(data, &roundTrip) != nil {
				t.Fatal("configuration JSON round trip failed")
			}
		} else {
			data, err := yaml.Marshal(captured)
			if err != nil || yaml.Unmarshal(data, &roundTrip) != nil {
				t.Fatal("configuration YAML round trip failed")
			}
		}
		if !reflect.DeepEqual(captured, &roundTrip) || roundTrip.Validate("codex") != nil {
			t.Fatalf("lost versioned roots through %s: %+v", format, roundTrip)
		}
	}
	var legacy NativeConfiguration
	if err := json.Unmarshal([]byte(`{"roots":{"CODEX_HOME":"/original"}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	environment, err := nativeEnvironment("codex", &legacy, []string{"USERPROFILE=/inherited-profile", "CODEX_HOME=/different"})
	if err != nil || environmentMap(environment)["USERPROFILE"] != "/inherited-profile" || environmentMap(environment)["CODEX_HOME"] != "/original" {
		t.Fatalf("changed legacy snapshot semantics: %v %v", environment, err)
	}
	for _, version := range []int{-1, 2} {
		if _, err := nativeEnvironment("codex", &NativeConfiguration{Version: version}, nil); err == nil {
			t.Fatal("accepted unknown root snapshot version", version)
		}
	}
	if (&NativeConfiguration{Roots: map[string]string{"USERPROFILE": "/unversioned"}}).Validate("codex") == nil {
		t.Fatal("accepted a new root in an unversioned snapshot")
	}
}

func TestNativeConfigurationPreservesUnsetProfileAndPlatformCase(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	if err := os.Unsetenv("USERPROFILE"); err != nil {
		t.Fatal(err)
	}
	c := captureNativeConfiguration("codex")
	environment, err := nativeEnvironment("codex", c, []string{"USERPROFILE=/different"})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := environmentMap(environment)["USERPROFILE"]; exists {
		t.Fatal("inherited a root that was explicitly absent")
	}
	c.Roots["USERPROFILE"] = "/original"
	environment, err = nativeEnvironment("codex", c, []string{"userprofile=/different", "Home=/different", "codex_home=/different", "PATH=/native", "UNRELATED=kept"})
	if err != nil {
		t.Fatal(err)
	}
	values := environmentMap(environment)
	for _, name := range []string{"userprofile", "Home", "codex_home"} {
		_, exists := values[name]
		if exists == (runtime.GOOS == "windows") {
			t.Fatal("incorrect platform case handling for", name)
		}
	}
	if values["USERPROFILE"] != "/original" || values["PATH"] != "/native" || values["UNRELATED"] != "kept" {
		t.Fatal("changed unrelated environment or failed to restore original profile")
	}
}

// Exercise both real process boundaries. The execution helper invokes a copy
// of this test executable as a native CLI; it reads only a synthetic root file.
func TestNativeProfileRootAcrossPreflightAndExecution(t *testing.T) {
	stage := os.Getenv("WEBMUX_TEST_PROFILE_STAGE")
	if stage == "configured" {
		var request NativeResume
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			os.Exit(2)
		}
		_ = os.Setenv("WEBMUX_TEST_PROFILE_STAGE", "cli")
		if err := RunConfiguredCLI([]string{"codex", "-test.run=^TestNativeProfileRootAcrossPreflightAndExecution$"}, "codex", request.Activity.Configuration); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		os.Exit(0)
	}
	if stage == "preflight" || stage == "cli" {
		if stage == "preflight" {
			var request NativeResume
			if json.NewDecoder(os.Stdin).Decode(&request) != nil {
				os.Exit(4)
			}
		}
		root := os.Getenv("USERPROFILE")
		if runtime.GOOS == "windows" {
			if home, err := os.UserHomeDir(); err != nil || home != root {
				os.Exit(5)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "native-fixture")); err != nil {
			os.Exit(6)
		}
		_ = json.NewEncoder(os.Stdout).Encode(nativePreflightResult{Argv: []string{"codex", "resume", root}})
		os.Exit(0)
	}
	original, different, cliDir := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(original, "native-fixture"), []byte("synthetic native state"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", original)
	c := captureNativeConfiguration("codex")
	t.Setenv("USERPROFILE", different)
	t.Setenv("WEBMUX_TEST_PROFILE_STAGE", "preflight")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	request := NativeResume{Activity: Activity{CLI: "codex", Configuration: c}}
	args, err := preflightWithCommand(request, []string{self, "-test.run=^TestNativeProfileRootAcrossPreflightAndExecution$"})
	if err != nil || !reflect.DeepEqual(args, []string{"codex", "resume", original}) {
		t.Fatalf("preflight used another terminal's profile: %v %v", args, err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(cliDir, name), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", cliDir)
	t.Setenv("WEBMUX_RECOVERY_INSTANCE", "")
	t.Setenv("WEBMUX_TEST_PROFILE_STAGE", "configured")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, self, "-test.run=^TestNativeProfileRootAcrossPreflightAndExecution$")
	command.Stdin = strings.NewReader(string(encoded))
	output, err := command.CombinedOutput()
	var result nativePreflightResult
	if err != nil || json.Unmarshal(output, &result) != nil || !reflect.DeepEqual(result.Argv, []string{"codex", "resume", original}) {
		t.Fatalf("execution used another terminal's profile: %s %v", output, err)
	}
	if os.Getenv("USERPROFILE") != different {
		t.Fatal("helper changed the controller's profile")
	}
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
