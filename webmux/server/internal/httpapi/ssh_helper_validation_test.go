package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

// Failed terminal launches are retained for recovery. Their presence must not
// confer validation on their destination when a companion is opened later.
func TestRetainedFailedSSHSessionCannotLaunchBrowserHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable SSH witness uses a POSIX shell")
	}
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ssh-started")
	t.Setenv("WEBMUX_HELPER_SSH_MARKER", marker)
	// Emitting a frame after the witness makes a pre-fix success synchronous:
	// the HTTP response cannot arrive before the helper has written the marker.
	script := `#!/bin/sh
printf '%s\n' "$@" >> "$WEBMUX_HELPER_SSH_MARKER"
while IFS= read -r line; do printf '%s\n' '{"protocol":3}'; done
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, handler := fixture(t, "none")
	created := request(handler, "POST", "/api/sessions", `{"hostname":"owned.invalid","username":"invalid$user","transport":"ssh"}`, "")
	requireStatus(t, created, 201)
	var value session.Session
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.State != "error" || value.Username != "invalid$user" {
		t.Fatalf("failed session was not retained: %+v", value)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("invalid terminal launch started SSH", err)
	}
	opened := request(handler, "POST", "/api/sessions/"+value.ID+"/browser", `{"action":"start","viewer":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, "")
	if opened.Code != 409 || !strings.Contains(opened.Body.String(), "Invalid username") {
		_, witnessErr := os.Stat(marker)
		t.Fatalf("stored invalid SSH identity reached companion launch: HTTP%d helperExecuted=%t", opened.Code, witnessErr == nil)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("companion started SSH for an invalid retained identity", err)
	}
	// The retained record remains readable; file transfer keeps its existing
	// connected-state refusal rather than treating presence as liveness.
	requireStatus(t, request(handler, "GET", "/api/sessions/"+value.ID, "", ""), 200)
	requireStatus(t, request(handler, "POST", "/api/sessions/"+value.ID+"/files", "", ""), 409)
}
