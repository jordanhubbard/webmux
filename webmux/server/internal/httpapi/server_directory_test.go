package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func directoryJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

func TestFederationManagementRepairsCredentialsAndPreservesSessions(t *testing.T) {
	a, b, handler, remote, secret := federationPair(t)
	admin := token(t, a, "owner")
	peerBody := map[string]any{"name": "Remote", "url": remote, "allow_http": true, "token_env": "WEBMUX_MISSING_CREDENTIAL"}
	requireStatus(t, request(handler, "PUT", "/api/servers/remote", directoryJSON(peerBody), admin), 204)
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", admin), 503)
	peerBody["credential"] = secret
	requireStatus(t, request(handler, "PUT", "/api/servers/remote", directoryJSON(peerBody), admin), 204)
	created, err := b.sessions.Create("owner", session.CreateRequest{Hostname: "localhost", Username: "owner", Transport: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		response := request(handler, "GET", "/api/servers/remote/sessions", "", admin)
		requireStatus(t, response, 200)
		var sessions []session.Session
		if err := json.Unmarshal(response.Body.Bytes(), &sessions); err != nil || len(sessions) != 1 || sessions[0].ID != created.ID {
			t.Fatalf("refresh changed sessions: %v", err)
		}
	}
	for _, path := range []string{"/api/servers", "/api/server-directory"} {
		response := request(handler, "GET", path, "", admin)
		requireStatus(t, response, 200)
		if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), `"credential":`) {
			t.Fatal("credential leaked")
		}
	}
	d, err := a.serverDirectory()
	if err != nil || d.Servers[0].Credential != secret || d.Servers[0].TokenEnv != "" {
		t.Fatal("credential repair not persisted", err)
	}
	info, err := os.Stat(a.store.ConfigPath("servers.yaml"))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		t.Fatal("configuration is not private", err)
	}
	// An edit with a blank credential preserves the saved secret.
	peerBody["credential"], peerBody["token_env"], peerBody["name"] = "", "", "Renamed"
	requireStatus(t, request(handler, "PUT", "/api/servers/remote", directoryJSON(peerBody), admin), 204)
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", admin), 200)
	requireStatus(t, request(handler, "DELETE", "/api/servers/remote", "", admin), 204)
	requireStatus(t, request(handler, "GET", "/api/servers/remote/sessions", "", admin), 503)
	if _, err := b.sessions.Get("owner", created.ID); err != nil {
		t.Fatal("removing peer removed provider")
	}
}

func TestFederationManagementAuthorizationValidationAndGrants(t *testing.T) {
	s, handler := fixture(t, "local")
	federationAccounts(t, s)
	admin, member := token(t, s, "owner"), token(t, s, "other")
	secret := strings.Repeat("managed-grant-", 4)
	for _, route := range []struct{ method, path string }{{"GET", "/api/server-directory"}, {"PUT", "/api/servers/remote"}, {"DELETE", "/api/servers/remote"}, {"PUT", "/api/server-grants/remote"}, {"DELETE", "/api/server-grants/remote"}} {
		requireStatus(t, request(handler, route.method, route.path, `{}`, ""), 401)
		requireStatus(t, request(handler, route.method, route.path, `{}`, member), 403)
		requireStatus(t, request(handler, route.method, route.path, `{}`, secret), 401)
	}
	for _, input := range []map[string]any{
		{"name": "Unsafe", "url": "http://host", "credential": secret},
		{"name": "Unsafe", "url": "https://user:pass@host", "credential": secret},
		{"name": "Unsafe", "url": "https://host/api", "credential": secret},
		{"name": "Unsafe", "url": "https://host", "credential": "short"},
		{"name": "Unsafe", "url": "https://host", "credential": secret + "\n"},
		{"name": "Unsafe", "url": "https://host"},
	} {
		requireStatus(t, request(handler, "PUT", "/api/servers/new", directoryJSON(input), admin), 400)
	}
	for _, id := range []string{"local", "all", "bad.id"} {
		requireStatus(t, request(handler, "PUT", "/api/servers/"+id, fmt.Sprintf(`{"name":"Invalid","url":"https://host","credential":%q}`, secret), admin), 400)
	}
	grant := map[string]any{"owner": "missing", "credential": secret}
	requireStatus(t, request(handler, "PUT", "/api/server-grants/remote", directoryJSON(grant), admin), 400)
	grant["owner"] = "owner"
	requireStatus(t, request(handler, "PUT", "/api/server-grants/remote", directoryJSON(grant), admin), 204)
	requireStatus(t, request(handler, "GET", "/api/federation/sessions", "", secret), 200)
	requireStatus(t, request(handler, "PUT", "/api/server-grants/duplicate", directoryJSON(grant), admin), 400)
	// Read API contains grant metadata, never its saved credential.
	response := request(handler, "GET", "/api/server-directory", "", admin)
	if strings.Contains(response.Body.String(), secret) {
		t.Fatal("grant credential leaked")
	}
	grant["credential"] = strings.Repeat("rotated-grant-", 4)
	requireStatus(t, request(handler, "PUT", "/api/server-grants/remote", directoryJSON(grant), admin), 204)
	requireStatus(t, request(handler, "GET", "/api/federation/sessions", "", secret), http.StatusUnauthorized)
	requireStatus(t, request(handler, "GET", "/api/federation/sessions", "", grant["credential"].(string)), 200)
	requireStatus(t, request(handler, "DELETE", "/api/server-grants/remote", "", admin), 204)
	requireStatus(t, request(handler, "GET", "/api/federation/sessions", "", grant["credential"].(string)), http.StatusUnauthorized)
	// Rejected updates have not introduced peers or removed unrelated entries.
	d, err := s.serverDirectory()
	if err != nil || len(d.Servers) != 0 || len(d.Grants) != 0 {
		t.Fatal("unexpected directory state", err)
	}
}
