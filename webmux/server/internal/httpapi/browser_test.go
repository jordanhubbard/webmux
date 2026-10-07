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
		requireStatus(t, request(handler, method, path, `{"action":"start"}`, ""), 401)
		requireStatus(t, request(handler, method, path, `{"action":"start"}`, other), 404)
	}
	requireStatus(t, request(handler, "POST", path, `{"action":"start"}`, owner), 409)
	requireStatus(t, request(handler, "POST", path, `{"action":"navigate","url":"file:///etc/passwd"}`, owner), 400)
	// Modifier flags are a bitmask, including combinations. They must survive
	// HTTP decoding before reaching the worker (which is intentionally absent).
	for modifiers := 0; modifiers < 16; modifiers++ {
		requireStatus(t, request(handler, "POST", path, fmt.Sprintf(`{"action":"key","key":"Shift","modifiers":%d}`, modifiers), owner), 503)
	}
	requireStatus(t, request(handler, "DELETE", path, "", owner), 204)
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
	requireStatus(t, request(handler, "DELETE", path, "", owner), 204)
}
