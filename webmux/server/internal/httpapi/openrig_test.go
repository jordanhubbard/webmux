package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func TestOpenRigRequiresAdministratorAndReportsMissingInstallation(t *testing.T) {
	_, handler := fixture(t, "local")
	t.Setenv("PATH", t.TempDir())
	requireStatus(t, request(handler, "GET", "/api/openrig", "", ""), 401)
	admin := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"member","password":"member-password"}`, admin), 201)
	member := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"member","password":"member-password"}`, ""))
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/openrig"}, {"POST", "/api/openrig/attach"}} {
		requireStatus(t, request(handler, endpoint.method, endpoint.path, "", member), 403)
	}
	requireStatus(t, request(handler, "GET", "/api/openrig", "", admin), 200)
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", "", admin), 409)
}

func TestOpenRigFollowsTrustedModeHostAccess(t *testing.T) {
	_, handler := fixture(t, "none")
	t.Setenv("PATH", t.TempDir())
	requireStatus(t, request(handler, "GET", "/api/openrig", "", ""), 200)
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", "", ""), 409)
}

func TestOpenRigRevokedAdminCannotReconnectOrKeepSocket(t *testing.T) {
	s, handler := fixture(t, "local")
	admin := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password","admin":true}`, admin), 201)
	// An unavailable launcher leaves a recoverable dashboard record. No real
	// process is needed to test authorization of its viewer/control paths.
	_, _ = s.sessions.EnsureOpenRig("owner", "webmux-openrig-test-missing-executable")
	value := s.sessions.OpenRigSession("owner")
	if value == nil {
		t.Fatal("missing dashboard record")
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/term/" + value.ID + "?token=" + admin
	viewer := dialSocket(t, url)
	socketEvent(t, viewer, "viewer_join")
	socketEvent(t, viewer, "status")
	if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		for i := range c.Auth.Users {
			if c.Auth.Users[i].Username == "owner" {
				c.Auth.Users[i].Admin = false
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "POST", "/api/sessions/"+value.ID+"/reconnect", `{}`, admin), 403)
	if err := viewer.WriteJSON(map[string]string{"type": "input", "data": "should not reach dashboard"}); err != nil {
		t.Fatal(err)
	}
	socketClose(t, viewer, 1008)
	denied := dialSocket(t, url)
	socketEvent(t, denied, "error")
	socketClose(t, denied, 1008)
}
