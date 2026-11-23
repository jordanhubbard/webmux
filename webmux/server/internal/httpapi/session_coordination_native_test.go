package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

// Opt-in native model acceptance, with an ordinary broker-owned PTY and the
// production alias binary. No browser viewer or special managed launch form.
// Existing user credentials/configuration remain at their original locations.
func TestCoordinationNativeAgentBootstrap(t *testing.T) {
	helper := os.Getenv("WEBMUX_QUALIFY_COORDINATION_NATIVE_BINARY")
	if helper == "" {
		t.Skip("requires explicitly built production alias and native authenticated CLI")
	}
	if runtime.GOOS == "windows" {
		t.Skip("native Windows alias qualification requires its own executable custody fixture")
	}
	cli := os.Getenv("WEBMUX_QUALIFY_COORDINATION_NATIVE_CLI")
	if cli != "codex" && cli != "claude" {
		t.Fatal("select codex or claude explicitly")
	}
	if !filepath.IsAbs(helper) {
		t.Fatal("production binary must be absolute")
	}
	if _, err := exec.LookPath(cli); err != nil {
		t.Fatal(err)
	}
	s, _, _, records, _ := coordinationAPI(t)
	// The agent receives its coordination instructions only from native MCP.
	// The task itself does not name WebMux, MCP, a capability or any tool.
	prompt := "Before changing anything, check whether anyone else is working in this project. Do not modify files, run commands, use shell/filesystem tools, or start/delegate to other agents. Briefly report the available collaboration context, then stop."
	actions := os.Getenv("WEBMUX_QUALIFY_COORDINATION_NATIVE_ACTIONS") == "1"
	if actions {
		prompt = "Please test the available collaboration interface in this project. Check who else is working here, register a short task and an advisory file claim named native-interface-check.go, briefly acquire the local heavy-worker lease, verify that it is current, then release the same original record/fence and your claim. Do not edit files, run shell/filesystem commands, delegate, or launch another agent. Report only what the interface actually confirms and then stop."
	}
	args := []string{cli}
	if cli == "codex" {
		args = append(args, "-a", "never", "exec", "--sandbox", "read-only", "--json", prompt)
	} else {
		turns := "4"
		if actions {
			turns = "16"
		}
		args = append(args, "--print", "--output-format", "stream-json", "--verbose", "--max-turns", turns, prompt)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
	words := make([]string, len(args))
	for i, value := range args {
		words[i] = quote(value)
	}
	command := "cd " + quote(records[0].ExecCwd) + " && exec /usr/bin/env SHELL=/bin/bash WEBMUX_RECOVERY_CACHE_HOME=" + quote(t.TempDir()) + " " + quote(helper) + " --browser-shell"
	created, err := s.sessions.Create("owner", session.CreateRequest{Hostname: "localhost", Username: "owner", Transport: "exec", Cols: 120, Rows: 40, ExecCommand: command})
	if err != nil {
		t.Fatal(err)
	}
	ready := time.Now().Add(10 * time.Second)
	for {
		state, err := s.sessions.Get("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.CoordinationActivity != nil && state.CoordinationActivity.Directory == records[0].ExecCwd {
			break
		}
		if time.Now().After(ready) {
			t.Fatal("ordinary shell did not install its aliases and signed directory hook")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// This is ordinary test-user input, not an injected coordination message or
	// startup prompt. ShellEnvironment installs the native aliases automatically.
	if cli == "claude" && os.Getenv("WEBMUX_QUALIFY_COORDINATION_NATIVE_APPROVED_TOOLS") == "1" {
		node, err := s.coordinationNode()
		if err != nil {
			t.Fatal(err)
		}
		ref := coordinationRef(node, created)
		sum := sha256.Sum256([]byte(ref.Key()))
		name := "mcp__webmux_" + hex.EncodeToString(sum[:6]) + "__"
		// A task-selected native permission profile, not an adapter policy change.
		args = append(args[:len(args)-1], "--allowedTools", name+"webmux_coordination_view", name+"webmux_coordination_act", "--", prompt)
		words = make([]string, len(args))
		for i, value := range args {
			words[i] = quote(value)
		}
	}
	if err := s.sessions.Input("owner", created.ID, strings.Join(words, " ")+"; exit\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(120 * time.Second)
	var output string
	var current session.Session
	observedVersion := ""
	for time.Now().Before(deadline) {
		current, err = s.sessions.Get("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		output, err = s.sessions.Scrollback("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if evidence := os.Getenv("WEBMUX_QUALIFY_COORDINATION_NATIVE_OUTPUT"); evidence != "" {
			if !filepath.IsAbs(evidence) {
				t.Fatal("private native evidence path must be absolute")
			}
			if err := os.WriteFile(evidence, []byte(output), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if current.CoordinationActivity != nil && current.CoordinationActivity.Interface == "mcp-configured" {
			observedVersion = current.CoordinationActivity.Version
		}
		if current.State == "disconnected" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if current.State != "disconnected" {
		t.Fatal("native agent did not finish within qualification deadline")
	}
	// Accept only an actual native tool result for this session, not model prose
	// claiming that the tool ran. This also proves cwd grouping with recording off.
	toolIDs := map[string]string{}
	claimed, reserved, released := false, false, false
	reservationRecord := ""
	reservationFence := float64(0)
	var inspectAction func(any)
	inspectAction = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if v["status"] == "claimed" {
				if record, ok := v["record_id"].(string); ok && len(record) == 64 {
					claimed = true
				}
			}
			if v["status"] == "reserved" {
				lease, _ := v["lease"].(map[string]any)
				record, _ := v["record_id"].(string)
				fence, _ := v["fence"].(float64)
				if lease["current"] == true && len(record) == 64 && fence > 0 {
					reserved = true
					reservationRecord = record
					reservationFence = fence
				}
			}
			if reserved && v["status"] == "released" && v["record_id"] == reservationRecord && v["fence"] == reservationFence {
				released = true
			}
			for _, child := range v {
				inspectAction(child)
			}
		case []any:
			for _, child := range v {
				inspectAction(child)
			}
		case string:
			var decoded any
			if len(v) > 0 && (v[0] == '{' || v[0] == '[') && json.Unmarshal([]byte(v), &decoded) == nil {
				inspectAction(decoded)
			}
		}
	}
	sawTool, sawSuccess := false, false
	var inspect func(any)
	inspect = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			self, _ := v["self"].(map[string]any)
			ref, _ := self["ref"].(map[string]any)
			members, _ := v["members"].([]any)
			if v["contract"] == "webmux-session-coordination/v1" && v["scope"] == "node-local" && ref["session_id"] == created.ID && self["project_status"] == "verified-git" && len(members) == 3 {
				sawSuccess = true
			}
			for _, child := range v {
				inspect(child)
			}
		case []any:
			for _, child := range v {
				inspect(child)
			}
		case string:
			var decoded any
			if len(v) > 0 && (v[0] == '{' || v[0] == '[') && json.Unmarshal([]byte(v), &decoded) == nil {
				inspect(decoded)
			}
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if item, ok := event["item"].(map[string]any); ok && item["type"] == "mcp_tool_call" && item["tool"] == "webmux_coordination_view" {
			sawTool = true
			if item["status"] == "completed" {
				inspect(item["result"])
			}
		}
		if item, ok := event["item"].(map[string]any); ok && item["type"] == "mcp_tool_call" && item["tool"] == "webmux_coordination_act" && item["status"] == "completed" {
			inspectAction(item["result"])
		}
		message, _ := event["message"].(map[string]any)
		content, _ := message["content"].([]any)
		for _, part := range content {
			block, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if event["type"] == "assistant" && block["type"] == "tool_use" {
				name, _ := block["name"].(string)
				if strings.HasSuffix(name, "__webmux_coordination_view") {
					id, _ := block["id"].(string)
					toolIDs[id] = "view"
					sawTool = true
				}
			}
			if event["type"] == "assistant" && block["type"] == "tool_use" {
				name, _ := block["name"].(string)
				if strings.HasSuffix(name, "__webmux_coordination_act") {
					id, _ := block["id"].(string)
					toolIDs[id] = "act"
				}
			}
			if event["type"] == "user" && block["type"] == "tool_result" {
				id, _ := block["tool_use_id"].(string)
				if block["is_error"] != true {
					if toolIDs[id] == "view" {
						inspect(block["content"])
					}
					if toolIDs[id] == "act" {
						inspectAction(block["content"])
					}
				}
			}
		}
	}
	if !sawTool || !sawSuccess {
		t.Fatalf("native scoped view not proven: tool=%v verified-project/three-peers=%v; inspect private native evidence", sawTool, sawSuccess)
	}
	if actions && (!claimed || !reserved || !released) {
		t.Fatalf("native actions not proven: claimed=%v current-reserved=%v same-fence-released=%v; inspect private native evidence", claimed, reserved, released)
	}
	if actions {
		t.Log("native MCP actions proven: advisory claim, current local lease and explicit original-record/fence release; no command execution or filesystem edits")
	}
	if observedVersion == "" {
		t.Fatal("native bootstrap evidence was not retained")
	}
	t.Logf("native %s %s invoked scoped MCP view from ordinary broker-owned PTY before any browser; verified Git project with three same-account members; node-local only; original policy/configuration preserved", cli, observedVersion)
}
