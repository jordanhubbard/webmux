package recovery

import (
	"errors"
	"strings"
)

// Preserve supported invocation restrictions, rather than silently resuming
// with a more permissive default. Arbitrary settings payloads can contain
// credentials or change policy; their invocations require manual recovery.
func nativeResumeOptions(cli string, original []string) ([]string, error) {
	if cli == "claude" {
		return claudeResumeOptions(original)
	}
	var options []string
	for i := 1; i < len(original); i++ {
		if original[i] == "[credential arguments omitted]" {
			return nil, errors.New("recorded CLI arguments were redacted; restore manually to preserve its configuration")
		}
		key, value, assigned := strings.Cut(original[i], "=")
		if key == "--" {
			break
		}
		boolean, valued := false, false
		switch cli {
		case "codex":
			boolean = key == "--no-daemon" || key == "--no-alt-screen" || key == "--strict-config"
			valued = key == "--sandbox" || key == "-s" || key == "--profile" || key == "-p" || key == "--ask-for-approval" || key == "-a" || key == "--disable" || key == "--enable"
			if key == "--config" || key == "-c" || key == "--remote" || key == "--ignore-user-config" {
				return nil, errors.New("this Codex invocation uses configuration overrides or a remote app server; restore it manually to preserve its execution policy")
			}
		case "cursor":
			boolean = key == "--plan"
			valued = key == "--mode" || key == "--sandbox"
		case "opencode":
			boolean = key == "--pure"
			valued = key == "--agent"
		}
		if boolean && !assigned {
			options = append(options, key)
			continue
		}
		if !valued {
			continue
		}
		if !assigned {
			i++
			if i >= len(original) {
				return nil, errors.New("recorded CLI restriction is incomplete; restore manually")
			}
			value = original[i]
		}
		if strings.HasPrefix(value, "-") || value == "" {
			return nil, errors.New("recorded CLI restriction is invalid; restore manually")
		}
		// Never reintroduce explicit bypass modes from an earlier invocation.
		if cli == "codex" && (key == "--sandbox" || key == "-s") && value != "read-only" && value != "workspace-write" {
			continue
		}
		if cli == "cursor" && key == "--sandbox" && value != "enabled" {
			continue
		}
		if cli == "cursor" && key == "--mode" && value != "ask" && value != "plan" {
			return nil, errors.New("the recorded Cursor mode is unsupported; restore manually")
		}
		if cli == "opencode" && key == "--agent" && value != "plan" {
			return nil, errors.New("this OpenCode invocation uses a custom agent policy; restore it manually")
		}
		options = append(options, key, value)
	}
	return options, nil
}
