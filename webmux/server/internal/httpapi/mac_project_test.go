package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/coordinationmcp"
	"github.com/jordanhubbard/webmux/server/internal/maccoord"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Real WebMux broker/capability/account boundaries, with only MAC's authenticated
// HTTP publisher replaced. No agent-supplied repository or project is trusted.
func TestMACProjectBrowserAndAgentShareScopedReadAndRefuseCompetingAuthority(t *testing.T) {
	s, browser, agent, records, access := coordinationAPI(t)
	const secret = "mac-observer-private-fixture-token-123456789"
	var reads atomic.Int32
	var revoked atomic.Bool
	var revokeAfter atomic.Int32
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Header.Get("Authorization") != "Bearer "+secret || revoked.Load() {
			w.WriteHeader(403)
			return
		}
		count := reads.Add(1)
		if r.URL.Path != "/dashboard/observe/projects/alpha/graph" || r.URL.RawQuery != "" {
			t.Error("escaped fixed project route", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema": maccoord.GraphSchema, "project": "alpha", "found": true, "server_time": time.Now().UTC().Format(time.RFC3339Nano), "degraded": []any{},
			"graph": map[string]any{"nodes": []any{}, "edges": []any{}, "total": 0, "shown": 0, "omitted": 0, "omitted_live": 0, "omitted_edges": 0, "truncated": false},
		})
		if limit := revokeAfter.Load(); limit > 0 && count >= limit {
			revoked.Store(true)
		}
	}))
	defer publisher.Close()
	node, _ := s.coordinationNode()
	inputs := s.coordinationInputs(node, records)
	repository := inputs[0].Project
	if repository == nil {
		t.Fatal("fixture project not verified")
	}
	const sourceText = "# Project roadmap\n\n- [ ] Project-owned requirement\n"
	if err := os.MkdirAll(filepath.Join(repository.Worktree, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository.Worktree, "docs", "roadmap.md"), []byte(sourceText), 0600); err != nil {
		t.Fatal(err)
	}
	coordinationGit(t, repository.Worktree, "add", "docs/roadmap.md")
	coordinationGit(t, repository.Worktree, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "source roadmap")
	directory := t.TempDir()
	credential := filepath.Join(directory, "enrollment.json")
	policy := filepath.Join(directory, "mac-observation.json")
	write := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		protectMACProjectFixture(t, path)
	}
	write(credential, map[string]any{"schema": "mac.client_enrollment.v1", "client_id": "webmux-http-fixture", "connection": map[string]any{"api_url": publisher.URL}, "credential": map[string]any{"token": secret, "scopes": []string{"read"}, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}})
	write(policy, map[string]any{
		"schema": maccoord.RuntimeSchema,
		"bindings": map[string]any{"schema": maccoord.BindingsSchema,
			"authorities": []any{map[string]any{"id": "fleet-a", "origin": publisher.URL}},
			"bindings":    []any{map[string]any{"owner": "owner", "repository_id": repository.ID, "repository": repository.Repository, "authority_id": "fleet-a", "project_key": "alpha", "capabilities": []string{maccoord.ObserveCapability, maccoord.RoadmapObserveCapability}, "roadmap_path": "docs/roadmap.md"}}},
		"credentials": []any{map[string]any{"authority_id": "fleet-a", "client_id": "webmux-http-fixture", "enrollment_file": credential}},
	})
	// The fixture replaces accepted configuration before making concurrent reads;
	// production loads it only at startup, never from a browser request.
	s.macObserver = maccoord.OpenRuntime(policy, publisher.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs)
	ownerToken := token(t, s, "owner")
	path := "/api/session-coordination/projects/" + records[0].ID
	human := request(browser, "GET", path, "", ownerToken)
	requireStatus(t, human, 200)
	machine := request(agent, "GET", "/project", "", access[0].Capability)
	requireStatus(t, machine, 200)
	for _, response := range []*httptest.ResponseRecorder{human, machine} {
		var got maccoord.SessionProjectObservation
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || got.Project.Schema != maccoord.ProjectSchema || got.Subject != access[0].Subject || len(got.Sessions) != 2 || got.HistoryState != "retained_unreconciled" {
			t.Fatal("human/agent scoped read differs", err, response.Body.String())
		}
		if got.Project.ConflictVerdict != "unknown" || got.Project.OrderingVerdict != "unknown" || got.Project.ProjectExistence != "unverified" {
			t.Fatal("empty observation claimed project/order/conflict certainty", got.Project)
		}
		if got.SourceState != "observed" || got.Source == nil || got.Source.RepositoryID != repository.ID || got.Source.Worktree != repository.Worktree || got.Source.Commit == "" || got.SourceTaskLink != "unlinked" {
			t.Fatal("missing verified source or invented task linkage", got.SourceState, got.SourceTaskLink)
		}
		if got.RoadmapState != "observed_committed_snapshot" || got.Roadmap == nil || got.Roadmap.Text != sourceText || got.Roadmap.Commit != got.Source.Commit || got.Roadmap.Path != "docs/roadmap.md" || got.Roadmap.TaskLinkState != "unlinked" || got.Project.RoadmapLinkState != "unlinked" {
			t.Fatal("browser/agent source roadmap lost scope or inferred a task mapping")
		}
		for _, session := range got.Sessions {
			if session.Session.Owner != "owner" || session.MACLinkState != "unlinked" || session.MACWorkerID != nil {
				t.Fatal("account or session/worker boundary lost", session)
			}
		}
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), directory) || strings.Contains(response.Body.String(), publisher.URL) {
			t.Fatal("secret/local configuration exposed or cached")
		}
	}
	remote := records[0]
	remote.Transport = "ssh"
	// This path really exists locally: inspecting it would create a convincing
	// but incorrect source result for a remote execution host.
	if source, state := s.macSourceForSession(t.Context(), remote, repository); source != nil || state != "execution_host_unavailable" {
		t.Fatal("remote execution path was inspected locally", state)
	}
	localSource, err := s.macObserver.ObserveSource(t.Context(), records[0].Owner, repository)
	if err != nil {
		t.Fatal(err)
	}
	if roadmap, state := s.macRoadmapForSession(t.Context(), remote, repository, &localSource); roadmap != nil || state != "execution_host_unavailable" {
		t.Fatal("remote source roadmap was read from a local path", state)
	}
	listener := httptest.NewServer(agent)
	defer listener.Close()
	sender := access[0]
	sender.Endpoint = listener.URL
	accessPath := filepath.Join(t.TempDir(), "scoped-webmux-access.json")
	if err := coordinationclient.WriteAccess(accessPath, sender); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", accessPath)
	t.Setenv("WEBMUX_RECOVERY_SESSION", "")
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "")
	one, two := mcp.NewInMemoryTransports()
	mcpServer, err := coordinationmcp.NewServer().Connect(t.Context(), one, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mcpServer.Close()
	mcpClient, err := mcp.NewClient(&mcp.Implementation{Name: "project-roadmap-read", Version: "1"}, nil).Connect(t.Context(), two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mcpClient.Close()
	toolResult, err := mcpClient.CallTool(t.Context(), &mcp.CallToolParams{Name: "webmux_mac_project_view", Arguments: map[string]any{}})
	if err != nil || toolResult.IsError {
		t.Fatal("same scoped source observation failed through the actual MCP tool", err)
	}
	toolJSON, err := json.Marshal(toolResult.StructuredContent)
	var toolView maccoord.SessionProjectObservation
	if err != nil || json.Unmarshal(toolJSON, &toolView) != nil || toolView.Roadmap == nil || toolView.Roadmap.Text != sourceText || toolView.Roadmap.Commit != localSource.Commit || toolView.Roadmap.TaskLinkState != "unlinked" {
		t.Fatal("MCP and browser did not share the same exact roadmap snapshot")
	}
	before := reads.Load()
	requireStatus(t, request(browser, "GET", path, "", token(t, s, "other")), 404)
	requireStatus(t, request(browser, "GET", path+"?project=another", "", ownerToken), 400)
	requireStatus(t, request(browser, "GET", path+"?roadmap_path=private", "", ownerToken), 400)
	requireStatus(t, request(agent, "GET", "/project?offset=201", "", access[0].Capability), 400)
	requireStatus(t, request(agent, "GET", "/project", "", ownerToken), 401)
	requireStatus(t, request(browser, "GET", path, "", access[0].Capability), 401)
	requireStatus(t, request(agent, "POST", "/act", `{"request_id":"mac-bound-refuse-work","action":"work","text":"must not enter standalone authority"}`, access[0].Capability), 409)
	requireStatus(t, request(agent, "GET", "/view", "", access[0].Capability), 409)
	if reads.Load() != before {
		t.Fatal("unauthorized input or standalone operation reached MAC")
	}
	// Collection's own final check succeeds, then the credential is revoked
	// while local source data is sampled. The shared reader must requalify after
	// that work and discard task/source/roadmap data rather than releasing it.
	revokeAfter.Store(reads.Load() + 2)
	midRead := request(browser, "GET", path, "", ownerToken)
	requireStatus(t, midRead, 503)
	if strings.Contains(midRead.Body.String(), sourceText) || strings.Contains(midRead.Body.String(), "alpha") {
		t.Fatal("source sampling extended a previously successful MAC authorization")
	}
	revoked.Store(true)
	refused := request(agent, "GET", "/project", "", access[0].Capability)
	requireStatus(t, refused, 503)
	if strings.Contains(refused.Body.String(), "alpha") || strings.Contains(refused.Body.String(), secret) {
		t.Fatal("failed auth returned a previous successful observation")
	}
	if live, err := s.sessions.Get("owner", records[0].ID); err != nil || live.CreatedAt != records[0].CreatedAt || live.State != "connected" {
		t.Fatal("read/refusal altered live provider lifecycle", err)
	}
}
