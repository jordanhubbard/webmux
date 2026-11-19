package openrigcoord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestObservationsExcludeMessagesAndPrivateActivityEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig terminal integration is Unix-only")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required by embedded coordinator")
	}
	home, bin := t.TempDir(), t.TempDir()
	scripts := map[string]string{
		"tmux": "#!/bin/sh\nif [ \"$1\" = -V ]; then echo 'tmux 3.8'; fi\n",
		"rig": `#!/bin/sh
case "$*" in
 "ps --nodes"*) printf '%s\n' '{"entries":[{"rigId":"r","rigName":"test","logicalId":"agents.one","canonicalSessionName":"native","startupStatus":"ready","agentActivity":{"state":"running","evidenceSource":"runtime_hook","sampledAt":"2026-11-17T12:00:00Z","generationProvenance":"resolved","evidence":"PRIVATE_EVIDENCE","rawEvent":"PRIVATE_EVENT"}}]}' ;;
 ps*) printf '%s\n' '{"entries":[{"rigId":"r","rigName":"test"}]}' ;;
 *) printf 'forbidden command: %s\n' "$*" >&2; exit 19 ;;
esac
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	sum := sha256.Sum256([]byte("owner"))
	directory := filepath.Join(home, "data", "coordination", hex.EncodeToString(sum[:12]))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	// A launch record would normally cause outbox reads; observations must not.
	if err := os.WriteFile(filepath.Join(directory, "native.agent.json"), []byte(`{"target":"native","tmux":"native","host":"localhost"}`), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := Run(context.Background(), home, "owner", "observations", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "PRIVATE_") || !strings.Contains(string(output), `"messages":[]`) || !strings.Contains(string(output), `"startupStatus":"ready"`) {
		t.Fatalf("unsafe observation projection: %s", output)
	}
}

func TestBasicStatusDoesNotDependOnEnrollmentDiscovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig terminal integration is Unix-only")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required by the coordinator")
	}
	home, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(home, "commands")
	t.Setenv("WEBMUX_ENROLLMENT_TEST_LOG", log)
	scripts := map[string]string{
		"tmux": "#!/bin/sh\nif [ \"$1\" = -V ]; then echo 'tmux 3.8'; fi\n",
		"rig": `#!/bin/sh
printf '%s\n' "$*" >> "$WEBMUX_ENROLLMENT_TEST_LOG"
case "$*" in
 ps*) printf '%s\n' '{"entries":[]}' ;;
 *) exit 23 ;;
esac
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := Run(context.Background(), home, "owner", "status", ""); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "daemon") {
		t.Fatalf("basic discovery depends on enrollment probe: %s", calls)
	}
	if strings.Count(string(calls), "--version") != 1 {
		t.Fatalf("expected one attachment-contract version probe: %s", calls)
	}
}

func TestStructuredAttachmentProjectsFullInventoryWithoutPrivateMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig terminal integration is Unix-only")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required by the coordinator")
	}
	home, bin := t.TempDir(), t.TempDir()
	for name, script := range map[string]string{
		"tmux": "#!/bin/sh\necho 'tmux 3.8'\n",
		"rig": `#!/bin/sh
case "$*" in
 --version) echo '0.7.18 (3bf87c62)' ;;
 'ps --nodes -A --json --full') printf '%s\n' '[{"rigId":"r","logicalId":"agents.one","canonicalSessionName":"same","attachmentType":"tmux","tmuxServerArgs":["-S","/tmp/exact socket"],"recoveryGuidance":"PRIVATE_GUIDANCE","env":{"token":"PRIVATE_TOKEN"},"agentActivity":{"state":"running","rawEvent":"PRIVATE_EVENT"}}]' ;;
 'ps --include-archived --json --fields '*) echo '[]' ;;
 *) exit 23 ;;
esac
`,
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := Run(context.Background(), home, "owner", "status", "")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Seats []struct {
			Openable bool   `json:"openable"`
			Flag     string `json:"tmuxServerFlag"`
			Socket   string `json:"tmuxServerValue"`
		} `json:"seats"`
	}
	if err := json.Unmarshal(output, &data); err != nil || strings.Contains(string(output), "PRIVATE_") || len(data.Seats) != 1 || !data.Seats[0].Openable || data.Seats[0].Flag != "-S" || data.Seats[0].Socket != "/tmp/exact socket" {
		t.Fatalf("structured inventory selection/projection failed: %s, %v", output, err)
	}
}
