package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestBrowserOwnershipAndValidation(t *testing.T) {
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	_, handler := fixture(t, "local")
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, owner), 201)
	other := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"other","password":"other-password"}`, ""))
	created := request(handler, "POST", "/api/sessions", `{"hostname":"localhost","username":"fixture","transport":"exec"}`, owner)
	requireStatus(t, created, 201)
	var terminal session.Session
	if err := json.Unmarshal(created.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	path := "/api/sessions/" + terminal.ID + "/browser"
	for _, method := range []string{"POST", "DELETE"} {
		requireStatus(t, request(handler, method, path, `{"action":"start","viewer":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, ""), 401)
		requireStatus(t, request(handler, method, path, `{"action":"start","viewer":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, other), 404)
	}
	for _, action := range []string{"upload", "download", "dialog", "select_option", "new_tab", "close_tab", "resize", "copy"} {
		body := fmt.Sprintf(`{"action":%q}`, action)
		requireStatus(t, request(handler, "POST", path, body, ""), 401)
		requireStatus(t, request(handler, "POST", path, body, other), 404)
	}
	requireStatus(t, request(handler, "POST", path, `{"action":"start","viewer":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, owner), 409)
	requireStatus(t, request(handler, "POST", path, `{"action":"navigate","url":"file:///etc/passwd"}`, owner), 400)
	// Modifier flags are a bitmask, including combinations. They must survive
	// HTTP decoding before reaching the worker (which is intentionally absent).
	for modifiers := 0; modifiers < 16; modifiers++ {
		requireStatus(t, request(handler, "POST", path, fmt.Sprintf(`{"action":"key","key":"Shift","modifiers":%d}`, modifiers), owner), 503)
	}
	requireStatus(t, request(handler, "DELETE", path, `{}`, owner), 204)
	response := request(handler, "POST", path, `{"action":"frame"}`, owner)
	requireStatus(t, response, 503)
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("browser response may be cached")
	}
}

func TestBrowserTrafficHasSeparateBoundedBudget(t *testing.T) {
	_, handler := fixture(t, "none")
	for i := 0; i < 1800; i++ {
		requireStatus(t, request(handler, "POST", "/api/sessions/missing/browser", `{"action":"frame"}`, ""), 404)
	}
	requireStatus(t, request(handler, "POST", "/api/sessions/missing/browser", `{"action":"frame"}`, ""), 429)
	requireStatus(t, request(handler, "GET", "/api/auth/status", "", ""), 200)
	requireStatus(t, request(handler, "GET", "/api/sessions", "", ""), 200)
}

func TestAuthHandoffOwnershipAndInvalidTargets(t *testing.T) {
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	_, handler := fixture(t, "local")
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, owner), 201)
	other := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"other","password":"other-password"}`, ""))
	created := request(handler, "POST", "/api/sessions", `{"hostname":"localhost","username":"fixture","transport":"exec"}`, owner)
	requireStatus(t, created, 201)
	var terminal session.Session
	if err := json.Unmarshal(created.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	path := "/api/sessions/" + terminal.ID + "/auth-handoff"
	for _, method := range []string{"POST", "DELETE"} {
		requireStatus(t, request(handler, method, path, `{}`, ""), 401)
		requireStatus(t, request(handler, method, path, `{}`, other), 404)
	}
	response := request(handler, "POST", path, `{"authorization":"https://example.com"}`, owner)
	requireStatus(t, response, 409) // exec has no provable callback host
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable callback")
	}
	requireStatus(t, request(handler, "DELETE", path, `{}`, owner), 204)
}

func TestBrowserProfilesStayWithOwnerAndExecutionPrincipal(t *testing.T) {
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	_, handler := fixture(t, "local")
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, owner), 201)
	other := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"other","password":"other-password"}`, ""))
	create := func(token, host string) session.Session {
		t.Helper()
		response := request(handler, "POST", "/api/sessions", fmt.Sprintf(`{"hostname":%q,"username":"fixture","transport":"ssh"}`, host), token)
		requireStatus(t, response, 201)
		var s session.Session
		if err := json.Unmarshal(response.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	a := create(owner, "first.example")
	same := create(owner, "first.example")
	different := create(owner, "second.example")
	b := create(other, "first.example")
	path := func(id string) string { return "/api/sessions/" + id + "/browser/profiles" }
	created := request(handler, "POST", path(a.ID), `{"name":"Sign-in"}`, owner)
	requireStatus(t, created, 201)
	var value map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 2 || value["id"] == "" || value["name"] != "Sign-in" {
		t.Fatal("private registry fields leaked", value)
	}
	for _, test := range []struct {
		id, token string
		count     int
	}{{same.ID, owner, 1}, {different.ID, owner, 0}, {b.ID, other, 0}} {
		response := request(handler, "GET", path(test.id), "", test.token)
		requireStatus(t, response, 200)
		var values []map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != test.count {
			t.Fatal("profile scope crossed", values)
		}
	}
	requireStatus(t, request(handler, "GET", path(a.ID), "", other), 404)
	requireStatus(t, request(handler, "POST", path(a.ID), `{"name":"Sign-in"}`, owner), 409)
	body := fmt.Sprintf(`{"action":"start","viewer":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","profileId":%q}`, value["id"])
	requireStatus(t, request(handler, "POST", "/api/sessions/"+different.ID+"/browser", body, owner), 404)
	requireStatus(t, request(handler, "POST", "/api/sessions/"+b.ID+"/browser", body, other), 404)
}

func TestBrowserProfileScopeUsesPrincipalRatherThanTransportOrKey(t *testing.T) {
	first := session.Session{Transport: "ssh", Hostname: "HOST.example", Username: "alice", KeyID: "old"}
	original, _ := browserProfileScope(first)
	second := first
	second.Transport = "mosh"
	second.Hostname = "host.example"
	second.KeyID = "new"
	second.Port = 22
	same, _ := browserProfileScope(second)
	if same != original {
		t.Fatal("key rotation or Mosh changed principal")
	}
	for _, change := range []func(*session.Session){func(s *session.Session) { s.Username = "bob" }, func(s *session.Session) { s.Port = 2222 }, func(s *session.Session) { s.Hostname = "other.example" }, func(s *session.Session) { s.BrowserLocal = true }} {
		s := first
		change(&s)
		different, _ := browserProfileScope(s)
		if different == original {
			t.Fatal("execution principal was not scoped")
		}
	}
}

func TestBrowserProfileFailedHostDeletionRetainsRegistry(t *testing.T) {
	_, handler := fixture(t, "none")
	response := request(handler, "POST", "/api/sessions", `{"transport":"local"}`, "")
	requireStatus(t, response, 201)
	var terminal session.Session
	if err := json.Unmarshal(response.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	path := "/api/sessions/" + terminal.ID + "/browser/profiles"
	created := request(handler, "POST", path, `{"name":"Keep metadata"}`, "")
	requireStatus(t, created, 201)
	var profile map[string]string
	json.Unmarshal(created.Body.Bytes(), &profile)
	// The test executable cannot execute the production worker mode. A failed
	// remote/worker removal must not silently orphan the saved-profile registry.
	removed := request(handler, "DELETE", path, fmt.Sprintf(`{"profileId":%q}`, profile["id"]), "")
	requireStatus(t, removed, 409)
	listed := request(handler, "GET", path, "", "")
	requireStatus(t, listed, 200)
	var profiles []map[string]string
	json.Unmarshal(listed.Body.Bytes(), &profiles)
	if len(profiles) != 1 || profiles[0]["id"] != profile["id"] {
		t.Fatal("failed removal lost registry", profiles)
	}
}
