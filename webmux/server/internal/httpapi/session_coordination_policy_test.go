package httpapi

import (
	"encoding/json"
	"testing"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
)

func TestSessionCoordinationPolicyReadAuthorizationAndPersistence(t *testing.T) {
	s, h := fixture(t, "local")
	federationAccounts(t, s)
	owner := token(t, s, "owner")
	for _, method := range []string{"GET", "PUT"} {
		body := ""
		if method == "PUT" {
			body = `{"enabled":false}`
		}
		requireStatus(t, request(h, method, "/api/session-coordination/policy", body, ""), 401)
		requireStatus(t, request(h, method, "/api/session-coordination/policy", body, token(t, s, "other")), 403)
	}
	check := func(want bool) {
		t.Helper()
		response := request(h, "GET", "/api/session-coordination/policy", "", owner)
		requireStatus(t, response, 200)
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("policy may be cached")
		}
		var policy struct {
			Contract string `json:"contract"`
			Enabled  *bool  `json:"enabled"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &policy); err != nil || policy.Contract != "webmux-session-coordination-policy/v1" || policy.Enabled == nil || *policy.Enabled != want {
			t.Fatal("wrong authoritative policy", response.Body.String(), err)
		}
	}
	check(true) // Refused writes cannot turn the default policy off.
	requireStatus(t, request(h, "PUT", "/api/session-coordination/policy", `{"enabled":false}`, owner), 204)
	check(false)
	// Read from a separately opened durable ledger, without relying on UI memory.
	if err := s.sessionCoord.service.Close(); err != nil {
		t.Fatal(err)
	}
	s.sessionCoord.service = plane.New(s.store.Home)
	check(false)
	requireStatus(t, request(h, "PUT", "/api/session-coordination/policy", `{"enabled":true}`, owner), 204)
	check(true)
}
