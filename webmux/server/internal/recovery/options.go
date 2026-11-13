package recovery

import (
	"errors"
	"regexp"
	"strings"
)

var namedOption = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// This is the one configuration override supported for native resume.
// Keep a single TOML string assignment: accepting a suffix or
// another assignment could change execution policy when the CLI parses it.
var codexWindowsSandbox = regexp.MustCompile(`^[ \t]*windows\.sandbox[ \t]*=[ \t]*("unelevated"|'unelevated')[ \t]*$`)

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
		// omitted flags are approval or trust automation, never carried into
		// a recovery command: the resumed CLI asks instead. dropped options
		// carry an initial prompt, which is never replayed.
		boolean, valued, selector, omitted, dropped := false, false, false, false, false
		switch cli {
		case "codex":
			omitted = key == "--dangerously-bypass-approvals-and-sandbox" || key == "--full-auto" || key == "--last" || key == "--all" || key == "--approve-for-me" || key == "--dangerously-bypass-hook-trust"
			boolean = key == "--no-daemon" || key == "--no-alt-screen" || key == "--strict-config" || key == "--search" || key == "--oss"
			valued = key == "--config" || key == "-c" || key == "--sandbox" || key == "-s" || key == "--profile" || key == "-p" || key == "--ask-for-approval" || key == "-a" || key == "--disable" || key == "--enable" || key == "--model" || key == "-m" || key == "--add-dir" || key == "--local-provider"
			if key == "--remote" || key == "--ignore-user-config" {
				return nil, errors.New("this Codex invocation uses a remote app server or ignores user configuration; restore it manually to preserve its execution policy")
			}
		case "cursor":
			selector = key == "--resume"
			omitted = key == "--yolo" || key == "--force" || key == "-f" || key == "--auto-review" || key == "--approve-mcps" || key == "--trust"
			boolean = key == "--plan"
			valued = key == "--mode" || key == "--sandbox" || key == "--model" || key == "--add-dir" || key == "--plugin-dir"
		case "opencode":
			selector = key == "--session" || key == "-s"
			omitted = key == "--auto" || key == "--continue" || key == "-c"
			boolean = key == "--pure" || key == "--mini" || key == "--no-replay" || key == "--print-logs"
			valued = key == "--agent" || key == "--model" || key == "-m" || key == "--log-level" || key == "--replay-limit"
			dropped = key == "--prompt"
		}
		if boolean && !assigned {
			options = append(options, key)
			continue
		}
		if omitted && !assigned {
			continue
		}
		if !valued && !selector && !dropped {
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
		if dropped {
			continue
		}
		if selector {
			if !ValidNativeID(cli, value) {
				return nil, errors.New("recorded CLI selector is invalid; restore manually")
			}
			continue
		}
		if cli == "codex" && (key == "--config" || key == "-c") && !codexWindowsSandbox.MatchString(value) {
			return nil, errors.New("this Codex configuration override is unsupported; restore it manually to preserve its execution policy")
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
		// A named agent is resumed by name, as Claude's named agents are; its
		// definition stays in the user's OpenCode configuration.
		if cli == "opencode" && key == "--agent" && !namedOption.MatchString(value) {
			return nil, errors.New("the recorded OpenCode agent name is invalid; restore it manually")
		}
		options = append(options, key, value)
	}
	return options, nil
}
