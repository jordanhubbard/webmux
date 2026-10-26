package openrigcoord

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type workspaceListing struct {
	Host       string `json:"host"`
	Path       string `json:"path"`
	Repository bool   `json:"repository"`
	Entries    []struct {
		Name       string `json:"name"`
		Repository bool   `json:"repository"`
	} `json:"entries"`
}

func browseForTest(t *testing.T, home, input string) (workspaceListing, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Rig coordination and its SSH workspace browser are unsupported on native Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := Run(ctx, home, "test-owner", "browse", input)
	if err != nil {
		return workspaceListing{}, err
	}
	var value workspaceListing
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	return value, nil
}

func TestBrowseLocalWorkspaceReportsRepositories(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "projects")
	for _, name := range []string{"zeta", "Alpha"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "Alpha", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"provider": "rig", "host": "localhost", "path": root})
	listing, err := browseForTest(t, home, string(input))
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if listing.Host != "localhost" || listing.Path != realRoot || len(listing.Entries) != 2 {
		t.Fatalf("unexpected listing: %+v", listing)
	}
	if listing.Entries[0].Name != "Alpha" || !listing.Entries[0].Repository || listing.Entries[1].Name != "zeta" {
		t.Fatalf("directory ordering or repository detection failed: %+v", listing.Entries)
	}
}

func TestBrowseRemoteWorkspaceUsesStrictNonInteractiveSSH(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required by the embedded coordination runner")
	}
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.Symlink(node, filepath.Join(bin, "node")); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(bin, "ssh.args")
	ssh := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$WEBMUX_SSH_TEST_LOG\"\nprintf '%s\\n' '{\"host\":\"devbox\",\"path\":\"/srv/work\",\"parent\":\"/srv\",\"repository\":true,\"entries\":[]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("WEBMUX_SSH_TEST_LOG", logPath)
	input := `{"provider":"rig","host":"devbox","path":"/srv/work"}`
	listing, err := browseForTest(t, home, input)
	if err != nil {
		t.Fatal(err)
	}
	if listing.Host != "devbox" || listing.Path != "/srv/work" || !listing.Repository {
		t.Fatalf("unexpected remote listing: %+v", listing)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	command := string(args)
	for _, required := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "ConnectTimeout=8", "devbox", "/srv/work"} {
		if !strings.Contains(command, required) {
			t.Fatalf("remote browser omitted %q: %s", required, command)
		}
	}
}

func TestBrowseRejectsShellAndRelativePathInput(t *testing.T) {
	for _, input := range []string{
		`{"host":"devbox;touch-bad","path":"/srv/work"}`,
		`{"host":"localhost","path":"relative"}`,
	} {
		if _, err := browseForTest(t, t.TempDir(), input); err == nil {
			t.Fatalf("accepted invalid browse input: %s", input)
		}
	}
}
