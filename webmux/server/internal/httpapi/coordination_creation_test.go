package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUncertainProjectCreationReturnsAcceptedWithoutCreatingViewer(t *testing.T) {
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
		if err := os.WriteFile(filepath.Join(bins, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	body := `{"provider":"rig","mode":"new","projectName":"project","name":"coder","workspace":"/work","host":"localhost","runtime":"codex"}`
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
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !validCoordinationCreation(result.Creation) || len(result.Session) != 0 {
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

func TestProjectInspectionCannotLaunchThroughAnotherProviderOrExistingMode(t *testing.T) {
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "1")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s)
	for _, body := range []string{
		`{"provider":"agent-deck","mode":"new","inspect":true}`,
		`{"provider":"rig","mode":"existing","inspect":true}`,
	} {
		requireStatus(t, request(handler, "POST", "/api/coordination/agents", body, ""), 400)
	}
}
