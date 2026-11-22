package httpapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
)

// These explicit opt-in roles exercise the real API on two hosts. They are
// protocol/component qualification, not discovery or real-agent acceptance.
type nodeNetworkConnectionKey struct{}

type nodeNetworkManifest struct {
	Original    string `json:"original"`
	Address     string `json:"address"`
	MovedPort   int    `json:"moved_port"`
	NodeID      string `json:"node_id"`
	Certificate string `json:"certificate"`
}

func nodeNetworkSecret(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "credential"))
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSpace(string(data))
	if !validSavedCredential(secret) {
		t.Fatal("invalid isolated fixture credential")
	}
	return secret
}
func TestNodeIdentityNativeNetworkServer(t *testing.T) {
	if os.Getenv("WEBMUX_NODE_NETWORK_ROLE") != "server" {
		t.Skip("explicit isolated two-host protocol qualification only")
	}
	dir := os.Getenv("WEBMUX_NODE_NETWORK_DIR")
	host := os.Getenv("WEBMUX_NODE_NETWORK_BIND")
	address := net.ParseIP(host)
	if dir == "" || address == nil || address.IsLoopback() || address.IsUnspecified() {
		t.Fatal("explicit non-loopback fixture bind address and directory required")
	}
	secret := nodeNetworkSecret(t, dir)
	var b *Server
	var bh http.Handler
	var nativeAccess []coordinationclient.Access
	if os.Getenv("WEBMUX_COORDINATION_PEER_NETWORK") == "1" {
		deterministicCoordinationGit(t)
		b, bh, _, _, nativeAccess = coordinationAPI(t)
	} else {
		b, bh = fixture(t, "local")
		federationAccounts(t, b)
	}
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "qualification", Owner: "owner", Credential: secret}}})
	policy, err := discovery.LoadNodePolicy(b.store)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "WebMux isolated node qualification"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"webmux-node-qualification.local"}, IPAddresses: []net.IP{address}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	config := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, NextProtos: []string{"http/1.1"}}
	original, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	moved, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Close()
	stopped := make(chan struct{})
	var stop sync.Once
	var mu sync.Mutex
	proofs := map[net.Conn]bool{}
	proofCount, bearerCount := 0, 0
	messageAdmits, messageStatuses := 0, 0
	overlapAdmits, overlapStatuses := 0, 0
	lostMessageReply := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, ok := r.Context().Value(nodeNetworkConnectionKey{}).(net.Conn)
		if !ok {
			t.Error("missing native connection witness")
			w.WriteHeader(500)
			return
		}
		if r.URL.Path == nodeIdentityPath {
			if r.Header.Get("Authorization") != "" {
				t.Error("bearer arrived in an identity proof")
			}
			proof := httptest.NewRecorder()
			bh.ServeHTTP(proof, r)
			if proof.Code == 200 {
				mu.Lock()
				proofs[connection] = true
				proofCount++
				mu.Unlock()
			}
			for name, values := range proof.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(proof.Code)
			_, _ = w.Write(proof.Body.Bytes())
			return
		}
		if r.Header.Get("Authorization") != "" {
			mu.Lock()
			proven := proofs[connection]
			bearerCount++
			mu.Unlock()
			if !proven {
				t.Error("bearer arrived before same-connection proof")
				w.WriteHeader(403)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/_qualification/") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
				w.WriteHeader(401)
				return
			}
			switch r.URL.Path {
			case "/_qualification/move":
				_ = original.Close()
			case "/_qualification/share-coordination", "/_qualification/revoke-coordination":
				if os.Getenv("WEBMUX_COORDINATION_PEER_NETWORK") != "1" {
					w.WriteHeader(404)
					return
				}
				directory, err := b.serverDirectory()
				if err != nil {
					w.WriteHeader(500)
					return
				}
				directory.Grants[0].Coordination = r.URL.Path == "/_qualification/share-coordination"
				if !directory.Grants[0].Coordination {
					directory.Grants[0].CoordinationMessages = false
				}
				if err = b.store.WriteConfig("servers.yaml", directory); err != nil {
					w.WriteHeader(500)
					return
				}
			case "/_qualification/allow-messages":
				if os.Getenv("WEBMUX_COORDINATION_MESSAGE_NETWORK") != "1" || !plane.ValidSenderScope(r.URL.Query().Get("source_node"), "owner") {
					w.WriteHeader(400)
					return
				}
				directory, err := b.serverDirectory()
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				directory.Grants[0].Coordination = true
				directory.Grants[0].CoordinationMessages = true
				directory.Grants[0].SourceNode = r.URL.Query().Get("source_node")
				directory.Grants[0].SourceOwner = "owner"
				if err = b.store.WriteConfig("servers.yaml", directory); err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
			case "/_qualification/complete-message":
				if os.Getenv("WEBMUX_COORDINATION_MESSAGE_NETWORK") != "1" {
					w.WriteHeader(404)
					return
				}
				id, target := r.URL.Query().Get("record"), r.URL.Query().Get("session")
				found := false
				for _, access := range nativeAccess {
					if access.Subject.Session == target && access.Subject.Owner == "owner" {
						viewResponse := request(b.CoordinationAgentHandler(), "GET", "/view", "", access.Capability)
						var view plane.View
						if json.Unmarshal(viewResponse.Body.Bytes(), &view) != nil {
							t.Error("invalid recipient view")
							w.WriteHeader(500)
							return
						}
						count := 0
						for _, message := range view.Inbox {
							if message.ID == id {
								count++
								text, err := b.sessions.Scrollback("owner", target)
								if err != nil || strings.Contains(text, message.Text) {
									t.Error("remote message entered terminal")
									w.WriteHeader(500)
									return
								}
							}
						}
						if count != 1 {
							t.Error("remote inbox admission not unique", count)
							w.WriteHeader(500)
							return
						}
						for _, action := range []string{"delivered", "acknowledge", "complete"} {
							result := request(b.CoordinationAgentHandler(), "POST", "/act", directoryJSON(plane.Request{ID: "native-remote-" + id + "-" + action, Action: action, Record: id}), access.Capability)
							if result.Code != 200 || !strings.Contains(result.Body.String(), "recorded") {
								t.Error("explicit recipient receipt failed")
								w.WriteHeader(500)
								return
							}
						}
						found = true
						break
					}
				}
				if !found {
					w.WriteHeader(404)
					return
				}
			case "/_qualification/claim-overlap":
				if os.Getenv("WEBMUX_COORDINATION_CLAIM_NETWORK") != "1" {
					w.WriteHeader(404)
					return
				}
				found := false
				for _, access := range nativeAccess {
					if access.Subject.Owner == "owner" && access.Subject.Session == r.URL.Query().Get("session") {
						result := request(b.CoordinationAgentHandler(), "POST", "/act", directoryJSON(plane.Request{ID: "native-overlap-directory", Action: "claim", Kind: "directory", Resource: "native-overlap"}), access.Capability)
						if result.Code != 200 || !strings.Contains(result.Body.String(), "claimed") {
							t.Error("native remote claim failed")
							w.WriteHeader(500)
							return
						}
						found = true
						break
					}
				}
				if !found {
					w.WriteHeader(404)
					return
				}
			case "/_qualification/stop":
				defer stop.Do(func() { close(stopped) })
			default:
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(204)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		if os.Getenv("WEBMUX_COORDINATION_MESSAGE_NETWORK") == "1" && r.URL.Path == coordinationMessagesPath {
			data, err := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			var exchange coordinationMessageExchange
			if json.Unmarshal(data, &exchange) != nil {
				w.WriteHeader(400)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			mu.Lock()
			drop := false
			if exchange.Envelope.Kind == "overlap" {
				if exchange.Operation == "admit" {
					overlapAdmits++
				} else {
					overlapStatuses++
				}
			} else if exchange.Operation == "admit" {
				messageAdmits++
				if !lostMessageReply {
					lostMessageReply = true
					drop = true
				}
			} else {
				messageStatuses++
			}
			mu.Unlock()
			if drop {
				recorded := httptest.NewRecorder()
				bh.ServeHTTP(recorded, r)
				if recorded.Code != 200 {
					t.Error("native message admission failed", recorded.Code)
					w.WriteHeader(500)
					return
				}
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.Close()
				return
			}
		}
		bh.ServeHTTP(w, r)
	})
	connectionContext := func(ctx context.Context, conn net.Conn) context.Context {
		return context.WithValue(ctx, nodeNetworkConnectionKey{}, conn)
	}
	first, second := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ConnContext: connectionContext}, &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ConnContext: connectionContext}
	defer first.Close()
	defer second.Close()
	go func() { _ = first.Serve(tls.NewListener(original, config)) }()
	go func() { _ = second.Serve(tls.NewListener(moved, config)) }()
	manifest := nodeNetworkManifest{Original: "https://" + original.Addr().String(), Address: host, MovedPort: moved.Addr().(*net.TCPAddr).Port, NodeID: policy.ID, Certificate: string(certPEM)}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	temporary := filepath.Join(dir, "manifest.tmp")
	if err = os.WriteFile(temporary, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(temporary, filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(90 * time.Second):
		t.Fatal("isolated protocol fixture timed out")
	}
	mu.Lock()
	defer mu.Unlock()
	if proofCount < 4 || bearerCount < 4 {
		t.Fatal("incomplete wire workload", proofCount, bearerCount)
	}
	if os.Getenv("WEBMUX_COORDINATION_MESSAGE_NETWORK") == "1" {
		if messageAdmits != 1 || messageStatuses < 1 || !lostMessageReply {
			t.Fatal("lost message reply did not reconcile through status", messageAdmits, messageStatuses)
		}
		if os.Getenv("WEBMUX_COORDINATION_CLAIM_NETWORK") == "1" {
			if overlapAdmits != 1 || overlapStatuses < 1 {
				t.Fatal("native overlap notice workload incomplete", overlapAdmits, overlapStatuses)
			}
			t.Logf("native overlap: %d admit and %d status requests; original claim-cycle identity and explicit completion", overlapAdmits, overlapStatuses)
		}
		t.Logf("native remote messaging: one admit request, %d receipt status requests, deliberately dropped first committed reply, one recipient inbox record, explicit delivered/acknowledged/completed and no terminal input", messageStatuses)
	}
	t.Logf("native receiver observed %d proofs and %d requests with bearer credentials, all on proven TLS connections", proofCount, bearerCount)
}

func TestNodeIdentityNativeNetworkClient(t *testing.T) {
	if os.Getenv("WEBMUX_NODE_NETWORK_ROLE") != "client" {
		t.Skip("explicit isolated two-host protocol qualification only")
	}
	dir := os.Getenv("WEBMUX_NODE_NETWORK_DIR")
	if dir == "" {
		t.Fatal("explicit fixture directory required")
	}
	secret := nodeNetworkSecret(t, dir)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest nodeNetworkManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	a, h := fixture(t, "local")
	federationAccounts(t, a)
	a.peerRoots = x509.NewCertPool()
	if !a.peerRoots.AppendCertsFromPEM([]byte(manifest.Certificate)) {
		t.Fatal("fixture certificate missing")
	}
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Native peer", URL: manifest.Original, Credential: secret}}})
	admin := token(t, a, "owner")
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	peer := pinnedPeer(t, a)
	if peer.Identity.NodeID != manifest.NodeID || peer.Identity.Owner != "owner" {
		t.Fatal("native node/account identity differs")
	}
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 200)
	control := func(p serverPeer, path string) {
		t.Helper()
		transport := http.DefaultTransport.(*http.Transport).Clone()
		defer transport.CloseIdleConnections()
		transport.Proxy = nil
		transport.ForceAttemptHTTP2 = false
		transport.DialTLSContext = a.nodePeerDialer(p, secret)
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		req, err := http.NewRequest("POST", p.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header = nodeScopeHeaders(p, secret)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != 204 {
			t.Fatal("isolated fixture control refused", response.StatusCode)
		}
	}
	control(peer, "/_qualification/move")
	candidate := discovery.Candidate{Instance: "native-qualification", ClaimedID: manifest.NodeID, Host: "webmux-node-qualification.local", Address: manifest.Address, Port: uint16(manifest.MovedPort), TLS: true, Expires: time.Now().Add(time.Minute)}
	if !a.reconcileNodePeer(context.Background(), peer, []discovery.Candidate{candidate}) {
		t.Fatal("native authenticated address migration failed")
	}
	after := pinnedPeer(t, a)
	if after.ID != peer.ID || after.Identity != peer.Identity || after.URL == peer.URL || after.DiscoveredAddress != manifest.Address {
		t.Fatal("migration changed peer/account identity")
	}
	requireStatus(t, request(h, "GET", "/api/servers/remote/sessions", "", admin), 200)
	// Reconstruct the real controller against the same retained store; its pin
	// remains enforced. This is reconstruction, not a host-reboot claim.
	restored, err := New(a.store, Options{Name: "restored"})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.peerRoots = a.peerRoots
	if pinnedPeer(t, restored) != after {
		t.Fatal("reconstruction lost verified peer state")
	}
	requireStatus(t, request(restored.Handler(), "GET", "/api/servers/remote/sessions", "", token(t, restored, "owner")), 200)
	control(after, "/_qualification/stop")
	digest := sha256.Sum256([]byte(after.Identity.NodeID + "/" + after.Identity.Owner + "/" + after.Identity.TLSKey))
	t.Log("native two-host TLS enrollment, same-connection HTTP, endpoint loss, authenticated address migration and controller reconstruction passed; identity binding digest=" + hex.EncodeToString(digest[:]) + " migrated port=" + strconv.Itoa(manifest.MovedPort))
}
