//go:build !windows

package recovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoordinationDirectoryQueryIsAuthenticatedReadOnlyAndBounded(t *testing.T) {
	hostFixture(t)
	bin := t.TempDir()
	marker := filepath.Join(bin, "queried")
	directory := t.TempDir()
	t.Setenv("WEBMUX_FIXTURE_CWD", directory)
	t.Setenv("WEBMUX_FIXTURE_SESSION", testHostSession)
	t.Setenv("WEBMUX_FIXTURE_MARKER", marker)
	script := `#!/bin/sh
[ "$1" = display-message ] && [ "$2" = -p ] && [ "$3" = -t ] && [ "$4" = "=webmux-$WEBMUX_FIXTURE_SESSION:" ] && [ "$5" = '#{pane_current_path}' ] || exit 93
printf queried > "$WEBMUX_FIXTURE_MARKER"
printf '%s\n' "$WEBMUX_FIXTURE_CWD"
`
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	request := RPCRequest{Protocol: 1, SessionID: testHostSession, Key: strings.Repeat("a", 32), Operation: "coordination-observe"}
	if response := HostRPC(context.Background(), request); response.Error == "" {
		t.Fatal("foreign key queried provider directory")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("unauthenticated request executed tmux")
	}
	request.Key = testHostKey
	before := hostSnapshot(t)
	response := HostRPC(context.Background(), request)
	if response.Error != "" || response.CoordinationDirectory != directory || len(response.Events) != 0 {
		t.Fatal(response)
	}
	after := hostSnapshot(t)
	if after.Revision != before.Revision || after.Shell != nil || after.Command != nil {
		t.Fatal("read-only query retained directory history")
	}
	for _, invalid := range []string{"relative", directory + "\r", strings.Repeat("x", 5000)} {
		t.Setenv("WEBMUX_FIXTURE_CWD", invalid)
		if response := HostRPC(context.Background(), request); response.Error == "" || response.CoordinationDirectory != "" {
			t.Fatal("unbounded or invalid directory accepted")
		}
	}
}
