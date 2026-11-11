package openrigcoord

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// Starting an agent on an unreachable or unprepared SSH host reports the cause
// instead of the probe's command line, and starts no seat.
func TestCreateReportsHostProbeFailureClearly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig coordination is unsupported on native Windows")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required by the embedded coordination runner")
	}
	for _, c := range []struct {
		status    int
		stderr    string
		want      string
		forbidden string
	}{
		{255, "ssh: connect to host devbox port 22: Connection refused", "Cannot reach devbox over SSH: ssh: connect to host devbox port 22: Connection refused. Nothing was started.", "is not ready"},
		{1, "", "devbox is not ready for this agent: /srv/work must exist and claude, rig and tmux must be on its PATH.", "Cannot reach"},
	} {
		bin := t.TempDir()
		if err := os.Symlink(node, filepath.Join(bin, "node")); err != nil {
			t.Fatal(err)
		}
		calls := filepath.Join(bin, "calls")
		for name, content := range map[string]string{
			"ssh":  "#!/bin/sh\necho ssh >> " + calls + "\necho '" + c.stderr + "' >&2\nexit " + strconv.Itoa(c.status) + "\n",
			"rig":  "#!/bin/sh\necho \"rig $*\" >> " + calls + "\necho '{\"rigId\":\"01ARZ3NDEKTSV4RRFFQ69G5FAV\"}'\n",
			"tmux": "#!/bin/sh\necho \"tmux $*\" >> " + calls + "\n[ \"$1\" = -V ] && echo 'tmux 3.4' && exit 0\nexit 1\n",
		} {
			if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("PATH", bin)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := Run(ctx, t.TempDir(), "test-owner", "create", `{"mode":"new","rigName":"team","name":"builder","host":"devbox","cwd":"/srv/work","runtime":"claude-code"}`)
		cancel()
		if err == nil || err.Error() != c.want || strings.Contains(err.Error(), "Command failed") || strings.Contains(err.Error(), c.forbidden) {
			t.Fatalf("unclear probe failure: %v", err)
		}
		data, _ := os.ReadFile(calls)
		if strings.Contains(string(data), "new-session") || strings.Contains(string(data), "rig attach") {
			t.Fatalf("a failed host probe started a seat: %s", data)
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
