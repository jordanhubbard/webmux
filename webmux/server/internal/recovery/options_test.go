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

func TestCodexWindowsSandboxOverridePreservesRestrictions(t *testing.T) {
	for _, flag := range []string{"--config", "-c"} {
		for _, value := range []string{`windows.sandbox='unelevated'`, `windows.sandbox="unelevated"`, " \twindows.sandbox = 'unelevated'\t"} {
			for _, assigned := range []bool{false, true} {
				args := []string{"codex", "--sandbox", "read-only", "--ask-for-approval", "never"}
				if assigned {
					args = append(args, flag+"="+value)
				} else {
					args = append(args, flag, value)
				}
				got, err := nativeResumeOptions("codex", safeArgv(args))
				want := []string{"--sandbox", "read-only", "--ask-for-approval", "never", flag, value}
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("changed original restrictions: %q %v", got, err)
				}
				cfg := captureNativeInvocationConfiguration("codex", safeArgv(args))
				if cfg.ManualReason != "" {
					t.Fatal(cfg.ManualReason)
				}
				activity := Activity{CLI: "codex", NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeEvidence: "launch-selector", Configuration: cfg}
				if !activity.NativeRecoverable() || activity.CanAutoResumeNative() {
					t.Fatal("changed the requirement for a deliberate conversation choice")
				}
			}
		}
	}
	for _, suffix := range [][]string{
		{"-c", `sandbox_mode="danger-full-access"`},
		{"--remote", "https://example.invalid"},
		{"--ignore-user-config"},
		{"--cd", "/different-project"},
	} {
		args := append([]string{"codex", "-c", `windows.sandbox='unelevated'`}, suffix...)
		if _, err := nativeResumeOptions("codex", args); err == nil {
			t.Fatalf("accepted unsupported policy following the Windows override: %q", args)
		}
	}
}

func TestCodexWindowsSandboxOverrideRejectsOtherTOML(t *testing.T) {
	for _, value := range []string{
		"", "windows.sandbox=unelevated", `windows.sandbox='elevated'`,
		`windows.sandbox='disabled'`, `windows.sandbox='unelevated' # comment`,
		"windows.sandbox='unelevated'\nsandbox_mode='danger-full-access'",
		"windows.sandbox='unelevated'\r", `windows.sandbox="unelevated"; other=true`,
		`windows.sandbox='''unelevated'''`, `windows.sandbox="unelevated\u0022"`,
		`windows.sandbox='unelevated'junk`, `windows.sandbox=['unelevated']`,
		"windows.sandbox='unelevated'\x00", `windows.Sandbox='unelevated'`,
		`other.windows.sandbox='unelevated'`, `windows.sandbox='unelevated'` + "\n",
	} {
		args := []string{"codex", "--config", value}
		if _, err := nativeResumeOptions("codex", args); err == nil {
			t.Fatalf("accepted unsupported configuration: %q", value)
		}
		if cfg := captureNativeInvocationConfiguration("codex", args); cfg.ManualReason == "" {
			t.Fatalf("preview offered unsupported configuration: %q", value)
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
