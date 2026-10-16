package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMacSetupRequiresAdmin(t *testing.T) {
	_, handler := fixture(t, "local")
	requireStatus(t, request(handler, "GET", "/api/setup/macos", "", ""), 401)
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"member","password":"member-password"}`, owner), 201)
	member := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"member","password":"member-password"}`, ""))
	requireStatus(t, request(handler, "GET", "/api/setup/macos", "", member), 403)
	requireStatus(t, request(handler, "POST", "/api/setup/macos", `{"action":"restart"}`, member), 403)
	result := request(handler, "GET", "/api/setup/macos", "", owner)
	requireStatus(t, result, 200)
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("setup status must not be cached")
	}
}
func TestMacSetupUnauthenticatedRequiresLocalSameOrigin(t *testing.T) {
	_, handler := fixture(t, "none")
	for _, tc := range []struct {
		remote, origin, site string
		want                 int
	}{
		{"192.0.2.1:1234", "", "", 403},
		{"127.0.0.1:1234", "http://evil.example", "", 403},
		{"127.0.0.1:1234", "", "cross-site", 403},
		{"127.0.0.1:1234", "http://example.com", "same-origin", 200},
		{"127.0.0.1:1234", "https://example.com", "same-origin", 200},
		{"[::1]:1234", "", "", 200},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/setup/macos", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		requireStatus(t, w, tc.want)
	}
	// Authorization runs before parsing an action or launching a helper.
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/setup/macos", strings.NewReader(`{"action":"privacy"}`))
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	requireStatus(t, w, 403)
}
