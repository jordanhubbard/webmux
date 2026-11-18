package httpapi

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRigPodDiscoveryRequiresCurrentProjectAndNeverCreatesViewer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses POSIX shell")
	}
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	bins, state := t.TempDir(), filepath.Join(t.TempDir(), "input.json")
	t.Setenv("WEBMUX_POD_INPUT", state)
	t.Setenv("WEBMUX_POD_REPLY", `{"projectId":"01JQPWV4E7QP26M799M3S0MR6E","projectName":"empty-project","pods":["research","workers"],"source":"native-spec"}`)
	for name, body := range map[string]string{
		"rig":  "#!/bin/sh\nexit 0\n",
		"node": "#!/bin/sh\n[ \"$2\" = project-pods ] || exit 8\n/bin/cat > \"$WEBMUX_POD_INPUT\"\nprintf '%s\\n' \"$WEBMUX_POD_REPLY\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	valid := `{"projectId":"01JQPWV4E7QP26M799M3S0MR6E","projectName":"empty-project"}`
	for _, body := range []string{`{}`, `{"projectId":"host-rig/foreign","projectName":"empty-project"}`, strings.TrimSuffix(valid, "}") + `,"directory":"/foreign"}`} {
		requireStatus(t, request(handler, "POST", "/api/coordination/rig/pods", body, ""), 400)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("invalid selection invoked coordinator")
	}
	response := request(handler, "POST", "/api/coordination/rig/pods", valid, "")
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"research"`) || len(s.sessions.List("anonymous")) != 0 {
		t.Fatal("namespace discovery omitted the empty pod or created a viewer")
	}
	for _, reply := range []string{
		`{"projectId":"01JQPWV4E7QP26M799M3S0MR6F","projectName":"empty-project","pods":["research"],"source":"native-spec"}`,
		`{"projectId":"01JQPWV4E7QP26M799M3S0MR6E","projectName":"empty-project","pods":["research","research"],"source":"native-spec"}`,
		`{"projectId":"01JQPWV4E7QP26M799M3S0MR6E","projectName":"empty-project","pods":null,"source":"native-spec"}`,
	} {
		t.Setenv("WEBMUX_POD_REPLY", reply)
		requireStatus(t, request(handler, "POST", "/api/coordination/rig/pods", valid, ""), 502)
	}
	setCoordinationIntegrations(t, s)
	requireStatus(t, request(handler, "POST", "/api/coordination/rig/pods", valid, ""), 409)
}
