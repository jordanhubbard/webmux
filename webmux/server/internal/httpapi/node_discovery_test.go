package httpapi

import (
	"strings"
	"testing"
)

func TestNodeDiscoveryAdministratorBoundary(t *testing.T) {
	s, h := fixture(t, "local")
	federationAccounts(t, s)
	requireStatus(t, request(h, "GET", "/api/discovered-nodes", "", ""), 401)
	requireStatus(t, request(h, "GET", "/api/discovered-nodes", "", token(t, s, "other")), 403)
	response := request(h, "GET", "/api/discovered-nodes", "", token(t, s, "owner"))
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"candidates":[]`) || !strings.Contains(response.Body.String(), `"started":false`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Body.String())
	}
	// Federation grants cannot enumerate this network or configure discovery.
	secret := strings.Repeat("discovery-grant-", 4)
	requireStatus(t, request(h, "PUT", "/api/server-grants/peer", directoryJSON(map[string]any{"owner": "owner", "credential": secret}), token(t, s, "owner")), 204)
	requireStatus(t, request(h, "GET", "/api/discovered-nodes", "", secret), 401)
}
