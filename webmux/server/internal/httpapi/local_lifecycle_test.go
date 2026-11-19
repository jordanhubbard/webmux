package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestLocalStopRouteRetainsOwnershipAndPrivateState(t *testing.T) {
	s, h := fixture(t, "local")
	owner := responseToken(t, request(h, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(h, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, owner), 201)
	other := responseToken(t, request(h, "POST", "/api/auth/login", `{"username":"other","password":"other-password"}`, ""))
	id := "11111111-1111-4111-8111-111111111111"
	// An unavailable original owner must remain inspectable; no test process is
	// selected as a stop target on any platform.
	saved := session.Session{ID: id, Owner: "owner", Transport: "local", Kind: "terminal", Hostname: "localhost", LocalOwnerManaged: true, RecoveryKey: "private-recovery-key", LocalOwner: &terminal.LocalOwnerTarget{Version: 0, Session: id, Endpoint: "private-endpoint", OwnerPID: 0}}
	if err := s.sessions.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.store.WriteSessions(struct {
		Sessions []session.Session `yaml:"sessions"`
	}{[]session.Session{saved}}); err != nil {
		t.Fatal(err)
	}
	next, err := session.New(s.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.sessions = next
	requireStatus(t, request(h, "GET", "/api/sessions/"+id, "", other), 404)
	visible := request(h, "GET", "/api/sessions/"+id, "", owner)
	requireStatus(t, visible, 200)
	if strings.Contains(visible.Body.String(), "private-endpoint") || strings.Contains(visible.Body.String(), "private-recovery-key") || strings.Contains(visible.Body.String(), "owner_pid") {
		t.Fatal("private owner state exposed")
	}
	requireStatus(t, request(h, "DELETE", "/api/sessions/"+id, "", owner), 409)
	body := `{"id":"22222222-2222-4222-8222-222222222222","selection":"` + strings.Repeat("a", 64) + `","mode":"stop","confirm":true}`
	requireStatus(t, request(h, "POST", "/api/sessions/"+id+"/local-stop", body, ""), 401)
	requireStatus(t, request(h, "POST", "/api/sessions/"+id+"/local-stop", body, other), 404)
	requireStatus(t, request(h, "POST", "/api/sessions/"+id+"/local-stop", strings.Replace(body, `"stop"`, `"force"`, 1), owner), 400)
	var value session.Session
	if err := json.Unmarshal(visible.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.LocalLifecycle == nil {
		t.Fatal("missing explicit lifecycle fallback")
	}
	requireStatus(t, request(h, "GET", "/api/sessions/"+id, "", owner), 200)
}
