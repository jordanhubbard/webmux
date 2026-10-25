package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func clientFixture(t *testing.T) (*Server, http.Handler, string, string) {
	t.Helper()
	s, handler := fixture(t, "local")
	for name, data := range map[string]string{"hosts.yaml": "hosts: []\n", "clients.yaml": "clients: []\n"} {
		if err := os.WriteFile(s.store.ConfigPath(name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, owner), 201)
	other := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"other","password":"other-password"}`, ""))
	return s, handler, owner, other
}

func TestClientEnrollmentHostBindingAndRevocation(t *testing.T) {
	s, handler, owner, other := clientFixture(t)
	requireStatus(t, request(handler, "POST", "/api/clients/enroll", `{"label":"worker"}`, ""), 401)
	enrolled := request(handler, "POST", "/api/clients/enroll", `{"label":"worker"}`, owner)
	requireStatus(t, enrolled, 201)
	var client struct {
		ID         string `json:"id"`
		Credential string `json:"credential"`
		PairCode   string `json:"pair_code"`
	}
	if err := json.Unmarshal(enrolled.Body.Bytes(), &client); err != nil || client.ID == "" || client.Credential == "" || len(client.PairCode) != 16 {
		t.Fatalf("invalid enrollment: %v", err)
	}
	stored, err := os.ReadFile(s.store.ConfigPath("clients.yaml"))
	if err != nil || strings.Contains(string(stored), client.Credential) || strings.Contains(string(stored), client.PairCode) {
		t.Fatal("client credential or pairing code persisted in plaintext")
	}
	if listed := request(handler, "GET", "/api/clients", "", other); listed.Code != 200 || strings.Contains(listed.Body.String(), client.ID) {
		t.Fatal("client leaked to another user")
	}
	host := request(handler, "POST", "/api/hosts", `{"hostname":"worker.internal","username":"dev","port":22}`, owner)
	requireStatus(t, host, 201)
	var saved struct{ ID string }
	if err := json.Unmarshal(host.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	path := "/api/hosts/" + saved.ID + "/client"
	requireStatus(t, request(handler, "POST", path, `{"pair_code":"`+client.PairCode+`"}`, other), 404)
	requireStatus(t, request(handler, "POST", path, `{"pair_code":"`+client.PairCode+`"}`, owner), 200)
	requireStatus(t, request(handler, "POST", path, `{"pair_code":"`+client.PairCode+`"}`, owner), 409)
	requireStatus(t, request(handler, "PUT", "/api/hosts/"+saved.ID, `{"hostname":"elsewhere.internal"}`, owner), 409)
	requireStatus(t, request(handler, "PUT", "/api/hosts/"+saved.ID, `{"tags":["managed"]}`, owner), 200)

	web := httptest.NewServer(handler)
	t.Cleanup(web.Close)
	headers := http.Header{"Authorization": {"Bearer " + client.Credential}}
	socket, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/api/clients/connect", headers)
	if err != nil {
		t.Fatalf("client connection: %v (%v)", err, response)
	}
	defer socket.Close()
	var hello clientFrame
	if err := socket.ReadJSON(&hello); err != nil || hello.Type != "hello" || hello.Protocol != clientProtocol {
		t.Fatalf("protocol hello: %+v %v", hello, err)
	}
	if err := socket.WriteJSON(clientFrame{Type: "snapshot", Snapshot: &clientSnapshot{Hostname: "worker", OS: "linux", Arch: "amd64", CPUs: 8}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		listed := request(handler, "GET", "/api/clients", "", owner)
		if strings.Contains(listed.Body.String(), `"online":true`) && strings.Contains(listed.Body.String(), `"linux"`) {
			if strings.Contains(listed.Body.String(), client.Credential) || strings.Contains(listed.Body.String(), client.PairCode) {
				t.Fatal("secret leaked to UI")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("client did not report facts: %s", listed.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	refreshDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { refreshDone <- request(handler, "POST", "/api/clients/"+client.ID+"/refresh", `{}`, owner) }()
	var command clientFrame
	if err := socket.ReadJSON(&command); err != nil || command.Type != "request" || command.Command != "system.snapshot" {
		t.Fatalf("refresh request: %+v %v", command, err)
	}
	if err := socket.WriteJSON(clientFrame{Type: "snapshot", ID: command.ID, Snapshot: &clientSnapshot{Hostname: "worker", OS: "linux", Arch: "amd64", CPUs: 12}}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-refreshDone:
		requireStatus(t, result, 200)
		if !strings.Contains(result.Body.String(), `"cpus":12`) {
			t.Fatalf("stale refresh: %s", result.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not finish")
	}
	requireStatus(t, request(handler, "POST", "/api/clients/"+client.ID+"/refresh", `{}`, other), 404)
	second, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/api/clients/connect", headers)
	if err != nil {
		t.Fatalf("client reconnect: %v (%v)", err, response)
	}
	defer second.Close()
	if err := second.ReadJSON(&hello); err != nil {
		t.Fatal(err)
	}
	if err := second.WriteJSON(clientFrame{Type: "snapshot", Snapshot: &clientSnapshot{Hostname: "worker", OS: "linux", Arch: "amd64", CPUs: 16}}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		listed := request(handler, "GET", "/api/clients", "", owner)
		if strings.Contains(listed.Body.String(), `"cpus":16`) && strings.Contains(listed.Body.String(), `"online":true`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reconnected client lost: %s", listed.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	requireStatus(t, request(handler, "DELETE", "/api/hosts/"+saved.ID, "", owner), 204)
	if listed := request(handler, "GET", "/api/clients", "", owner); strings.Contains(listed.Body.String(), `"host_id"`) {
		t.Fatalf("deleted host remains linked: %s", listed.Body.String())
	}
	requireStatus(t, request(handler, "DELETE", "/api/clients/"+client.ID, "", owner), 204)
	if listed := request(handler, "GET", "/api/clients", "", owner); strings.Contains(listed.Body.String(), client.ID) {
		t.Fatal("revoked client remains listed")
	}
	requester := httptest.NewRequest("POST", "/api/clients/pair-code", strings.NewReader(`{}`))
	requester.Header.Set("Authorization", "Bearer "+client.Credential)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, requester)
	requireStatus(t, result, 401)
}
