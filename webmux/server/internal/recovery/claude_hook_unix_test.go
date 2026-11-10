//go:build darwin || linux

package recovery

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeHookPrivateChannelBindsTokenProcessAndLifetime(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	observation, _ := claudeObservationFixture(t)
	o := newClaudeObserver("2.1.322 (Claude Code)", nil)
	if o == nil {
		t.Fatal("private observer unavailable")
	}
	defer o.close()
	o.start(os.Getpid())
	for _, file := range []string{o.endpoint, filepath.Dir(o.endpoint)} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("observer endpoint is not private", file, err)
		}
	}
	// The additional settings must contain only the session hook, never a hook
	// enable override, permission rule or user configuration replacement.
	var settings map[string]json.RawMessage
	if json.Unmarshal([]byte(o.settings), &settings) != nil || len(settings) != 1 || settings["hooks"] == nil {
		t.Fatal("observer changed non-hook native settings", o.settings)
	}
	data, _ := json.Marshal(observation)
	if err := ClaudeSessionHook([]string{o.endpoint, strings.Repeat("0", 32)}, strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	select {
	case <-o.observations:
		t.Fatal("foreign token published")
	default:
	}
	if err := ClaudeSessionHook([]string{o.endpoint, o.token}, strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-o.observations:
		if got != observation {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valid native observation was not delivered")
	}
	// Even a correct token cannot authorize a different process tree.
	child := exec.Command("sleep", "10")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	other := newClaudeObserver("2.1.322 (Claude Code)", nil)
	if other == nil {
		t.Fatal("second observer unavailable")
	}
	defer other.close()
	other.start(child.Process.Pid)
	_ = ClaudeSessionHook([]string{other.endpoint, other.token}, strings.NewReader(string(data)))
	select {
	case <-other.observations:
		t.Fatal("unrelated process tree published native identity")
	default:
	}
	o.close()
	if _, err := os.Stat(filepath.Dir(o.endpoint)); !os.IsNotExist(err) {
		t.Fatal("observer endpoint survived native process lifetime", err)
	}
	// A stopped observer is a harmless, silent no-op to the native hook.
	if err := ClaudeSessionHook([]string{o.endpoint, o.token}, strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeHookCleanupBeforeNativeProcessStarts(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	o := newClaudeObserver("2.1.322 (Claude Code)", nil)
	if o == nil {
		t.Fatal("observer unavailable")
	}
	o.close()
	if _, err := os.Stat(filepath.Dir(o.endpoint)); !os.IsNotExist(err) {
		t.Fatal("failed native startup leaked observer endpoint", err)
	}
}

func TestNestedCodingCLILeavesForegroundRecoveryIdentityUntouched(t *testing.T) {
	hostFixture(t)
	t.Setenv("WEBMUX_RECOVERY_INSTANCE", "foreground-instance")
	t.Setenv("WEBMUX_RECOVERY_NATIVE_PARENT", "foreground-instance")
	directory := t.TempDir()
	binary := filepath.Join(directory, "claude")
	argsFile := filepath.Join(directory, "args")
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", directory)
	t.Setenv("WEBMUX_NESTED_TEST_ARGS", argsFile)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WEBMUX_NESTED_TEST_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	foreground := Event{Kind: "cli", CLI: "claude", Instance: "foreground-instance", SessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "native-hook"}
	if err := RecordHostEvent(foreground); err != nil {
		t.Fatal(err)
	}
	if err := RunCLI("claude", []string{"--print", "nested synthetic task"}); err != nil {
		t.Fatal(err)
	}
	state := hostSnapshot(t)
	if state.CLI == nil || state.CLI.SessionID != foreground.SessionID || state.Complete != nil {
		t.Fatal("nested CLI replaced or completed the foreground conversation", state)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil || string(args) != "--print\nnested synthetic task\n" {
		t.Fatal("nested CLI launch was modified", string(args), err)
	}
}
