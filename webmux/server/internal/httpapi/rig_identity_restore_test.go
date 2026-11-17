package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestManagedRestoreRejectsReusedRigAddress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig supports Unix hosts only")
	}
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	for name, script := range map[string]string{
		"rig":  "#!/bin/sh\n[ \"$1\" = ps ] || exit 2\nprintf '%s\\n' '{\"entries\":[{\"rigId\":\"other-rig\",\"logicalId\":\"other-worker\",\"canonicalSessionName\":\"reused-address\",\"runtime\":\"codex\",\"tmuxAttachCommand\":\"tmux attach -t reused-address\"}]}'\n",
		"tmux": "#!/bin/sh\necho \"$1\" >> '" + calls + "'\n[ \"$1\" = -V ] && echo 'tmux 3.8'\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, _ := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	saved := session.RigIdentity{SeatID: localRigSeatID("original-rig", "original-worker"), Target: "reused-address", Runtime: "codex"}
	launch, err := s.resolveManagedReattachment(context.Background(), "anonymous", saved)
	if err == nil || len(launch.Argv) != 0 {
		t.Fatal("old address authorized another logical seat", launch, err)
	}
	data, _ := os.ReadFile(calls)
	if strings.Contains(string(data), "has-session") || strings.Contains(string(data), "attach-session") {
		t.Fatal("canonical mismatch reached a native attachment probe", string(data))
	}
}
