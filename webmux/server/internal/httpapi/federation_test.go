package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func federationAccounts(t *testing.T, s *Server) {
	t.Helper()
	if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		c.Auth.Users = []auth.User{{Username: "owner", PasswordHash: "fixture", Admin: true}, {Username: "other", PasswordHash: "fixture"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func federationConfig(t *testing.T, s *Server, d serverDirectory) {
	t.Helper()
	if err := s.store.WriteConfig("servers.yaml", d); err != nil {
		t.Fatal(err)
	}
}
func federationPair(t *testing.T) (*Server, *Server, http.Handler, string, string) {
	t.Helper()
	a, handler := fixture(t, "local")
	b, bHandler := fixture(t, "local")
	federationAccounts(t, a)
	federationAccounts(t, b)
	secret := strings.Repeat("test-credential-", 3)
	t.Setenv("WEBMUX_TEST_PEER_TOKEN", secret)
	remote := httptest.NewServer(bHandler)
	t.Cleanup(remote.Close)
	federationConfig(t, a, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote host", URL: remote.URL, TokenEnv: "WEBMUX_TEST_PEER_TOKEN", AllowHTTP: true}}})
	federationConfig(t, b, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "owner", TokenEnv: "WEBMUX_TEST_PEER_TOKEN"}}})
	return a, b, handler, remote.URL, secret
}
func TestFederationDirectoryAuthenticationAndIsolation(t *testing.T) {
	a, b, handler, _, secret := federationPair(t)
	owner := token(t, a, "owner")
	other := token(t, a, "other")
	requireStatus(t, request(handler, "GET", "/api/servers", "", ""), 401)
	requireStatus(t, request(handler, "GET", "/api/servers", "", other), 403)
	directory := request(handler, "GET", "/api/servers", "", owner)
	requireStatus(t, directory, 200)
	if strings.Contains(directory.Body.String(), secret) || strings.Contains(directory.Body.String(), "token_env") || strings.Contains(directory.Body.String(), "http://") {
		t.Fatal("server credentials exposed", directory.Body.String())
	}
	for _, who := range []string{"owner", "other"} {
		_, err := b.sessions.Create(who, session.CreateRequest{Hostname: "localhost", Username: who, Transport: "exec"})
		if err != nil {
			t.Fatal(err)
		}
	}
	response := request(handler, "GET", "/api/servers/remote/sessions?token=browser-secret", "", owner)
	requireStatus(t, response, 200)
	var values []session.Session
	if err := json.Unmarshal(response.Body.Bytes(), &values); err != nil || len(values) != 1 || values[0].Owner != "owner" {
		t.Fatal("owner isolation", response.Body.String(), err)
	}
	for _, body := range []string{`{"message":"hello"}`, `{"message":"line\nnext"}`, `{"message":"\u001btest"}`} {
		want := 409
		if body != `{"message":"hello"}` {
			want = 400
		}
		requireStatus(t, request(handler, "POST", "/api/servers/remote/sessions/"+values[0].ID+"/messages", body, owner), want)
	}
	requireStatus(t, request(handler, "POST", "/api/servers/remote/sessions/foreign/messages", `{"message":"hello"}`, owner), 404)
	requireStatus(t, request(handler, "POST", "/api/servers/remote/sessions/"+values[0].ID+"/config", `{}`, owner), 404)
	requireStatus(t, request(handler, "GET", "/api/servers/missing/sessions", "", owner), 503)
	inbound := b.Handler()
	for _, credential := range []string{"", "short", "invalid-long-credential-xxxxxxxxxxxx"} {
		requireStatus(t, request(inbound, "GET", "/api/federation/sessions", "", credential), 401)
	}
	requireStatus(t, request(inbound, "GET", "/api/federation/sessions", "", secret), 200)
	requireStatus(t, request(inbound, "GET", "/api/config", "", secret), 401)
	federationConfig(t, b, serverDirectory{Version: 1})
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", owner), 502)
	requireStatus(t, request(inbound, "GET", "/api/federation/sessions", "", secret), 401)
}
func TestFederationDirectoryRejectsUnsafeOriginsAndDuplicateIDs(t *testing.T) {
	s, _ := fixture(t, "none")
	for _, origin := range []string{"http://host", "file:///etc/passwd", "https://user:secret@host", "https://host/api", "https://host?token=secret", "https://host/#secret", "ftp://host"} {
		federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "test", Name: "Test", URL: origin, TokenEnv: "TOKEN"}}})
		if _, err := s.serverDirectory(); err == nil {
			t.Fatal("accepted unsafe origin", origin)
		}
	}
	for _, id := range []string{"local", "all", "../config", "bad/id", ""} {
		federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: id, Name: "Test", URL: "https://host", TokenEnv: "TOKEN"}}})
		if _, err := s.serverDirectory(); err == nil {
			t.Fatal("accepted invalid ID", id)
		}
	}
}
func TestFederationProxyDoesNotForwardBrowserCredentialsOrFollowRedirects(t *testing.T) {
	s, handler := fixture(t, "none")
	t.Setenv("WEBMUX_TEST_PEER_TOKEN", strings.Repeat("x", 40))
	requests := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/federation/sessions" || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("x", 40) {
			t.Errorf("unsafe forwarded request: %s %v", r.URL, r.Header)
		}
		w.Header().Set("Set-Cookie", "secret=server")
		w.Header().Set("Location", "http://elsewhere.invalid")
		w.WriteHeader(302)
	}))
	defer remote.Close()
	federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote", URL: remote.URL, TokenEnv: "WEBMUX_TEST_PEER_TOKEN", AllowHTTP: true}}})
	req := httptest.NewRequest("GET", "/api/servers/remote/sessions?token=browser-secret", nil)
	req.Header.Set("Cookie", "browser=secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	requireStatus(t, response, 502)
	if requests != 1 || response.Header().Get("Set-Cookie") != "" || strings.Contains(response.Body.String(), "browser-secret") {
		t.Fatal("redirect or credential escape")
	}
	remote.Close()
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", ""), 502)
}
func TestFederationTerminalChild(t *testing.T) {
	if os.Getenv("WEBMUX_FEDERATION_CHILD") != "1" {
		return
	}
	fmt.Printf("FEDERATION_READY:%d\n", os.Getpid())
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println("FEDERATION_REPLY:" + scanner.Text())
	}
	os.Exit(0)
}
func TestFederationTwoServerNativeTerminalMessageAndRevocation(t *testing.T) {
	a, b, handler, _, _ := federationPair(t)
	t.Setenv("WEBMUX_FEDERATION_CHILD", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	current, err := b.sessions.Create("owner", session.CreateRequest{Hostname: "localhost", Username: "fixture", Transport: "exec", ExecCommand: `"` + exe + `" -test.run=^TestFederationTerminalChild$`})
	if err != nil || current.State == "error" {
		t.Fatal("native fixture", current, err)
	}
	central := httptest.NewServer(handler)
	defer central.Close()
	endpoint := "ws" + strings.TrimPrefix(central.URL, "http") + "/api/servers/remote/term/" + current.ID + "?token=" + token(t, a, "owner")
	c := dialSocket(t, endpoint)
	waitText := func(needle string) {
		t.Helper()
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		text := ""
		for !strings.Contains(text, needle) {
			var event session.Event
			if err := c.ReadJSON(&event); err != nil {
				t.Fatal("waiting for", needle, err, text)
			}
			if data, ok := event["data"].(string); ok {
				text += data
			}
		}
	}
	waitText("FEDERATION_READY:")
	if err := c.WriteJSON(session.Event{"type": "input", "data": "interactive-proof\r"}); err != nil {
		t.Fatal(err)
	}
	waitText("FEDERATION_REPLY:interactive-proof")
	response := request(handler, "POST", "/api/servers/remote/sessions/"+current.ID+"/messages", `{"message":"cross-host-proof"}`, token(t, a, "owner"))
	requireStatus(t, response, 202)
	if !strings.Contains(response.Body.String(), `"processed":false`) {
		t.Fatal("claimed processing", response.Body.String())
	}
	waitText("cross-host-proof")
	if err := c.WriteJSON(session.Event{"type": "resize", "cols": 105, "rows": 35}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		value, _ := b.sessions.Get("owner", current.ID)
		if value.Cols == 105 && value.Rows == 35 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("remote resize was not applied", value)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = c.Close()
	if len(b.sessions.List("owner")) != 1 {
		t.Fatal("closing a viewer changed provider inventory")
	}
	c = dialSocket(t, endpoint)
	waitText("FEDERATION_READY:")
	// Rotating the token revokes an already-connected peer, not only new requests.
	t.Setenv("WEBMUX_TEST_PEER_TOKEN", strings.Repeat("rotated", 6))
	socketClose(t, c, 1008)
	if len(b.sessions.List("owner")) != 1 {
		t.Fatal("revocation deleted provider")
	}
}

func TestFederationDiscoveryDoesNotConsumeMutationBudget(t *testing.T) {
	s, handler := fixture(t, "none")
	for i := 0; i < 350; i++ {
		requireStatus(t, request(handler, "GET", "/api/servers/local/sessions", "", ""), 200)
	}
	requireStatus(t, request(handler, "POST", "/api/sessions", `{}`, ""), 400) // Validation still runs instead of hitting the read-poll budget.
	federationConfig(t, s, serverDirectory{Version: 1})
	requireStatus(t, request(handler, "GET", "/api/servers", "", ""), 200)
}

func TestFederationVerifiesTLSAndRejectsMissingOutboundCredentials(t *testing.T) {
	s, handler := fixture(t, "none")
	t.Setenv("WEBMUX_TEST_PEER_TOKEN", strings.Repeat("x", 40))
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer peer.Close()
	federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "remote", Name: "Remote", URL: peer.URL, TokenEnv: "WEBMUX_TEST_PEER_TOKEN"}}})
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", ""), 502)
	t.Setenv("WEBMUX_TEST_PEER_TOKEN", "")
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", ""), 503)
}

func TestFederationSilentPeerConnectionHasDeadline(t *testing.T) {
	s, handler := fixture(t, "none")
	t.Setenv("WEBMUX_TEST_SILENT_TOKEN", strings.Repeat("silent-peer-", 4))
	release := make(chan struct{})
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		// Deliberately never read: unlike a healthy peer, this cannot answer pings.
		<-release
	}))
	defer peer.Close()
	defer close(release)
	federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "silent", Name: "Silent", URL: peer.URL, TokenEnv: "WEBMUX_TEST_SILENT_TOKEN", AllowHTTP: true}}})
	controller := httptest.NewServer(handler)
	defer controller.Close()
	connection := dialSocket(t, "ws"+strings.TrimPrefix(controller.URL, "http")+"/api/servers/silent/term/session")
	_ = connection.SetReadDeadline(time.Now().Add(20 * time.Second))
	_, _, err := connection.ReadMessage()
	var closed *websocket.CloseError
	if !errors.As(err, &closed) || closed.Code != 1001 {
		t.Fatalf("silent peer did not close within the heartbeat envelope: %v", err)
	}
}
