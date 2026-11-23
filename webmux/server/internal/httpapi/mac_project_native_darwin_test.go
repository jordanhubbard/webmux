//go:build darwin

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/coordinationmcp"
	"github.com/jordanhubbard/webmux/server/internal/maccoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Opt-in native authority qualification. Creates only disposable local WebMux
// sessions and private test configuration. The supplied MAC project selection
// is a qualification context, never product enrollment or a worker identity.
// Native MAC credentials, project/task data and source paths stay private.
func TestNativeMACSharedBrowserAndMCPRead(t *testing.T) {
	if os.Getenv("WEBMUX_MAC_NATIVE_SHARED_READ") != "1" {
		t.Skip("requires explicit private native MAC read qualification")
	}
	manifestPath := os.Getenv("WEBMUX_MAC_NATIVE_MANIFEST")
	selectionPath := os.Getenv("WEBMUX_MAC_NATIVE_TASK_SELECTION")
	origin := os.Getenv("WEBMUX_MAC_NATIVE_ORIGIN")
	pid, err := strconv.Atoi(os.Getenv("WEBMUX_MAC_NATIVE_PUBLISHER_PID"))
	if err != nil || pid < 1 {
		t.Fatal("explicit expected native publisher PID is required")
	}
	fingerprint := os.Getenv("WEBMUX_MAC_NATIVE_PUBLISHER_FINGERPRINT")
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != 32 || fingerprint != strings.ToLower(fingerprint) {
		t.Fatal("explicit qualified publisher fingerprint is required")
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("native qualification requires the explicit pinned IPv4 loopback publisher")
	}
	readPrivate := func(path string, out any) {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
			t.Fatal("native input must be a bounded private regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, out) != nil {
			t.Fatal("native private input is unavailable")
		}
	}
	var manifest struct {
		ClientID string `json:"client_id"`
	}
	readPrivate(manifestPath, &manifest)
	var selected struct {
		Schema     string `json:"schema"`
		ProjectKey string `json:"project_key"`
		TaskID     string `json:"task_id"`
		Eligible   bool   `json:"test_only_mapping_eligible"`
	}
	readPrivate(selectionPath, &selected)
	if selected.Schema != maccoord.TaskSchema || !selected.Eligible || selected.ProjectKey == "" || selected.TaskID == "" {
		t.Fatal("explicit user-supplied qualification context is required")
	}
	s, browser, agent, records, access := coordinationAPI(t)
	node, _ := s.coordinationNode()
	inputs := s.coordinationInputs(node, records)
	repository := inputs[0].Project
	if repository == nil {
		t.Fatal("native WebMux session repository is unverified")
	}
	policy := map[string]any{
		"schema": maccoord.RuntimeSchema,
		"bindings": map[string]any{"schema": maccoord.BindingsSchema,
			"authorities": []any{map[string]any{"id": "native-qualified-mac", "origin": origin}},
			"bindings": []any{map[string]any{"owner": "owner", "repository_id": repository.ID, "repository": repository.Repository,
				"authority_id": "native-qualified-mac", "project_key": selected.ProjectKey, "capabilities": []string{maccoord.ObserveCapability}}}},
		"credentials": []any{map[string]any{"authority_id": "native-qualified-mac", "client_id": manifest.ClientID, "enrollment_file": manifestPath,
			"publisher": map[string]any{"pid": pid, "fingerprint": fingerprint}}},
	}
	encodedPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatal("native test policy encoding failed")
	}
	configPath := filepath.Join(t.TempDir(), "mac-observation.json")
	if err := os.WriteFile(configPath, encodedPolicy, 0600); err != nil {
		t.Fatal("native private test policy could not be created")
	}
	s.macObserver = maccoord.OpenRuntime(configPath, nil)
	start := time.Now()
	humanResponse := request(browser, "GET", "/api/session-coordination/projects/"+records[0].ID, "", token(t, s, "owner"))
	if humanResponse.Code != 200 {
		t.Fatal("native browser route refused project observation; status", humanResponse.Code)
	}
	var human maccoord.SessionProjectObservation
	if err := json.Unmarshal(humanResponse.Body.Bytes(), &human); err != nil {
		t.Fatal("native browser project DTO could not be decoded")
	}
	listener := httptest.NewServer(agent)
	defer listener.Close()
	sender := access[0]
	sender.Endpoint = listener.URL
	accessPath := filepath.Join(t.TempDir(), "webmux-session-access.json")
	if err := coordinationclient.WriteAccess(accessPath, sender); err != nil {
		t.Fatal("native session MCP capability fixture could not be retained")
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", accessPath)
	t.Setenv("WEBMUX_RECOVERY_SESSION", "")
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "")
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	one, two := mcp.NewInMemoryTransports()
	server, err := coordinationmcp.NewServer().Connect(ctx, one, nil)
	if err != nil {
		t.Fatal("native project MCP transport failed")
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "native-project-read-qualification", Version: "1"}, nil).Connect(ctx, two, nil)
	if err != nil {
		t.Fatal("native project MCP session failed")
	}
	defer client.Close()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_mac_project_view", Arguments: map[string]any{"offset": 0}})
	if err != nil || result.IsError {
		t.Fatal("native project MCP observation failed")
	}
	encodedAgent, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal("native MCP project DTO could not be encoded")
	}
	var machine maccoord.SessionProjectObservation
	if json.Unmarshal(encodedAgent, &machine) != nil {
		t.Fatal("native MCP project DTO could not be decoded")
	}
	for _, view := range []maccoord.SessionProjectObservation{human, machine} {
		if view.Schema != "webmux.mac.session-project-view.v1" || view.Subject != sender.Subject || view.Project.ProjectKey != selected.ProjectKey || view.Project.AuthorityID != "native-qualified-mac" || len(view.Project.Tasks) == 0 {
			t.Fatal("native consumer did not observe its authorized project/tasks")
		}
		if view.Project.ConflictVerdict != "unknown" || view.Project.OrderingVerdict != "unknown" || view.Project.ProjectExistence != "unverified" || view.HistoryState != "retained_unreconciled" || view.SourceTaskLink != "unlinked" {
			t.Fatal("native consumers inferred execution/roadmap authority from observation")
		}
		if view.SourceState != "observed" || view.Source == nil || view.Source.RepositoryID != repository.ID || view.Source.Commit == "" {
			t.Fatal("native consumer lost verified local source context")
		}
		for _, row := range view.Project.Tasks {
			if row.Task.AuthorityID != view.Project.AuthorityID || row.Task.ProjectKey != selected.ProjectKey || row.Task.SourceLinkState != "unlinked" || row.Task.SessionLinkState != "unlinked" {
				t.Fatal("native task crossed scope or gained a fabricated source/session link")
			}
		}
	}
	common := 0
	ids := map[string]bool{}
	for _, row := range human.Project.Tasks {
		ids[row.Task.TaskID] = true
	}
	for _, row := range machine.Project.Tasks {
		if ids[row.Task.TaskID] {
			common++
		}
	}
	if common == 0 {
		t.Fatal("fresh native pages share no task identity; qualification is inconclusive")
	}
	var browserProof map[string]any
	if os.Getenv("WEBMUX_MAC_NATIVE_SHARED_BROWSER") == "1" {
		root, err := filepath.Abs("../../../..")
		if err != nil {
			t.Fatal("native browser source root unavailable")
		}
		s.webDir = filepath.Join(root, "webmux", "web")
		if info, err := os.Stat(filepath.Join(s.webDir, "index.html")); err != nil || !info.Mode().IsRegular() {
			t.Fatal("build the production frontend before native browser qualification")
		}
		// The API-only fixture has minimal settings and overlapping tile positions.
		// Use full production defaults and distinct rows when mounting the actual
		// UI; these are disposable fixture settings, not installed user settings.
		for _, name := range []string{"app.yaml", "hosts.yaml", "clients.yaml"} {
			data, err := os.ReadFile(filepath.Join(root, "webmux", "config.defaults", name))
			if err != nil || os.WriteFile(s.store.ConfigPath(name), data, 0600) != nil {
				t.Fatal("native browser fixture catalog initialization failed")
			}
		}
		for i, record := range records {
			row, col := i, 0
			if _, err := s.sessions.Patch(record.Owner, record.ID, session.Patch{Row: &row, Col: &col}); err != nil {
				t.Fatal("native fixture tile layout initialization failed")
			}
		}
		ui := httptest.NewServer(s.Handler())
		defer ui.Close()
		artifacts := os.Getenv("WEBMUX_MAC_NATIVE_SHARED_BROWSER_ARTIFACTS")
		if artifacts == "" {
			artifacts = t.TempDir()
		}
		input, err := json.Marshal(map[string]any{"origin": ui.URL, "token": token(t, s, "owner"), "session_id": records[0].ID,
			"project_key": selected.ProjectKey, "task_ids": taskIDs(machine), "artifact_directory": artifacts,
			"executable": os.Getenv("PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH")})
		if err != nil {
			t.Fatal("native browser private input unavailable")
		}
		node := os.Getenv("WEBMUX_MAC_NATIVE_BROWSER_NODE")
		if !filepath.IsAbs(node) {
			t.Fatal("explicit absolute Node path is required for native browser qualification")
		}
		command := exec.CommandContext(ctx, node, filepath.Join(root, "webmux", "scripts", "mac-project-native-browser.mts"))
		command.Stdin = bytes.NewReader(input)
		var output bytes.Buffer
		command.Stdout = &output
		command.Stderr = &bytes.Buffer{} // Errors may contain private native task text.
		if err := command.Run(); err != nil || json.Unmarshal(output.Bytes(), &browserProof) != nil || browserProof["completed"] != true {
			t.Fatal("native browser UI qualification failed; inspect its private local artifacts")
		}
	}
	denial, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_coordination_act", Arguments: map[string]any{"request_id": "native-project-no-standalone-work", "action": "work", "text": "This must not be admitted by the standalone ledger"}})
	if err != nil || !denial.IsError {
		t.Fatal("native MAC-bound session admitted competing standalone work")
	}
	// Alter only disposable observer configuration; neither consumer can reuse
	// its successful observation, and the accepted MAC policy still fences writes.
	if os.WriteFile(configPath, append(encodedPolicy, '\n'), 0600) != nil {
		t.Fatal("native test policy change failed")
	}
	refused := request(browser, "GET", "/api/session-coordination/projects/"+records[0].ID, "", token(t, s, "owner"))
	if refused.Code != http.StatusServiceUnavailable || strings.Contains(refused.Body.String(), selected.ProjectKey) {
		t.Fatal("native browser retained facts after configuration changed")
	}
	refusedMCP, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "webmux_mac_project_view", Arguments: map[string]any{}})
	if err != nil || !refusedMCP.IsError {
		t.Fatal("native MCP retained facts after configuration changed")
	}
	for _, record := range records {
		live, err := s.sessions.Get(record.Owner, record.ID)
		if err != nil || live.CreatedAt != record.CreatedAt || live.State != "connected" {
			t.Fatal("native read/refusal altered the existing provider lifecycle")
		}
	}
	// Only an explicitly supplied private path receives the native payloads.
	// Public receipts should copy bounded metadata/hashes, never these rows.
	if output := os.Getenv("WEBMUX_MAC_NATIVE_SHARED_RECEIPT"); output != "" {
		receipt := map[string]any{"schema": "webmux.mac.native-shared-read-qualification.v1", "observed_at": time.Now().UTC(), "elapsed_seconds": time.Since(start).Seconds(),
			"scope":                 "Isolated local WebMux sessions and explicit fleet test context; not live WebMux product enrollment or three-host participant acceptance",
			"publisher_fingerprint": fingerprint, "shared_task_identities": common, "human": human, "agent": machine,
			"browser":                 browserProof,
			"human_projection_sha256": hexDigest(humanResponse.Body.Bytes()), "agent_projection_sha256": hexDigest(encodedAgent),
			"competing_standalone_work_refused": true, "changed_policy_refused_both_consumers": true, "provider_incarnations_preserved": true,
			"installed_config_profile_service_changes": false, "MAC_dispatch_or_worker_registration": false, "NAS_access": false}
		data, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil || os.WriteFile(output, append(data, '\n'), 0600) != nil {
			t.Fatal("native private shared-read receipt unavailable")
		}
	}
	t.Log("Real MAC browser-route and MCP reads share scoped task identities; policy refusal and provider preservation verified")
}

func taskIDs(view maccoord.SessionProjectObservation) []string {
	out := make([]string, 0, len(view.Project.Tasks))
	for _, row := range view.Project.Tasks {
		out = append(out, row.Task.TaskID)
	}
	return out
}

func hexDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
