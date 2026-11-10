package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func nativeManagerFixture(t *testing.T) (*agentDeckProvider, session.RigIdentity, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck executes on Unix hosts")
	}
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "native.jsonl")
	id := "08bbfe0e-d31c-4cb6-9d18-546eea56a08c"
	writeJSON := func(name string, value any) {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("native.jsonl", map[string]string{"sessionId": id, "cwd": project})
	writeJSON("inventory.json", []map[string]any{{"id": "seat", "title": "coder", "path": project, "group": "project", "tool": "claude", "profile": "work", "claude_session_id": id, "transcript_path": state}})
	writeJSON("receipt.json", map[string]any{"instance_id": "seat", "admissible": true, "pane_attached": false, "survivors": []any{}})
	executable := filepath.Join(root, "agent-deck")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$WEBMUX_NATIVE_MANAGER_FIXTURE/calls"
if [ "$1" = '-p' ]; then
 [ "$2" = 'work' ] || exit 1
 shift 2
fi
case "$*" in
 "version") printf '%s\n' 'Agent Deck v1.16.34' ;;
 "list --all --json") cat "$WEBMUX_NATIVE_MANAGER_FIXTURE/inventory.json" ;;
 "group list --json") printf '%s\n' '{"groups":[]}' ;;
 "remote list --json") printf '%s\n' '[]' ;;
 "session ownership inspect seat --json")
  if [ -f "$WEBMUX_NATIVE_MANAGER_FIXTURE/live" ]; then
   printf '%s\n' '{"instance_id":"seat","state":"live","verdict":"owned","admissible":true,"pane_attached":true,"survivors":[]}'
  else cat "$WEBMUX_NATIVE_MANAGER_FIXTURE/receipt.json"; fi ;;
 "session start seat --json")
  touch "$WEBMUX_NATIVE_MANAGER_FIXTURE/live"
  printf '%s\n' '{"id":"seat","success":true}' ;;
 *) printf 'unexpected native manager command\n' >&2;exit 2 ;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_NATIVE_MANAGER_FIXTURE", root)
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:seat", Host: "localhost", Workspace: project, Runtime: "claude", ManagerProfile: "work", NativeSessionID: id, NativeStateFile: state, NativeSource: "agent-deck-inventory"}
	return &agentDeckProvider{executable: executable}, identity, root
}

func TestManagedNativeProviderResumesExactConversationThroughOwnerAndReattachesSurvivor(t *testing.T) {
	p, saved, root := nativeManagerFixture(t)
	execute, err := p.PlanNativeRecovery(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if strings.Contains(string(calls), "session start") {
		t.Fatal("planning started native process")
	}
	argv, err := execute(context.Background())
	want := []string{p.executable, "-p", "work", "session", "attach", "seat"}
	if err != nil || !reflect.DeepEqual(argv, want) {
		t.Fatal(argv, err)
	}
	// A subsequent explicit action must attach the existing process, not start it.
	execute, err = p.PlanNativeRecovery(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls, _ = os.ReadFile(filepath.Join(root, "calls"))
	if strings.Count(string(calls), "-p work session start seat --json") != 1 {
		t.Fatal("native start duplicated", string(calls))
	}
}

func TestManagedNativeProviderRejectsChangedDestinationsAndMissingOrForeignState(t *testing.T) {
	for _, kind := range []string{"host", "profile", "project", "native-id", "missing", "foreign-native-project", "escaped-process", "unqualified-version"} {
		t.Run(kind, func(t *testing.T) {
			p, saved, root := nativeManagerFixture(t)
			switch kind {
			case "host":
				saved.Host = "other-host"
			case "profile":
				saved.ManagerProfile = "other-profile"
			case "project":
				saved.Workspace = root
			case "native-id":
				saved.NativeSessionID = "18bbfe0e-d31c-4cb6-9d18-546eea56a08c"
			case "missing":
				if err := os.Remove(saved.NativeStateFile); err != nil {
					t.Fatal(err)
				}
			case "foreign-native-project":
				data, _ := json.Marshal(map[string]string{"sessionId": saved.NativeSessionID, "cwd": root})
				if err := os.WriteFile(saved.NativeStateFile, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "unqualified-version":
				source, err := os.ReadFile(p.executable)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.executable, []byte(strings.ReplaceAll(string(source), "Agent Deck v1.16.34", "Agent Deck v1.16.340")), 0700); err != nil {
					t.Fatal(err)
				}
			case "escaped-process":
				if err := os.WriteFile(filepath.Join(root, "receipt.json"), []byte(`{"instance_id":"seat","admissible":true,"pane_attached":false,"state":"live","verdict":"owned","survivors":[{"pid":123}]}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := p.PlanNativeRecovery(context.Background(), saved); err == nil {
				t.Fatal("unsafe native recovery accepted", kind)
			}
			calls, _ := os.ReadFile(filepath.Join(root, "calls"))
			if strings.Contains(string(calls), "session start") {
				t.Fatal("rejected plan started a process")
			}
		})
	}
}

func TestManagedNativeProviderRevalidatesEvidenceImmediatelyBeforeStart(t *testing.T) {
	p, saved, root := nativeManagerFixture(t)
	execute, err := p.PlanNativeRecovery(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(saved.NativeStateFile); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(context.Background()); err == nil {
		t.Fatal("deleted conversation was resumed")
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if strings.Contains(string(calls), "session start") {
		t.Fatal("stale plan started a process")
	}
}

func TestManagedRecoveryHTTPPreservesOwnerAndApprovalBoundaries(t *testing.T) {
	p, identity, _ := nativeManagerFixture(t)
	t.Setenv("PATH", filepath.Dir(p.executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "local")
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"member","password":"member-password"}`, owner), 201)
	member := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"member","password":"member-password"}`, ""))
	setCoordinationIntegrations(t, s, "agent-deck")
	if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"session_logging": map[string]any{"enabled": true}, "coordination": map[string]any{"enabled_providers": []string{"agent-deck"}}}}); err != nil {
		t.Fatal(err)
	}
	saved, err := s.sessions.EnsureRigTerminal("owner", identity.Target, "coder", []string{"/bin/sh", "-c", "read line"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/session-recovery/" + saved.ID
	requireStatus(t, request(handler, "POST", endpoint, `{}`, ""), 401)
	requireStatus(t, request(handler, "POST", endpoint, `{}`, member), 404)
	memberSession, err := s.sessions.EnsureRigTerminal("member", identity.Target, "member viewer", []string{"/bin/sh", "-c", "read line"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "POST", "/api/session-recovery/"+memberSession.ID, `{"kind":"managed","confirm":true}`, member), 403)
	preview, err := s.sessions.Recovery("owner", saved.ID)
	if err != nil || !preview.CanResumeManaged {
		t.Fatal(preview, err)
	}
	input := session.RecoveryRequest{ID: "28bbfe0e-d31c-4cb6-9d18-546eea56a08c", Kind: "managed", Instance: preview.Instance, Revision: preview.State.Revision}
	body, _ := json.Marshal(input)
	requireStatus(t, request(handler, "POST", endpoint, string(body), owner), 400)
	input.Confirm = true
	input.Context = "do something from output"
	body, _ = json.Marshal(input)
	requireStatus(t, request(handler, "POST", endpoint, string(body), owner), 400)
	input.Context = ""
	body, _ = json.Marshal(input)
	setCoordinationIntegrations(t, s)
	requireStatus(t, request(handler, "POST", endpoint, string(body), owner), 400)
	setCoordinationIntegrations(t, s, "agent-deck")
	requireStatus(t, request(handler, "POST", endpoint, string(body), owner), 200)
	requireStatus(t, request(handler, "POST", endpoint, string(body), owner), 200)
}
