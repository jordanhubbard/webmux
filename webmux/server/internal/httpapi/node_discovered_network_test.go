package httpapi

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/discovery"
)

// This mode retains the original network workload and its deadlines. Only the
// explicit discovered workload advertises the two actual owned TLS listeners.
func startNetworkDiscoveryAdvertiser(s *Server, port int) error {
	policy, err := discovery.LoadNodePolicy(s.store)
	if err != nil {
		return err
	}
	policy.Browse, policy.Advertise = false, true
	if err := s.store.WriteConfig("node-discovery.yaml", policy); err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	s.StartNodeDiscovery(host, 0, port, "")
	if s.nodes == nil || s.nodeDiscoveryError != "" {
		return fmt.Errorf("owned native advertisement did not start: %s", s.nodeDiscoveryError)
	}
	return s.nodes.Err()
}

func qualifyDiscoveredEnrollment(t *testing.T, secret string, manifest nodeNetworkManifest) {
	t.Helper()
	a, h := fixture(t, "local")
	federationAccounts(t, a)
	a.peerRoots = x509.NewCertPool()
	if !a.peerRoots.AppendCertsFromPEM([]byte(manifest.Certificate)) {
		t.Fatal("isolated fixture trust root missing")
	}
	policy, err := discovery.LoadNodePolicy(a.store)
	if err != nil {
		t.Fatal(err)
	}
	policy.Browse, policy.Advertise = true, false
	if err := a.store.WriteConfig("node-discovery.yaml", policy); err != nil {
		t.Fatal(err)
	}
	// Give the controller an actual private HTTP listener, as required by the
	// production startup contract. Browsing and maintenance run in the server.
	controller := httptest.NewServer(h)
	defer controller.Close()
	u, err := url.Parse(controller.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	a.StartNodeDiscovery(host, port, 0, "")
	if a.nodes == nil || a.nodeDiscoveryError != "" {
		t.Fatal("production discovery did not start", a.nodeDiscoveryError)
	}
	admin := token(t, a, "owner")
	requireStatus(t, request(h, "GET", "/api/discovered-nodes", "", ""), 401)
	requireStatus(t, request(h, "GET", "/api/discovered-nodes", "", token(t, a, "other")), 403)
	find := func(wantedPort int) discovery.Candidate {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			response := request(h, "GET", "/api/discovered-nodes", "", admin)
			requireStatus(t, response, 200)
			var view struct {
				Started    bool                  `json:"started"`
				Candidates []discovery.Candidate `json:"candidates"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || !view.Started {
				t.Fatal("native discovery unavailable", err)
			}
			for _, candidate := range view.Candidates {
				if candidate.ClaimedID == manifest.NodeID && int(candidate.Port) == wantedPort && candidate.TLS && candidate.Address == manifest.Address {
					return candidate
				}
			}
			// Keep both discovery windows below the unchanged 300/minute
			// API budget, including when no advertisement arrives. A 25ms
			// loop can exhaust that budget before the eight-second bound.
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("actual discovery API did not observe the exact owned TLS listener within eight seconds")
		return discovery.Candidate{}
	}
	original, err := url.Parse(manifest.Original)
	if err != nil {
		t.Fatal(err)
	}
	originalPort, err := strconv.Atoi(original.Port())
	if err != nil {
		t.Fatal(err)
	}
	observed := find(originalPort)
	directory, err := a.serverDirectory()
	if err != nil || len(directory.Servers) != 0 {
		t.Fatal("untrusted discovery enrolled a peer", err)
	}
	// Configure only the address observed through the real API. The manifest's
	// expected node/ports are assertions, never injected discovery candidates.
	origin := "https://" + net.JoinHostPort(observed.Address, strconv.Itoa(int(observed.Port)))
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Discovered native peer", URL: origin, Credential: secret}}})
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	before := pinnedPeer(t, a)
	if before.Identity.NodeID != manifest.NodeID || before.Identity.Owner != "owner" {
		t.Fatal("enrollment did not prove the observed node/account")
	}
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 200)
	control := func(peer serverPeer, path string) {
		t.Helper()
		transport := http.DefaultTransport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		transport.Proxy, transport.ForceAttemptHTTP2 = nil, false
		transport.DialTLSContext = a.nodePeerDialer(peer, secret)
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		req, err := http.NewRequest("POST", peer.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header = nodeScopeHeaders(peer, secret)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != 204 {
			t.Fatal("owned control refused", response.StatusCode)
		}
	}
	control(before, "/_qualification/move")
	_ = find(manifest.MovedPort)
	// Do not call reconcileNodePeer or supply a candidate. The real ten-second
	// server maintenance timer must observe, prove and persist the migration.
	deadline := time.Now().Add(20 * time.Second)
	after := before
	for time.Now().Before(deadline) {
		after = pinnedPeer(t, a)
		if after.URL != before.URL {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	migrated, err := url.Parse(after.URL)
	if err != nil || after.ID != before.ID || after.Identity != before.Identity || migrated.Port() != strconv.Itoa(manifest.MovedPort) || after.DiscoveredAddress != observed.Address {
		t.Fatal("background discovery did not retain identity and migrate the endpoint", err)
	}
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 200)
	restored, err := New(a.store, Options{Name: "restored discovered controller"})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.peerRoots = a.peerRoots
	if pinnedPeer(t, restored) != after {
		t.Fatal("reconstruction lost enrolled peer identity")
	}
	requireStatus(t, request(restored.Handler(), "GET", "/api/servers/remote/sessions", "", token(t, restored, "owner")), 200)
	control(after, "/_qualification/stop")
	t.Log("actual mDNS/API discovery, no implicit enrollment, authenticated enrollment, background endpoint migration and retained identity after controller reconstruction passed")
}
