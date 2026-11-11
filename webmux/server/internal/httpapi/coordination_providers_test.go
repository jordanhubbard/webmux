package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/session"
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
  "session ownership inspect local-1 --json")
    printf '%s\n' '{"instance_id":"local-1","state":"live","verdict":"owned","admissible":true,"pane_attached":true}'
    ;;
  "remote sparky show remote-1 --json")
    printf '%s\n' '{"id":"remote-1","status":"running","tmux_session":"agentdeck_remote-reviewer_1"}'
    ;;
  "group list --json")
    printf '%s\n' '{"groups":[{"name":"webmux","path":"webmux","session_count":1}]}'
    ;;
  "remote list --json")
    printf '%s\n' '[{"name":"sparky","host":"jkh@sparky.local","last_poll_status":"ok","profile":"work"}]'
    ;;
  "remote sessions sparky --json --with-errors")
    printf '%s\n' '{"sessions":[{"id":"remote-1","title":"remote reviewer","path":"/home/jkh/Src/webmux","group":"webmux","tool":"claude","status":"running","claude_session_id":"claude-remote"}],"errors":[]}'
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

func TestAgentDeckProviderAcceptsFreshInstallEmptyState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck integration uses Unix interactive terminal transports")
	}
	executable := filepath.Join(t.TempDir(), "agent-deck")
	script := `#!/bin/sh
case "$*" in
  "list --all --json") printf 'No profiles found.\n' ;;
  "group list --json") printf '{"groups":[],"total_groups":0,"total_sessions":0}\n' ;;
  "remote list --json") printf 'No remotes configured.\n\nAdd one with: agent-deck remote add <name> <user@host>\n' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := &agentDeckProvider{executable: executable}
	status, err := provider.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Projects) != 0 || len(status.Seats) != 0 || len(status.Errors) != 0 {
		t.Fatalf("fresh Agent Deck install was not empty: %+v", status)
	}
	if _, err := provider.remoteHost(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "no longer configured") {
		t.Fatalf("unexpected missing remote result: %v", err)
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

func TestManagedReattachmentResolvesExactProviderDestination(t *testing.T) {
	s, _ := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	executable, calls := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:local-1", Host: "localhost", Workspace: "/src/local", Runtime: "codex"}
	launch, err := s.resolveManagedReattachment(context.Background(), "anonymous", identity)
	if err != nil || launch.Target != "local:local-1" || !reflect.DeepEqual(launch.Argv, []string{executable, "session", "attach", "local-1"}) {
		t.Fatal("did not resolve the original provider-qualified attachment", launch, err)
	}
	for _, changed := range []session.RigIdentity{
		{Provider: "agent-deck", Target: identity.Target, Host: "other-host", Workspace: identity.Workspace},
		{Provider: "agent-deck", Target: identity.Target, Host: identity.Host, Workspace: "/other-project"},
		{Provider: "agent-deck", Target: identity.Target, Host: identity.Host, Workspace: identity.Workspace, Runtime: "claude"},
		{Provider: "agent-deck", Target: identity.Target, Host: identity.Host, Workspace: identity.Workspace, Runtime: identity.Runtime, ManagerProfile: "other-profile"},
		{Provider: "agent-deck", Target: "agent-deck:local:deleted", Host: identity.Host},
		{Provider: "agent-deck", Target: "rig:local:local-1", Host: identity.Host},
	} {
		if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", changed); err == nil {
			t.Fatal("changed or missing managed destination was accepted", changed)
		}
	}
	// Openable alone is not liveness: Agent Deck exposes stopped sessions as
	// openable too. Reconnection must not implicitly start one of those jobs.
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte(strings.ReplaceAll(strings.ReplaceAll(string(script), `"status":"waiting"`, `"status":"stopped"`), `"state":"live"`, `"state":"dead"`)), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", identity); err == nil {
		t.Fatal("stopped provider session was treated as a surviving process")
	}
	setCoordinationIntegrations(t, s)
	if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", identity); err == nil {
		t.Fatal("disabled lifecycle manager was bypassed")
	}
	data, err := os.ReadFile(calls)
	if err != nil || strings.Contains(string(data), "launch") || strings.Contains(string(data), "session start") || strings.Contains(string(data), "session attach") {
		t.Fatal("resolving an attachment launched or restarted a managed job", string(data), err)
	}
}

func TestManagedReconnectHTTPUsesFreshAttachment(t *testing.T) {
	executable, _ := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:local-1", Host: "localhost", Workspace: "/src/local", Runtime: "codex"}
	value, err := s.sessions.EnsureRigTerminal("anonymous", identity.Target, "coder", []string{"/bin/sh", "-c", "read line"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/sessions/" + value.ID + "/reconnect"
	setCoordinationIntegrations(t, s)
	requireStatus(t, request(handler, "POST", endpoint, `{}`, ""), 409)
	unchanged, err := s.sessions.Get("anonymous", value.ID)
	if err != nil || unchanged.ExecArgv[0] != "/bin/sh" || unchanged.Rig == nil || *unchanged.Rig != identity {
		t.Fatal("failed resolution changed the saved viewer", unchanged, err)
	}
	setCoordinationIntegrations(t, s, "agent-deck")
	response := request(handler, "POST", endpoint, `{}`, "")
	requireStatus(t, response, 200)
	var result session.Session
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != value.ID || result.Rig == nil || *result.Rig != identity || !reflect.DeepEqual(result.ExecArgv, []string{executable, "session", "attach", "local-1"}) {
		t.Fatal("HTTP reconnect lost its managed identity or used saved arguments", result)
	}
}

func TestAgentDeckSurvivalUsesOwnedProcessIdentityAndRemoteNamespace(t *testing.T) {
	executable, calls := fakeAgentDeck(t)
	p := &agentDeckProvider{executable: executable}
	for _, target := range []string{"local:local-1", "sparky:remote-1"} {
		live, err := p.Survives(context.Background(), target)
		if err != nil || !live {
			t.Fatal("exact owned process was not verified", target, live, err)
		}
	}
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte(strings.ReplaceAll(string(script), `"instance_id":"local-1"`, `"instance_id":"other-session"`)), 0o700); err != nil {
		t.Fatal(err)
	}
	if live, err := p.Survives(context.Background(), "local:local-1"); err != nil || live {
		t.Fatal("another instance's ownership receipt was accepted", live, err)
	}
	data, err := os.ReadFile(calls)
	// Agent Deck 1.16.34 rejects forwarded ownership receipts; the remote
	// manager's forwarded session state is used instead.
	if err != nil || !strings.Contains(string(data), "remote sparky show remote-1 --json") || strings.Contains(string(data), "remote sparky session ownership") || strings.Contains(string(data), "reconcile") || strings.Contains(string(data), "start") {
		t.Fatal("survival probe changed lifecycle state or lost remote identity", string(data), err)
	}
	running := `{"id":"remote-1","status":"running","tmux_session":"agentdeck_remote-reviewer_1"}`
	for _, state := range []string{
		`{"id":"remote-1","status":"stopped","tmux_session":"agentdeck_remote-reviewer_1"}`,
		`{"id":"remote-1","status":"error","tmux_session":"agentdeck_remote-reviewer_1"}`,
		`{"id":"remote-1","status":"running","tmux_session":""}`,
		`{"id":"other","status":"running","tmux_session":"agentdeck_remote-reviewer_1"}`,
	} {
		if err := os.WriteFile(executable, []byte(strings.ReplaceAll(string(script), running, state)), 0o700); err != nil {
			t.Fatal(err)
		}
		if live, err := p.Survives(context.Background(), "sparky:remote-1"); err != nil || live {
			t.Fatal("a remote session that is not running was treated as surviving", state, live, err)
		}
	}
}

func TestAgentDeckLookupsPollOnlyTheOwningRemote(t *testing.T) {
	executable, calls := fakeAgentDeck(t)
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	unreachable := strings.ReplaceAll(string(script), `  "remote sessions sparky --json --with-errors")
`, `  "remote sessions sparky --json --with-errors")
    printf '%s\n' '{"sessions":[],"errors":[{"name":"sparky","host":"jkh@sparky.local","error":"ssh command failed: exit status 255: ssh: connect to host sparky.local port 22: Operation timed out\r\n"}]}'; exit 1
`)
	if err := os.WriteFile(executable, []byte(unreachable), 0o700); err != nil {
		t.Fatal(err)
	}
	p := &agentDeckProvider{executable: executable}
	if live, err := p.Survives(context.Background(), "local:local-1"); err != nil || !live {
		t.Fatal("an unreachable remote affected a local session", live, err)
	}
	if _, err := p.Attach(context.Background(), "local:local-1"); err != nil {
		t.Fatal("an unreachable remote affected local attachment", err)
	}
	data, _ := os.ReadFile(calls)
	if strings.Contains(string(data), "remote sessions") {
		t.Fatal("a local lookup polled remote hosts", string(data))
	}
	_, err = p.Survives(context.Background(), "sparky:remote-1")
	var down *agentDeckRemoteUnavailable
	if !errors.As(err, &down) || !strings.Contains(err.Error(), `remote "sparky" did not respond`) || !strings.Contains(err.Error(), "timed out") {
		t.Fatal("an unreachable remote host was not reported as such", err)
	}
	if _, err := p.Survives(context.Background(), "retired:remote-1"); err == nil || !strings.Contains(err.Error(), "no longer configured") || errors.As(err, &down) {
		t.Fatal("a removed remote was not reported as unconfigured", err)
	}
	// The inventory view still lists local sessions and reports the remote.
	status, err := p.Status(context.Background())
	if err != nil || len(status.Errors) == 0 || len(status.Seats) != 1 {
		t.Fatal("inventory did not degrade to local sessions", status, err)
	}
}

func TestManagedReattachmentAcceptsVerifiedNativeInventoryWithoutDisplayStatus(t *testing.T) {
	executable, _ := fakeAgentDeck(t)
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte(strings.ReplaceAll(string(script), `"status":"waiting",`, "")), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, _ := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:local-1", Host: "localhost", Workspace: "/src/local", Runtime: "codex"}
	if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", identity); err != nil {
		t.Fatal("native inventory without display status blocked an owned process", err)
	}
}

func TestAgentDeckLocalProfileScopesAttachmentAndOwnership(t *testing.T) {
	executable, calls := fakeAgentDeck(t)
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(script), `"id":"local-1","title"`, `"id":"local-1","profile":"work","transcript_path":"/native/conversation.jsonl","title"`)
	text = strings.ReplaceAll(text, `"session ownership inspect local-1 --json")`, `"-p work session ownership inspect local-1 --json")`)
	if err := os.WriteFile(executable, []byte(text), 0o700); err != nil {
		t.Fatal(err)
	}
	p := &agentDeckProvider{executable: executable}
	launch, err := p.Attach(context.Background(), "local:local-1")
	if err != nil || launch.Profile != "work" || launch.NativeStateFile != "/native/conversation.jsonl" || !reflect.DeepEqual(launch.Argv, []string{executable, "-p", "work", "session", "attach", "local-1"}) {
		t.Fatal("attachment lost its native profile", launch, err)
	}
	if live, err := p.Survives(context.Background(), "local:local-1"); err != nil || !live {
		t.Fatal("ownership used another profile", live, err)
	}
	data, _ := os.ReadFile(calls)
	if !strings.Contains(string(data), "-p work session ownership inspect local-1 --json") {
		t.Fatal("ownership did not name the original profile", string(data))
	}
}

func TestAgentDeckDuplicateProfileIDsDoNotSelectAnArbitraryRegistry(t *testing.T) {
	executable, _ := fakeAgentDeck(t)
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(script), "\n")
	for index, line := range lines {
		if strings.Contains(line, `"id":"local-1","title"`) {
			lines[index] = strings.Replace(line, `}]'`, `},{"id":"local-1","profile":"work","title":"different profile","path":"/other","group":"other","tool":"codex"}]'`, 1)
		}
	}
	if err := os.WriteFile(executable, []byte(strings.Join(lines, "\n")), 0o700); err != nil {
		t.Fatal(err)
	}
	p := &agentDeckProvider{executable: executable}
	if _, err := p.Attach(context.Background(), "local:local-1"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("ambiguous profile selected for attachment", err)
	}
	if live, err := p.Survives(context.Background(), "local:local-1"); err == nil || live {
		t.Fatal("ambiguous profile used for process ownership", live, err)
	}
}
