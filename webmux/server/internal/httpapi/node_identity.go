package httpapi

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/discovery"
)

const nodeIdentityPath = "/api/federation/node-identity"
const nodeExporterLabel = "EXPORTER-WebMux-Node-Identity-v1"

// A node pin includes the receiving account: another grant on the same server
// must not silently become the original peer. The TLS key survives renewals
// using the same key; replacing the key requires explicit enrollment again.
type nodeIdentity struct {
	NodeID string `yaml:"node_id" json:"node_id"`
	Owner  string `yaml:"owner" json:"owner"`
	TLSKey string `yaml:"tls_key_sha256" json:"tls_key_sha256"`
}

type nodeIdentityRequest struct {
	Version      int    `json:"version"`
	Challenge    string `json:"challenge"`
	CredentialID string `json:"credential_id"`
	NodeID       string `json:"expected_node_id"`
	Owner        string `json:"expected_owner"`
	Proof        string `json:"proof"`
}
type nodeIdentityResponse struct {
	Version   int    `json:"version"`
	Challenge string `json:"challenge"`
	NodeID    string `json:"node_id"`
	Owner     string `json:"owner"`
	Proof     string `json:"proof"`
}

func nodeMAC(secret, transcript string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(transcript))
	return hex.EncodeToString(mac.Sum(nil))
}
func nodeRequestMAC(secret string, r nodeIdentityRequest) string {
	return nodeMAC(secret, "webmux-node-request/v1\n"+r.Challenge+"\n"+r.NodeID+"\n"+r.Owner)
}
func nodeResponseMAC(secret string, r nodeIdentityResponse, binding []byte) string {
	return nodeMAC(secret, "webmux-node-response/v1\n"+r.Challenge+"\n"+r.NodeID+"\n"+r.Owner+"\n"+hex.EncodeToString(binding))
}
func validNodeHex(s string, size int) bool {
	if len(s) != size*2 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func validNodeOwner(owner string) bool {
	return owner != "" && len(owner) <= 256 && !strings.ContainsFunc(owner, unicode.IsControl)
}
func validNodePin(pin nodeIdentity) bool {
	return discovery.ValidNodeID(pin.NodeID) && validNodeOwner(pin.Owner) && validNodeHex(pin.TLSKey, 32)
}
func nodeMACEqual(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }

// This endpoint accepts proof of the existing owner grant, never its bearer
// credential. TLS exporter binding prevents forwarding the challenge through a
// different TLS connection and using its answer to impersonate the real node.
func (s *Server) proveNodeIdentity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.TLS == nil {
		writeError(w, 400, "Node identity requires end-to-end HTTPS")
		return
	}
	var input nodeIdentityRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Version != 1 || !validNodeHex(input.Challenge, 32) || !validNodeHex(input.CredentialID, 32) || !validNodeHex(input.Proof, 32) || (input.NodeID != "" && !discovery.ValidNodeID(input.NodeID)) || (input.Owner != "" && !validNodeOwner(input.Owner)) {
		writeError(w, 400, "Invalid node identity challenge")
		return
	}
	directory, err := s.serverDirectory()
	if err != nil {
		s.internalError(w, err)
		return
	}
	var grant serverGrant
	var secret string
	for _, g := range directory.Grants {
		credential := serverCredential(g.TokenEnv, g.Credential)
		if validSavedCredential(credential) && nodeMACEqual(input.CredentialID, nodeMAC(credential, "webmux-node-credential/v1")) && nodeMACEqual(input.Proof, nodeRequestMAC(credential, input)) && s.federationOwnerAllowed(g.Owner) {
			grant, secret = g, credential
			break
		}
	}
	if secret == "" || (input.Owner != "" && input.Owner != grant.Owner) {
		writeError(w, 401, "Node identity proof refused")
		return
	}
	policy, err := discovery.LoadNodePolicy(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if input.NodeID != "" && input.NodeID != policy.ID {
		writeError(w, 401, "Node identity proof refused")
		return
	}
	binding, err := r.TLS.ExportKeyingMaterial(nodeExporterLabel, nil, 32)
	if err != nil {
		writeError(w, 503, "TLS channel binding is unavailable")
		return
	}
	response := nodeIdentityResponse{Version: 1, Challenge: input.Challenge, NodeID: policy.ID, Owner: grant.Owner}
	response.Proof = nodeResponseMAC(secret, response, binding)
	writeJSON(w, 200, response)
}

// dialNodePeer returns the SAME TLS connection on which the grant/node/account
// proof succeeded. A separate preflight followed by an unverified connection
// would permit rebinding or forwarding attacks before bearer transmission.
func dialNodePeer(parent context.Context, peer serverPeer, secret string, roots *x509.CertPool) (_ net.Conn, identity nodeIdentity, resultErr error) {
	target, err := url.Parse(peer.URL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" {
		return nil, identity, errors.New("node identity requires HTTPS")
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	address := target.Host
	if target.Port() == "" {
		address = net.JoinHostPort(target.Hostname(), "443")
	}
	if peer.DiscoveredAddress != "" {
		port := target.Port()
		if port == "" {
			port = "443"
		}
		address = net.JoinHostPort(peer.DiscoveredAddress, port)
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: target.Hostname(), NextProtos: []string{"http/1.1"}}}
	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, identity, errors.New("cannot establish trusted node TLS connection")
	}
	connection := raw.(*tls.Conn)
	defer func() {
		if resultErr != nil {
			_ = connection.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			return nil, identity, err
		}
	}
	state := connection.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, identity, errors.New("missing node TLS certificate")
	}
	key := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
	identity.TLSKey = hex.EncodeToString(key[:])
	if peer.Identity != (nodeIdentity{}) && identity.TLSKey != peer.Identity.TLSKey {
		return nil, identity, errors.New("node TLS identity changed")
	}
	challenge := make([]byte, 32)
	if _, err = rand.Read(challenge); err != nil {
		return nil, identity, err
	}
	input := nodeIdentityRequest{Version: 1, Challenge: hex.EncodeToString(challenge), CredentialID: nodeMAC(secret, "webmux-node-credential/v1"), NodeID: peer.Identity.NodeID, Owner: peer.Identity.Owner}
	input.Proof = nodeRequestMAC(secret, input)
	encoded, _ := json.Marshal(input)
	endpoint := *target
	endpoint.Path = nodeIdentityPath
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint.String(), strings.NewReader(string(encoded)))
	if err != nil {
		return nil, identity, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err = request.Write(connection); err != nil {
		return nil, identity, errors.New("cannot request node identity")
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, identity, errors.New("cannot read node identity")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 || response.StatusCode != 200 || response.Close {
		return nil, identity, errors.New("node identity proof refused")
	}
	var proof nodeIdentityResponse
	if err = json.Unmarshal(body, &proof); err != nil || proof.Version != 1 || proof.Challenge != input.Challenge || !discovery.ValidNodeID(proof.NodeID) || !validNodeOwner(proof.Owner) || !validNodeHex(proof.Proof, 32) {
		return nil, identity, errors.New("invalid node identity response")
	}
	binding, err := state.ExportKeyingMaterial(nodeExporterLabel, nil, 32)
	if err != nil || !nodeMACEqual(proof.Proof, nodeResponseMAC(secret, proof, binding)) {
		return nil, identity, errors.New("node identity channel proof failed")
	}
	identity.NodeID, identity.Owner = proof.NodeID, proof.Owner
	if peer.Identity != (nodeIdentity{}) && identity != peer.Identity {
		return nil, identity, errors.New("node or receiving account identity changed")
	}
	if reader.Buffered() != 0 {
		return nil, identity, errors.New("unexpected data after node identity proof")
	}
	if err = connection.SetDeadline(time.Time{}); err != nil {
		return nil, identity, err
	}
	return connection, identity, nil
}

func (s *Server) nodePeerDialer(peer serverPeer, secret string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		connection, _, err := dialNodePeer(ctx, peer, secret, s.peerRoots)
		return connection, err
	}
}

// The account scope must also be checked when the application request is
// admitted: a grant can change after the preceding channel proof completed.
func (s *Server) nodeRequestScope(r *http.Request, grant serverGrant) bool {
	node, owner := r.Header.Get("X-WebMux-Node-ID"), r.Header.Get("X-WebMux-Owner")
	if node == "" && owner == "" {
		return true
	} // Existing manually configured peers.
	if r.TLS == nil || !discovery.ValidNodeID(node) || owner != grant.Owner {
		return false
	}
	policy, err := discovery.LoadNodePolicy(s.store)
	return err == nil && policy.ID == node
}
func nodeScopeHeaders(peer serverPeer, secret string) http.Header {
	headers := http.Header{"Authorization": {"Bearer " + secret}}
	if peer.Identity != (nodeIdentity{}) {
		headers.Set("X-WebMux-Node-ID", peer.Identity.NodeID)
		headers.Set("X-WebMux-Owner", peer.Identity.Owner)
	}
	return headers
}
