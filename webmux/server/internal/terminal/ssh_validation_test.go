package terminal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrowserCapabilityProbeRefusesInvalidSSHAddressBeforeExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SSH executable witness uses a POSIX shell")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	t.Setenv("WEBMUX_PROBE_SSH_MARKER", marker)
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$WEBMUX_PROBE_SSH_MARKER\"\nprintf '1\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	bad := testRequest()
	bad.Username = "invalid$user"
	if remoteBrowserAvailable(bad, "") {
		t.Fatal("invalid destination reached successful SSH capability witness")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("invalid probe executed SSH", err)
	}
	if !remoteBrowserAvailable(testRequest(), "") {
		t.Fatal("valid SSH capability witness refused")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("positive SSH witness did not execute", err)
	}
}

func TestRecoverySSHRejectsOverlongStoredAddress(t *testing.T) {
	for _, request := range []LaunchRequest{
		{Transport: "ssh", Hostname: strings.Repeat("a", 256), Username: "fixture", Port: 22},
		{Transport: "ssh", Hostname: "owned.invalid", Username: strings.Repeat("a", 65), Port: 22},
	} {
		if args, err := recoverySSHArgs(request, ""); err == nil || args != nil {
			t.Fatalf("overlong recovery identity yielded SSH arguments: %q %v", args, err)
		}
	}
}
