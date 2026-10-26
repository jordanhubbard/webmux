package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeAgentDeck(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck integration uses Unix interactive terminal transports")
	}
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls")
	executable := filepath.Join(directory, "agent-deck")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$WEBMUX_AGENT_DECK_TEST_LOG"
case "$*" in
  "list --all --json")
    printf '%s\n' '[{"id":"local-1","title":"local coder","path":"/src/local","group":"webmux","tool":"codex","status":"waiting","substate":"input-required","codex_session_id":"codex-local","conductor_name":"webmux","last_activity_at":"2026-10-26T08:00:00Z"}]'
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
  "inbox peek --json local-1")
    printf '%s\n' '[{"child_session_id":"remote-1","child_title":"remote reviewer","from_status":"running","to_status":"waiting","timestamp":"2026-10-26T08:01:00Z","source_remote":"sparky","tier":"urgent","text":"Review is ready."}]'
    ;;
  "session send local-1 --message-file - --json --require-input-prompt --queue")
    cat > "$WEBMUX_AGENT_DECK_TEST_LOG.message"
    printf '%s\n' '{"success":true,"send_id":"send-local","state":"queued","verdict":"queued","delivery":"queued","submitted":false,"confirmation":"unknown"}'
    ;;
  "remote sparky session send remote-1 --message-file - --json --require-input-prompt --no-wait")
    cat > "$WEBMUX_AGENT_DECK_TEST_LOG.message"
    printf '%s\n' '{"success":true,"delivery":"delivered","submitted":true,"confirmation":"confirmed"}'
    ;;
  "session send-status send-local --json")
    printf '%s\n' '{"send_id":"send-local","state":"landed","verdict":"delivered","message":"secret body"}'
    ;;
  "remote drain sparky --into local-1 --json")
    printf '%s\n' '{"remote":"sparky","fetched":1,"written":1,"duplicates":0,"unknown":0,"woke":true,"writer":{"running":true,"detail":"notify daemon active"},"records":[{"child_session_id":"remote-1","child_title":"remote reviewer","to_status":"waiting","timestamp":"2026-10-26T08:02:00Z","source_remote":"sparky","kind":"finished","done_status":"ok","done_summary":"Review complete","tier":"urgent"}]}'
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
	if status.Attention != 2 || len(status.Messages) != 1 || status.Messages[0].From != "sparky:remote-1" {
		t.Fatalf("missing Agent Deck attention/inbox state: %+v", status)
	}
	local := status.Seats[0]
	for _, seat := range status.Seats {
		if seat.ID == "local:local-1" {
			local = seat
		}
	}
	if local.Role != "conductor" || !local.CanReceive || local.Substate != "input-required" || !local.Attention {
		t.Fatalf("missing conductor metadata: %+v", local)
	}
	host, err := provider.remoteHost(context.Background(), "sparky")
	if err != nil || host != "jkh@sparky.local" {
		t.Fatalf("remote resolution = %q, %v", host, err)
	}
}

func TestAgentDeckProviderSendsQueuesAndReportsStatus(t *testing.T) {
	executable, logPath := fakeAgentDeck(t)
	provider := &agentDeckProvider{executable: executable}
	queued, err := provider.Send(context.Background(), coordinationSendRequest{Target: "local:local-1", Message: "do the review", Queue: true})
	if err != nil {
		t.Fatal(err)
	}
	if !queued.Queued || queued.SendID != "send-local" || queued.Submitted {
		t.Fatalf("unexpected queued result: %+v", queued)
	}
	body, err := os.ReadFile(logPath + ".message")
	if err != nil || string(body) != "do the review" {
		t.Fatalf("message was not delivered over stdin: %q, %v", body, err)
	}
	immediate, err := provider.Send(context.Background(), coordinationSendRequest{Target: "sparky:remote-1", Message: "report status"})
	if err != nil {
		t.Fatal(err)
	}
	if immediate.Queued || !immediate.Submitted || immediate.Delivery != "delivered" {
		t.Fatalf("unexpected immediate result: %+v", immediate)
	}
	status, err := provider.SendStatus(context.Background(), "local:local-1", "send-local")
	if err != nil {
		t.Fatal(err)
	}
	if status["state"] != "landed" || status["message"] != nil || status["target"] != "local:local-1" {
		t.Fatalf("unexpected sanitized send status: %+v", status)
	}
	log, _ := os.ReadFile(logPath)
	if strings.Contains(string(log), "do the review") || strings.Contains(string(log), "report status") {
		t.Fatalf("message body leaked into process arguments: %s", log)
	}
}

func TestAgentDeckProviderDrainsRemoteIntoLocalConductor(t *testing.T) {
	executable, _ := fakeAgentDeck(t)
	provider := &agentDeckProvider{executable: executable}
	result, err := provider.Drain(context.Background(), "sparky", "local:local-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 1 || !result.Woke || result.WriterRunning == nil || !*result.WriterRunning || len(result.Messages) != 1 {
		t.Fatalf("unexpected drain result: %+v", result)
	}
	if result.Messages[0].From != "sparky:remote-1" || result.Messages[0].Body != "Review complete" {
		t.Fatalf("remote provenance was not retained: %+v", result.Messages[0])
	}
	if _, err := provider.Drain(context.Background(), "sparky", "sparky:remote-1"); err == nil {
		t.Fatal("accepted remote drain into a remote worker")
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

func TestRealAgentDeckReadOnlyQualification(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_AGENT_DECK") != "1" {
		t.Skip("set WEBMUX_REAL_AGENT_DECK=1 to qualify the installed Agent Deck profile and registered remotes")
	}
	provider := newAgentDeckProvider()
	descriptor := provider.Descriptor()
	if !descriptor.Available {
		t.Fatal("WEBMUX_REAL_AGENT_DECK=1 but agent-deck is not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, err := provider.Status(ctx)
	if err != nil {
		t.Fatalf("installed Agent Deck did not satisfy the read-only coordination contract: %v", err)
	}
	if descriptor.ID != "agent-deck" || descriptor.Contract != "agent-deck-cli/v1" {
		t.Fatalf("unexpected provider descriptor: %+v", descriptor)
	}
	if len(status.Errors) != 0 {
		t.Fatalf("installed Agent Deck returned partial profile or remote failures: %v", status.Errors)
	}
	for _, seat := range status.Seats {
		if seat.ID == "" || seat.Host == "" || seat.ProjectID == "" {
			t.Fatalf("Agent Deck returned an unqualified session identity: %+v", seat)
		}
	}
}
