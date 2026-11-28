package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestCoordinationInventoryKeepsNativeSessionsAndRejectsRemovedProviders(t *testing.T) {
	_, handler := fixture(t, "none")
	for _, path := range []string{"/api/openrig", "/api/openrig/agents", "/api/openrig/proof/stop", "/api/coordination/messages", "/api/coordination/agents", "/api/coordination/changes"} {
		method := "GET"
		if strings.Contains(path, "proof") || strings.HasSuffix(path, "messages") || strings.HasSuffix(path, "agents") {
			method = "POST"
		}
		requireStatus(t, request(handler, method, path, `{}`, ""), 404)
	}
	created := request(handler, "POST", "/api/sessions", `{"hostname":"localhost","username":"fixture","transport":"exec"}`, "")
	requireStatus(t, created, 201)
	var value session.Session
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "PATCH", "/api/sessions/"+value.ID, `{"minimized":true}`, ""), 200)
	requireStatus(t, request(handler, "POST", "/api/session-recovery/"+value.ID, `{"kind":"managed"}`, ""), 409)
	if got := request(handler, "GET", "/api/sessions/"+value.ID, "", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"minimized":true`) {
		t.Fatalf("managed recovery changed the session: %s", got.Body.String())
	}
	inventory := request(handler, "GET", "/api/coordination", "", "")
	requireStatus(t, inventory, 200)
	if !strings.Contains(inventory.Body.String(), value.ID) || !strings.Contains(inventory.Body.String(), `"enabledProviders":[]`) || !strings.Contains(inventory.Body.String(), `"removedProviders":["agent-deck","rig"]`) {
		t.Fatal(inventory.Body.String())
	}
	requireStatus(t, request(handler, "POST", "/api/coordination/attach", `{"provider":"rig","sessionId":"`+value.ID+`"}`, ""), 409)
	reopened := request(handler, "POST", "/api/coordination/attach", `{"provider":"webmux","sessionId":"`+value.ID+`"}`, "")
	requireStatus(t, reopened, 200)
	if strings.Contains(reopened.Body.String(), `"minimized":true`) {
		t.Fatal("webmux attach left the terminal minimized")
	}

	_, local := fixture(t, "local")
	owner := responseToken(t, request(local, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(local, "POST", "/api/auth/register", `{"username":"member","password":"member-password"}`, owner), 201)
	member := responseToken(t, request(local, "POST", "/api/auth/login", `{"username":"member","password":"member-password"}`, ""))
	requireStatus(t, request(local, "GET", "/api/coordination", "", member), 403)
	requireStatus(t, request(local, "GET", "/api/coordination", "", owner), 200)
}
