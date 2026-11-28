//go:build linux || darwin

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/ai"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/credentialhub"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func credentialRequest(h http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+bearer)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type credentialFixture struct {
	spoke, hub                                     *Server
	handler, hubHandler                            http.Handler
	peer                                           serverPeer
	token, secretID, grantID, key, admin, hubAdmin string
}

func newCredentialFixture(t *testing.T, sharing bool) credentialFixture {
	t.Helper()
	a, b, h, _, linkToken := nodePair(t, nil)
	admin, hubAdmin := token(t, a, "owner"), token(t, b, "owner")
	requireStatus(t, request(h, "POST", "/api/servers/remote/verify-identity", "{}", admin), 204)
	node, e := discovery.LoadNodePolicy(a.store)
	if e != nil {
		t.Fatal(e)
	}
	bh := b.Handler()
	body, _ := json.Marshal(map[string]any{"owner": "owner", "credential": linkToken, "credential_sharing": sharing, "source_node_id": node.ID, "source_owner": "owner"})
	requireStatus(t, credentialRequest(bh, "PUT", "/api/server-grants/controller", string(body), hubAdmin), 204)
	requireStatus(t, credentialRequest(bh, "POST", "/api/credentials/setup", "{}", hubAdmin), 204)
	key := "dedicated-http-api-key-not-a-real-provider-secret"
	body, _ = json.Marshal(map[string]any{"name": "Portable fixture", "value": key, "consumer": "ai.openai", "enabled": true})
	w := credentialRequest(bh, "POST", "/api/credentials/secrets", string(body), hubAdmin)
	requireStatus(t, w, 200)
	var secret credentialhub.Secret
	if json.Unmarshal(w.Body.Bytes(), &secret) != nil {
		t.Fatal("bad secret metadata")
	}
	f := credentialFixture{spoke: a, hub: b, handler: h, hubHandler: bh, peer: pinnedPeer(t, a), token: linkToken, secretID: secret.ID, key: key, admin: admin, hubAdmin: hubAdmin}
	if sharing {
		body, _ = json.Marshal(map[string]string{"secret_id": f.secretID, "enrollment_id": "controller", "consumer": "ai.openai"})
		w = credentialRequest(bh, "POST", "/api/credentials/grants", string(body), hubAdmin)
		requireStatus(t, w, 200)
		var grant credentialhub.Grant
		if json.Unmarshal(w.Body.Bytes(), &grant) != nil {
			t.Fatal("bad grant metadata")
		}
		f.grantID = grant.ID
	}
	return f
}
func (f credentialFixture) selectHub(t *testing.T) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"peer": "remote", "secret_id": f.secretID, "consumer": "ai.openai"})
	requireStatus(t, credentialRequest(f.handler, "PUT", "/api/credentials/selection", string(b), f.admin), 204)
}

type credentialProviderTransport struct {
	target    *url.URL
	transport http.RoundTripper
}

func (t credentialProviderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != "https://api.openai.com/v1/chat/completions" {
		return nil, errors.New("unexpected provider destination")
	}
	c := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.target.Scheme
	u.Host = t.target.Host
	c.URL = &u
	return t.transport.RoundTrip(c)
}
func TestCredentialHubAIUsesPrivatePinnedTLSPath(t *testing.T) {
	f := newCredentialFixture(t, true)
	var calls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+f.key {
			t.Error("private consumer received wrong key")
		}
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), f.key) {
			t.Error("key entered provider prompt")
		}
		writeJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "fixture response " + f.key}}}})
	}))
	defer provider.Close()
	u, _ := url.Parse(provider.URL)
	f.spoke.ai = ai.New(func(k string) string {
		if k == "NVIDIA_API_KEY" {
			return "ambient-key-must-not-be-used"
		}
		return ""
	}, &http.Client{Transport: credentialProviderTransport{u, provider.Client().Transport}})
	f.selectHub(t)
	w := request(f.handler, "GET", "/api/ai/status", "", f.admin)
	requireStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), `"available":true`) {
		t.Fatal(w.Body.String())
	}
	w = request(f.handler, "POST", "/api/ai/chat", `{"message":"fixture prompt","history":[],"context":"private fixture terminal"}`, f.admin)
	requireStatus(t, w, 200)
	if calls.Load() != 1 || strings.Contains(w.Body.String(), f.key) || !strings.Contains(w.Body.String(), "[redacted credential]") {
		t.Fatal("private consumer/redaction contract failed", calls.Load(), w.Body.String())
	}
	for _, pair := range []struct {
		h     http.Handler
		token string
	}{{f.handler, f.admin}, {f.hubHandler, f.hubAdmin}} {
		for _, path := range []string{"/api/credentials", "/api/server-directory", "/api/servers"} {
			w = credentialRequest(pair.h, "GET", path, "", pair.token)
			requireStatus(t, w, 200)
			if strings.Contains(w.Body.String(), f.key) || strings.Contains(w.Body.String(), f.token) {
				t.Fatal("normal metadata exposed credential", path)
			}
		}
	}
	requireStatus(t, credentialRequest(f.hubHandler, "DELETE", "/api/credentials/grants/"+f.grantID, "", f.hubAdmin), 204)
	w = request(f.handler, "POST", "/api/ai/chat", `{"message":"must refuse","history":[]}`, f.admin)
	requireStatus(t, w, 403)
	if calls.Load() != 1 {
		t.Fatal("revoked grant reached provider or ambient fallback")
	}
}
func TestCredentialHubRealTLSReplayAndRotation(t *testing.T) {
	f := newCredentialFixture(t, true)
	ctx := context.Background()
	input := credentialExchangeInput{Operation: "issue", SecretID: f.secretID, Consumer: "ai.openai"}
	issued, e := f.spoke.exchangeCredential(ctx, "owner", f.peer, f.token, input)
	if e != nil {
		t.Fatal(e)
	}
	input.Operation = "redeem"
	input.Handle = issued.Handle
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			out, e := f.spoke.exchangeCredential(ctx, "owner", f.peer, f.token, input)
			if e == nil {
				if out.Value != f.key {
					t.Error("wrong redeemed value")
				}
				successes.Add(1)
			} else if !errors.Is(e, credentialhub.ErrDenied) {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("wire replay admission count", successes.Load())
	}
	issued, e = f.spoke.exchangeCredential(ctx, "owner", f.peer, f.token, credentialExchangeInput{Operation: "issue", SecretID: f.secretID, Consumer: "ai.openai"})
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"name": "Portable fixture", "value": "rotated-dedicated-fixture-key", "consumer": "ai.openai", "enabled": true})
	requireStatus(t, credentialRequest(f.hubHandler, "PUT", "/api/credentials/secrets/"+f.secretID, string(body), f.hubAdmin), 200)
	input.Handle = issued.Handle
	if _, e = f.spoke.exchangeCredential(ctx, "owner", f.peer, f.token, input); !errors.Is(e, credentialhub.ErrDenied) {
		t.Fatal("rotation failed to fence handle", e)
	}
}
func TestCredentialHubRequiresDedicatedGrantAndRecipient(t *testing.T) {
	t.Run("ordinary-link", func(t *testing.T) {
		f := newCredentialFixture(t, false)
		_, e := f.spoke.exchangeCredential(context.Background(), "owner", f.peer, f.token, credentialExchangeInput{Operation: "issue", SecretID: f.secretID, Consumer: "ai.openai"})
		if !errors.Is(e, credentialhub.ErrDenied) {
			t.Fatal("ordinary peer accessed vault", e)
		}
	})
	for _, name := range []string{"consumer", "project", "secret", "recipient-account", "recipient-node", "pin", "enrollment-generation", "account-generation", "removed-account"} {
		t.Run(name, func(t *testing.T) {
			f := newCredentialFixture(t, true)
			in := credentialExchangeInput{Operation: "issue", SecretID: f.secretID, Consumer: "ai.openai"}
			owner := "owner"
			switch name {
			case "consumer":
				in.Consumer = "ai.nvidia"
			case "project":
				in.Project = "foreign-project"
			case "secret":
				in.SecretID = randomCredentialNonce()
			case "recipient-account":
				owner = "other"
			case "recipient-node":
				if e := storage.UpdateConfig(f.spoke.store, "node-discovery.yaml", func(p *discovery.NodePolicy) error { p.ID = strings.Repeat("a", 32); return nil }); e != nil {
					t.Fatal(e)
				}
			case "pin":
				f.peer.Identity.TLSKey = strings.Repeat("0", 64)
			case "enrollment-generation":
				if e := storage.UpdateConfig(f.hub.store, "servers.yaml", func(d *serverDirectory) error { d.Grants[0].CredentialEpoch = newCredentialEpoch(); return nil }); e != nil {
					t.Fatal(e)
				}
			case "account-generation":
				if e := storage.UpdateConfig(f.hub.store, "auth.yaml", func(c *auth.Config) error {
					c.Auth.Users[0].PasswordHash = "different-independent-account-generation"
					return nil
				}); e != nil {
					t.Fatal(e)
				}
			case "removed-account":
				if e := storage.UpdateConfig(f.hub.store, "auth.yaml", func(c *auth.Config) error { c.Auth.Users = nil; return nil }); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := f.spoke.exchangeCredential(context.Background(), owner, f.peer, f.token, in); e == nil {
				t.Fatal("foreign or stale authority accepted")
			}
		})
	}
}
func TestCredentialHubUncertainOutageNeverFallsBack(t *testing.T) {
	f := newCredentialFixture(t, true)
	f.selectHub(t)
	var calls atomic.Int32
	f.spoke.ai = ai.New(func(string) string { return "ambient-credential-not-authorized" }, &http.Client{Transport: credentialRejectTransport{&calls}})
	if e := storage.UpdateConfig(f.spoke.store, "servers.yaml", func(d *serverDirectory) error { d.Servers[0].URL = "https://127.0.0.1:1"; return nil }); e != nil {
		t.Fatal(e)
	}
	w := request(f.handler, "POST", "/api/ai/chat", `{"message":"must not fallback"}`, f.admin)
	if w.Code != 403 && w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatal("unavailable hub used ambient provider")
	}
}

type credentialRejectTransport struct{ calls *atomic.Int32 }

func (t credentialRejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("unexpected ambient fallback")
}
func TestCredentialHubManagementAndSchemaAreBounded(t *testing.T) {
	f := newCredentialFixture(t, true)
	requireStatus(t, request(f.handler, "POST", "/api/credentials/setup", "{}", f.admin), 400)
	requireStatus(t, credentialRequest(f.hubHandler, "POST", "/api/credentials/setup", `{"unexpected":"must not be echoed"}`, f.hubAdmin), 400)
	body, _ := json.Marshal(map[string]any{"name": "oversized", "value": strings.Repeat("x", 4097), "consumer": "ai.openai", "enabled": true})
	w := credentialRequest(f.hubHandler, "POST", "/api/credentials/secrets", string(body), f.hubAdmin)
	requireStatus(t, w, 403)
	if strings.Contains(w.Body.String(), strings.Repeat("x", 100)) {
		t.Fatal("error echoed secret")
	}
	requireStatus(t, credentialRequest(f.handler, "GET", "/api/credentials", "", token(t, f.spoke, "other")), 403)
	requireStatus(t, request(f.handler, "POST", "/api/servers/remote/credentials", "{}", f.admin), 404)
	if !sameTerminalPeer(f.peer, func() serverPeer { p := f.peer; p.CredentialEpoch = newCredentialEpoch(); return p }()) {
		t.Fatal("credential-only generation changed terminal identity")
	}
}

func TestCredentialHubNamedSelectionAndEnrollmentRecreation(t *testing.T) {
	f := newCredentialFixture(t, true)
	w := credentialRequest(f.handler, "GET", "/api/credentials/peers/remote/available?consumer=ai.openai", "", f.admin)
	requireStatus(t, w, 200)
	var available []credentialhub.Secret
	if json.Unmarshal(w.Body.Bytes(), &available) != nil || len(available) != 1 || available[0].ID != f.secretID || available[0].Name != "Portable fixture" || strings.Contains(w.Body.String(), f.key) {
		t.Fatal("named metadata contract", w.Body.String())
	}
	w = credentialRequest(f.handler, "GET", "/api/credentials/peers/remote/available?consumer=ai.nvidia", "", f.admin)
	requireStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("foreign consumer metadata", w.Body.String())
	}
	// Legacy pins gain an independent credential generation only on explicit selection.
	if e := storage.UpdateConfig(f.spoke.store, "servers.yaml", func(d *serverDirectory) error { d.Servers[0].CredentialEpoch = ""; return nil }); e != nil {
		t.Fatal(e)
	}
	f.selectHub(t)
	if !validNodeHex(pinnedPeer(t, f.spoke).CredentialEpoch, 32) {
		t.Fatal("legacy generation not initialized")
	}
	requireStatus(t, credentialRequest(f.hubHandler, "DELETE", "/api/server-grants/controller", "", f.hubAdmin), 204)
	node, e := discovery.LoadNodePolicy(f.spoke.store)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"owner": "owner", "credential": f.token, "credential_sharing": true, "source_node_id": node.ID, "source_owner": "owner"})
	requireStatus(t, credentialRequest(f.hubHandler, "PUT", "/api/server-grants/controller", string(body), f.hubAdmin), 204)
	w = credentialRequest(f.hubHandler, "GET", "/api/credentials", "", f.hubAdmin)
	requireStatus(t, w, 200)
	var info struct {
		Vault credentialhub.View `json:"vault"`
	}
	if json.Unmarshal(w.Body.Bytes(), &info) != nil || len(info.Vault.Grants) != 1 || info.Vault.Grants[0].Active {
		t.Fatal("stale enrollment metadata presented as active")
	}
	w = credentialRequest(f.handler, "GET", "/api/credentials/peers/remote/available?consumer=ai.openai", "", f.admin)
	requireStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("recreated enrollment revived grant", w.Body.String())
	}
}

func TestCredentialHubReceivingAccountGenerationCannotInheritSelection(t *testing.T) {
	f := newCredentialFixture(t, true)
	f.selectHub(t)
	if e := storage.UpdateConfig(f.spoke.store, "auth.yaml", func(c *auth.Config) error {
		c.Auth.Users[0].PasswordHash = "replacement-receiving-account-generation"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	generation, _ := f.spoke.auth.Generation(f.admin)
	if _, _, selected, e := f.spoke.credentialForAI(context.Background(), "owner", generation); !selected || !errors.Is(e, credentialhub.ErrDenied) {
		t.Fatal("recreated receiving account inherited selection", selected, e)
	}
}

func TestCredentialHubRejectsLegacyAndStaleLocalLoginSessions(t *testing.T) {
	f := newCredentialFixture(t, true)
	f.selectHub(t)
	legacy, e := f.spoke.auth.SignGeneration("owner", "")
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, credentialRequest(f.handler, "GET", "/api/credentials", "", legacy), 401)
	requireStatus(t, request(f.handler, "POST", "/api/ai/chat", `{"message":"fixture"}`, legacy), 401)
	requireStatus(t, request(f.handler, "GET", "/api/sessions", "", legacy), 401)
	refresh := request(f.handler, "POST", "/api/auth/refresh", "{}", legacy)
	requireStatus(t, refresh, 401)
	var result map[string]string
	hash, e := auth.HashPassword("independent-fixture-password")
	if e != nil {
		t.Fatal(e)
	}
	if e = storage.UpdateConfig(f.hub.store, "auth.yaml", func(c *auth.Config) error { c.Auth.Users[0].PasswordHash = hash; return nil }); e != nil {
		t.Fatal(e)
	}
	requireStatus(t, credentialRequest(f.hubHandler, "GET", "/api/credentials", "", f.hubAdmin), 401)
	requireStatus(t, credentialRequest(f.hubHandler, "POST", "/api/credentials/setup", "{}", f.hubAdmin), 401)
	requireStatus(t, request(f.hubHandler, "POST", "/api/auth/refresh", "{}", f.hubAdmin), 401)
	login := request(f.hubHandler, "POST", "/api/auth/login", `{"username":"owner","password":"independent-fixture-password"}`, "")
	requireStatus(t, login, 200)
	if json.Unmarshal(login.Body.Bytes(), &result) != nil {
		t.Fatal("bad fresh login")
	}
	info := credentialRequest(f.hubHandler, "GET", "/api/credentials", "", result["token"])
	requireStatus(t, info, 200)
	if strings.Contains(info.Body.String(), f.secretID) {
		t.Fatal("new generation inherited prior vault metadata")
	}
	requireStatus(t, credentialRequest(f.hubHandler, "POST", "/api/credentials/setup", "{}", result["token"]), 204)
}

func TestCredentialHubLocalAnonymousUsernameRetainsBoundRefresh(t *testing.T) {
	s, h := fixture(t, "local")
	if e := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		c.Auth.Users = []auth.User{{Username: "anonymous", PasswordHash: "independent-anonymous-fixture-hash", Admin: true}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	bearer := token(t, s, "anonymous")
	w := request(h, "POST", "/api/auth/refresh", "{}", bearer)
	requireStatus(t, w, 200)
	var body map[string]string
	if json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatal("bad refresh")
	}
	if generation, e := s.auth.Generation(body["token"]); e != nil || generation == "" || body["mode"] != "local" {
		t.Fatal("local account confused with trusted-mode identity", e)
	}
}
