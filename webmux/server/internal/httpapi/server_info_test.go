package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServerInfoPreservesAccountAndFederationBoundaries(t *testing.T) {
	s, handler := fixture(t, "local")
	federationAccounts(t, s)
	secret := strings.Repeat("server-info-secret-", 3)
	t.Setenv("WEBMUX_INFO_GRANT", secret)
	federationConfig(t, s, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "controller", Owner: "owner", TokenEnv: "WEBMUX_INFO_GRANT"}}})
	for _, credential := range []string{"", "invalid", secret} {
		requireStatus(t, request(handler, "GET", "/api/server-info", "", credential), 401)
	}
	for _, owner := range []string{"owner", "other"} {
		response := request(handler, "GET", "/api/server-info", "", token(t, s, owner))
		requireStatus(t, response, 200)
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("compatibility response can be cached")
		}
		var value struct {
			Contract string `json:"contract"`
			Access   struct {
				Mode, Identity, Coordination, AccountAdministration string
				Federation                                          struct{ ServerInfo, Coordination, OSSandbox bool }
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || value.Contract != "webmux-server-info/v1" || value.Access.Mode != "local" || value.Access.Identity != "account-jwt" || value.Access.Coordination != "administrator" || value.Access.AccountAdministration != "administrator" || value.Access.Federation.ServerInfo || value.Access.Federation.Coordination || value.Access.Federation.OSSandbox {
			t.Fatalf("changed access contract: %s (%v)", response.Body.String(), err)
		}
		if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "token_env") || strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), `"username"`) {
			t.Fatal("private authentication facts exposed")
		}
	}
	member := token(t, s, "other")
	requireStatus(t, request(handler, "GET", "/api/coordination", "", member), 403)
	for _, path := range []string{"/api/federation/server-info", "/api/servers/local/server-info"} {
		requireStatus(t, request(handler, "GET", path, "", secret), 404)
	}
	// Public liveness/auth discovery remain their original, minimal contracts.
	if value := request(handler, "GET", "/api/health", "", ""); value.Body.String() != "{\"name\":\"fixture\",\"status\":\"ok\"}\n" {
		t.Fatal("public health contract changed", value.Body.String())
	}
	if value := request(handler, "GET", "/api/auth/status", "", ""); value.Body.String() != "{\"bootstrap_required\":false,\"mode\":\"local\"}\n" {
		t.Fatal("public auth discovery changed", value.Body.String())
	}
}

func TestServerInfoTrustedModeAndUnknownBuildAreExplicit(t *testing.T) {
	_, handler := fixture(t, "none")
	t.Setenv("WEBMUX_VERSION", "99.99.99")
	t.Setenv("WEBMUX_REVISION", strings.Repeat("b", 40))
	for _, credential := range []string{"", "not-an-account-token"} {
		response := request(handler, "GET", "/api/server-info", "", credential)
		requireStatus(t, response, 200)
		var value map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if string(value["build"]) != `{"version":null,"revision":null,"modified":null,"provenance":"unavailable","attested":false}` {
			t.Fatal("plain build inferred environment/checkout identity", string(value["build"]))
		}
		var access struct{ Mode, Identity, Coordination, AccountAdministration string }
		if err := json.Unmarshal(value["access"], &access); err != nil || access.Mode != "none" || access.Identity != "trusted-anonymous" || access.Coordination != "trusted-anonymous" || access.AccountAdministration != "unavailable" {
			t.Fatal("trusted mode mislabeled authenticated", response.Body.String(), err)
		}
	}
	requireStatus(t, request(handler, "GET", "/api/auth/users", "", ""), 401)
}

func TestServerInfoDoesNotDiscoverEnabledProviders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("optional Agent Deck CLI uses Unix terminal transports")
	}
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	executable, calls := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	response := request(handler, "GET", "/api/server-info", "", "")
	requireStatus(t, response, 200)
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatal("metadata invoked provider discovery", err)
	}
	if !strings.Contains(response.Body.String(), `"providerAvailability":"not-observed"`) || strings.Contains(response.Body.String(), "codex-local") || strings.Contains(response.Body.String(), "/src/local") {
		t.Fatal("metadata advertised observed/private provider state", response.Body.String())
	}
	// The same enabled provider really is discoverable through the separately
	// authorized inventory route; absence of calls above is not a broken trap.
	requireStatus(t, request(handler, "GET", "/api/coordination", "", ""), 200)
	if data, err := os.ReadFile(calls); err != nil || len(data) == 0 {
		t.Fatal("provider observation control did not execute", err)
	}
}
