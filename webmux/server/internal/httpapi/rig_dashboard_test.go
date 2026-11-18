package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRigDashboardObservesLocalEndpointWithoutStartingDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig dashboard supports Unix hosts only")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "rig")
	calls := filepath.Join(root, "calls")
	t.Setenv("WEBMUX_DASHBOARD_TEST_CALLS", calls)
	t.Setenv("OPENRIG_HOST_SELECTED", "unrelated-host")
	script := "#!/bin/sh\nprintf '%s\\n' \"$OPENRIG_HOST_SELECTED $*\" >> \"$WEBMUX_DASHBOARD_TEST_CALLS\"\n[ \"$*\" = 'daemon status' ] || exit 23\nprintf '%s\\n' \"$WEBMUX_DASHBOARD_TEST_STATUS\"\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		status string
		port   string
	}{
		{"Daemon running on port 23456 (pid 123)", "23456"},
		{"Daemon running on port 23457\r\n", "23457"},
		{"Daemon stopped", ""},
		{"Daemon running on port 23456 (pid 123) — process present but UNHEALTHY: healthz failed", ""},
		{"Daemon running on port 80", ""},
		{"Daemon running on port 65536", ""},
		{"Daemon running on port 23456\nDaemon running on port 23457", ""},
	} {
		t.Setenv("WEBMUX_DASHBOARD_TEST_STATUS", row.status)
		want := []string{executable, "tui"}
		if row.port != "" {
			want = append([]string{"/usr/bin/env", "OPENRIG_URL=http://127.0.0.1:" + row.port}, want...)
		}
		if got := rigDashboardArgv(context.Background(), executable); !reflect.DeepEqual(got, want) {
			t.Fatalf("dashboard endpoint for %q: %q, want %q", row.status, got, want)
		}
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "local daemon status" {
			t.Fatal("dashboard mutated or followed general host selection", line)
		}
	}
}
