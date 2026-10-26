package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeAgentDeck(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls")
	executable := filepath.Join(directory, "agent-deck")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$WEBMUX_AGENT_DECK_TEST_LOG"
case "$*" in
  "list --all --json")
    printf '%s\n' '[{"id":"local-1","title":"local coder","path":"/src/local","group":"webmux","tool":"codex","status":"waiting","codex_session_id":"codex-local"}]'
    ;;
  "group list --json")
    printf '%s\n' '{"groups":[{"name":"webmux","path":"webmux","session_count":1}]}'
    ;;
  "remote list --json")
    printf '%s\n' '[{"name":"sparky","host":"jkh@sparky.local","last_poll_status":"ok","profile":"work"}]'
    ;;
  "remote sessions sparky --json")
    printf '%s\n' '[{"id":"remote-1","title":"remote reviewer","path":"/home/jkh/Src/webmux","group":"webmux","tool":"claude","status":"running","claude_session_id":"claude-remote"}]'
    ;;
  "remote sparky group list --json")
    printf '%s\n' '{"groups":[{"name":"webmux","path":"webmux","session_count":1}]}'
    ;;
  launch*) printf '%s\n' '{"id":"launched-local"}' ;;
  "remote sparky launch"*) printf '%s\n' '{"id":"launched-remote"}' ;;
  *) printf 'unexpected command: %s\n' "$*" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_AGENT_DECK_TEST_LOG", logPath)
	return executable, logPath
}

func TestAgentDeckProviderDiscoversHostQualifiedNativeSessions(t *testing.T) {
	executable, _ := fakeAgentDeck(t)
	provider := &agentDeckProvider{executable: executable}
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 2 || len(status.Seats) != 2 {
		t.Fatalf("unexpected inventory: %+v", status)
	}
	want := map[string]struct {
		host, native, workspace string
	}{
		"local:local-1":   {"localhost", "codex-local", "/src/local"},
		"sparky:remote-1": {"sparky", "claude-remote", "/home/jkh/Src/webmux"},
	}
	for _, seat := range status.Seats {
		expected, ok := want[seat.ID]
		if !ok {
			t.Fatalf("unexpected seat identity %q", seat.ID)
		}
		if seat.Host != expected.host || seat.NativeSession != expected.native || seat.Workspace != expected.workspace || !seat.Openable {
			t.Fatalf("unexpected seat: %+v", seat)
		}
	}
	host, err := provider.remoteHost(context.Background(), "sparky")
	if err != nil || host != "jkh@sparky.local" {
		t.Fatalf("remote resolution = %q, %v", host, err)
	}
}

func TestAgentDeckProviderUsesExactLocalAndRemoteArgv(t *testing.T) {
	executable, logPath := fakeAgentDeck(t)
	provider := &agentDeckProvider{executable: executable}

	local, err := provider.Attach(context.Background(), "local:local-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(local.Argv, []string{executable, "session", "attach", "local-1"}) {
		t.Fatalf("local attach argv: %#v", local.Argv)
	}
	remote, err := provider.Attach(context.Background(), "sparky:remote-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(remote.Argv, []string{executable, "remote", "attach", "sparky", "remote-1"}) {
		t.Fatalf("remote attach argv: %#v", remote.Argv)
	}
	launched, err := provider.Launch(context.Background(), coordinationLaunchRequest{
		Mode: "existing", ProjectID: "sparky:webmux", ProjectName: "webmux", Name: "reviewer",
		Host: "sparky", Workspace: "/home/jkh/Src/webmux", Runtime: "claude-code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if launched.Target != "sparky:launched-remote" || !reflect.DeepEqual(launched.Argv, []string{executable, "remote", "attach", "sparky", "launched-remote"}) {
		t.Fatalf("remote launch: %+v", launched)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "remote sparky launch /home/jkh/Src/webmux -t reviewer -g webmux -c claude --json") {
		t.Fatalf("launch did not use the registered remote: %s", log)
	}
}

func TestAgentDeckTargetRejectsAmbiguousIdentity(t *testing.T) {
	for _, value := range []string{"", "missing-location", ":id", "host:", "host:id\nother"} {
		if _, _, err := splitAgentDeckTarget(value); err == nil {
			t.Fatalf("accepted invalid identity %q", value)
		}
	}
}
