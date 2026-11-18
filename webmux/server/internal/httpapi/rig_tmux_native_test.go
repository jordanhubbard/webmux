package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

// The manager's inventory is a protocol fixture. Both tmux servers, their
// terminal processes, WebMux PTYs, input and controller restore are real.
func TestRigTmuxNativeSocketRebindPreservesProviderAndViewer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig tmux attachment is Unix-only")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("native tmux is not installed")
	}
	version, err := exec.Command(tmux, "-V").Output()
	match := regexp.MustCompile(`^tmux (?:next-)?(\d+)\.(\d+)`).FindStringSubmatch(string(version))
	if err != nil || len(match) != 3 {
		t.Skip("cannot verify native tmux version")
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	if major < 3 || major == 3 && minor < 4 {
		t.Skip("Rig requires native tmux 3.4 or newer")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("coordinator requires Node")
	}
	// Keep Unix-domain socket names below macOS's path limit.
	root, err := os.MkdirTemp("/tmp", "wm464-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	sockets := []string{filepath.Join(root, "first.sock"), filepath.Join(root, "second.sock")}
	command := func(socket string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, tmux, append([]string{"-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("owned tmux %q: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	for _, socket := range sockets {
		script := socket + ".sh"
		body := "#!/bin/sh\nprintf '%s\\n' $$ >> '" + socket + ".starts'\nprintf 'READY:%s;\\n' $$\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> '" + socket + ".input'; done\n"
		if err := os.WriteFile(script, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
		command(socket, "new-session", "-d", "-s", "same-name", "/bin/sh", script)
		command(socket, "set-option", "-t", "same-name", "mouse", "off")
		t.Cleanup(func() { _ = exec.Command(tmux, "-S", socket, "kill-server").Run() })
	}
	// The controller's inherited environment deliberately points at the decoy.
	t.Setenv("TMUX", sockets[1]+",1,0")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	bin := t.TempDir()
	rowFile := filepath.Join(root, "inventory.json")
	calls := filepath.Join(root, "rig-calls")
	t.Setenv("WEBMUX_RIG464_ROW", rowFile)
	t.Setenv("WEBMUX_RIG464_CALLS", calls)
	script := `#!/usr/bin/env node
const fs = require('node:fs'), args = process.argv.slice(2);
fs.appendFileSync(process.env.WEBMUX_RIG464_CALLS, JSON.stringify(args)+'\n');
if (args[0] === '--version') console.log('0.7.18 (3bf87c62)');
else if (args[0] === 'ps') console.log(args.includes('--nodes') ? fs.readFileSync(process.env.WEBMUX_RIG464_ROW, 'utf8') : '[]');
else process.exit(23);
`
	if err := os.WriteFile(filepath.Join(bin, "rig"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeRow := func(socket string) {
		t.Helper()
		data, _ := json.Marshal([]map[string]any{{"rigId": "fixture-rig", "logicalId": "agents.worker", "canonicalSessionName": "same-name", "attachmentType": "tmux", "tmuxServerArgs": []string{"-S", socket}, "runtime": "terminal", "lifecycleState": "running"}})
		if err := os.WriteFile(rowFile, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(description string, ready func() bool) {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			if ready() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal(description)
	}
	read := func(file string) string { data, _ := os.ReadFile(file); return string(data) }
	writeRow(sockets[0])
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	requireStatus(t, request(handler, "PUT", "/api/config", `{"app":{"session_logging":{"enabled":true}}}`, ""), 200)
	id := localRigSeatID("fixture-rig", "agents.worker")
	open := func(server *Server) session.Session {
		t.Helper()
		launch, err := server.coordinationProvider("anonymous", "rig").Attach(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		value, err := server.attachCoordinationTerminal(context.Background(), "anonymous", launch, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	first := open(s)
	if first.Rig.TmuxServerValue != sockets[0] {
		t.Fatal("viewer lost socket identity", first.Rig)
	}
	legacy := *first.Rig
	legacy.TmuxServerFlag, legacy.TmuxServerValue = "", ""
	if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", legacy); err == nil {
		t.Fatal("automatic reconnect silently migrated an environment-selected legacy destination")
	}
	wait("canonical viewer did not attach", func() bool {
		return command(sockets[0], "display-message", "-p", "-t", "=same-name:", "#{session_attached}") == "1"
	})
	if command(sockets[1], "show-option", "-v", "-t", "same-name", "mouse") != "off" {
		t.Fatal("scrollback setup mutated the decoy server")
	}
	if err := s.sessions.Input("anonymous", first.ID, "before-rebind\r"); err != nil {
		t.Fatal(err)
	}
	wait("input missed canonical server", func() bool { return strings.Contains(read(sockets[0]+".input"), "before-rebind") })
	wait("first provider history was not retained", func() bool {
		recovery, err := s.sessions.Recovery("anonymous", first.ID)
		return err == nil && strings.Contains(recovery.History, "READY:"+strings.TrimSpace(read(sockets[0]+".starts")))
	})
	if read(sockets[1]+".input") != "" {
		t.Fatal("input reached the decoy")
	}
	writeRow(sockets[1])
	if _, err := s.resolveManagedReattachment(context.Background(), "anonymous", *first.Rig); err == nil {
		t.Fatal("automatic reconnect followed a changed socket")
	}
	requireStatus(t, request(handler, "POST", "/api/sessions/"+first.ID+"/reconnect", `{}`, ""), 409)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(s.store, Options{Name: "restored", Logger: s.logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if err := restored.sessions.Restore(); err != nil {
		t.Fatal(err)
	}
	if err := restored.sessions.RestoreManaged(context.Background(), func(ctx context.Context, owner string, saved session.RigIdentity) ([]string, error) {
		launch, err := restored.resolveManagedReattachment(ctx, owner, saved)
		return launch.Argv, err
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := restored.sessions.Get("anonymous", first.ID)
	if err != nil || saved.Rig.TmuxServerValue != sockets[0] || saved.State != "disconnected" {
		t.Fatal("controller restore changed the saved destination", saved, err)
	}
	second := open(restored)
	if second.ID != first.ID || second.Row != first.Row || second.Col != first.Col || second.Rig.TmuxServerValue != sockets[1] {
		t.Fatal("explicit Open failed to rebind the same viewer", first, second)
	}
	if err := restored.sessions.Input("anonymous", second.ID, "after-rebind\r"); err != nil {
		t.Fatal(err)
	}
	wait("input missed rebound server", func() bool { return strings.Contains(read(sockets[1]+".input"), "after-rebind") })
	wait("socket rebind lost original history", func() bool {
		recovery, err := restored.sessions.Recovery("anonymous", first.ID)
		return err == nil && strings.Contains(recovery.History, "READY:"+strings.TrimSpace(read(sockets[0]+".starts"))) && strings.Contains(recovery.History, "READY:"+strings.TrimSpace(read(sockets[1]+".starts")))
	})
	if strings.Contains(read(sockets[0]+".input"), "after-rebind") {
		t.Fatal("rebound input reached the old provider")
	}
	for _, socket := range sockets {
		if len(strings.Fields(read(socket+".starts"))) != 1 || command(socket, "has-session", "-t", "=same-name") != "" {
			t.Fatal("provider was replaced, restarted or stopped", socket)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(read(calls)), "\n") {
		var args []string
		if json.Unmarshal([]byte(line), &args) != nil || len(args) == 0 || args[0] != "ps" && args[0] != "--version" {
			t.Fatal("attachment or recovery invoked a provider mutation", line)
		}
	}
}
