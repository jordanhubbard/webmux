package recovery

import (
	"reflect"
	"testing"
)

func TestClaudeResumePreservesNamedConfigurationAndVariadicPolicies(t *testing.T) {
	args := []string{"claude", "--resume", "launch-query", "--model=sonnet", "--agent", "reviewer", "--effort", "high", "--setting-sources", "user,project", "--system-prompt-snapshot", "off", "--tools", "Read", "Bash", "--allowedTools", "Read", "Bash(git status)", "--disallowed-tools", "Write", "Edit", "--add-dir", "/extra path", "../second", "--permission-mode", "plan", "--restricted", "--strict-mcp-config", "--verbose"}
	want := []string{"--model", "sonnet", "--agent", "reviewer", "--effort", "high", "--setting-sources", "user,project", "--system-prompt-snapshot", "off", "--tools", "Read", "Bash", "--allowedTools", "Read", "Bash(git status)", "--disallowed-tools", "Write", "Edit", "--add-dir", "/extra path", "../second", "--permission-mode", "plan", "--restricted", "--strict-mcp-config", "--verbose"}
	got, err := nativeResumeOptions("claude", args)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("Claude invocation settings were changed", got, err)
	}
}

func TestClaudeResumeKeepsExplicitEmptySourcesAndTools(t *testing.T) {
	args := []string{"claude", "--setting-sources=", "--tools", "", "--permission-mode", "dontAsk", "--", "prompt with --model text"}
	want := []string{"--setting-sources", "", "--tools", "", "--permission-mode", "dontAsk"}
	got, err := nativeResumeOptions("claude", args)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
}

func TestClaudeResumeRejectsOpaqueUnknownOrIncompleteSettings(t *testing.T) {
	for _, args := range [][]string{
		{"--agents", `{"custom":{"prompt":"private"}}`}, {"--system-prompt", "custom"}, {"--append-system-prompt", "custom"}, {"--plugin-dir", "/custom"}, {"--future-policy", "restricted"},
		{"--model"}, {"--agent", "--dangerously-skip-permissions"}, {"--tools=--dangerously-skip-permissions"}, {"--allowedTools"}, {"--disallowedTools="}, {"--setting-sources", "managed"}, {"--effort", "unknown"}, {"--permission-mode", "typo"}, {"--system-prompt-snapshot", "unknown"},
	} {
		if got, err := nativeResumeOptions("claude", append([]string{"claude"}, args...)); err == nil {
			t.Fatal("unverified policy was silently dropped", args, got)
		}
	}
}

func TestClaudeResumeDoesNotReplayOriginalSelectorsPromptsOrBypass(t *testing.T) {
	args := []string{"claude", "--session-id", "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", "--fork-session", "--permission-mode", "bypassPermissions", "--dangerously-skip-permissions", "--allow-dangerously-skip-permissions", "initial prompt", "--model", "opus"}
	got, err := nativeResumeOptions("claude", args)
	if err != nil || !reflect.DeepEqual(got, []string{"--model", "opus"}) {
		t.Fatal("original selector or bypass mode was replayed", got, err)
	}
}

func TestClaudeInlineAgentSettingsAreNotPersistedAsRecoveryArguments(t *testing.T) {
	args := []string{"claude", "--agents", `{"private":{"prompt":"configuration payload"}}`}
	got := safeArgv(args)
	if len(got) != 2 || got[1] != "[credential arguments omitted]" {
		t.Fatal("opaque agent JSON leaked into recovery arguments", got)
	}
	if _, replayable := SafeCommand(`claude --agents '{"private":{"prompt":"configuration payload"}}'`); replayable {
		t.Fatal("opaque agent configuration remained replayable")
	}
}

func TestClaudeUnsupportedInvocationHasManualExplanationBeforeRecovery(t *testing.T) {
	id := "08bbfe0e-d31c-4cb6-9d18-546eea56a08c"
	cfg := captureNativeInvocationConfiguration("claude", []string{"claude", "--append-system-prompt", "private original context"})
	if cfg.ManualReason == "" {
		t.Fatal("unrestorable settings were not explained in the descriptor")
	}
	a := &Activity{CLI: "claude", NativeSessionID: id, Configuration: cfg}
	if a.NativeRecoverable() {
		t.Fatal("preview offered an action that discards original settings")
	}
	supported := captureNativeInvocationConfiguration("claude", []string{"claude", "--model", "sonnet", "--agent", "reviewer", "--tools", "Read"})
	if supported.ManualReason != "" {
		t.Fatal("restorable named settings incorrectly blocked", supported.ManualReason)
	}
}
