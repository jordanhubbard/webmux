package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestSessionHTTPOwnershipAndPatchPrecedence(t *testing.T) {
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	_, handler := fixture(t, "local")
	password := rand.Text()
	body := func(fields map[string]string) string {
		t.Helper()
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	credentials := func(username string) string {
		return body(map[string]string{"username": username, "password": password})
	}
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", credentials("owner"), ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", credentials("other"), owner), 201)
	other := responseToken(t, request(handler, "POST", "/api/auth/login", credentials("other"), ""))
	requireStatus(t, request(handler, "GET", "/api/sessions", "", ""), 401)
	requireStatus(t, request(handler, "POST", "/api/sessions", `{"hostname":"localhost"}`, owner), 400)
	created := request(handler, "POST", "/api/sessions/", body(map[string]string{"hostname": "localhost", "username": "alice", "transport": "exec", "password": password}), owner)
	requireStatus(t, created, 201)
	var value session.Session
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.State != "error" || value.Title != "localhost:22" || value.Owner != "owner" || value.Cols != 80 || value.Rows != 24 {
		t.Fatalf("defaults: %+v", value)
	}
	if strings.Contains(created.Body.String(), password) {
		t.Fatal("password exposed")
	}
	path := "/api/sessions/" + value.ID
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		requireStatus(t, request(handler, method, path, `{}`, other), 404)
	}
	requireStatus(t, request(handler, "POST", path+"/reconnect", `{}`, other), 404)
	listed := request(handler, "GET", "/api/sessions", "", other)
	requireStatus(t, listed, 200)
	if strings.TrimSpace(listed.Body.String()) != "[]" {
		t.Fatal("foreign session listed")
	}
	patched := request(handler, "PATCH", path, `{"minimized":true,"title":"ignored","row":2,"col":3}`, owner)
	requireStatus(t, patched, 200)
	if err := json.Unmarshal(patched.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Minimized || value.Title != "localhost:22" || value.Row != 0 {
		t.Fatal("patch precedence changed")
	}
	requireStatus(t, request(handler, "PATCH", path, `{"title":"   "}`, owner), 400)
	requireStatus(t, request(handler, "PATCH", path, `{"row":-1,"col":0}`, owner), 400)
	requireStatus(t, request(handler, "PATCH", path, `{"row":1.5,"col":0}`, owner), 400)
	requireStatus(t, request(handler, "PATCH", path, `{"title":" renamed "}`, owner), 200)
	requireStatus(t, request(handler, "POST", path+"/reconnect", `{}`, owner), 500)
	requireStatus(t, request(handler, "DELETE", path, "", owner), 204)
	requireStatus(t, request(handler, "GET", path, "", owner), 404)
}
