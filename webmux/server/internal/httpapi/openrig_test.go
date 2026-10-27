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
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/coordination"}, {"POST", "/api/coordination/agents"}, {"POST", "/api/coordination/workspaces"}, {"POST", "/api/coordination/attach"}, {"POST", "/api/coordination/messages"}, {"POST", "/api/coordination/send-status"}, {"POST", "/api/coordination/drain"}, {"GET", "/api/openrig"}, {"GET", "/api/openrig/agents"}, {"POST", "/api/openrig/attach"}, {"POST", "/api/openrig/proof/stop"}} {
		requireStatus(t, request(handler, endpoint.method, endpoint.path, "", member), 403)
	}
	requireStatus(t, request(handler, "GET", "/api/openrig", "", admin), 200)
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", "", admin), 409)
}

func TestAgentDeckCommunicationEndpointsPreserveProviderIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck integration uses Unix interactive terminal transports")
	}
	_, handler := fixture(t, "none")
	executable, _ := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))

	sent := request(handler, "POST", "/api/coordination/messages", `{"provider":"agent-deck","target":"local:local-1","message":"review this","queue":true}`, "")
	requireStatus(t, sent, 200)
	if !strings.Contains(sent.Body.String(), `"sendId":"send-local"`) || !strings.Contains(sent.Body.String(), `"queued":true`) {
		t.Fatalf("unexpected send response: %s", sent.Body.String())
	}
	status := request(handler, "POST", "/api/coordination/send-status", `{"provider":"agent-deck","target":"local:local-1","sendId":"send-local"}`, "")
	requireStatus(t, status, 200)
	if strings.Contains(status.Body.String(), "secret body") || !strings.Contains(status.Body.String(), `"target":"local:local-1"`) {
		t.Fatalf("unsafe send status: %s", status.Body.String())
	}
	drained := request(handler, "POST", "/api/coordination/drain", `{"provider":"agent-deck","remote":"sparky","target":"local:local-1"}`, "")
	requireStatus(t, drained, 200)
	if !strings.Contains(drained.Body.String(), `"from":"sparky:remote-1"`) || !strings.Contains(drained.Body.String(), `"writerRunning":true`) {
		t.Fatalf("unexpected drain response: %s", drained.Body.String())
	}
	requireStatus(t, request(handler, "POST", "/api/coordination/messages", `{"provider":"rig","target":"seat","message":"not a fallback","queue":true}`, ""), 409)
}

func TestOpenRigFollowsTrustedModeHostAccess(t *testing.T) {
	_, handler := fixture(t, "none")
	t.Setenv("PATH", t.TempDir())
	requireStatus(t, request(handler, "GET", "/api/openrig", "", ""), 200)
	requireStatus(t, request(handler, "POST", "/api/openrig/attach", "", ""), 409)
}

func TestDisabledCoordinationDoesNotDiscoverMachineWideProviders(t *testing.T) {
	_, handler := fixture(t, "none")
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "1")
	response := request(handler, "GET", "/api/coordination", "", "")
	requireStatus(t, response, 200)
	var status struct {
		Contract        string            `json:"contract"`
		DefaultProvider string            `json:"defaultProvider"`
		Providers       []json.RawMessage `json:"providers"`
		Projects        []json.RawMessage `json:"projects"`
		Seats           []json.RawMessage `json:"seats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Contract != "webmux-coordination/v1" || status.DefaultProvider != "choose" || len(status.Providers) != 0 || len(status.Projects) != 0 || len(status.Seats) != 0 {
		t.Fatalf("disabled coordination leaked provider state: %+v", status)
	}
	requireStatus(t, request(handler, "POST", "/api/coordination/attach", `{"provider":"rig","target":"seat"}`, ""), 409)
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
