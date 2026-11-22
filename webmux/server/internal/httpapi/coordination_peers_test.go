package httpapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"github.com/jordanhubbard/webmux/server/internal/coordinationmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
)

type coordinationPair struct {
	a, b           *Server
	ah, bh, agent  http.Handler
	access, remote []coordinationclient.Access
	tls            *httptest.Server
	secret         string
	mu             sync.Mutex
	paths          []struct {
		path, address string
		bearer        bool
	}
}

func peerPair(t *testing.T) *coordinationPair { return peerPairHook(t, "owner", nil) }
func peerPairHook(t *testing.T, remoteOwner string, hook func(http.Handler, http.ResponseWriter, *http.Request)) *coordinationPair {
	t.Helper()
	a, ah, agent, records, access := coordinationAPI(t)
	b, bh, _, _, remote := coordinationAPIRepository(t, records[0].ExecCwd)
	pair := &coordinationPair{a: a, b: b, ah: ah, bh: bh, agent: agent, access: access, remote: remote, secret: strings.Repeat("isolated-snapshot-grant-", 3)}
	pair.tls = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pair.mu.Lock()
		pair.paths = append(pair.paths, struct {
			path, address string
			bearer        bool
		}{r.URL.Path, r.RemoteAddr, r.Header.Get("Authorization") != ""})
		pair.mu.Unlock()
		if hook != nil {
			hook(bh, w, r)
		} else {
			bh.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(pair.tls.Close)
	a.peerRoots = x509.NewCertPool()
	a.peerRoots.AddCert(pair.tls.Certificate())
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: remoteOwner, Credential: pair.secret}}})
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote", URL: pair.tls.URL, Credential: pair.secret}}})
	requireStatus(t, request(ah, "POST", "/api/servers/remote/verify-identity", "{}", token(t, a, "owner")), 204)
	return pair
}
func (p *coordinationPair) mapOwner(t *testing.T, owner string) {
	t.Helper()
	body := directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_owner": owner})
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", body, token(t, p.a, "owner")), 204)
}
func (p *coordinationPair) share(t *testing.T, value bool) {
	t.Helper()
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", directoryJSON(map[string]any{"owner": "owner", "coordination": value}), token(t, p.b, "owner")), 204)
}
func (p *coordinationPair) refresh(t *testing.T) {
	t.Helper()
	if err := p.a.maintainSessionCoordination(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (p *coordinationPair) view(t *testing.T, index int) plane.View {
	t.Helper()
	response := request(p.agent, "GET", "/view", "", p.access[index].Capability)
	requireStatus(t, response, 200)
	var out plane.View
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Body.String(), p.secret) || strings.Contains(response.Body.String(), "wmc1.") {
		t.Fatal("coordination view exposed a credential")
	}
	return out
}
func TestCoordinationPeerAuthorizationProjectIsolationAndTLS(t *testing.T) {
	p := peerPair(t)
	if got := p.view(t, 0); len(got.Peers) != 0 {
		t.Fatal("terminal grant acquired coordination implicitly")
	}
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_owner": "missing-account"}), token(t, p.a, "owner")), 400)
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", `{}`, token(t, p.a, "other")), 403)
	p.mapOwner(t, "owner")
	if got := p.view(t, 0); len(got.Peers) != 1 || got.Peers[0].State != "pending" {
		t.Fatal("new node permission was not visible as pending")
	}
	p.refresh(t)
	if got := p.view(t, 0); got.Peers[0].State != "unauthorized" || got.Peers[0].Snapshot != nil {
		t.Fatal("terminal-only receiving grant exposed project data")
	}
	// Existing terminal access continues independently of the new permission.
	requireStatus(t, request(p.ah, "GET", "/api/servers/remote/sessions", "", token(t, p.a, "owner")), 200)
	p.share(t, true)
	target := p.remote[1].Subject
	body := directoryJSON(plane.Request{ID: "private-peer-handoff", Action: "message", Kind: "handoff", Target: &target, Text: "private-inbox-do-not-export"})
	requireStatus(t, request(p.b.CoordinationAgentHandler(), "POST", "/act", body, p.remote[0].Capability), 200)
	requireStatus(t, request(p.b.CoordinationAgentHandler(), "POST", "/act", directoryJSON(plane.Request{ID: "shared-peer-file-claim", Action: "claim", Kind: "file", Resource: "src/shared.go"}), p.remote[0].Capability), 200)
	p.refresh(t)
	view := p.view(t, 0)
	if len(view.Peers) != 1 || view.Peers[0].State != "current" || view.Peers[0].Mode != "read-only" || view.Peers[0].Snapshot == nil {
		t.Fatal("authorized peer context was not available")
	}
	snapshot := view.Peers[0].Snapshot
	pin := pinnedPeer(t, p.a).Identity
	if !snapshot.Valid(pin.NodeID, pin.Owner, view.Self.Project.ID) || len(snapshot.Members) != 2 || len(snapshot.Claims) != 1 {
		t.Fatal("project/account projection incorrect")
	}
	for _, member := range snapshot.Members {
		if member.Ref.Node == view.Self.Ref.Node || member.Ref.Owner != "owner" {
			t.Fatal("remote identity flattened or foreign account leaked")
		}
	}
	data, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{"private-inbox-do-not-export", `"inbox"`, `"outbox"`, "wmc1.", p.secret} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("snapshot exported private messaging/credentials")
		}
	}
	if len(p.view(t, 2).Peers) != 0 {
		t.Fatal("another local account borrowed the peer credential")
	}
	jobs, err := p.a.coordinationPeerJobs([]plane.MemberInput{view.Self.MemberInput})
	if err != nil || len(jobs) != 1 {
		t.Fatal("missing scoped project job", err)
	}
	wrong := jobs[0]
	wrong.project = "unrelated-project"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unrelated, state := p.a.fetchCoordinationSnapshot(ctx, wrong)
	if state != "current" || len(unrelated.Members) != 0 || len(unrelated.Claims) != 0 {
		t.Fatal("unrelated project was inferred from matching folders")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	saw := false
	proven := map[string]bool{}
	for _, event := range p.paths {
		if event.path == nodeIdentityPath && !event.bearer {
			proven[event.address] = true
		}
		if event.path == coordinationSnapshotPath && event.bearer {
			if !proven[event.address] {
				t.Fatal("snapshot credential was not preceded by same-connection identity proof")
			}
			saw = true
		}
	}
	if !saw {
		t.Fatal("no real authenticated TLS snapshot exchange")
	}
}
func TestCoordinationPeerRevocationStalenessAndPolicy(t *testing.T) {
	p := peerPair(t)
	p.mapOwner(t, "owner")
	p.share(t, true)
	p.refresh(t)
	if p.view(t, 0).Peers[0].State != "current" {
		t.Fatal("initial observation unavailable")
	}
	p.share(t, false)
	p.refresh(t)
	if got := p.view(t, 0).Peers[0]; got.State != "unauthorized" || got.Snapshot != nil {
		t.Fatal("revoked grant retained cached data")
	}
	p.share(t, true)
	requireStatus(t, request(p.bh, "PUT", "/api/session-coordination/policy", `{"enabled":false}`, token(t, p.b, "owner")), 204)
	p.refresh(t)
	if got := p.view(t, 0).Peers[0]; got.State != "disabled" || got.Snapshot != nil {
		t.Fatal("remote global disable retained data")
	}
	requireStatus(t, request(p.bh, "PUT", "/api/session-coordination/policy", `{"enabled":true}`, token(t, p.b, "owner")), 204)
	p.refresh(t)
	p.tls.Close()
	p.refresh(t)
	if got := p.view(t, 0).Peers[0]; got.State != "stale" || got.Snapshot == nil {
		t.Fatal("endpoint loss was reported as fresh or empty")
	}
	p.a.sessionCoord.peers.mu.Lock()
	for key, old := range p.a.sessionCoord.peers.values {
		expired := time.Now().Add(-time.Second)
		old.Expires = &expired
		p.a.sessionCoord.peers.values[key] = old
	}
	p.a.sessionCoord.peers.mu.Unlock()
	if got := p.view(t, 0).Peers[0]; got.State != "unavailable" || got.Snapshot != nil {
		t.Fatal("expired peer data remained usable")
	}
	p.mapOwner(t, "")
	if len(p.view(t, 0).Peers) != 0 {
		t.Fatal("local mapping revocation retained access until maintenance")
	}
}
func TestCoordinationSharingConfigurationDoesNotUpgradeLegacyClients(t *testing.T) {
	p := peerPair(t)
	p.mapOwner(t, "owner")
	p.share(t, true)
	// An old editor omitting the new fields preserves only the same scope.
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Renamed", "url": p.tls.URL}), token(t, p.a, "owner")), 204)
	if pinnedPeer(t, p.a).CoordinationOwner != "owner" {
		t.Fatal("legacy edit revoked explicit mapping")
	}
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", `{"owner":"owner"}`, token(t, p.b, "owner")), 204)
	directory, err := p.b.serverDirectory()
	if err != nil || !directory.Grants[0].Coordination {
		t.Fatal("legacy edit changed same-owner permission")
	}
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", `{"owner":"other"}`, token(t, p.b, "owner")), 204)
	directory, err = p.b.serverDirectory()
	if err != nil || directory.Grants[0].Coordination {
		t.Fatal("owner change silently regranted coordination")
	}
	// A direct terminal credential cannot use plaintext or substitute a browser JWT.
	directory.Grants[0].Coordination = true
	federationConfig(t, p.b, directory)
	requireStatus(t, request(p.bh, "POST", coordinationSnapshotPath, `{"project_id":"project"}`, p.secret), 403)
	requireStatus(t, request(p.bh, "POST", coordinationSnapshotPath, `{"project_id":"project"}`, token(t, p.b, "owner")), 401)
}

func TestCoordinationPeerDifferentAccountMappingAndMCP(t *testing.T) {
	p := peerPairHook(t, "other", nil)
	p.mapOwner(t, "owner")
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", `{"owner":"other","coordination":true}`, token(t, p.b, "owner")), 204)
	p.refresh(t)
	view := p.view(t, 0)
	if len(view.Peers) != 1 || view.Peers[0].Snapshot == nil || view.Peers[0].Owner != "other" || len(view.Peers[0].Snapshot.Members) != 1 || view.Self.Ref.Owner != "owner" {
		t.Fatal("local and receiving accounts were conflated")
	}
	file := filepath.Join(t.TempDir(), "access.json")
	if err := coordinationclient.WriteAccess(file, p.access[0]); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", p.access[0].Subject.Session)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	one, two := mcp.NewInMemoryTransports()
	server, err := coordinationmcp.NewServer().Connect(ctx, one, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "scoped-peer-context", Version: "1"}, nil).Connect(ctx, two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_view", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal("real peer context failed MCP schema/transport", err)
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), `"peers"`) || !strings.Contains(string(data), `"read-only"`) || strings.Contains(string(data), p.secret) || strings.Contains(string(data), "wmc1.") {
		t.Fatal("MCP peer context missing or secret")
	}
	resource := "node:" + view.Peers[0].Node + ":heavy"
	result, err = client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: map[string]any{"request_id": "reject-remote-reservation", "action": "reserve", "resource": resource}})
	if err != nil || !result.IsError {
		t.Fatal("local tool acquired remote reservation authority")
	}
	if p.view(t, 0).Resources[0].Occupied {
		t.Fatal("remote intent accidentally acquired local resource")
	}
	if len(p.view(t, 2).Peers) != 0 {
		t.Fatal("same-name local account borrowed mapped account scope")
	}
}

func TestCoordinationLateSnapshotCannotResurrectRefusedScope(t *testing.T) {
	var delayed atomic.Bool
	admitted, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	p := peerPairHook(t, "owner", func(real http.Handler, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == coordinationSnapshotPath && delayed.CompareAndSwap(false, true) {
			captured := httptest.NewRecorder()
			real.ServeHTTP(captured, r)
			close(admitted)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			for name, values := range captured.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(captured.Code)
			_, _ = w.Write(captured.Body.Bytes())
			return
		}
		real.ServeHTTP(w, r)
	})
	p.mapOwner(t, "owner")
	p.share(t, true)
	snapshots, err := p.a.coordinationSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.a.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	inputs := p.a.coordinationInputs(node, snapshots)
	done := make(chan error, 1)
	go func() { done <- p.a.refreshCoordinationPeers(context.Background(), inputs) }()
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("first snapshot was not admitted")
	}
	p.share(t, false)
	if err := p.a.refreshCoordinationPeers(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	if got := p.view(t, 0).Peers[0]; got.State != "unauthorized" || got.Snapshot != nil {
		t.Fatal("new refusal was not recorded")
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old fetch did not finish")
	}
	if got := p.view(t, 0).Peers[0]; got.State != "unauthorized" || got.Snapshot != nil {
		t.Fatal("late response resurrected revoked context")
	}
}

func TestCoordinationLateSnapshotCannotResurrectRemovedMapping(t *testing.T) {
	var delayed atomic.Bool
	admitted, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	p := peerPairHook(t, "owner", func(real http.Handler, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == coordinationSnapshotPath && delayed.CompareAndSwap(false, true) {
			captured := httptest.NewRecorder()
			real.ServeHTTP(captured, r)
			close(admitted)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			for name, values := range captured.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(captured.Code)
			_, _ = w.Write(captured.Body.Bytes())
			return
		}
		real.ServeHTTP(w, r)
	})
	p.mapOwner(t, "owner")
	p.share(t, true)
	snapshots, err := p.a.coordinationSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.a.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	inputs := p.a.coordinationInputs(node, snapshots)
	done := make(chan error, 1)
	go func() { done <- p.a.refreshCoordinationPeers(context.Background(), inputs) }()
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("first snapshot was not admitted")
	}
	// Revoke while the first response is still in flight, before there is any
	// cached value to prune. An old request must not populate the new mapping.
	p.mapOwner(t, "")
	if err := p.a.refreshCoordinationPeers(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	if len(p.view(t, 0).Peers) != 0 {
		t.Fatal("removed mapping still exposed a peer")
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old fetch did not finish")
	}
	p.mapOwner(t, "owner")
	if got := p.view(t, 0).Peers; len(got) != 1 || got[0].State != "pending" || got[0].Snapshot != nil {
		t.Fatal("late response populated a reactivated mapping before fresh verification")
	}
	p.refresh(t)
	if got := p.view(t, 0).Peers; len(got) != 1 || got[0].State != "current" || got[0].Snapshot == nil {
		t.Fatal("fresh verification did not restore the reactivated mapping")
	}
}

func TestCoordinationLateSnapshotCannotSurviveGlobalDisable(t *testing.T) {
	for _, mode := range []string{"maintenance", "policy-api", "direct-refresh"} {
		t.Run(mode, func(t *testing.T) {
			var armed, delayed atomic.Bool
			ready, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			p := peerPairHook(t, "owner", func(next http.Handler, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == coordinationSnapshotPath && armed.Load() && delayed.CompareAndSwap(false, true) {
					captured := httptest.NewRecorder()
					next.ServeHTTP(captured, r)
					if captured.Code != 200 {
						t.Error("initial snapshot was refused")
					}
					close(ready)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					for name, values := range captured.Header() {
						w.Header()[name] = values
					}
					w.WriteHeader(captured.Code)
					_, _ = w.Write(captured.Body.Bytes())
					return
				}
				next.ServeHTTP(w, r)
			})
			p.a.sessionCoord.cancel()
			<-p.a.sessionCoord.done
			p.a.sessionCoord.cancel = nil
			p.mapOwner(t, "owner")
			p.share(t, true)
			p.refresh(t)
			if got := p.view(t, 0).Peers; len(got) != 1 || got[0].State != "current" {
				t.Fatal("initial peer context missing", got)
			}
			node, _ := p.a.coordinationNode()
			inputs := mustCoordinationInputs(t, p.a, node)
			armed.Store(true)
			done := make(chan error, 1)
			go func() { done <- p.a.refreshCoordinationPeers(context.Background(), inputs) }()
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("delayed snapshot did not arrive")
			}
			if mode == "policy-api" {
				requireStatus(t, request(p.ah, "PUT", "/api/session-coordination/policy", `{"enabled":false}`, token(t, p.a, "owner")), 204)
			} else {
				if err := p.a.sessionCoord.service.SetEnabled(node, false); err != nil {
					t.Fatal(err)
				}
				if mode == "direct-refresh" {
					if err := p.a.refreshCoordinationPeers(context.Background(), inputs); err != nil {
						t.Fatal(err)
					}
				} else {
					p.refresh(t) // Observe persisted policy without starting another network request.
				}
			}
			once.Do(func() { close(release) })
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("old snapshot did not finish")
			}
			requireStatus(t, request(p.ah, "PUT", "/api/session-coordination/policy", `{"enabled":true}`, token(t, p.a, "owner")), 204)
			if got := p.view(t, 0).Peers; len(got) != 1 || got[0].State != "pending" || got[0].Snapshot != nil {
				t.Fatal("pre-disable context survived without a fresh peer observation", got)
			}
			p.refresh(t)
			if got := p.view(t, 0).Peers; len(got) != 1 || got[0].State != "current" || got[0].Snapshot == nil {
				t.Fatal("fresh peer observation did not recover", got)
			}
		})
	}
}
