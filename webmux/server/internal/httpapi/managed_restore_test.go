package httpapi

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func TestServerRestoreRevalidatesManagedViewerAndRetainsHistory(t *testing.T) {
	executable, calls := fakeAgentDeck(t)
	script, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), "case \"$*\" in", `case "$*" in
  "session attach local-1")
    printf 'MANAGED_AFTER_REATTACH\n'
    while IFS= read -r line; do :; done
    ;;`, 1))
	if err := os.WriteFile(executable, script, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	first, _ := fixture(t, "none")
	configure := func(enabled bool) {
		t.Helper()
		providers := []string{}
		if enabled {
			providers = append(providers, "agent-deck")
		}
		if err := first.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{
			"session_logging": map[string]any{"enabled": true},
			"coordination":    map[string]any{"enabled_providers": providers, "default_launch_provider": "webmux", "default_view": "all"},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	configure(false)
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:local-1", Host: "localhost", Workspace: "/src/local", Runtime: "codex"}
	value, err := first.sessions.EnsureRigTerminal("anonymous", identity.Target, "coder", []string{"/bin/sh", "-c", "printf 'MANAGED_BEFORE_RESTART\n'; read line"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	waitHistory := func(server *Server, marker string) session.RecoveryView {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			view, err := server.sessions.Recovery("anonymous", value.ID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(view.History, marker) {
				return view
			}
			if time.Now().After(deadline) {
				t.Fatal("retained history unavailable", marker, view)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitHistory(first, "MANAGED_BEFORE_RESTART")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(first.store, Options{Name: "restored", Logger: first.logger})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreSessions(); err != nil {
		t.Fatal("disabled provider prevented WebMux startup", err)
	}
	view := waitHistory(restored, "MANAGED_BEFORE_RESTART")
	saved, err := restored.sessions.Get("anonymous", value.ID)
	if err != nil || saved.State != "disconnected" || view.State.Disposition != "managed-unavailable" || !strings.Contains(view.Message, "disabled") || saved.ExecArgv[0] != "/bin/sh" {
		t.Fatal("startup reused stale argv or lost the recovery fallback", view, saved, err)
	}
	var durable struct {
		Sessions []session.Session `json:"sessions" yaml:"sessions"`
	}
	if err := restored.store.ReadSessions(&durable); err != nil {
		t.Fatal(err)
	}
	if len(durable.Sessions) != 1 || durable.Sessions[0].Recovery == nil || durable.Sessions[0].Recovery.ManagerMessage != view.State.ManagerMessage {
		t.Fatal("unavailable-manager explanation was not persisted", durable)
	}
	configure(true)
	if err := restored.RestoreSessions(); err != nil {
		t.Fatal(err)
	}
	view = waitHistory(restored, "MANAGED_AFTER_REATTACH")
	saved, err = restored.sessions.Get("anonymous", value.ID)
	if err != nil || saved.Rig == nil || *saved.Rig != identity || !reflect.DeepEqual(saved.ExecArgv, []string{executable, "session", "attach", "local-1"}) || view.State.ManagerMessage != "" || view.State.Disposition != "reattached" || !strings.Contains(view.History, "MANAGED_BEFORE_RESTART") {
		t.Fatal("provider restoration lost original identity or history", saved, view, err)
	}
	if err := restored.RestoreSessions(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(calls)
	if err != nil || strings.Count(string(data), "session attach local-1\n") != 1 || strings.Contains(string(data), "session start") || strings.Contains(string(data), "launch") {
		t.Fatal("repeated restoration duplicated or launched a managed job", string(data), err)
	}
}

func TestManagedStartupChecksCurrentOwnerAccessBeforeProviderDiscovery(t *testing.T) {
	executable, calls := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	first, _ := fixture(t, "local")
	federationAccounts(t, first)
	setCoordinationIntegrations(t, first, "agent-deck")
	identity := session.RigIdentity{Provider: "agent-deck", Target: "agent-deck:local:local-1", Host: "localhost", Workspace: "/src/local", Runtime: "codex"}
	value, err := first.sessions.EnsureRigTerminal("owner", identity.Target, "coder", []string{"/bin/sh", "-c", "read line"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := storage.UpdateConfig(first.store, "auth.yaml", func(c *auth.Config) error {
		for i := range c.Auth.Users {
			if c.Auth.Users[i].Username == "owner" {
				c.Auth.Users[i].Admin = false
			}
			// Preserve an explicit administrator so the legacy first-account
			// ownership fallback does not keep "owner" authorized.
			if c.Auth.Users[i].Username == "other" {
				c.Auth.Users[i].Admin = true
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := New(first.store, Options{Name: "restored", Logger: first.logger})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreSessions(); err != nil {
		t.Fatal(err)
	}
	view, err := restored.sessions.Recovery("owner", value.ID)
	if err != nil || view.State == nil || view.State.Disposition != "managed-unavailable" {
		t.Fatal("revoked owner regained a managed attachment", view, err)
	}
	data, err := os.ReadFile(calls)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatal("provider was queried before current access was checked", string(data))
	}
}

func TestStartupStartsRigManagerBeforeRestoringManagedViewers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenRig supports Unix hosts only")
	}
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	// The daemon is down (as after a host reboot) and takes a moment to start.
	// Restoration must not query Rig's inventory before that start completes.
	for name, content := range map[string]string{
		"rig": "#!/bin/sh\ncase \"$1 $2\" in\n" +
			"  'daemon status') exit 1 ;;\n" +
			"  'daemon start') sleep 1; echo 'daemon start' >> " + calls + "; echo 'Daemon started' ;;\n" +
			"  *) echo \"$1\" >> " + calls + "; echo '{\"entries\":[]}' ;;\n" +
			"esac\n",
		"tmux": "#!/bin/sh\n[ \"$1\" = -V ] && echo 'tmux 3.4'\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	first, _ := fixture(t, "none")
	setCoordinationIntegrations(t, first, "rig")
	identity := session.RigIdentity{Provider: "rig", Target: "builder@my-team", Host: "localhost", Workspace: "/src/team", Runtime: "claude-code"}
	if _, err := first.sessions.EnsureRigTerminal("anonymous", identity.Target, "builder", []string{"/bin/sh", "-c", "read line"}, identity); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(calls); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	restored, err := New(first.store, Options{Name: "restored", Logger: first.logger})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	// The first ensure is synchronous: when supervision has started, the
	// manager start has completed. A background-only ensure returns first.
	restored.startCoordination()
	if data, _ := os.ReadFile(calls); strings.TrimSpace(string(data)) != "daemon start" {
		t.Fatalf("supervision returned before starting the manager: %q", data)
	}
	if err := restored.RestoreSessions(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 || lines[0] != "daemon start" || !slices.Contains(lines[1:], "ps") {
		t.Fatalf("managed restoration queried Rig before its manager was started: %q", lines)
	}
}
