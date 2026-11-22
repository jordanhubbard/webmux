package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/coordinationmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPMarkupMessageMatchesCLIAndRetainsEncodedBodyLimit(t *testing.T) {
	_, _, agent, _, access := coordinationAPI(t)
	var posts atomic.Int64
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts.Add(1)
		}
		agent.ServeHTTP(w, r)
	}))
	defer listener.Close()
	sender := access[0]
	sender.Endpoint = listener.URL
	file := filepath.Join(t.TempDir(), "access.json")
	if err := coordinationclient.WriteAccess(file, sender); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", file)
	t.Setenv("WEBMUX_RECOVERY_SESSION", "")
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	text := strings.Repeat("<>&", 1000)
	target, _ := json.Marshal(access[1].Subject)
	// These test-only strings contain no quotes or backslashes. This is the compact
	// JSON an ordinary CLI caller supplies, independently of MCP's encoder.
	raw := fmt.Sprintf(`{"request_id":"markup-cli-1","action":"message","kind":"message","target":%s,"text":"%s"}`, target, text)
	var cli bytes.Buffer
	if err := coordinationclient.Request(ctx, sender, "POST", "/act", strings.NewReader(raw), &cli); err != nil {
		t.Fatal(err)
	}
	var baseline plane.Result
	if err := json.Unmarshal(cli.Bytes(), &baseline); err != nil || baseline.Status != "admitted" {
		t.Fatal("compact CLI message refused", err)
	}
	one, two := mcp.NewInMemoryTransports()
	server, err := coordinationmcp.NewServer().Connect(ctx, one, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "markup-regression", Version: "1"}, nil).Connect(ctx, two, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	input := plane.Request{ID: "markup-mcp-1", Action: "message", Kind: "message", Target: &access[1].Subject, Text: text}
	call := func(value plane.Request) *mcp.CallToolResult {
		t.Helper()
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: value})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	var admitted plane.Result
	for i := 0; i < 2; i++ {
		result := call(input)
		if result.IsError {
			data, _ := json.Marshal(result)
			t.Fatalf("MCP rejected text admitted through the CLI: %s", data)
		}
		data, _ := json.Marshal(result.StructuredContent)
		var current plane.Result
		if err := json.Unmarshal(data, &current); err != nil || current.Status != "admitted" || current.Record == "" {
			t.Fatal("missing MCP admission", err)
		}
		if i == 0 {
			admitted = current
		} else if current.Record != admitted.Record {
			t.Fatal("same-intent retry created a different message")
		}
	}
	before := posts.Load()
	input.ID, input.Text = "oversized-mcp-1", "x"
	// All fields here are ASCII without JSON escapes: make the operation exactly
	// one byte over the existing encoded-body cap, below the MCP stdio cap.
	overhead, _ := json.Marshal(input)
	input.Text = strings.Repeat("x", (16<<10)+1-(len(overhead)-1))
	oversized, _ := json.Marshal(input)
	if len(oversized) != (16<<10)+1 {
		t.Fatal("incorrect boundary control")
	}
	refused := call(input)
	data, _ := json.Marshal(refused)
	if !refused.IsError || !bytes.Contains(data, []byte("exceeds 16 KiB")) || posts.Load() != before {
		t.Fatal("oversized encoded operation crossed the original pre-dispatch limit")
	}
	response := request(agent, "GET", "/view", "", access[1].Capability)
	requireStatus(t, response, 200)
	var recipient plane.View
	if err := json.Unmarshal(response.Body.Bytes(), &recipient); err != nil || len(recipient.Inbox) != 2 {
		t.Fatal("message loss, duplicate admission or oversized dispatch", err)
	}
	ids := map[string]bool{}
	for _, message := range recipient.Inbox {
		if message.Text != text || message.From != sender.Subject || message.To != access[1].Subject || message.State != "admitted" {
			t.Fatal("MCP changed message content, identity or receipt state")
		}
		ids[message.ID] = true
	}
	if !ids[baseline.Record] || !ids[admitted.Record] || baseline.Record == admitted.Record {
		t.Fatal("source inbox does not confirm both original admissions")
	}
}
