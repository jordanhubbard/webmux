package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/credentialhub"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

const credentialExchangePath = "/api/federation/credentials"

func randomCredentialNonce() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func credentialEnrollment(g serverGrant) string {
	return credentialhub.Fingerprint(struct {
		ID, Owner, Node, SourceOwner, Epoch, Token string
		Enabled                                    bool
	}{g.ID, g.Owner, g.SourceNode, g.SourceOwner, g.CredentialEpoch, serverCredential(g.TokenEnv, g.Credential), g.CredentialSharing})
}
func credentialPeerFingerprint(p serverPeer, token string) string {
	return credentialhub.Fingerprint(struct {
		ID, URL, Epoch, Token string
		Identity              nodeIdentity
	}{p.ID, p.URL, p.CredentialEpoch, token, p.Identity})
}

type credentialBinding struct {
	Principal   string         `yaml:"principal" json:"-"`
	ID          string         `yaml:"id" json:"id"`
	Peer        string         `yaml:"peer" json:"peer"`
	SecretID    string         `yaml:"secret_id" json:"secret_id"`
	Consumer    string         `yaml:"consumer" json:"consumer"`
	Fingerprint string         `yaml:"fingerprint" json:"-"`
	Extra       map[string]any `yaml:",inline" json:"-"`
}
type credentialSelections struct {
	Version  int                          `yaml:"version"`
	Bindings map[string]credentialBinding `yaml:"bindings"`
	Extra    map[string]any               `yaml:",inline"`
}

func readCredentialSelections(read func(string, any) error) (credentialSelections, error) {
	d := credentialSelections{Version: 1, Bindings: map[string]credentialBinding{}}
	e := read("credential-hubs.yaml", &d)
	if errors.Is(e, os.ErrNotExist) {
		return d, nil
	}
	if e != nil || d.Version != 1 || d.Bindings == nil || len(d.Bindings) > 64 || len(d.Extra) != 0 {
		return d, credentialhub.ErrUnavailable
	}
	for owner, b := range d.Bindings {
		if owner == "" || len(owner) > 256 || !validNodeHex(b.Principal, 32) || !credentialhub.ValidID(b.ID) || !federationID.MatchString(b.Peer) || !credentialhub.ValidID(b.SecretID) || !credentialhub.Consumer(b.Consumer) || !validNodeHex(b.Fingerprint, 32) || len(b.Extra) != 0 {
			return d, credentialhub.ErrUnavailable
		}
	}
	return d, nil
}

// Namespace vault ownership by the authenticated account generation, not a
// reusable username. Account removal/recreation or password reset cannot revive
// old secrets, grants or handles. Never expose the private namespace in replies.
func credentialPrincipal(read func(string, any) error, owner string) (string, error) {
	var c auth.Config
	if read("auth.yaml", &c) != nil || c.Validate() != nil || c.Auth.Mode != "local" || !c.HasUser(owner) || !auth.IsAdmin(c.Auth.Users, owner) {
		return "", credentialhub.ErrDenied
	}
	for _, u := range c.Auth.Users {
		if u.Username == owner {
			return auth.AccountGeneration(owner, u.PasswordHash), nil
		}
	}
	return "", credentialhub.ErrDenied
}

type credentialSessionKey struct{}

func credentialRequestPrincipal(read func(string, any) error, r *http.Request, owner string) (string, error) {
	principal, e := credentialPrincipal(read, owner)
	expected, _ := r.Context().Value(credentialSessionKey{}).(string)
	if e != nil || expected == "" || principal != expected {
		return "", credentialhub.ErrDenied
	}
	return principal, nil
}
func credentialRequestAccount(read func(string, any) error, r *http.Request, owner string) error {
	_, e := credentialRequestPrincipal(read, r, owner)
	return e
}
func credentialAccount(read func(string, any) error, owner string) error {
	_, e := credentialPrincipal(read, owner)
	return e
}
func credentialScope(g serverGrant, secret, consumer, project, principal string) credentialhub.Scope {
	return credentialhub.Scope{Owner: principal, SecretID: secret, EnrollmentID: g.ID, EnrollmentFingerprint: credentialEnrollment(g), SourceNode: g.SourceNode, SourceOwner: g.SourceOwner, Consumer: consumer, Project: project}
}
func credentialGrantReady(g serverGrant) bool {
	return g.CredentialSharing && validNodeHex(g.CredentialEpoch, 32) && discovery.ValidNodeID(g.SourceNode) && validNodeOwner(g.SourceOwner)
}
func credentialDecode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		writeError(w, 400, "Invalid credential request")
		return false
	}
	return true
}
func (s *Server) credentialError(w http.ResponseWriter, owner, operation string, e error) {
	_ = s.store.AppendEvent(map[string]any{"event": "credential.operation_refused", "owner": owner, "operation": operation})
	code := 503
	message := credentialhub.ErrUnavailable.Error()
	if errors.Is(e, credentialhub.ErrDenied) {
		code = 403
		message = credentialhub.ErrDenied.Error()
	}
	if errors.Is(e, credentialhub.ErrLimit) {
		code = 429
		message = credentialhub.ErrLimit.Error()
	}
	writeError(w, code, message)
}
func secureCredentialManagement(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	u, e := url.Parse("http://" + r.Host)
	if e != nil || ip == nil || !ip.IsLoopback() {
		return false
	}
	dest := net.ParseIP(u.Hostname())
	return strings.EqualFold(u.Hostname(), "localhost") || (dest != nil && dest.IsLoopback())
}
func (s *Server) credentialLocal(next authenticatedHandler) http.Handler {
	return s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
		w.Header().Set("Cache-Control", "no-store")
		if !secureCredentialManagement(r) {
			writeError(w, 400, "Credential management requires HTTPS or loopback access")
			return
		}
		generation, e := s.auth.Generation(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if e != nil {
			writeError(w, 403, "Sign in again to use shared credentials")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), credentialSessionKey{}, generation))
		next(w, r, owner)
	})
}
func (s *Server) registerCredentialHub(mux *http.ServeMux) {
	mux.Handle("GET /api/credentials", s.credentialLocal(s.credentialInfo))
	mux.Handle("GET /api/credentials/peers/{server}/available", s.credentialLocal(s.credentialAvailable))
	mux.Handle("POST /api/credentials/setup", s.credentialLocal(s.credentialSetup))
	mux.Handle("POST /api/credentials/secrets", s.credentialLocal(s.credentialPut))
	mux.Handle("PUT /api/credentials/secrets/{secret}", s.credentialLocal(s.credentialPut))
	mux.Handle("DELETE /api/credentials/secrets/{secret}", s.credentialLocal(s.credentialDelete))
	mux.Handle("POST /api/credentials/grants", s.credentialLocal(s.credentialGrant))
	mux.Handle("DELETE /api/credentials/grants/{grant}", s.credentialLocal(s.credentialRevoke))
	mux.Handle("PUT /api/credentials/selection", s.credentialLocal(s.credentialSelect))
	mux.Handle("DELETE /api/credentials/selection", s.credentialLocal(s.credentialUnselect))
	mux.HandleFunc("POST "+credentialExchangePath, s.credentialExchange)
}
func (s *Server) credentialInfo(w http.ResponseWriter, r *http.Request, owner string) {
	var view credentialhub.View
	var binding *credentialBinding
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		d, e := readCredentialSelections(read)
		if e != nil {
			return e
		}
		if b, ok := d.Bindings[owner]; ok {
			binding = &b
		}
		principal, e := credentialRequestPrincipal(read, r, owner)
		if e != nil {
			return e
		}
		view, e = s.credentialVault.View(principal)
		var directory serverDirectory
		de := read("servers.yaml", &directory)
		if de != nil && !errors.Is(de, os.ErrNotExist) {
			return credentialhub.ErrUnavailable
		}
		if de == nil && validateServerDirectory(directory) != nil {
			return credentialhub.ErrUnavailable
		}
		for i := range view.Grants {
			g := &view.Grants[i]
			current := false
			for _, enrollment := range directory.Grants {
				if enrollment.ID == g.EnrollmentID && enrollment.Owner == owner && credentialGrantReady(enrollment) && credentialEnrollment(enrollment) == g.EnrollmentFingerprint {
					current = true
				}
			}
			g.Active = g.Active && current
		}

		for i := range view.Secrets {
			view.Secrets[i].Owner = owner
		}
		return e
	})
	if e != nil {
		s.credentialError(w, owner, "metadata", e)
		return
	}
	writeJSON(w, 200, map[string]any{"vault": view, "binding": binding, "storage_supported": runtime.GOOS == "linux" || runtime.GOOS == "darwin"})
}
func (s *Server) credentialSetup(w http.ResponseWriter, r *http.Request, owner string) {
	var input struct{}
	if !credentialDecode(w, r, &input) {
		return
	}
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		principal, e := credentialRequestPrincipal(read, r, owner)
		if e != nil {
			return e
		}
		return s.credentialVault.Setup(principal)
	})
	if e != nil {
		s.credentialError(w, owner, "setup", e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) credentialPut(w http.ResponseWriter, r *http.Request, owner string) {
	var input struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Consumer string `json:"consumer"`
		Enabled  bool   `json:"enabled"`
	}
	if !credentialDecode(w, r, &input) {
		return
	}
	var secret credentialhub.Secret
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		var e error
		principal, pe := credentialRequestPrincipal(read, r, owner)
		if pe != nil {
			return pe
		}
		secret, e = s.credentialVault.Put(principal, r.PathValue("secret"), input.Name, input.Value, input.Consumer, input.Enabled)
		return e
	})
	if e != nil {
		s.credentialError(w, owner, "store", e)
		return
	}
	secret.Owner = owner
	writeJSON(w, 200, secret)
}
func (s *Server) credentialDelete(w http.ResponseWriter, r *http.Request, owner string) {
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		principal, e := credentialRequestPrincipal(read, r, owner)
		if e != nil {
			return e
		}
		return s.credentialVault.Delete(principal, r.PathValue("secret"))
	})
	if e != nil {
		s.credentialError(w, owner, "delete", e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) credentialGrant(w http.ResponseWriter, r *http.Request, owner string) {
	var input struct {
		SecretID     string `json:"secret_id"`
		EnrollmentID string `json:"enrollment_id"`
		Consumer     string `json:"consumer"`
	}
	if !credentialDecode(w, r, &input) {
		return
	}
	var grant credentialhub.Grant
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		var d serverDirectory
		if read("servers.yaml", &d) != nil || validateServerDirectory(d) != nil {
			return credentialhub.ErrDenied
		}
		for _, g := range d.Grants {
			if g.ID == input.EnrollmentID && g.Owner == owner && credentialGrantReady(g) {
				var e error
				principal, pe := credentialRequestPrincipal(read, r, owner)
				if pe != nil {
					return pe
				}
				grant, e = s.credentialVault.Grant(credentialScope(g, input.SecretID, input.Consumer, "", principal))
				return e
			}
		}
		return credentialhub.ErrDenied
	})
	if e != nil {
		s.credentialError(w, owner, "grant", e)
		return
	}
	grant.Active = true
	writeJSON(w, 200, grant)
}
func (s *Server) credentialRevoke(w http.ResponseWriter, r *http.Request, owner string) {
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		principal, e := credentialRequestPrincipal(read, r, owner)
		if e != nil {
			return e
		}
		return s.credentialVault.Revoke(principal, r.PathValue("grant"))
	})
	if e != nil {
		s.credentialError(w, owner, "revoke", e)
		return
	}
	w.WriteHeader(204)
}

type credentialExchangeInput struct {
	Operation string `json:"operation"`
	SecretID  string `json:"secret_id"`
	Consumer  string `json:"consumer"`
	Project   string `json:"project,omitempty"`
	Handle    string `json:"handle,omitempty"`
}
type credentialExchangeReply struct {
	Secrets        []credentialhub.Secret `json:"secrets,omitempty"`
	RecipientNode  string                 `json:"recipient_node"`
	RecipientOwner string                 `json:"recipient_owner"`
	Version        int                    `json:"version"`
	Secret         *credentialhub.Secret  `json:"secret,omitempty"`
	Handle         string                 `json:"handle,omitempty"`
	Expires        time.Time              `json:"expires_at,omitempty"`
	Value          string                 `json:"value,omitempty"`
}

func (s *Server) credentialExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.TLS == nil {
		s.credentialError(w, "", "transport", credentialhub.ErrDenied)
		return
	}
	var input credentialExchangeInput
	if !credentialDecode(w, r, &input) {
		return
	}
	var reply credentialExchangeReply
	reply.Version = 1
	owner := ""
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		var d serverDirectory
		if read("servers.yaml", &d) != nil || validateServerDirectory(d) != nil {
			return credentialhub.ErrDenied
		}
		var local discovery.NodePolicy
		if read("node-discovery.yaml", &local) != nil || local.Validate() != nil || r.Header.Get("X-WebMux-Node-ID") != local.ID {
			return credentialhub.ErrDenied
		}
		bearer := r.Header.Get("Authorization")
		if !strings.HasPrefix(bearer, "Bearer ") || len(bearer) < 39 {
			return credentialhub.ErrDenied
		}
		bearer = strings.TrimPrefix(bearer, "Bearer ")
		for _, g := range d.Grants {
			expected := serverCredential(g.TokenEnv, g.Credential)
			if len(expected) < 32 || subtle.ConstantTimeCompare([]byte(expected), []byte(bearer)) != 1 {
				continue
			}
			owner = g.Owner
			if !credentialGrantReady(g) || r.Header.Get("X-WebMux-Owner") != g.Owner || credentialAccount(read, g.Owner) != nil {
				return credentialhub.ErrDenied
			}
			reply.RecipientNode = g.SourceNode
			reply.RecipientOwner = g.SourceOwner
			principal, pe := credentialPrincipal(read, g.Owner)
			if pe != nil {
				return pe
			}
			scope := credentialScope(g, input.SecretID, input.Consumer, input.Project, principal)
			switch input.Operation {
			case "list":
				if input.Handle != "" || input.SecretID != "" {
					return credentialhub.ErrDenied
				}
				var e error
				reply.Secrets, e = s.credentialVault.Available(scope)
				for i := range reply.Secrets {
					reply.Secrets[i].Owner = g.Owner
				}
				return e
			case "availability":
				if input.Handle != "" {
					return credentialhub.ErrDenied
				}
				secret, e := s.credentialVault.Check(scope)
				secret.Owner = g.Owner
				reply.Secret = &secret
				return e
			case "issue":
				if input.Handle != "" {
					return credentialhub.ErrDenied
				}
				var e error
				reply.Handle, reply.Expires, e = s.credentialVault.Issue(scope)
				return e
			case "redeem":
				var e error
				reply.Value, e = s.credentialVault.Redeem(scope, input.Handle)
				return e
			default:
				return credentialhub.ErrDenied
			}
		}
		return credentialhub.ErrDenied
	})
	if e != nil {
		s.credentialError(w, owner, "exchange", e)
		return
	}
	writeJSON(w, 200, reply)
}
func (s *Server) credentialBindingSnapshot(owner string) (credentialBinding, serverPeer, string, bool, error) {
	var b credentialBinding
	var peer serverPeer
	var token string
	found := false
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		d, e := readCredentialSelections(read)
		if e != nil {
			return e
		}
		b, found = d.Bindings[owner]
		if !found {
			return nil
		}
		principal, e := credentialPrincipal(read, owner)
		if e != nil || principal != b.Principal {
			return credentialhub.ErrDenied
		}
		var dir serverDirectory
		if read("servers.yaml", &dir) != nil || validateServerDirectory(dir) != nil {
			return credentialhub.ErrDenied
		}
		for _, p := range dir.Servers {
			if p.ID != b.Peer {
				continue
			}
			token = serverCredential(p.TokenEnv, p.Credential)
			u, e := url.Parse(p.URL)
			if e != nil || u.Scheme != "https" || p.AllowHTTP || !validNodePin(p.Identity) || !validNodeHex(p.CredentialEpoch, 32) || len(token) < 32 || credentialPeerFingerprint(p, token) != b.Fingerprint {
				return credentialhub.ErrDenied
			}
			peer = p
			return nil
		}
		return credentialhub.ErrDenied
	})
	return b, peer, token, found, e
}
func (s *Server) exchangeCredential(ctx context.Context, owner string, p serverPeer, token string, input credentialExchangeInput) (credentialExchangeReply, error) {
	var out credentialExchangeReply
	var local discovery.NodePolicy
	if s.store.ReadConfig("node-discovery.yaml", &local) != nil || local.Validate() != nil {
		return out, credentialhub.ErrDenied
	}
	u, e := url.Parse(p.URL)
	if e != nil || u.Scheme != "https" || !validNodePin(p.Identity) {
		return out, credentialhub.ErrDenied
	}
	u.Path = credentialExchangePath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	b, e := json.Marshal(input)
	if e != nil {
		return out, credentialhub.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(b))
	if e != nil {
		return out, credentialhub.ErrUnavailable
	}
	req.Header = nodeScopeHeaders(p, token)
	req.Header.Set("Content-Type", "application/json")
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.ForceAttemptHTTP2 = false
	tr.DialTLSContext = s.nodePeerDialer(p, token)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return credentialhub.ErrDenied }}
	response, e := client.Do(req)
	if e != nil {
		return out, credentialhub.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return out, credentialhub.ErrDenied
	}
	if response.StatusCode != 200 {
		return out, credentialhub.ErrUnavailable
	}
	limit := 8192
	if input.Operation == "list" {
		limit = 32 << 10
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if e != nil || len(data) > limit {
		return out, credentialhub.ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&out) != nil || dec.Decode(new(any)) != io.EOF || out.Version != 1 {
		return credentialExchangeReply{}, credentialhub.ErrUnavailable
	}
	if out.RecipientNode != local.ID || out.RecipientOwner != owner {
		return credentialExchangeReply{}, credentialhub.ErrDenied
	}
	switch input.Operation {
	case "list":
		if len(out.Secrets) > 32 || out.Value != "" || out.Handle != "" || out.Secret != nil {
			return credentialExchangeReply{}, credentialhub.ErrUnavailable
		}
		for _, secret := range out.Secrets {
			if !credentialhub.ValidID(secret.ID) || secret.Consumer != input.Consumer || secret.Owner != p.Identity.Owner || !secret.Enabled || secret.Version == 0 || len(secret.Name) > 80 {
				return credentialExchangeReply{}, credentialhub.ErrUnavailable
			}
		}
	case "availability":
		if out.Secret == nil || out.Secret.ID != input.SecretID || out.Secret.Consumer != input.Consumer || !out.Secret.Enabled || out.Secret.Version == 0 || out.Value != "" || out.Handle != "" {
			return credentialExchangeReply{}, credentialhub.ErrUnavailable
		}
	case "issue":
		if !credentialhub.ValidID(out.Handle) || out.Value != "" || out.Secret != nil || !time.Now().Before(out.Expires) || out.Expires.After(time.Now().Add(35*time.Second)) {
			return credentialExchangeReply{}, credentialhub.ErrUnavailable
		}
	case "redeem":
		if out.Value == "" || len(out.Value) > 4096 || strings.ContainsAny(out.Value, "\r\n") || out.Handle != "" || out.Secret != nil {
			return credentialExchangeReply{}, credentialhub.ErrUnavailable
		}
	default:
		return credentialExchangeReply{}, credentialhub.ErrDenied
	}
	return out, nil
}
func (s *Server) credentialSelect(w http.ResponseWriter, r *http.Request, owner string) {
	var input struct {
		Peer     string `json:"peer"`
		SecretID string `json:"secret_id"`
		Consumer string `json:"consumer"`
	}
	if !credentialDecode(w, r, &input) {
		return
	}
	if !credentialhub.ValidID(input.SecretID) || !credentialhub.Consumer(input.Consumer) {
		s.credentialError(w, owner, "select", credentialhub.ErrDenied)
		return
	}
	var peer serverPeer
	var token string
	var principal string
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		var e error
		principal, e = credentialRequestPrincipal(read, r, owner)
		if e != nil {
			return e
		}
		var d serverDirectory
		if read("servers.yaml", &d) != nil || validateServerDirectory(d) != nil {
			return credentialhub.ErrDenied
		}
		for _, p := range d.Servers {
			if p.ID == input.Peer {
				peer = p
				token = serverCredential(p.TokenEnv, p.Credential)
				u, e := url.Parse(p.URL)
				if e != nil || u.Scheme != "https" || p.AllowHTTP || !validNodePin(p.Identity) || (p.CredentialEpoch != "" && !validNodeHex(p.CredentialEpoch, 32)) || len(token) < 32 {
					return credentialhub.ErrDenied
				}
				return nil
			}
		}
		return credentialhub.ErrDenied
	})
	if e == nil && peer.CredentialEpoch == "" {
		next := peer
		next.CredentialEpoch = newCredentialEpoch()
		e = s.commitNodePeer(peer, next, token)
		if e == nil {
			peer = next
		}
	}
	if e == nil {
		_, e = s.exchangeCredential(r.Context(), owner, peer, token, credentialExchangeInput{Operation: "availability", SecretID: input.SecretID, Consumer: input.Consumer})
	}
	if e != nil {
		s.credentialError(w, owner, "select", e)
		return
	}
	binding := credentialBinding{Principal: principal, ID: randomCredentialNonce(), Peer: peer.ID, SecretID: input.SecretID, Consumer: input.Consumer, Fingerprint: credentialPeerFingerprint(peer, token)}
	e = storage.UpdateConfigOrCreate(s.store, "credential-hubs.yaml", credentialSelections{Version: 1, Bindings: map[string]credentialBinding{}}, func(d *credentialSelections) error {
		if d.Version != 1 || d.Bindings == nil || len(d.Extra) != 0 || (len(d.Bindings) >= 64 && d.Bindings[owner].ID == "") {
			return credentialhub.ErrUnavailable
		}
		d.Bindings[owner] = binding
		return nil
	})
	if e != nil {
		s.credentialError(w, owner, "select", e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) credentialUnselect(w http.ResponseWriter, r *http.Request, owner string) {
	e := s.store.InspectConfigs(func(read func(string, any) error) error { return credentialRequestAccount(read, r, owner) })
	if e == nil {
		e = storage.UpdateConfigOrCreate(s.store, "credential-hubs.yaml", credentialSelections{Version: 1, Bindings: map[string]credentialBinding{}}, func(d *credentialSelections) error {
			if d.Version != 1 || d.Bindings == nil || len(d.Extra) != 0 {
				return credentialhub.ErrUnavailable
			}
			delete(d.Bindings, owner)
			return nil
		})
	}
	if e != nil {
		s.credentialError(w, owner, "unselect", e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) credentialForAI(ctx context.Context, owner, generation string) (string, string, bool, error) {
	b, p, token, found, e := s.credentialBindingSnapshot(owner)
	if e != nil || !found {
		return "", "", found, e
	}
	if generation == "" || generation != b.Principal {
		return "", "", true, credentialhub.ErrDenied
	}
	reply, e := s.exchangeCredential(ctx, owner, p, token, credentialExchangeInput{Operation: "issue", SecretID: b.SecretID, Consumer: b.Consumer})
	if e != nil {
		return "", "", true, e
	}
	reply, e = s.exchangeCredential(ctx, owner, p, token, credentialExchangeInput{Operation: "redeem", SecretID: b.SecretID, Consumer: b.Consumer, Handle: reply.Handle})
	if e != nil {
		return "", "", true, e
	}
	current, _, _, ok, e := s.credentialBindingSnapshot(owner)
	if e != nil || !ok || current.ID != b.ID {
		return "", "", true, credentialhub.ErrDenied
	}
	return reply.Value, b.Consumer, true, nil
}

func (s *Server) credentialAvailable(w http.ResponseWriter, r *http.Request, owner string) {
	consumer := r.URL.Query().Get("consumer")
	if !credentialhub.Consumer(consumer) || len(r.URL.Query()) != 1 {
		s.credentialError(w, owner, "list", credentialhub.ErrDenied)
		return
	}
	var peer serverPeer
	var token string
	e := s.store.InspectConfigs(func(read func(string, any) error) error {
		if e := credentialRequestAccount(read, r, owner); e != nil {
			return e
		}
		var d serverDirectory
		if read("servers.yaml", &d) != nil || validateServerDirectory(d) != nil {
			return credentialhub.ErrDenied
		}
		for _, p := range d.Servers {
			if p.ID == r.PathValue("server") {
				u, e := url.Parse(p.URL)
				if e != nil || u.Scheme != "https" || p.AllowHTTP || !validNodePin(p.Identity) {
					return credentialhub.ErrDenied
				}
				peer = p
				token = serverCredential(p.TokenEnv, p.Credential)
				if len(token) < 32 {
					return credentialhub.ErrDenied
				}
				return nil
			}
		}
		return credentialhub.ErrDenied
	})
	if e != nil {
		s.credentialError(w, owner, "list", e)
		return
	}
	reply, e := s.exchangeCredential(r.Context(), owner, peer, token, credentialExchangeInput{Operation: "list", Consumer: consumer})
	if e != nil {
		s.credentialError(w, owner, "list", e)
		return
	}
	if reply.Secrets == nil {
		reply.Secrets = []credentialhub.Secret{}
	}
	writeJSON(w, 200, reply.Secrets)
}
