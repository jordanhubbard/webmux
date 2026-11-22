package httpapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func deterministicCoordinationGit(t *testing.T) {
	t.Helper()
	// Both hosts create the same empty-root history through real Git, then use
	// independent repositories/worktrees and broker-owned ordinary child PTYs.
	t.Setenv("GIT_AUTHOR_DATE", "2026-11-21T00:00:00+00:00")
	t.Setenv("GIT_COMMITTER_DATE", "2026-11-21T00:00:00+00:00")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}
func TestCoordinationPeerNativeNetworkClient(t *testing.T) {
	if os.Getenv("WEBMUX_NODE_NETWORK_ROLE") != "client" || os.Getenv("WEBMUX_COORDINATION_PEER_NETWORK") != "1" {
		t.Skip("explicit isolated two-host coordination component only")
	}
	deterministicCoordinationGit(t)
	dir := os.Getenv("WEBMUX_NODE_NETWORK_DIR")
	secret := nodeNetworkSecret(t, dir)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest nodeNetworkManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	a, ah, agent, _, access := coordinationAPI(t)
	a.peerRoots = x509.NewCertPool()
	if !a.peerRoots.AppendCertsFromPEM([]byte(manifest.Certificate)) {
		t.Fatal("fixture TLS certificate missing")
	}
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Native coordination peer", URL: manifest.Original, Credential: secret}}})
	admin := token(t, a, "owner")
	requireStatus(t, request(ah, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	requireStatus(t, request(ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Native coordination peer", "url": manifest.Original, "coordination_owner": "owner"}), admin), 204)
	view := func() plane.View {
		response := request(agent, "GET", "/view", "", access[0].Capability)
		requireStatus(t, response, 200)
		var out plane.View
		if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	waitState := func(state string) plane.View {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			observed := view()
			if len(observed.Peers) == 1 && observed.Peers[0].State == state {
				return observed
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("native background refresh did not reach expected peer state", state)
		return plane.View{}
	}
	if got := waitState("unauthorized"); got.Peers[0].Snapshot != nil {
		t.Fatal("native terminal-only grant disclosed coordination")
	}
	control := func(path string) {
		peer := pinnedPeer(t, a)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.ForceAttemptHTTP2 = false
		transport.DialTLSContext = a.nodePeerDialer(peer, secret)
		defer transport.CloseIdleConnections()
		req, _ := http.NewRequestWithContext(ctx, "POST", manifest.Original+path, nil)
		req.Header = nodeScopeHeaders(peer, secret)
		response, err := (&http.Client{Transport: transport}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 204 {
			t.Fatal("native fixture control refused", response.StatusCode)
		}
	}
	control("/_qualification/share-coordination")
	current := waitState("current")
	if len(current.Peers) != 1 || current.Peers[0].State != "current" || current.Peers[0].Snapshot == nil || len(current.Peers[0].Snapshot.Members) != 2 || current.Self == nil || current.Self.Project == nil || !current.Peers[0].Snapshot.Valid(manifest.NodeID, "owner", current.Self.Project.ID) {
		t.Fatal("native cross-host verified project/account discovery failed")
	}
	foreign := request(agent, "GET", "/view", "", access[2].Capability)
	requireStatus(t, foreign, 200)
	var isolated plane.View
	if json.Unmarshal(foreign.Body.Bytes(), &isolated) != nil || len(isolated.Peers) != 0 {
		t.Fatal("foreign account borrowed native peer credential")
	}
	control("/_qualification/revoke-coordination")
	if got := waitState("unauthorized").Peers[0]; got.Snapshot != nil {
		t.Fatal("native revocation retained data")
	}
	control("/_qualification/stop")
	t.Log("native two-host background pinned HTTPS project discovery passed (no browser or manual refresh): terminal-only refusal, explicit node/account sharing, two same-project remote PTY members, foreign-account isolation and revocation; no native coding-agent or remote-mutation acceptance")
}
