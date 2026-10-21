package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/coordination"}, {"POST", "/api/coordination/agents"}, {"POST", "/api/coordination/attach"}, {"GET", "/api/openrig"}, {"GET", "/api/openrig/agents"}, {"POST", "/api/openrig/attach"}, {"POST", "/api/openrig/proof/stop"}} {
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

func TestOpenRigAttachesOnlyDaemonListedTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenRig supports Unix hosts only")
	}
	_, handler := fixture(t, "none")
	directory := t.TempDir()
	t.Setenv("PATH", directory)
	for name, content := range map[string]string{
		"rig":  "#!/bin/sh\nprintf '%s\\n' '{\"entries\":[{\"canonicalSessionName\":\"builder@my-team\",\"runtime\":\"codex\",\"rigName\":\"my-team\",\"sessionStatus\":\"running\"}]}'\n",
		"tmux": "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	listing := request(handler, "GET", "/api/openrig/agents", "", "")
	requireStatus(t, listing, 200)
	if !strings.Contains(listing.Body.String(), `"target":"builder@my-team"`) {
		t.Fatal(listing.Body.String())
	}
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", `{"view":"agent","target":"unlisted; touch /tmp/unsafe"}`, ""), 404)
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", `{"view":"arbitrary-command"}`, ""), 400)
	attached := request(handler, "POST", "/api/openrig/attach", `{"view":"agent","target":"builder@my-team"}`, "")
	requireStatus(t, attached, 200)
	var value struct {
		Argv []string `json:"exec_argv"`
		View string   `json:"openrig_view"`
	}
	if err := json.Unmarshal(attached.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.View != "agent:builder@my-team" || len(value.Argv) != 4 || value.Argv[1] != "attach-session" || value.Argv[3] != "=builder@my-team" {
		t.Fatalf("unsafe or unexpected launch: %+v", value)
	}
}
