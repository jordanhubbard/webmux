package coordinationclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// NativeLaunch adds only this session's stdio MCP server. It never rewrites user
// config, credentials, sandbox/approval flags, prompts, or another server entry.
// Unknown versions and explicitly restricted MCP invocations pass through intact.
func NativeLaunch(cli, version, helper string, args []string) ([]string, string) {
	unchanged := func(status string) ([]string, string) { return append([]string(nil), args...), status }
	fields := strings.Fields(version)
	qualified := cli == "codex" && len(fields) == 2 && fields[0] == "codex-cli" && fields[1] == "0.171.0" || cli == "claude" && len(fields) >= 1 && fields[0] == "2.1.327"
	if !qualified {
		return unchanged("mcp-version-unqualified")
	}
	if !filepath.IsAbs(helper) {
		return unchanged("mcp-interface-unavailable")
	}
	legacyTransport := cli == "codex"
	for i, arg := range args {
		if arg == "--" {
			break
		}
		key, value, assigned := strings.Cut(arg, "=")
		switch key {
		case "--safe-mode", "--bare", "--restricted", "--strict-mcp-config", "--mcp-config", "--remote", "--remote-auth-token-env", "--setting-sources", "--tools", "--agent", "--bg", "--background":
			return unchanged("mcp-policy-preserved")
		}
		if key == "--enable" || key == "--disable" {
			feature := value
			if !assigned && i+1 < len(args) {
				feature = args[i+1]
			}
			if feature == "code_mode_host_grpc" {
				legacyTransport = false
			}
		}
		if key == "-c" || key == "--config" {
			if !assigned && i+1 < len(args) {
				value = args[i+1]
			}
			if strings.HasPrefix(strings.TrimSpace(value), "features.code_mode_host_grpc") {
				legacyTransport = false
			}
			if strings.HasPrefix(strings.TrimSpace(value), "mcp_servers") {
				return unchanged("mcp-policy-preserved")
			}
		}
	}
	if cli == "claude" && (os.Getenv("CLAUDE_CODE_SAFE_MODE") != "" || os.Getenv("CLAUDE_CODE_SIMPLE") != "") {
		return unchanged("mcp-policy-preserved")
	}
	access, err := Load()
	if err != nil {
		return unchanged("mcp-interface-unavailable")
	}
	name, err := AccessPath()
	if err != nil {
		return unchanged("mcp-interface-unavailable")
	}
	digest := sha256.Sum256([]byte(access.Subject.Key()))
	server := "webmux_" + hex.EncodeToString(digest[:6])
	environment := map[string]string{"WEBMUX_COORDINATION_FILE": name, "WEBMUX_COORDINATION_EXPECTED_SESSION": access.Subject.Session}
	var prefix []string
	if cli == "codex" {
		value, _ := json.Marshal(helper)
		envFile, _ := json.Marshal(name)
		expected, _ := json.Marshal(access.Subject.Session)
		prefix = []string{"-c", "mcp_servers." + server + ".command=" + string(value), "-c", "mcp_servers." + server + ".args=[\"coordinate\",\"mcp\"]", "-c", "mcp_servers." + server + ".env={WEBMUX_COORDINATION_FILE=" + string(envFile) + ",WEBMUX_COORDINATION_EXPECTED_SESSION=" + string(expected) + "}"}
		// Codex 0.171.0's default gRPC host timed out during native acceptance;
		// the same host/tools pass using its supported non-gRPC transport. This is
		// a version-bound transport adjustment, never a sandbox/approval override.
		if legacyTransport {
			prefix = append([]string{"--disable", "code_mode_host_grpc"}, prefix...)
		}
	} else {
		config, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{server: map[string]any{"type": "stdio", "command": helper, "args": []string{"coordinate", "mcp"}, "env": environment}}})
		// The assigned form prevents Claude's variadic option consuming a prompt.
		prefix = []string{"--mcp-config=" + string(config)}
	}
	return append(prefix, args...), "mcp-configured"
}
