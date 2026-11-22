package httpapi

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
)

type nodeWireEvent struct{ path, credential, address string }

func nodePair(t *testing.T, observe func(*http.Request)) (*Server, *Server, http.Handler, *httptest.Server, string) {
	t.Helper()
	a, h := fixture(t, "local")
	b, bh := fixture(t, "local")
	federationAccounts(t, a)
	federationAccounts(t, b)
	secret := strings.Repeat("node-owner-grant-", 3)
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "owner", Credential: secret}}})
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observe != nil {
			observe(r)
		}
		bh.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	a.peerRoots = x509.NewCertPool()
	a.peerRoots.AddCert(remote.Certificate())
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote", URL: remote.URL, Credential: secret}}})
	return a, b, h, remote, secret
}
func pinnedPeer(t *testing.T, s *Server) serverPeer {
	t.Helper()
	d, err := s.serverDirectory()
	if err != nil || len(d.Servers) != 1 {
		t.Fatal(d, err)
	}
	return d.Servers[0]
}

func TestNodeIdentityEnrollmentBindsSameTLSConnection(t *testing.T) {
	var mu sync.Mutex
	var events []nodeWireEvent
	a, b, h, _, secret := nodePair(t, func(r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, nodeWireEvent{r.URL.Path, r.Header.Get("Authorization"), r.RemoteAddr})
	})
	admin := token(t, a, "owner")
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	pin := pinnedPeer(t, a).Identity
	if !validNodePin(pin) || pin.Owner != "owner" {
		t.Fatal(pin)
	}
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 200)
	mu.Lock()
	got := append([]nodeWireEvent(nil), events...)
	mu.Unlock()
	if len(got) != 3 || got[0].path != nodeIdentityPath || got[1].path != nodeIdentityPath || got[2].path != "/api/federation/sessions" {
		t.Fatal(got)
	}
	if got[0].credential != "" || got[1].credential != "" || got[2].credential != "Bearer "+secret || got[1].address != got[2].address {
		t.Fatal("proof/bearer connection boundary incorrect", got)
	}
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	if pinnedPeer(t, a).Identity != pin {
		t.Fatal("verification replaced pin")
	}
	public := request(h, "GET", "/api/server-directory", "", admin)
	requireStatus(t, public, 200)
	if strings.Contains(public.Body.String(), secret) || !strings.Contains(public.Body.String(), pin.NodeID) {
		t.Fatal("pin metadata or secret boundary incorrect")
	}
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "other", Credential: secret}}})
	mu.Lock()
	events = nil
	mu.Unlock()
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 502)
	mu.Lock()
	defer mu.Unlock()
	for _, event := range events {
		if event.credential != "" {
			t.Fatal("credential sent after receiving account changed")
		}
	}
	if pinnedPeer(t, a).Identity != pin {
		t.Fatal("failed proof replaced original pin")
	}
}

func TestNodeIdentityTLSForwardingAndReplayAreRefused(t *testing.T) {
	a, _, _, remote, secret := nodePair(t, nil)
	original := pinnedPeer(t, a)
	var mu sync.Mutex
	var saved []byte
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		forwarded, err := http.NewRequest("POST", remote.URL+nodeIdentityPath, strings.NewReader(string(body)))
		if err != nil {
			t.Error(err)
			return
		}
		forwarded.Header.Set("Content-Type", "application/json")
		response, err := remote.Client().Do(forwarded)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		mu.Lock()
		saved = append([]byte(nil), data...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(data)
	}))
	defer relay.Close()
	a.peerRoots.AddCert(relay.Certificate())
	forged := original
	forged.URL = relay.URL
	connection, _, err := dialNodePeer(context.Background(), forged, secret, a.peerRoots)
	if connection != nil {
		connection.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "channel proof") {
		t.Fatal("relayed identity accepted", err)
	}
	replay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		data := append([]byte(nil), saved...)
		mu.Unlock()
		_, _ = w.Write(data)
	}))
	defer replay.Close()
	a.peerRoots.AddCert(replay.Certificate())
	forged.URL = replay.URL
	connection, _, err = dialNodePeer(context.Background(), forged, secret, a.peerRoots)
	if connection != nil {
		connection.Close()
	}
	if err == nil {
		t.Fatal("old challenge response accepted")
	}
}

func TestNodeIdentityPinAndCredentialChangesFailClosed(t *testing.T) {
	for _, change := range []string{"node", "key", "credential", "trust"} {
		t.Run(change, func(t *testing.T) {
			var mu sync.Mutex
			bearer := false
			a, _, h, _, _ := nodePair(t, func(r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					mu.Lock()
					bearer = true
					mu.Unlock()
				}
			})
			admin := token(t, a, "owner")
			requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
			d, err := a.serverDirectory()
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "node":
				d.Servers[0].Identity.NodeID = strings.Repeat("b", 32)
			case "key":
				d.Servers[0].Identity.TLSKey = strings.Repeat("b", 64)
			case "credential":
				d.Servers[0].Credential = strings.Repeat("wrong-credential", 3)
			case "trust":
				a.peerRoots = x509.NewCertPool()
			}
			federationConfig(t, a, d)
			requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 502)
			mu.Lock()
			defer mu.Unlock()
			if bearer {
				t.Fatal("bearer sent to an unverified node")
			}
		})
	}
}

func TestNodeIdentityEnrollmentAuthorizationAndConcurrentConfig(t *testing.T) {
	a, _, h, _, secret := nodePair(t, nil)
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", ""), 401)
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", token(t, a, "other")), 403)
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", secret), 401)
	old := pinnedPeer(t, a)
	connection, pin, err := dialNodePeer(context.Background(), old, secret, a.peerRoots)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	next := old
	next.Identity = pin
	edited := old
	edited.Name = "Concurrent edit"
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{edited}})
	if err = a.commitNodePeer(old, next, secret); err == nil || pinnedPeer(t, a).Name != "Concurrent edit" {
		t.Fatal("stale verification overwrote edit", err)
	}
	federationConfig(t, a, serverDirectory{Version: 1})
	if err = a.commitNodePeer(old, next, secret); err == nil {
		t.Fatal("removed peer resurrected")
	}
	duplicate := next
	duplicate.ID = "already-enrolled"
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{old, duplicate}})
	if err = a.commitNodePeer(old, next, secret); err == nil {
		t.Fatal("same node/account enrolled twice")
	}
	duplicate.Identity.Owner = "other"
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{old, duplicate}})
	if err = a.commitNodePeer(old, next, secret); err != nil {
		t.Fatal("distinct node accounts merged", err)
	}
}

func TestNodeIdentityEditsPreservePinAndManualHTTP(t *testing.T) {
	a, _, h, _, secret := nodePair(t, nil)
	admin := token(t, a, "owner")
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	original := pinnedPeer(t, a)
	body := map[string]any{"name": "Renamed", "url": "https://moved.local:8443", "credential": ""}
	requireStatus(t, request(h, "PUT", "/api/servers/remote", directoryJSON(body), admin), 204)
	if pinnedPeer(t, a).Identity != original.Identity {
		t.Fatal("editing erased identity pin")
	}
	body["url"] = "http://moved.local:8080"
	body["allow_http"] = true
	requireStatus(t, request(h, "PUT", "/api/servers/remote", directoryJSON(body), admin), 400)
	requireStatus(t, request(h, "PUT", "/api/servers/plain", directoryJSON(map[string]any{"name": "Manual HTTP", "url": "http://localhost:8080", "allow_http": true, "credential": secret}), admin), 204)
	requireStatus(t, request(h, "POST", "/api/servers/plain/verify-identity", "{}", admin), 400)
}

func TestNodeCandidateIdentityReconciliationBoundaries(t *testing.T) {
	now := time.Now()
	p := serverPeer{ID: "remote", URL: "https://old.local:8443", Identity: nodeIdentity{NodeID: strings.Repeat("a", 32), Owner: "owner", TLSKey: strings.Repeat("b", 64)}}
	candidate := discovery.Candidate{ClaimedID: p.Identity.NodeID, Host: "new.local", Address: "fe80::123%en0", Port: 9443, TLS: true, Expires: now.Add(time.Second)}
	next, ok := candidatePeer(p, candidate, now)
	if !ok || next.URL != "https://new.local:9443" || next.DiscoveredAddress != "fe80::123%en0" || next.Identity != p.Identity {
		t.Fatal(next, ok)
	}
	for _, change := range []string{"plain", "expired", "foreign", "unscoped", "loopback", "userinfo", "no-pin"} {
		t.Run(change, func(t *testing.T) {
			q, c := p, candidate
			switch change {
			case "plain":
				c.TLS = false
			case "expired":
				c.Expires = now
			case "foreign":
				c.ClaimedID = strings.Repeat("c", 32)
			case "unscoped":
				c.Address = "fe80::1"
			case "loopback":
				c.Address = "127.0.0.1"
			case "userinfo":
				c.Host = "user@new.local"
			case "no-pin":
				q.Identity = nodeIdentity{}
			}
			if _, ok := candidatePeer(q, c, now); ok {
				t.Fatal("unsafe candidate admitted")
			}
		})
	}
}

func TestNodeIdentityWebSocketUsesProvenTLSConnection(t *testing.T) {
	a, h := fixture(t, "local")
	b, bh := fixture(t, "local")
	federationAccounts(t, a)
	federationAccounts(t, b)
	secret := strings.Repeat("node-websocket-grant-", 3)
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "owner", Credential: secret}}})
	var mu sync.Mutex
	proofAddresses := map[string]bool{}
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == nodeIdentityPath {
			mu.Lock()
			proofAddresses[r.RemoteAddr] = true
			mu.Unlock()
			bh.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		proven := proofAddresses[r.RemoteAddr]
		mu.Unlock()
		if !proven || r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("WebSocket bearer arrived without same-connection proof")
			w.WriteHeader(403)
			return
		}
		socket, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_ = socket.WriteJSON(map[string]bool{"proven": true})
		for {
			if _, _, err = socket.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer remote.Close()
	a.peerRoots = x509.NewCertPool()
	a.peerRoots.AddCert(remote.Certificate())
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote", URL: remote.URL, Credential: secret}}})
	admin := token(t, a, "owner")
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	controller := httptest.NewServer(h)
	defer controller.Close()
	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(controller.URL, "http")+"/api/servers/remote/term/fixture?token="+url.QueryEscape(admin), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	var result map[string]bool
	if err = socket.ReadJSON(&result); err != nil || !result["proven"] {
		t.Fatal(result, err)
	}
}

func TestNodeIdentityEndpointRequiresGrantProof(t *testing.T) {
	_, b, _, remote, secret := nodePair(t, nil)
	response := request(b.Handler(), "POST", nodeIdentityPath, "{}", secret)
	requireStatus(t, response, 400)
	body, _ := json.Marshal(nodeIdentityRequest{Version: 1, Challenge: strings.Repeat("a", 64), CredentialID: nodeMAC(secret, "webmux-node-credential/v1"), Proof: strings.Repeat("b", 64)})
	req, _ := http.NewRequest("POST", remote.URL+nodeIdentityPath, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)
	result, err := remote.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != 401 {
		t.Fatal("bearer without challenge proof accepted", result.StatusCode)
	}
}

func TestNodeIdentityOriginalAccountScopeAtAdmission(t *testing.T) {
	a, b, _, remote, secret := nodePair(t, nil)
	old := pinnedPeer(t, a)
	connection, pin, err := dialNodePeer(context.Background(), old, secret, a.peerRoots)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "other", Credential: secret}}})
	old.Identity = pin
	req, _ := http.NewRequest("GET", remote.URL+"/api/federation/sessions", nil)
	req.Header = nodeScopeHeaders(old, secret)
	if err = req.Write(connection); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("grant reassignment changed the pinned receiving account", response.StatusCode)
	}
}

func TestNodeIdentityDirectoryRejectsDuplicateNodeAccount(t *testing.T) {
	pin := nodeIdentity{NodeID: strings.Repeat("a", 32), Owner: "owner", TLSKey: strings.Repeat("b", 64)}
	first := serverPeer{ID: "one", Name: "One", URL: "https://one.local", Credential: strings.Repeat("credential", 4), Identity: pin}
	second := first
	second.ID = "two"
	second.Name = "Two"
	second.URL = "https://two.local"
	second.Identity.TLSKey = strings.Repeat("c", 64)
	directory := serverDirectory{Version: 1, Servers: []serverPeer{first, second}}
	if err := validateServerDirectory(directory); err == nil {
		t.Fatal("duplicate node/account admitted with another certificate key")
	}
	directory.Servers[1].Identity.Owner = "other"
	if err := validateServerDirectory(directory); err != nil {
		t.Fatal("separate account scopes were collapsed", err)
	}
}
