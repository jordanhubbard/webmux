package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/rigobservation"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestRemoteRigAuthenticatedCheckinReconnectAndRevocation(t *testing.T) {
	s, handler, owner, _ := clientFixture(t)
	web := httptest.NewServer(handler)
	defer web.Close()
	provider := &rigProvider{server: s, owner: "owner"}
	report := &rigobservation.Report{Contract: rigobservation.Contract, Source: rigobservation.Source, State: "ok", Seats: []rigobservation.Seat{{RigID: "rig-1", RigName: "project", LogicalID: "agents.worker", CanonicalSessionName: "canonical", Runtime: "codex", LifecycleState: "running", SessionStatus: "running", TmuxTarget: "original-pane"}}}
	type registered struct {
		ID         string `json:"id"`
		Credential string `json:"credential"`
		PairCode   string `json:"pair_code"`
	}
	enroll := func() registered {
		t.Helper()
		response := request(handler, "POST", "/api/clients/enroll", `{"label":"worker"}`, owner)
		requireStatus(t, response, 201)
		var r registered
		if err := json.Unmarshal(response.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		host := request(handler, "POST", "/api/hosts", `{"hostname":"same-worker.example","username":"dev","port":2222}`, owner)
		requireStatus(t, host, 201)
		var h struct{ ID string }
		if err := json.Unmarshal(host.Body.Bytes(), &h); err != nil {
			t.Fatal(err)
		}
		requireStatus(t, request(handler, "POST", "/api/hosts/"+h.ID+"/client", `{"pair_code":"`+r.PairCode+`"}`, owner), 200)
		return r
	}
	dial := func(r registered) *websocket.Conn {
		t.Helper()
		socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/api/clients/connect", http.Header{"Authorization": {"Bearer " + r.Credential}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = socket.Close() })
		var hello clientFrame
		if err := socket.ReadJSON(&hello); err != nil {
			t.Fatal(err)
		}
		return socket
	}
	send := func(socket *websocket.Conn, r *rigobservation.Report) {
		t.Helper()
		if err := socket.WriteJSON(clientFrame{Type: "snapshot", Snapshot: &clientSnapshot{Hostname: "untrusted-name", OS: "linux", Arch: "amd64", CPUs: 2, Rig: r, At: "2099-01-01T00:00:00Z"}}); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(check func(coordinationProviderStatus) bool) coordinationProviderStatus {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			status := provider.remoteStatus()
			if check(status) {
				return status
			}
			if time.Now().After(deadline) {
				t.Fatalf("remote inventory did not converge: %+v", status)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	first := enroll()
	one := dial(first)
	send(one, report)
	status := wait(func(status coordinationProviderStatus) bool {
		return len(status.Seats) == 1 && status.Seats[0].Openable
	})
	id := status.Seats[0].ID
	if status.Seats[0].Host != "same-worker.example" || status.Seats[0].Managed || status.Seats[0].ObservationState != "current" || !status.Projects[0].ReadOnly {
		t.Fatal(status)
	}
	if len((&rigProvider{server: s, owner: "other"}).remoteStatus().Seats) != 0 {
		t.Fatal("cross-owner Rig inventory leak")
	}
	if _, err := exec.LookPath("ssh"); err == nil {
		launch, err := provider.Attach(context.Background(), id)
		if err != nil || !launch.Remote || !strings.Contains(strings.Join(launch.Argv, " "), "-p 2222 -l dev -- same-worker.example exec tmux attach-session -t '=original-pane'") {
			t.Fatal(launch, err)
		}
	}
	if _, err := provider.Launch(context.Background(), coordinationLaunchRequest{ProjectID: status.Projects[0].ID}); err == nil {
		t.Fatal("observed project allowed a launch")
	}
	second := enroll()
	two := dial(second)
	send(two, report)
	status = wait(func(s coordinationProviderStatus) bool { return len(s.Seats) == 2 })
	if status.Seats[0].ID == status.Seats[1].ID || status.Projects[0].ID == status.Projects[1].ID {
		t.Fatal("same host/session text collapsed authenticated identities")
	}
	// A failed observation keeps the last canonical lifecycle with explicit
	// unavailable provenance; it cannot silently become an empty/stopped rig.
	send(one, &rigobservation.Report{Contract: rigobservation.Contract, Source: rigobservation.Source, State: "unavailable", Seats: []rigobservation.Seat{}})
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.ObservationState == "unavailable" && seat.State == "running" && !seat.Openable
			}
		}
		return false
	})
	if _, err := provider.Attach(context.Background(), id); err == nil {
		t.Fatal("unavailable observation permitted attachment")
	}
	replacement := dial(first)
	send(replacement, report)
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.Openable
			}
		}
		return false
	})
	s.clientMu.Lock()
	fact := s.clientFacts[first.ID]
	fact.RigAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	s.clientFacts[first.ID] = fact
	s.clientMu.Unlock()
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.ObservationState == "stale" && seat.State == "running" && !seat.Openable
			}
		}
		return false
	})
	send(replacement, report)
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.Openable
			}
		}
		return false
	})
	_ = replacement.Close()
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.ObservationState == "offline" && seat.State == "running" && !seat.Openable
			}
		}
		return false
	})
	// With the connection gone, persisted snapshots still carry only last
	// observed lifecycle, never permission to attach.
	s.clientMu.Lock()
	delete(s.clientFacts, first.ID)
	s.clientMu.Unlock()
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return seat.ObservationState == "offline" && !seat.Openable
			}
		}
		return false
	})
	var hostID string
	config, err := s.readClients()
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range config.Clients {
		if client.ID == first.ID {
			hostID = client.HostID
		}
	}
	requireStatus(t, request(handler, "DELETE", "/api/hosts/"+hostID+"/client", "", owner), 204)
	wait(func(s coordinationProviderStatus) bool {
		for _, seat := range s.Seats {
			if seat.ID == id {
				return false
			}
		}
		return true
	})
	requireStatus(t, request(handler, "DELETE", "/api/clients/"+first.ID, "", owner), 204)
	wait(func(s coordinationProviderStatus) bool {
		if len(s.Seats) != 1 {
			return false
		}
		return s.Seats[0].ID != id
	})
	requireStatus(t, request(handler, "DELETE", "/api/clients/"+second.ID, "", owner), 204)
	wait(func(s coordinationProviderStatus) bool { return len(s.Seats) == 0 })
}

func TestRemoteRigCannotRequestLocalNativeRecovery(t *testing.T) {
	provider := &rigProvider{home: t.TempDir()}
	target := remoteRigTarget("client-1", rigobservation.Seat{RigID: "rig-1", LogicalID: "agents.worker"})
	execute, err := provider.PlanNativeRecovery(context.Background(), session.RigIdentity{Target: target, Runtime: "codex"})
	if err == nil || execute != nil || !strings.Contains(err.Error(), "execution host") {
		t.Fatal("remote conversation crossed the local recovery boundary", err)
	}
	if key := managedRemoteKey("owner", session.RigIdentity{Target: target}); key != "owner\x00rig\x00client-1" {
		t.Fatal("remote startup failure lost its host identity", key)
	}
}
