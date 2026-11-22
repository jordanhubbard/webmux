package coordinationclient

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func nativeAccess(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "access with spaces.json")
	t.Setenv("WEBMUX_COORDINATION_FILE", name)
	t.Setenv("WEBMUX_RECOVERY_SESSION", "one")
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "")
	if err := WriteAccess(name, Access{Contract: coordination.Contract, Endpoint: "http://127.0.0.1:12345", Capability: "wmc1.fixture.private-capability", Subject: coordination.Ref{Session: "one"}}); err != nil {
		t.Fatal(err)
	}
	return name
}
func TestNativeBootstrapPreservesPolicyAndCredentials(t *testing.T) {
	name := nativeAccess(t)
	encodedName, _ := json.Marshal(name)
	helper := filepath.Join(t.TempDir(), "webmux")
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	t.Setenv("CLAUDE_CODE_SIMPLE", "")
	for _, tc := range []struct {
		cli, version string
		args         []string
	}{
		{"codex", "codex-cli 0.171.0", []string{"--sandbox", "read-only", "-a", "never", "resume", "original-id", "owner prompt"}},
		{"claude", "2.1.327 (Claude Code)", []string{"--permission-mode", "plan", "--resume", "original-id", "owner prompt"}},
	} {
		got, status := NativeLaunch(tc.cli, tc.version, helper, tc.args)
		if status != "mcp-configured" || len(got) <= len(tc.args) || !reflect.DeepEqual(got[len(got)-len(tc.args):], tc.args) {
			t.Fatal("original launch changed", status, got)
		}
		joined := strings.Join(got, " ")
		if strings.Contains(joined, "private-capability") || !strings.Contains(joined, "coordinate") || !strings.Contains(joined, string(encodedName)) {
			t.Fatal("missing scoped server or credential leaked")
		}
		for _, flag := range []string{"--safe-mode", "--bare", "--restricted", "--strict-mcp-config", "--mcp-config=owner.json", "--remote=ws://127.0.0.1:3333", "--tools="} {
			original := append([]string{flag}, tc.args...)
			got, status := NativeLaunch(tc.cli, tc.version, helper, original)
			if status != "mcp-policy-preserved" || !reflect.DeepEqual(got, original) {
				t.Fatal("explicit policy changed", flag, status)
			}
		}
		original := append([]string{"-c", "mcp_servers.owner.enabled=false"}, tc.args...)
		if got, status := NativeLaunch(tc.cli, tc.version, helper, original); status != "mcp-policy-preserved" || !reflect.DeepEqual(got, original) {
			t.Fatal("explicit MCP config changed")
		}
		if got, status := NativeLaunch(tc.cli, "unknown-version", helper, tc.args); status != "mcp-version-unqualified" || !reflect.DeepEqual(got, tc.args) {
			t.Fatal(status)
		}
	}
	t.Setenv("WEBMUX_RECOVERY_SESSION", "other-session")
	if _, err := Load(); err == nil {
		t.Fatal("another session's private file was accepted")
	}
}

func TestCodexTransportCompatibilityHonorsExplicitChoice(t *testing.T) {
	nativeAccess(t)
	helper := filepath.Join(t.TempDir(), "webmux")
	for _, original := range [][]string{{"--enable", "code_mode_host_grpc", "task"}, {"--disable=code_mode_host_grpc", "task"}, {"-c", "features.code_mode_host_grpc=true", "task"}, {"-cfeatures.code_mode_host_grpc=true", "task"}} {
		args, status := NativeLaunch("codex", "codex-cli 0.171.0", helper, original)
		if status != "mcp-configured" {
			t.Fatal(status)
		}
		prefix := args[:len(args)-len(original)]
		for _, value := range prefix {
			if value == "code_mode_host_grpc" {
				t.Fatal("overrode explicit native transport")
			}
		}
	}
	args, status := NativeLaunch("codex", "codex-cli 0.171.0", helper, []string{"task"})
	if status != "mcp-configured" || len(args) < 2 || args[0] != "--disable" || args[1] != "code_mode_host_grpc" {
		t.Fatal("missing qualified transport adjustment")
	}
}

func TestCodexCompactMCPOverridePreservesPolicy(t *testing.T) {
	nativeAccess(t)
	helper := filepath.Join(t.TempDir(), "webmux")
	original := []string{`-cmcp_servers.fixture.command="noop"`, "mcp", "list", "--json"}
	args, status := NativeLaunch("codex", "codex-cli 0.171.0", helper, original)
	if status != "mcp-policy-preserved" || !reflect.DeepEqual(args, original) {
		t.Fatal("compact explicit MCP override was not preserved", status)
	}
	if os.Getenv("WEBMUX_QUALIFY_NATIVE_MCP_PARSERS") != "1" {
		return
	}
	// Native syntax witness: an isolated configuration lists only the owner's
	// server. No model, MCP subprocess or existing credential home is opened.
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = t.TempDir()
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("native compact MCP parser refused the preserved invocation", err)
	}
	var servers []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(output, &servers) != nil || len(servers) != 1 || servers[0].Name != "fixture" {
		t.Fatal("native parser did not retain only the explicitly configured MCP server")
	}
	t.Log("native Codex parser accepted the compact override with only the owner's server")
}

// This opt-in control uses native parsers with empty temporary configuration,
// without starting a model, reading credentials or changing user settings.
func TestNativeMCPConfigParsers(t *testing.T) {
	if os.Getenv("WEBMUX_QUALIFY_NATIVE_MCP_PARSERS") != "1" {
		t.Skip("requires explicitly selected installed native CLIs")
	}
	nativeAccess(t)
	helper := filepath.Join(t.TempDir(), "webmux")
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	t.Setenv("CLAUDE_CODE_SIMPLE", "")
	for _, tc := range []struct {
		cli, version, home string
		args               []string
	}{
		{"codex", "codex-cli 0.171.0", "CODEX_HOME", []string{"mcp", "list", "--json"}},
	} {
		t.Run(tc.cli, func(t *testing.T) {
			binary, err := exec.LookPath(tc.cli)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(tc.home, t.TempDir())
			args, status := NativeLaunch(tc.cli, tc.version, helper, tc.args)
			if status != "mcp-configured" {
				t.Fatal(status)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = t.TempDir()
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "webmux_") {
				t.Fatalf("native parser refused scoped config: %v %s", err, output)
			}
			t.Log("native parser accepted session-scoped MCP configuration")
		})
	}
}
