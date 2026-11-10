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
		{"claude", []string{"claude", "--tools", "Read"}},
		{"cursor", []string{"agent", "--mode"}},
	} {
		if _, err := nativeResumeOptions(tc.cli, tc.args); err == nil {
			t.Fatalf("silently dropped invocation policy: %s %v", tc.cli, tc.args)
		}
	}
}
