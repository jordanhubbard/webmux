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
		boolean, valued, selector, omitted := false, false, false, false
		switch cli {
		case "codex":
			omitted = key == "--dangerously-bypass-approvals-and-sandbox" || key == "--full-auto" || key == "--last" || key == "--all"
			boolean = key == "--no-daemon" || key == "--no-alt-screen" || key == "--strict-config"
			valued = key == "--sandbox" || key == "-s" || key == "--profile" || key == "-p" || key == "--ask-for-approval" || key == "-a" || key == "--disable" || key == "--enable"
			if key == "--config" || key == "-c" || key == "--remote" || key == "--ignore-user-config" {
				return nil, errors.New("this Codex invocation uses configuration overrides or a remote app server; restore it manually to preserve its execution policy")
			}
		case "cursor":
			selector = key == "--resume"
			omitted = key == "--yolo"
			boolean = key == "--plan"
			valued = key == "--mode" || key == "--sandbox"
		case "opencode":
			selector = key == "--session" || key == "-s"
			omitted = key == "--auto" || key == "--continue" || key == "-c"
			boolean = key == "--pure"
			valued = key == "--agent"
		}
		if boolean && !assigned {
			options = append(options, key)
			continue
		}
		if omitted && !assigned {
			continue
		}
		if !valued && !selector {
			if strings.HasPrefix(key, "-") {
				return nil, errors.New("this CLI invocation has unsupported settings; restore it manually to preserve its configuration")
			}
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
		if selector {
			if !ValidNativeID(cli, value) {
				return nil, errors.New("recorded CLI selector is invalid; restore manually")
			}
			continue
		}
		// Never reintroduce explicit bypass modes from an earlier invocation.
		if cli == "codex" && (key == "--sandbox" || key == "-s") && value != "read-only" && value != "workspace-write" {
			if value == "danger-full-access" {
				continue
			}
			return nil, errors.New("the recorded Codex sandbox is unsupported; restore manually")
		}
		if cli == "cursor" && key == "--sandbox" && value != "enabled" {
			if value == "disabled" {
				continue
			}
			return nil, errors.New("the recorded Cursor sandbox is unsupported; restore manually")
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
