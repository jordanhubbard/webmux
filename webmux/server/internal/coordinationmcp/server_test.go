package coordinationmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPGuidanceToolsAndAdmissionSemantics(t *testing.T) {
	var mu sync.Mutex
	response := `{"status":"busy"}`
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer wmc1.fixture.private" {
			t.Error("wrong scoped capability")
		}
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"contract":"webmux-session-coordination/v1","scope":"node-local","enabled":true,"members":[],"claims":[],"inbox":[],"outbox":[],"resources":[]}`))
			return
		}
		var input coordination.Request
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ID != "unchanged-request-id" {
			t.Error("stable intent changed")
		}
		mu.Lock()
		data := response
		mu.Unlock()
		_, _ = w.Write([]byte(data))
	}))
	defer remote.Close()
	file := filepath.Join(t.TempDir(), "access.json")
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	t.Setenv("WEBMUX_RECOVERY_SESSION", "")
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "")
	if err := coordinationclient.WriteAccess(file, coordinationclient.Access{Contract: coordination.Contract, Endpoint: remote.URL, Capability: "wmc1.fixture.private"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	one, two := mcp.NewInMemoryTransports()
	server, err := NewServer().Connect(ctx, one, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "native-agent-control", Version: "1"}, nil)
	session, err := client.Connect(ctx, two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !strings.Contains(session.InitializeResult().Instructions, "before starting work") {
		t.Fatal("no startup guidance")
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatal(tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_view", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), "node-local") || strings.Contains(string(data), "wmc1.") {
		t.Fatal("invalid or secret tool output", string(data))
	}
	args := map[string]any{"request_id": "unchanged-request-id", "action": "reserve", "resource": "node:fixture:heavy"}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: args})
	if err != nil || !result.IsError {
		t.Fatal("busy was treated as admission", result, err)
	}
	for _, refused := range []string{`{"status":"reserved","expires":"2000-01-01T00:00:00Z"}`, `{"status":"claimed","conflicts":[{"id":"existing-claim"}]}`} {
		mu.Lock()
		response = refused
		mu.Unlock()
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: args})
		if err != nil || !result.IsError {
			t.Fatal("expired or conflicting admission lost MCP error semantics", result, err)
		}
	}
	mu.Lock()
	response = `{"status":"admitted","record_id":"same-record","state":"admitted"}`
	mu.Unlock()
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: args})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	// Re-read access rather than retaining a dead controller endpoint/capability.
	if err := coordinationclient.WriteAccess(file, coordinationclient.Access{Contract: coordination.Contract, Endpoint: "http://example.test:8080", Capability: "wmc1.fixture.private"}); err != nil {
		t.Fatal(err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_view", Arguments: map[string]any{}})
	if err != nil || !result.IsError {
		t.Fatal("stale access cached or invalid endpoint accepted", result, err)
	}
}
