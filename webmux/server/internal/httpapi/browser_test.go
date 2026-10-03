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
