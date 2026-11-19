package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testPendingNativeCreation(t *testing.T, stage, mode string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Rig uses POSIX helper transport")
	}
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	bins := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.json")
	t.Setenv("WEBMUX_PENDING_INPUT", input)
	for name, contents := range map[string]string{
		"rig": "#!/bin/sh\nexit 0\n", "tmux": "#!/bin/sh\nexit 0\n",
		"node": "#!/bin/sh\n/bin/cat > \"$WEBMUX_PENDING_INPUT\"\nprintf '%s\\n' '{\"creation\":{\"state\":\"uncertain\",\"stage\":\"project\",\"code\":\"native_create_response_unavailable\",\"operationId\":\"11111111-1111-4111-8111-111111111111\"}}'\n",
	} {
		contents = strings.ReplaceAll(contents, `"stage":"project"`, `"stage":"`+stage+`"`)
		if err := os.WriteFile(filepath.Join(bins, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	body := `{"provider":"rig","mode":"new","projectName":"project","name":"coder","workspace":"/work","host":"localhost","runtime":"codex"}`
	if mode == "existing" {
		body = strings.Replace(body, `"mode":"new"`, `"mode":"existing","projectId":"01JQPWV4E7QP26M799M3S0MR6E","context":"agents"`, 1)
	}
	for _, inspect := range []bool{false, true} {
		var payload map[string]any
		_ = json.Unmarshal([]byte(body), &payload)
		if inspect {
			payload["inspect"] = true
		}
		encoded, _ := json.Marshal(payload)
		response := request(handler, "POST", "/api/coordination/agents", string(encoded), "")
		requireStatus(t, response, 202)
		var result struct {
			Creation coordinationCreation `json:"creation"`
			Session  json.RawMessage      `json:"session"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !validCoordinationCreation(result.Creation) || result.Creation.Stage != stage || len(result.Session) != 0 {
			t.Fatalf("pending allocation became a viewer: %s %v", response.Body.String(), err)
		}
		if len(s.sessions.List("anonymous")) != 0 {
			t.Fatal("uncertain topology created a terminal viewer")
		}
		raw, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		var forwarded map[string]any
		if json.Unmarshal(raw, &forwarded) != nil || (forwarded["inspect"] == true) != inspect || forwarded["rigName"] != "project" {
			t.Fatalf("wrong original inspection input: %s", raw)
		}
	}
}

func TestUncertainProjectCreationReturnsAcceptedWithoutCreatingViewer(t *testing.T) {
	testPendingNativeCreation(t, "project", "new")
}
func TestUncertainNativeSessionCreationReturnsAcceptedWithoutCreatingViewer(t *testing.T) {
	for _, mode := range []string{"new", "existing"} {
		t.Run(mode, func(t *testing.T) { testPendingNativeCreation(t, "agent", mode) })
	}
}

func TestCreationInspectionRejectsUnsupportedProviderOrDestination(t *testing.T) {
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "1")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s)
	for _, body := range []string{
		`{"provider":"agent-deck","mode":"new","inspect":true}`,
		`{"provider":"rig","mode":"existing","inspect":true}`,
		`{"provider":"rig","mode":"existing","runtime":"codex","host":"remote","inspect":true}`,
		`{"provider":"rig","mode":"existing","runtime":"claude-code","host":"localhost","inspect":true}`,
	} {
		requireStatus(t, request(handler, "POST", "/api/coordination/agents", body, ""), 400)
	}
}

func TestNativeTrustRefusalReturnsTypedConflictWithoutViewer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig uses POSIX helper transport")
	}
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	bins := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.json")
	t.Setenv("WEBMUX_PENDING_INPUT", input)
	payload := `{"blocked":{"state":"blocked","code":"native_project_trust_required","requestId":"` + strings.Repeat("a", 64) + `"}}`
	for name, contents := range map[string]string{
		"rig": "#!/bin/sh\nexit 0\n", "tmux": "#!/bin/sh\nexit 0\n",
		"node": "#!/bin/sh\n/bin/cat > \"$WEBMUX_PENDING_INPUT\"\nprintf '%s\\n' '" + payload + "'\n",
	} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, mode := range []string{"new", "existing"} {
		for _, token := range []string{"", strings.Repeat("a", 64)} {
			body := map[string]any{"provider": "rig", "mode": mode, "projectName": "project", "projectId": "01JQPWV4E7QP26M799M3S0MR6E", "context": "agents", "name": "coder", "workspace": "/work", "host": "localhost", "runtime": "codex"}
			if token != "" {
				body["trustRequest"] = token
			}
			encoded, _ := json.Marshal(body)
			response := request(handler, "POST", "/api/coordination/agents", string(encoded), "")
			requireStatus(t, response, 409)
			var result struct {
				Blocked coordinationCreationBlocked `json:"blocked"`
				Error   string                      `json:"error"`
				Session json.RawMessage             `json:"session"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !validCoordinationBlocked(result.Blocked) || len(result.Session) != 0 || !strings.Contains(result.Error, "native Codex") {
				t.Fatalf("invalid owner-action response: %s", response.Body.String())
			}
			var forwarded map[string]any
			raw, _ := os.ReadFile(input)
			if json.Unmarshal(raw, &forwarded) != nil || (token != "" && forwarded["trustRequest"] != token) || forwarded["cwd"] != "/work" {
				t.Fatalf("changed saved selection: %s", raw)
			}
			if len(s.sessions.List("anonymous")) != 0 {
				t.Fatal("blocked creation opened a viewer")
			}
		}
	}
	for _, invalid := range []string{
		strings.Replace(payload, `"state":"blocked"`, `"state":"uncertain"`, 1),
		payload + " trailing",
		strings.TrimSuffix(payload, "}") + `,"target":"foreign"}`,
		strings.Replace(payload, `"requestId":`, `"operationId":"11111111-1111-4111-8111-111111111111","requestId":`, 1),
	} {
		contents := "#!/bin/sh\n/bin/cat > \"$WEBMUX_PENDING_INPUT\"\nprintf '%s\\n' '" + invalid + "'\n"
		if err := os.WriteFile(filepath.Join(bins, "node"), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
		body := `{"provider":"rig","mode":"new","projectName":"project","name":"coder","workspace":"/work","host":"localhost","runtime":"codex"}`
		requireStatus(t, request(handler, "POST", "/api/coordination/agents", body, ""), 502)
		if len(s.sessions.List("anonymous")) != 0 {
			t.Fatal("invalid trust response opened a viewer")
		}
	}
	for _, body := range []string{
		`{"provider":"rig","mode":"new","runtime":"codex","host":"remote","trustRequest":"` + strings.Repeat("a", 64) + `"}`,
		`{"provider":"agent-deck","mode":"new","runtime":"codex","trustRequest":"` + strings.Repeat("a", 64) + `"}`,
		`{"provider":"rig","mode":"new","runtime":"claude-code","trustRequest":"` + strings.Repeat("a", 64) + `"}`,
		`{"provider":"rig","mode":"new","runtime":"codex","trustRequest":"foreign"}`,
	} {
		requireStatus(t, request(handler, "POST", "/api/coordination/agents", body, ""), 400)
	}
}

func TestInvalidNativeTrustResponsesCannotAuthorizeOwnerRetry(t *testing.T) {
	for _, value := range []coordinationCreationBlocked{
		{State: "uncertain", Code: "native_project_trust_required", RequestID: strings.Repeat("a", 64)},
		{State: "blocked", Code: "unrelated", RequestID: strings.Repeat("a", 64)},
		{State: "blocked", Code: "native_project_trust_required", RequestID: "../foreign"},
	} {
		if validCoordinationBlocked(value) {
			t.Fatalf("accepted foreign trust response: %+v", value)
		}
	}
}

func TestTrackedNativeCreationPreservesPublicIdentityAndPlacement(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native creation requires POSIX")
	}
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	bins, input := t.TempDir(), filepath.Join(t.TempDir(), "input.json")
	t.Setenv("WEBMUX_PENDING_INPUT", input)
	for name, contents := range map[string]string{
		"rig": "#!/bin/sh\nexit 0\n", "tmux": "#!/bin/sh\nexit 0\n",
		"node": "#!/bin/sh\n/bin/cat > \"$WEBMUX_PENDING_INPUT\"\nprintf '%s\\n' '{\"creation\":{\"state\":\"uncertain\",\"stage\":\"agent\",\"code\":\"native_create_response_unavailable\",\"operationId\":\"11111111-1111-4111-8111-111111111111\"}}'\n",
	} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	id := "2d438c43-c962-4b1d-b8ed-a35e07a8e964"
	endpoint := "/api/coordination/creation-requests/" + id
	original := map[string]any{"provider": "rig", "mode": "new", "projectName": "original", "name": "coder", "workspace": "/work", "runtime": "codex", "host": "localhost", "row": 1, "col": 2, "creationRequestId": id}
	for _, continuation := range []bool{false, true} {
		original["continueOriginal"] = continuation
		body, _ := json.Marshal(original)
		response := request(handler, "POST", endpoint, string(body), "")
		requireStatus(t, response, 202)
		var forwarded map[string]any
		raw, err := os.ReadFile(input)
		if err != nil || json.Unmarshal(raw, &forwarded) != nil || forwarded["creationRequestId"] != id || forwarded["continueOriginal"] != continuation || forwarded["row"] != float64(1) || forwarded["col"] != float64(2) || forwarded["rigName"] != "original" {
			t.Fatalf("original public identity changed: %s %v", raw, err)
		}
		if len(s.sessions.List("anonymous")) != 0 {
			t.Fatal("unconfirmed creation opened a viewer")
		}
	}
	for _, change := range []map[string]any{{"creationRequestId": "foreign"}, {"creationRequestId": "11111111-1111-4111-8111-111111111111"}, {"provider": "agent-deck"}, {"host": "worker"}, {"runtime": "claude-code"}, {"row": -1}, {"col": nil}, {"mode": "other"}} {
		body := map[string]any{}
		for key, value := range original {
			body[key] = value
		}
		for key, value := range change {
			body[key] = value
		}
		encoded, _ := json.Marshal(body)
		if err := os.Remove(input); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		response := request(handler, "POST", endpoint, string(encoded), "")
		requireStatus(t, response, 400)
		if !strings.Contains(response.Body.String(), `"refused"`) {
			t.Fatalf("missing pre-dispatch refusal: %s", response.Body.String())
		}
		if _, err := os.Stat(input); !os.IsNotExist(err) {
			t.Fatal("invalid tracked request reached coordinator")
		}
	}
	encoded, _ := json.Marshal(original)
	requireStatus(t, request(handler, "POST", "/api/coordination/agents", string(encoded), ""), 400)
}
