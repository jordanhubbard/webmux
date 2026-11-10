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
		for _, flags := range [][]string{{"--future-policy", "restricted"}, {"--model", "custom"}, {"--strict-config=false"}, {"--profile", ""}} {
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
