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

func (p *coordinationPair) messaging(t *testing.T, enabled bool) {
	t.Helper()
	node, err := p.a.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_owner": "owner", "coordination_messages": enabled}), token(t, p.a, "owner")), 204)
	requireStatus(t, request(p.bh, "PUT", "/api/server-grants/controller", directoryJSON(map[string]any{"owner": "owner", "coordination": true, "coordination_messages": enabled, "source_node_id": node, "source_owner": "owner"}), token(t, p.b, "owner")), 204)
	p.refresh(t)
}
func coordinationMessageWait(t *testing.T, p *coordinationPair, id, state, transport string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		view := p.view(t, 0)
		for _, message := range view.Outbox {
			if message.ID == id && message.State == state && (transport == "" || message.Transport == transport) {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("message did not reach %s/%s: %+v", state, transport, p.view(t, 0).Outbox)
}
func TestCoordinationRemoteMessageLostHTTPSReplyUsesStatusAndExplicitReceipts(t *testing.T) {
	var lost atomic.Bool
	var admits, statuses atomic.Int64
	p := peerPairHook(t, "owner", func(next http.Handler, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == coordinationMessagesPath {
			var body coordinationMessageExchange
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			data, _ := json.Marshal(body)
			r.Body = io.NopCloser(bytes.NewReader(data))
			if body.Operation == "admit" {
				admits.Add(1)
				if lost.CompareAndSwap(false, true) {
					recorded := httptest.NewRecorder()
					next.ServeHTTP(recorded, r)
					if recorded.Code != 200 {
						t.Errorf("admission before dropped reply: %d %s", recorded.Code, recorded.Body.String())
					}
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
			} else {
				statuses.Add(1)
			}
		}
		next.ServeHTTP(w, r)
	})
	p.messaging(t, true)
	target := p.remote[1].Subject
	operation := plane.Request{ID: "https-remote-handoff", Action: "message", Target: &target, Kind: "handoff", Text: "Remote inbox task: review ownership."}
	response := request(p.agent, "POST", "/act", directoryJSON(operation), p.access[0].Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Transport != "pending" {
		t.Fatal(result, err)
	}
	coordinationMessageWait(t, p, result.Record, "admitted", "remote-admitted")
	if admits.Load() != 1 || statuses.Load() < 1 {
		t.Fatal("lost reply replayed admission instead of querying status", admits.Load(), statuses.Load())
	}
	recipient := p.b.CoordinationAgentHandler()
	viewResponse := request(recipient, "GET", "/view", "", p.remote[1].Capability)
	requireStatus(t, viewResponse, 200)
	var view plane.View
	if err := json.Unmarshal(viewResponse.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range view.Inbox {
		if message.ID == result.Record {
			count++
			if message.State != "admitted" || message.From != p.access[0].Subject {
				t.Fatal(message)
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate or missing remote inbox", count)
	}
	foreign := request(recipient, "POST", "/act", directoryJSON(plane.Request{ID: "foreign-remote-receipt", Action: "delivered", Record: result.Record}), p.remote[2].Capability)
	requireStatus(t, foreign, 200)
	if !strings.Contains(foreign.Body.String(), "not-owned") {
		t.Fatal(foreign.Body.String())
	}
	for _, action := range []string{"delivered", "acknowledge", "complete"} {
		requireStatus(t, request(recipient, "POST", "/act", directoryJSON(plane.Request{ID: "remote-receipt-" + action, Action: action, Record: result.Record}), p.remote[1].Capability), 200)
	}
	coordinationMessageWait(t, p, result.Record, "completed", "remote-admitted")
	repeated := request(p.agent, "POST", "/act", directoryJSON(operation), p.access[0].Capability)
	requireStatus(t, repeated, 200)
	if repeated.Body.String() != response.Body.String() {
		t.Fatal("original admission changed")
	}
	text, err := p.b.sessions.Scrollback("owner", target.Session)
	if err != nil || strings.Contains(text, operation.Text) {
		t.Fatal("remote message entered the terminal", err)
	}
}

func TestCoordinationRemoteMessagePermissionsAndRevocation(t *testing.T) {
	p := peerPair(t)
	// Quiesce the isolated source before establishing fresh snapshots. Canceling
	// an in-flight background fetch later can correctly mark its cache stale,
	// which is unrelated to the outgoing-revocation assertion below.
	p.a.sessionCoord.cancel()
	<-p.a.sessionCoord.done
	p.a.sessionCoord.cancel = nil
	p.mapOwner(t, "owner")
	p.share(t, true)
	p.refresh(t)
	target := p.remote[0].Subject
	operation := plane.Request{ID: "summary-only-denial", Action: "message", Target: &target, Kind: "message", Text: "Review"}
	denied := request(p.agent, "POST", "/act", directoryJSON(operation), p.access[0].Capability)
	requireStatus(t, denied, 200)
	if !strings.Contains(denied.Body.String(), "peer-unavailable") {
		t.Fatal("summary grant gained mutation authority", denied.Body.String())
	}
	p.messaging(t, true)
	// A denied intent stays denied after permission is subsequently enabled.
	repeat := request(p.agent, "POST", "/act", directoryJSON(operation), p.access[0].Capability)
	if repeat.Body.String() != denied.Body.String() {
		t.Fatal("permission upgraded a frozen denial")
	}
	operation.ID = "revoked-before-dispatch"
	response := request(p.agent, "POST", "/act", directoryJSON(operation), p.access[0].Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	if result.Transport != "pending" {
		t.Fatal(result)
	}
	requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_messages": false}), token(t, p.a, "owner")), 204)
	if err := p.a.maintainCoordinationMessages(context.Background()); err != nil {
		t.Fatal(err)
	}
	coordinationMessageWait(t, p, result.Record, "admitted", "authorization-unavailable")
	node, _ := p.b.coordinationNode()
	recipient, err := p.b.sessionCoord.service.AgentView(node, p.remote[0].Capability, mustCoordinationInputs(t, p.b, node), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range recipient.Inbox {
		if message.ID == result.Record {
			t.Fatal("revoked outgoing authority delivered")
		}
	}
	// Message endpoint rejects a summary-only credential, before any inbox transition.
	p.share(t, false)
	wire := plane.WireMessage{}
	requireStatus(t, request(p.bh, "POST", coordinationMessagesPath, directoryJSON(coordinationMessageExchange{"admit", wire}), p.secret), 403)
}

func TestCoordinationMessageRevocationDuringStatusPreventsAdmission(t *testing.T) {
	var delayed atomic.Bool
	var admits atomic.Int64
	statusReady, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	p := peerPairHook(t, "owner", func(next http.Handler, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == coordinationMessagesPath {
			var body coordinationMessageExchange
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			data, _ := json.Marshal(body)
			r.Body = io.NopCloser(bytes.NewReader(data))
			if body.Operation == "admit" {
				admits.Add(1)
			}
			if body.Operation == "status" && delayed.CompareAndSwap(false, true) {
				captured := httptest.NewRecorder()
				next.ServeHTTP(captured, r)
				var receipt plane.RemoteReceipt
				if captured.Code != 200 || json.Unmarshal(captured.Body.Bytes(), &receipt) != nil || receipt.State != "absent" {
					t.Error("status did not prove authenticated absence")
				}
				close(statusReady)
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
	// Own dispatch deterministically; the receiver and its real TLS API stay live.
	p.a.sessionCoord.cancel()
	<-p.a.sessionCoord.done
	p.a.sessionCoord.cancel = nil
	p.messaging(t, true)
	target := p.remote[0].Subject
	response := request(p.agent, "POST", "/act", directoryJSON(plane.Request{ID: "revoke-during-status", Action: "message", Target: &target, Kind: "handoff", Text: "Review after permission check"}), p.access[0].Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Transport != "pending" {
		t.Fatal("message was not queued", err)
	}
	node, _ := p.a.coordinationNode()
	// Simulate interruption after the durable attempted marker, before sending.
	if allowed, err := p.a.sessionCoord.service.BeginRemoteAttempt(node, result.Record, mustCoordinationInputs(t, p.a, node)); err != nil || !allowed {
		t.Fatal("could not retain the uncertainty boundary", err)
	}
	records, err := p.a.sessionCoord.service.RemoteRecords(node)
	if err != nil || len(records) != 1 || !records[0].Attempted {
		t.Fatal("attempt marker was not durable", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.a.deliverCoordinationMessage(context.Background(), node, records[0]) }()
	select {
	case <-statusReady:
	case <-time.After(3 * time.Second):
		t.Fatal("status query did not arrive")
	}
	setOutgoing := func(enabled bool) {
		t.Helper()
		requireStatus(t, request(p.ah, "PUT", "/api/servers/remote", directoryJSON(map[string]any{"name": "Remote", "url": p.tls.URL, "coordination_messages": enabled}), token(t, p.a, "owner")), 204)
	}
	setOutgoing(false)
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("status recovery did not finish")
	}
	if admits.Load() != 0 {
		t.Fatal("a new admission was transmitted after outgoing permission was removed")
	}
	coordinationMessageWait(t, p, result.Record, "admitted", "authorization-unavailable")
	setOutgoing(true)
	if err := p.a.maintainCoordinationMessages(context.Background()); err != nil {
		t.Fatal(err)
	}
	coordinationMessageWait(t, p, result.Record, "admitted", "remote-admitted")
	if admits.Load() != 1 {
		t.Fatal("restored permission did not deliver exactly one admission")
	}
}

func TestCoordinationRemoteMessagePreservesValidEscapedText(t *testing.T) {
	p := peerPair(t)
	p.a.sessionCoord.cancel()
	<-p.a.sessionCoord.done
	p.a.sessionCoord.cancel = nil
	p.messaging(t, true)
	target, _ := json.Marshal(p.remote[0].Subject)
	text := strings.Repeat("<", 8192)
	// Raw JSON is a supported ordinary CLI input. It fits the agent API limit,
	// but JSON's HTML escaping expands the later wire envelope substantially.
	body := `{"request_id":"escaped-remote-message","action":"message","kind":"message","target":` + string(target) + `,"text":"` + text + `"}`
	response := request(p.agent, "POST", "/act", body, p.access[0].Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Transport != "pending" {
		t.Fatal("valid raw message was not queued", err)
	}
	if err := p.a.maintainCoordinationMessages(context.Background()); err != nil {
		t.Fatal(err)
	}
	node, _ := p.a.coordinationNode()
	records, err := p.a.sessionCoord.service.RemoteRecords(node)
	if err != nil || len(records) != 1 || !records[0].Admitted {
		t.Fatal("valid escaped message was not admitted remotely", err)
	}
	viewResponse := request(p.b.CoordinationAgentHandler(), "GET", "/view", "", p.remote[0].Capability)
	requireStatus(t, viewResponse, 200)
	var view plane.View
	if err := json.Unmarshal(viewResponse.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range view.Inbox {
		if message.ID == result.Record {
			count++
			if message.Text != text {
				t.Fatal("wire encoding changed the message text")
			}
		}
	}
	if count != 1 {
		t.Fatal("expected exactly one complete escaped message in the recipient inbox")
	}
	oversized := strings.Replace(body, "escaped-remote-message", "oversized-remote-message", 1)
	oversized = strings.Replace(oversized, text, text+"<", 1)
	requireStatus(t, request(p.agent, "POST", "/act", oversized, p.access[0].Capability), 400)
}
func mustCoordinationInputs(t *testing.T, s *Server, node string) []plane.MemberInput {
	t.Helper()
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	return s.coordinationInputs(node, snapshots)
}

func TestCoordinationRemoteMessageThroughScopedMCP(t *testing.T) {
	p := peerPair(t)
	p.messaging(t, true)
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
	client, err := mcp.NewClient(&mcp.Implementation{Name: "remote-message-qualification", Version: "1"}, nil).Connect(ctx, two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: map[string]any{"request_id": "scoped-mcp-remote-message", "action": "message", "kind": "message", "target": p.remote[0].Subject, "text": "MCP remote inbox task"}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var admitted plane.Result
	if json.Unmarshal(data, &admitted) != nil || admitted.Status != "admitted" || admitted.Transport != "pending" {
		t.Fatal(string(data))
	}
	coordinationMessageWait(t, p, admitted.Record, "admitted", "remote-admitted")
}
