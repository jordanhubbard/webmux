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

	"github.com/jordanhubbard/webmux/server/internal/maccoord"
)

// Real WebMux broker/capability/account boundaries, with only MAC's authenticated
// HTTP publisher replaced. No agent-supplied repository or project is trusted.
func TestMACProjectBrowserAndAgentShareScopedReadAndRefuseCompetingAuthority(t *testing.T) {
	s, browser, agent, records, access := coordinationAPI(t)
	const secret = "mac-observer-private-fixture-token-123456789"
	var reads atomic.Int32
	var revoked atomic.Bool
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Header.Get("Authorization") != "Bearer "+secret || revoked.Load() {
			w.WriteHeader(403)
			return
		}
		reads.Add(1)
		if r.URL.Path != "/dashboard/observe/projects/alpha/graph" || r.URL.RawQuery != "" {
			t.Error("escaped fixed project route", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema": maccoord.GraphSchema, "project": "alpha", "found": true, "server_time": time.Now().UTC().Format(time.RFC3339Nano), "degraded": []any{},
			"graph": map[string]any{"nodes": []any{}, "edges": []any{}, "total": 0, "shown": 0, "omitted": 0, "omitted_live": 0, "omitted_edges": 0, "truncated": false},
		})
	}))
	defer publisher.Close()
	node, _ := s.coordinationNode()
	inputs := s.coordinationInputs(node, records)
	repository := inputs[0].Project
	if repository == nil {
		t.Fatal("fixture project not verified")
	}
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
			"bindings":    []any{map[string]any{"owner": "owner", "repository_id": repository.ID, "repository": repository.Repository, "authority_id": "fleet-a", "project_key": "alpha", "capabilities": []string{maccoord.ObserveCapability}}}},
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
		for _, session := range got.Sessions {
			if session.Session.Owner != "owner" || session.MACLinkState != "unlinked" || session.MACWorkerID != nil {
				t.Fatal("account or session/worker boundary lost", session)
			}
		}
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), directory) || strings.Contains(response.Body.String(), publisher.URL) {
			t.Fatal("secret/local configuration exposed or cached")
		}
	}
	before := reads.Load()
	requireStatus(t, request(browser, "GET", path, "", token(t, s, "other")), 404)
	requireStatus(t, request(browser, "GET", path+"?project=another", "", ownerToken), 400)
	requireStatus(t, request(agent, "GET", "/project?offset=201", "", access[0].Capability), 400)
	requireStatus(t, request(agent, "GET", "/project", "", ownerToken), 401)
	requireStatus(t, request(browser, "GET", path, "", access[0].Capability), 401)
	requireStatus(t, request(agent, "POST", "/act", `{"request_id":"mac-bound-refuse-work","action":"work","text":"must not enter standalone authority"}`, access[0].Capability), 409)
	requireStatus(t, request(agent, "GET", "/view", "", access[0].Capability), 409)
	if reads.Load() != before {
		t.Fatal("unauthorized input or standalone operation reached MAC")
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
