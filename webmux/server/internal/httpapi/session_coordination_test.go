package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func coordinationGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture Git %v: %v %s", args, err, data)
	}
}
func coordinationAPI(t *testing.T) (*Server, http.Handler, http.Handler, []session.Session, []coordinationclient.Access) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git required for verified project control")
	}
	t.Setenv("WEBMUX_COORDINATION_CACHE_HOME", t.TempDir())
	t.Setenv("WEBMUX_TEST_COORDINATION_CHILD", "1")
	repo := t.TempDir()
	coordinationGit(t, repo, "init")
	coordinationGit(t, repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "--allow-empty", "-m", "root")
	coordinationGit(t, repo, "remote", "add", "origin", "https://example.test/team/shared.git")
	linked := filepath.Join(repo, ".worktrees", "peer")
	coordinationGit(t, repo, "worktree", "add", "-b", "peer", linked, "HEAD")
	old, _ := fixture(t, "local")
	federationAccounts(t, old)
	if err := old.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"session_logging": map[string]any{"enabled": false}}}); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	records := []session.Session{}
	for i, owner := range []string{"owner", "owner", "other"} {
		cwd := repo
		if i == 1 {
			cwd = linked
		}
		records = append(records, session.Session{ID: fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1), Owner: owner, Kind: "terminal", Transport: "exec", Hostname: "localhost", Username: owner, Port: 22, Cols: 80, Rows: 24, State: "connected", CreatedAt: "2026-11-21T12:00:00Z", ExecArgv: []string{executable, "-test.run=^TestCoordinationSessionChild$"}, ExecCwd: cwd})
	}
	if err = old.store.WriteSessions(struct {
		Sessions []session.Session `yaml:"sessions"`
	}{records}); err != nil {
		t.Fatal(err)
	}
	s, err := New(old.store, Options{Name: "coordination-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	h := s.Handler()
	agent := s.CoordinationAgentHandler()
	listener := httptest.NewServer(agent)
	t.Cleanup(listener.Close)
	s.EnableSessionCoordination(listener.URL)
	// Persisted connected flags alone do not grant live coordination authority.
	before := request(h, "GET", "/api/session-coordination", "", token(t, s, "owner"))
	requireStatus(t, before, 200)
	var initial plane.View
	if err = json.Unmarshal(before.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	for _, member := range initial.Members {
		if member.Active {
			t.Fatal("persisted state became live authority before restore")
		}
	}
	for _, record := range records {
		if _, err = s.sessions.Reconnect(record.Owner, record.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	s.StartSessionCoordination()
	access := []coordinationclient.Access{}
	deadline := time.Now().Add(12 * time.Second)
	for _, record := range records {
		ready := false
		for time.Now().Before(deadline) {
			text, err := s.sessions.Scrollback(record.Owner, record.ID)
			if err == nil && strings.Contains(text, "COORDINATION_CHILD_READY") {
				ready = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !ready {
			text, _ := s.sessions.Scrollback(record.Owner, record.ID)
			t.Fatalf("automatically provisioned child did not reach the scoped API: %s", text)
		}
		data, err := os.ReadFile(filepath.Join(s.store.Home, "data", "coordination", "access", record.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var entry coordinationclient.Access
		if err = json.Unmarshal(data, &entry); err != nil {
			t.Fatal(err)
		}
		access = append(access, entry)
	}
	return s, h, agent, records, access
}
func TestDefaultCoordinationRegistersOrdinarySessionsAndPreservesBoundaries(t *testing.T) {
	s, h, agent, records, access := coordinationAPI(t)
	admin, member := token(t, s, "owner"), token(t, s, "other")
	result := request(h, "GET", "/api/session-coordination", "", admin)
	requireStatus(t, result, 200)
	if strings.Contains(result.Body.String(), "wmc1.") {
		t.Fatal("owner view leaked a capability")
	}
	var view plane.View
	if err := json.Unmarshal(result.Body.Bytes(), &view); err != nil || len(view.Members) != 2 || !view.Enabled {
		t.Fatal(view, err)
	}
	if view.Members[0].Project == nil || view.Members[1].Project == nil || view.Members[0].Project.ID != view.Members[1].Project.ID || view.Members[0].Project.Worktree == view.Members[1].Project.Worktree {
		t.Fatal("linked worktrees were not grouped by verified repository", view.Members)
	}
	requireStatus(t, request(h, "GET", "/api/session-coordination", "", access[0].Capability), 401)
	requireStatus(t, request(h, "GET", "/api/config", "", access[0].Capability), 401)
	requireStatus(t, request(h, "GET", "/api/federation/sessions", "", access[0].Capability), 401)
	requireStatus(t, request(agent, "GET", "/view", "", admin), 401)
	requireStatus(t, request(agent, "GET", "/api/config", "", access[0].Capability), 404)
	requireStatus(t, request(h, "PUT", "/api/session-coordination/policy", `{"enabled":false}`, member), 403)
	for i, entry := range access {
		response := request(agent, "GET", "/view", "", entry.Capability)
		requireStatus(t, response, 200)
		var visible plane.View
		if err := json.Unmarshal(response.Body.Bytes(), &visible); err != nil {
			t.Fatal(err)
		}
		expected := 2
		if i == 2 {
			expected = 1
		}
		if len(visible.Members) != expected {
			t.Fatal("foreign account session leaked", visible.Members)
		}
	}
	requireStatus(t, request(h, "PUT", "/api/session-coordination/policy", `{"enabled":false}`, admin), 204)
	requireStatus(t, request(agent, "GET", "/view", "", access[0].Capability), 503)
	if live, err := s.sessions.Get("owner", records[0].ID); err != nil || live.State != "connected" {
		t.Fatal("global policy changed terminal lifecycle", live.State, err)
	}
	requireStatus(t, request(h, "PUT", "/api/session-coordination/policy", `{"enabled":true}`, admin), 204)
	requireStatus(t, request(agent, "GET", "/view", "", access[0].Capability), 200)
	if err := s.sessions.Delete("owner", records[0].ID); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(agent, "POST", "/act", `{"request_id":"stale-after-delete","action":"work","text":"stale"}`, access[0].Capability), 401)
}
func TestScopedAPIClaimsHandoffsAndWorkerReservation(t *testing.T) {
	_, _, agent, _, access := coordinationAPI(t)
	act := func(who int, operation plane.Request) plane.Result {
		t.Helper()
		data, _ := json.Marshal(operation)
		response := request(agent, "POST", "/act", string(data), access[who].Capability)
		requireStatus(t, response, 200)
		var result plane.Result
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := act(0, plane.Request{ID: "api-first-claim", Action: "claim", Kind: "directory", Resource: "src"})
	second := act(1, plane.Request{ID: "api-overlap-claim", Action: "claim", Kind: "file", Resource: "src/main.go"})
	if first.Status != "claimed" || second.Status != "claimed" || len(second.Conflicts) != 1 {
		t.Fatal(first, second)
	}
	target := access[1].Subject
	handoff := act(0, plane.Request{ID: "api-handoff-original", Action: "message", Kind: "handoff", Target: &target, Text: "Take over the overlapping file after acknowledging."})
	for _, action := range []string{"delivered", "acknowledge", "complete"} {
		if result := act(1, plane.Request{ID: "api-receipt-" + action, Action: action, Record: handoff.Record}); result.Status != "recorded" {
			t.Fatal(result)
		}
	}
	foreign := access[2].Subject
	if result := act(0, plane.Request{ID: "api-foreign-handoff", Action: "message", Kind: "handoff", Target: &foreign, Text: "Do not cross account scope"}); result.Status != "recipient-unavailable" {
		t.Fatal(result)
	}
	resource := "node:" + access[0].Subject.Node + ":heavy"
	lease := act(0, plane.Request{ID: "api-reserve-heavy", Action: "reserve", Resource: resource})
	if lease.Status != "reserved" || lease.Fence == 0 {
		t.Fatal(lease)
	}
	if result := act(1, plane.Request{ID: "api-conflicting-heavy", Action: "reserve", Resource: resource}); result.Status != "busy" {
		t.Fatal(result)
	}
	if result := act(0, plane.Request{ID: "api-release-heavy", Action: "release-reservation", Resource: resource, Record: lease.Record, Fence: lease.Fence}); result.Status != "released" {
		t.Fatal(result)
	}
}
func TestCoordinationSessionChild(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_COORDINATION_CHILD") != "1" {
		t.Skip("explicit native ordinary-session child")
	}
	if os.Getenv("WEBMUX_COORDINATION_FILE") == "" || os.Getenv("WEBMUX_COORDINATION_CLI") == "" {
		t.Fatal("ordinary session lacked automatically provisioned interface")
	}
	var out bytes.Buffer
	if err := coordinationclient.Run([]string{"view"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), plane.Contract) || strings.Contains(out.String(), "wmc1.") {
		t.Fatal("scoped client returned invalid/public secret metadata")
	}
	_, _ = io.WriteString(os.Stdout, "COORDINATION_CHILD_READY\n")
	time.Sleep(180 * time.Second)
}

func TestCoordinationRestoredFlagsDoNotReauthorizeStaleCapability(t *testing.T) {
	original, _, _, records, access := coordinationAPI(t)
	store := original.store
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	// Simulate a retained snapshot that still says connected. No broker run has
	// been verified in the new controller, so this must not restore authority.
	for i := range records {
		records[i].State = "connected"
	}
	if err := store.WriteSessions(struct {
		Sessions []session.Session `yaml:"sessions"`
	}{records}); err != nil {
		t.Fatal(err)
	}
	restored, err := New(store, Options{Name: "reconstructed"})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	requireStatus(t, request(restored.CoordinationAgentHandler(), "GET", "/view", "", access[0].Capability), 401)
}

func TestCoordinationAgentRefreshesPeersBeforeMaintenance(t *testing.T) {
	s, _, agent, _, access := coordinationAPI(t)
	s.sessionCoord.bindingsMu.Lock()
	clear(s.sessionCoord.bindings)
	s.sessionCoord.bindingsMu.Unlock()
	response := request(agent, "GET", "/view", "", access[0].Capability)
	requireStatus(t, response, 200)
	var view plane.View
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Members) != 2 || view.Self == nil || view.Self.ProjectStatus != "verified-git" {
		t.Fatal("cold agent view did not discover linked-worktree peer", len(view.Members))
	}
}
