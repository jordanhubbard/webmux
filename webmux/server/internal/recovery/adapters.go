package recovery

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var nativeUUID = regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var openCodeID = regexp.MustCompile(`^ses_[A-Za-z0-9_-]{8,128}$`)
var credentialArgument = regexp.MustCompile(`(?i)(--(?:api[-_]?key|token|password|secret|header)(?:[=\s]|$)|\b[A-Z_]*(?:API_KEY|TOKEN|PASSWORD|SECRET)=)`)
var inlineConfigurationArgument = regexp.MustCompile(`(?i)(\bOPENCODE_(?:CONFIG_CONTENT|PERMISSION)=|--settings(?:=|\s+)\s*['"]?\s*[\[{])`)

func SafeCommand(command string) (string, bool) {
	if credentialArgument.MatchString(command) {
		return "[command contains credential arguments; recover manually]", false
	}
	if inlineConfigurationArgument.MatchString(command) {
		return "[command contains inline configuration; recover manually]", false
	}
	return command, true
}

func safeArgv(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	if credentialArgument.MatchString(strings.Join(args, " ")) || inlineConfigurationArgument.MatchString(strings.Join(args, " ")) {
		return []string{args[0], "[credential arguments omitted]"}
	}
	return append([]string(nil), args...)
}

func CLIName(name string) string {
	switch strings.TrimSuffix(filepath.Base(name), ".exe") {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	case "opencode":
		return "opencode"
	case "agent", "cursor-agent":
		return "cursor"
	}
	return ""
}

func ValidNativeID(cli, id string) bool {
	if cli == "opencode" {
		return openCodeID.MatchString(id)
	}
	return (cli == "codex" || cli == "claude" || cli == "cursor") && nativeUUID.MatchString(id)
}

// ResumeArgv uses the provider-native session selector, never --last/--continue.
// Prior command-line approval or sandbox bypass flags are deliberately absent.
func ResumeArgv(cli, id string) ([]string, error) {
	if !ValidNativeID(cli, id) {
		return nil, errors.New("no verified native session ID is available")
	}
	switch cli {
	case "codex":
		return []string{"codex", "resume", id}, nil
	case "claude":
		return []string{"claude", "--resume", id}, nil
	case "opencode":
		return []string{"opencode", "--session", id}, nil
	case "cursor":
		return []string{"agent", "--resume", id}, nil
	}
	return nil, errors.New("this coding CLI has no qualified recovery adapter")
}

func SelectedNativeID(cli string, args []string) string {
	// Forks, worktrees and remote/cloud selectors can replace a requested ID
	// or project. Do not claim the source conversation as the active one.
	for _, arg := range args {
		key, _, _ := strings.Cut(arg, "=")
		if (cli == "claude" || cli == "cursor") && (key == "--fork-session" || key == "--worktree" || key == "-w") || cli == "claude" && (key == "--cloud" || key == "--teleport" || key == "--environment" || key == "--desktop" || key == "--bg" || key == "--background") || cli == "opencode" && key == "--fork" {
			return ""
		}
	}
	for i, arg := range args {
		key, value, assigned := strings.Cut(arg, "=")
		selected := cli == "codex" && key == "resume" || cli == "claude" && (key == "--resume" || key == "-r" || key == "--session-id") || cli == "opencode" && (key == "--session" || key == "-s") || cli == "cursor" && key == "--resume"
		if !selected {
			continue
		}
		if !assigned && i+1 < len(args) {
			value = args[i+1]
		}
		if ValidNativeID(cli, value) {
			return value
		}
	}
	return ""
}

func commandOnly(cli string, args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-v" || arg == "--help" || arg == "-h" {
			return true
		}
	}
	if len(args) == 0 {
		return false
	}
	if cli == "cursor" {
		for _, arg := range args {
			if arg == "--list-models" {
				return true
			}
		}
	}
	if cli == "opencode" && (args[0] == "agent" || args[0] == "acp" || args[0] == "github" || args[0] == "plug") || cli == "claude" && (args[0] == "agents" || args[0] == "auto-mode" || args[0] == "doctor" || args[0] == "gateway" || args[0] == "logs" || args[0] == "purge" || args[0] == "respawn" || args[0] == "rm" || args[0] == "setup-token" || args[0] == "stop" || args[0] == "kill" || args[0] == "ultrareview") {
		return true
	}
	switch args[0] {
	case "persist", "attach", "install-shell-integration", "uninstall-shell-integration", "generate-rule", "rule", "bedrock", "login", "logout", "auth", "providers", "models", "mcp", "plugin", "plugins", "upgrade", "update", "install", "uninstall", "completion", "completions", "debug", "features", "app-server", "serve", "web", "export", "import", "db", "session", "stats", "create-chat", "status", "whoami", "about", "team", "worker", "help":
		return true
	}
	return false
}
