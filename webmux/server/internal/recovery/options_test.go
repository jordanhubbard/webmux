package recovery

import (
	"reflect"
	"testing"
)

func TestResumePreservesRestrictionsWithoutReintroducingBypass(t *testing.T) {
	for _, tc := range []struct {
		cli            string
		original, want []string
	}{
		{"codex", []string{"codex", "resume", "id", "--sandbox=read-only", "--no-daemon", "--profile", "work", "--dangerously-bypass-approvals-and-sandbox"}, []string{"--sandbox", "read-only", "--no-daemon", "--profile", "work"}},
		{"claude", []string{"claude", "--safe-mode", "--tools", "", "--permission-mode", "plan", "--dangerously-skip-permissions"}, []string{"--safe-mode", "--tools", "", "--permission-mode", "plan"}},
		{"cursor", []string{"agent", "--mode=ask", "--sandbox", "enabled", "--yolo"}, []string{"--mode", "ask", "--sandbox", "enabled"}},
		{"opencode", []string{"opencode", "--pure", "--agent", "plan", "--auto"}, []string{"--pure", "--agent", "plan"}},
	} {
		got, err := nativeResumeOptions(tc.cli, tc.original)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v %v", tc.cli, got, err)
		}
	}
}

func TestUnrestorableConfigurationDoesNotSilentlyBroadenNativePolicy(t *testing.T) {
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"codex", []string{"codex", "-c", "sandbox_mode=read-only"}},
		{"codex", []string{"codex", "--profile", "--dangerously-bypass-approvals-and-sandbox"}},
		{"codex", []string{"codex", "[credential arguments omitted]"}},
		{"claude", []string{"claude", "--settings", "private-settings.json"}},
		{"claude", []string{"claude", "--tools=--dangerously-skip-permissions"}},
		{"cursor", []string{"agent", "--mode"}},
	} {
		if _, err := nativeResumeOptions(tc.cli, tc.args); err == nil {
			t.Fatalf("silently dropped invocation policy: %s %v", tc.cli, tc.args)
		}
	}
}

func TestUnsupportedNativeOptionsRequireManualRecoveryBeforeAction(t *testing.T) {
	for _, cli := range []string{"codex", "cursor", "opencode"} {
		for _, flags := range [][]string{{"--future-policy", "restricted"}, {"--model"}, {"--strict-config=false"}, {"--profile", ""}} {
			args := append([]string{cli}, flags...)
			cfg := captureNativeInvocationConfiguration(cli, args)
			if cfg.ManualReason == "" {
				t.Fatalf("%s silently discarded unsupported configuration %v", cli, flags)
			}
			activity := &Activity{CLI: cli, NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "process-state", Configuration: cfg}
			if activity.CanAutoResumeNative() {
				t.Fatalf("%s offered automatic recovery with unsupported settings", cli)
			}
		}
	}
}

func TestNativeResumeSelectorsAreReplacedWithoutDiscardingFollowingRestrictions(t *testing.T) {
	for _, tc := range []struct {
		cli        string
		args, want []string
	}{
		{"cursor", []string{"agent", "--resume=08bbfe0e-d31c-4cb6-9d18-546eea56a08c", "--mode", "ask"}, []string{"--mode", "ask"}},
		{"opencode", []string{"opencode", "--session", "ses_12345678", "--pure"}, []string{"--pure"}},
	} {
		got, err := nativeResumeOptions(tc.cli, tc.args)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.cli, got, err)
		}
	}
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"cursor", []string{"agent", "--resume", "--mode", "ask"}},
		{"opencode", []string{"opencode", "--session", "unknown", "--pure"}},
		{"codex", []string{"codex", "--sandbox", "unknown"}},
		{"cursor", []string{"agent", "--sandbox", "unknown"}},
	} {
		if _, err := nativeResumeOptions(tc.cli, tc.args); err == nil {
			t.Fatal("invalid selector or sandbox silently discarded", tc.cli, tc.args)
		}
	}
}

// Explicit models and other non-policy settings are resumed as given; approval
// and trust automation is left out, so the resumed CLI asks instead; initial
// prompts are not replayed.
func TestResumeKeepsModelAndOrdinarySettings(t *testing.T) {
	for _, tc := range []struct {
		cli            string
		original, want []string
	}{
		{"codex", []string{"codex", "-m", "gpt-6.1-sol", "--search", "--add-dir", "../shared", "--add-dir=/tmp/out", "--oss", "--local-provider", "ollama", "--approve-for-me", "--dangerously-bypass-hook-trust", "fix the build"},
			[]string{"-m", "gpt-6.1-sol", "--search", "--add-dir", "../shared", "--add-dir", "/tmp/out", "--oss", "--local-provider", "ollama"}},
		{"cursor", []string{"cursor-agent", "--model", "claude-opus-4-8[context=1m,effort=high]", "--add-dir", "../docs", "--plugin-dir", "plugins", "-f", "--force", "--auto-review", "--approve-mcps", "--trust", "--mode", "plan"},
			[]string{"--model", "claude-opus-4-8[context=1m,effort=high]", "--add-dir", "../docs", "--plugin-dir", "plugins", "--mode", "plan"}},
		{"opencode", []string{"opencode", "-m", "nvidia-inference/switchyard/openai/gpt-5.6-sol", "--agent", "build", "--mini", "--no-replay", "--replay-limit", "20", "--print-logs", "--log-level", "WARN", "--prompt", "start over"},
			[]string{"-m", "nvidia-inference/switchyard/openai/gpt-5.6-sol", "--agent", "build", "--mini", "--no-replay", "--replay-limit", "20", "--print-logs", "--log-level", "WARN"}},
	} {
		got, err := nativeResumeOptions(tc.cli, tc.original)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %q %v", tc.cli, got, err)
		}
		cfg := captureNativeInvocationConfiguration(tc.cli, tc.original)
		activity := &Activity{CLI: tc.cli, NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "process-state", Configuration: cfg}
		if tc.cli == "opencode" {
			activity.NativeSessionID = "ses_12345678"
		}
		if cfg.ManualReason != "" || !activity.NativeRecoverable() {
			t.Fatalf("%s: supported settings still require manual recovery: %s", tc.cli, cfg.ManualReason)
		}
	}
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"opencode", []string{"opencode", "--agent", "../escape"}},
		{"opencode", []string{"opencode", "--port", "4096"}},
		{"codex", []string{"codex", "-C", "/elsewhere"}},
		{"codex", []string{"codex", "--worktree"}},
		{"codex", []string{"codex", "-i", "screenshot.png"}},
		{"cursor", []string{"cursor-agent", "--workspace", "/elsewhere"}},
		{"cursor", []string{"cursor-agent", "--force=true"}},
	} {
		if _, err := nativeResumeOptions(tc.cli, tc.args); err == nil {
			t.Fatalf("%s: silently accepted %v", tc.cli, tc.args)
		}
	}
}
