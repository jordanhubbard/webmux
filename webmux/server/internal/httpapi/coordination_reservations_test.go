package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/coordinationmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
)

func (p *coordinationPair) reservations(t *testing.T, enabled bool) {
	t.Helper()
	node, err := p.a.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_owner": "owner", "coordination_reservations": enabled}), token(t, p.a, "owner")), 204)
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", directoryJSON(map[string]any{"owner": "owner", "coordination": true, "coordination_reservations": enabled, "source_node_id": node, "source_owner": "owner"}), token(t, p.b, "owner")), 204)
	p.refresh(t)
}
func sharedHTTPAct(t *testing.T, p *coordinationPair, r plane.Request) plane.Result {
	t.Helper()
	response := request(p.agent, "POST", "/act", directoryJSON(r), p.access[0].Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCoordinationSharedReservationLostReplyThenVerifiedLease(t *testing.T) {
	var lost atomic.Bool
	var admits, statuses atomic.Int64
	p := peerPairHook(t, "owner", func(next http.Handler, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == coordinationReservationsPath {
			var input reservationExchange
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				return
			}
			data, _ := json.Marshal(input)
			r.Body = io.NopCloser(bytes.NewReader(data))
			if input.Operation == "admit" {
				admits.Add(1)
				if lost.CompareAndSwap(false, true) {
					recorded := httptest.NewRecorder()
					next.ServeHTTP(recorded, r)
					if recorded.Code != 200 {
						t.Error("authority did not commit before reply loss", recorded.Code)
						return
					}
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
			} else {
				statuses.Add(1)
			}
		}
		next.ServeHTTP(w, r)
	})
	p.reservations(t, true)
	resource := "node:" + p.remote[0].Subject.Node + ":heavy"
	intent := plane.Request{ID: "lost-remote-worker-reserve", Action: "reserve", Resource: resource}
	pending := sharedHTTPAct(t, p, intent)
	if pending.Status != "reservation-pending" || pending.Shared == nil || pending.Shared.Transport != "uncertain" || pending.Lease != nil {
		t.Fatal(pending)
	}
	granted := sharedHTTPAct(t, p, intent)
	if granted.Status != "reserved" || granted.Fence != 1 || granted.Lease == nil || !granted.Lease.Current || granted.Expires == nil {
		t.Fatal(granted)
	}
	if admits.Load() != 1 || statuses.Load() != 1 {
		t.Fatal("uncertain reservation was submitted again", admits.Load(), statuses.Load())
	}
	// Local use of exactly the same authority is busy, preserving the global fence.
	local := request(p.b.CoordinationAgentHandler(), "POST", "/act", directoryJSON(plane.Request{ID: "local-contending-worker", Action: "reserve", Resource: resource}), p.remote[0].Capability)
	requireStatus(t, local, 200)
	if !strings.Contains(local.Body.String(), `"status":"busy"`) {
		t.Fatal(local.Body.String())
	}
	// Message permission was never enabled; reservation scope grants no inbox write.
	requireStatus(t, request(p.bh, "POST", coordinationMessagesPath, `{}`, p.secret), 403)
	renewal := plane.Request{ID: "renew-remote-worker", Action: "renew-reservation", Resource: resource, Record: granted.Record, Fence: granted.Fence}
	renewed := sharedHTTPAct(t, p, renewal)
	if renewed.Status != "renewed" || renewed.Fence != granted.Fence || renewed.Record != granted.Record || renewed.Lease == nil || !renewed.Lease.Current {
		t.Fatal(renewed)
	}
	released := sharedHTTPAct(t, p, plane.Request{ID: "release-remote-worker", Action: "release-reservation", Resource: resource, Record: granted.Record, Fence: granted.Fence})
	if released.Status != "released" {
		t.Fatal(released)
	}
	old := sharedHTTPAct(t, p, intent)
	if old.Status != "reserved" || old.Lease == nil || old.Lease.Current || !old.Expires.Equal(*granted.Expires) {
		t.Fatal("old receipt revived released lease", old)
	}
	next := request(p.b.CoordinationAgentHandler(), "POST", "/act", directoryJSON(plane.Request{ID: "local-next-worker", Action: "reserve", Resource: resource}), p.remote[0].Capability)
	requireStatus(t, next, 200)
	var localGrant plane.Result
	_ = json.Unmarshal(next.Body.Bytes(), &localGrant)
	if localGrant.Status != "reserved" || localGrant.Fence != 2 {
		t.Fatal(localGrant)
	}
}
func TestCoordinationSharedReservationSeparatePermissionAndSourceRevocation(t *testing.T) {
	p := peerPair(t)
	p.messaging(t, true)
	intent := plane.Request{ID: "message-scope-no-worker", Action: "reserve", Resource: "node:" + p.remote[0].Subject.Node + ":heavy"}
	denied := sharedHTTPAct(t, p, intent)
	if denied.Status != "peer-unavailable" || denied.Shared != nil {
		t.Fatal("message permission acquired a worker", denied)
	}
	p.reservations(t, true)
	if repeated := sharedHTTPAct(t, p, intent); repeated.Status != "peer-unavailable" {
		t.Fatal("frozen denial upgraded", repeated)
	}
	intent.ID = "allowed-remote-worker"
	granted := sharedHTTPAct(t, p, intent)
	if granted.Lease == nil || !granted.Lease.Current {
		t.Fatal(granted)
	}
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_reservations": false}), token(t, p.a, "owner")), 204)
	uncertain := sharedHTTPAct(t, p, intent)
	if uncertain.Lease == nil || uncertain.Lease.Current || uncertain.Shared.Transport != "authorization-unavailable" {
		t.Fatal("revoked outgoing permission retained a work grant", uncertain)
	}
	foreign := request(p.agent, "POST", "/act", directoryJSON(plane.Request{ID: "foreign-remote-worker", Action: "reserve", Resource: intent.Resource}), p.access[2].Capability)
	requireStatus(t, foreign, 200)
	if !strings.Contains(foreign.Body.String(), "peer-unavailable") {
		t.Fatal("foreign account borrowed mapped scope", foreign.Body.String())
	}
}

func TestCoordinationReservationRequiresLeaseAwareClient(t *testing.T) {
	p := peerPair(t)
	node, _ := p.a.coordinationNode()
	body := directoryJSON(plane.Request{ID: "old-client-reservation", Action: "reserve", Resource: "node:" + node + ":heavy"})
	req := httptest.NewRequest("POST", "/act", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+p.access[0].Capability)
	response := httptest.NewRecorder()
	p.agent.ServeHTTP(response, req)
	requireStatus(t, response, 428)
	view := p.view(t, 0)
	if view.Resources[0].Occupied {
		t.Fatal("legacy request acquired a lease")
	}
	current := request(p.agent, "POST", "/act", body, p.access[0].Capability)
	requireStatus(t, current, 200)
	var result plane.Result
	if json.Unmarshal(current.Body.Bytes(), &result) != nil || result.Lease == nil || !result.Lease.Current {
		t.Fatal(current.Body.String())
	}
}

func TestCoordinationSharedReservationThroughScopedMCP(t *testing.T) {
	p := peerPair(t)
	p.reservations(t, true)
	file := filepath.Join(t.TempDir(), "access.json")
	if err := coordinationclient.WriteAccess(file, p.access[0]); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", p.access[0].Subject.Session)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	one, two := mcp.NewInMemoryTransports()
	server, err := coordinationmcp.NewServer().Connect(ctx, one, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "shared-reservation-qualification", Version: "1"}, nil).Connect(ctx, two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: map[string]any{"request_id": "mcp-shared-worker", "action": "reserve", "resource": "node:" + p.remote[0].Subject.Node + ":heavy"}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	data, _ := json.Marshal(result.StructuredContent)
	var grant plane.Result
	if json.Unmarshal(data, &grant) != nil || grant.Status != "reserved" || grant.Lease == nil || !grant.Lease.Current || grant.Shared == nil {
		t.Fatal(string(data))
	}
}

func TestCoordinationSharedReservationRevocationDuringRecovery(t *testing.T) {
	for _, control := range []struct {
		name, operation, state string
		knownGrant, attempted  bool
	}{
		{"absent", "status", "absent", false, true},
		{"known-grant", "status", "result", true, true},
		{"new-grant", "admit", "result", false, false},
	} {
		t.Run(control.name, func(t *testing.T) {
			var delayed atomic.Bool
			var admits atomic.Int64
			replyReady, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			p := peerPairHook(t, "owner", func(next http.Handler, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == coordinationReservationsPath {
					var input reservationExchange
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
						return
					}
					data, _ := json.Marshal(input)
					r.Body = io.NopCloser(bytes.NewReader(data))
					if input.Operation == "admit" {
						admits.Add(1)
					}
					if input.Operation == control.operation && delayed.CompareAndSwap(false, true) {
						captured := httptest.NewRecorder()
						next.ServeHTTP(captured, r)
						var reply plane.ReservationReply
						if captured.Code != 200 || json.Unmarshal(captured.Body.Bytes(), &reply) != nil || reply.State != control.state {
							t.Error("reply did not return the expected authenticated reservation state")
						}
						close(replyReady)
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						for name, values := range captured.Header() {
							w.Header()[name] = values
						}
						w.WriteHeader(captured.Code)
						_, _ = w.Write(captured.Body.Bytes())
						return
					}
				}
				next.ServeHTTP(w, r)
			})
			p.a.sessionCoord.cancel()
			<-p.a.sessionCoord.done
			p.a.sessionCoord.cancel = nil
			p.reservations(t, true)
			node, _ := p.a.coordinationNode()
			inputs := mustCoordinationInputs(t, p.a, node)
			intent := plane.Request{ID: "revoke-reservation-during-status", Action: "reserve", Resource: "node:" + p.remote[0].Subject.Node + ":heavy"}
			var pending plane.Result
			if control.knownGrant {
				pending = sharedHTTPAct(t, p, intent)
				if pending.Status != "reserved" || pending.Shared == nil || pending.Lease == nil || !pending.Lease.Current {
					t.Fatal("initial reservation was not granted", pending)
				}
			} else {
				route := p.a.coordinationReservationRoute(p.access[0].Subject, intent, inputs)
				var err error
				pending, err = p.a.sessionCoord.service.ExecuteRouted(node, p.access[0].Capability, inputs, intent, route)
				if err != nil || pending.Status != "reservation-pending" || pending.Shared == nil {
					t.Fatal("reservation intent was not queued", pending, err)
				}
				// Simulate interruption after uncertainty is durable but before any RPC.
				if control.attempted {
					if allowed, err := p.a.sessionCoord.service.BeginSharedAttempt(node, p.access[0].Capability, inputs, intent); err != nil || !allowed {
						t.Fatal("could not establish the durable attempt boundary", err)
					}
				}
			}
			retained, _, err := p.a.reservationIntent(node, p.access[0].Capability, intent)
			if err != nil || retained.Attempted != control.attempted || retained.Resolved != control.knownGrant {
				t.Fatal("attempt boundary or original outcome was not retained", err)
			}
			before := admits.Load()
			// A held admission RPC starts and commits before revocation.
			if control.operation == "admit" {
				before++
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- request(p.agent, "POST", "/act", directoryJSON(intent), p.access[0].Capability) }()
			select {
			case <-replyReady:
			case <-time.After(3 * time.Second):
				t.Fatal("held reservation reply did not arrive")
			}
			setOutgoing := func(enabled bool) {
				t.Helper()
				requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_reservations": enabled}), token(t, p.a, "owner")), 204)
			}
			setOutgoing(false)
			once.Do(func() { close(release) })
			var response *httptest.ResponseRecorder
			select {
			case response = <-done:
			case <-time.After(7 * time.Second):
				t.Fatal("reservation recovery did not finish")
			}
			requireStatus(t, response, 200)
			if admits.Load() != before {
				t.Fatal("a new reservation admission was transmitted after permission was removed")
			}
			var blocked plane.Result
			if err := json.Unmarshal(response.Body.Bytes(), &blocked); err != nil || blocked.Shared == nil || blocked.Shared.Transport != "authorization-unavailable" || (blocked.Lease != nil && blocked.Lease.Current) {
				t.Fatal("revoked attempt retained a usable grant", blocked, err)
			}
			setOutgoing(true)
			granted := sharedHTTPAct(t, p, intent)
			if granted.Status != "reserved" || granted.Lease == nil || !granted.Lease.Current || granted.Shared == nil || granted.Shared.Intent != pending.Shared.Intent || granted.Fence != 1 || admits.Load() != 1 {
				t.Fatal("restored permission did not recover the original intent exactly once", granted)
			}
		})
	}
}
