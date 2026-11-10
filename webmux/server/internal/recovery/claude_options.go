package recovery

import (
	"errors"
	"strings"
)

// Selectors are replaced by the recorded native ID. All other options must be
// explicitly understood: an unfamiliar flag may change policy or context.
func claudeResumeOptions(original []string) ([]string, error) {
	var options []string
	invalid := func() ([]string, error) {
		return nil, errors.New("this Claude invocation has unsupported or incomplete settings; restore it manually to preserve its execution policy")
	}
	for i := 1; i < len(original); i++ {
		arg := original[i]
		if arg == "[credential arguments omitted]" {
			return invalid()
		}
		key, value, assigned := strings.Cut(arg, "=")
		if key == "--" {
			break
		}
		if !strings.HasPrefix(key, "-") {
			continue
		} // Initial prompts are never replayed.
		switch key {
		case "--continue", "-c", "--fork-session", "--dangerously-skip-permissions", "--allow-dangerously-skip-permissions":
			if assigned {
				return invalid()
			}
			continue
		case "--resume", "-r":
			if !assigned && i+1 < len(original) && !strings.HasPrefix(original[i+1], "-") {
				i++
			}
			continue
		case "--session-id":
			if !assigned {
				i++
				if i >= len(original) {
					return invalid()
				}
				value = original[i]
			}
			if !ValidNativeID("claude", value) {
				return invalid()
			}
			continue
		case "--safe-mode", "--restricted", "--bare", "--strict-mcp-config", "--disable-slash-commands", "--verbose":
			if assigned {
				return invalid()
			}
			options = append(options, key)
			continue
		case "--tools", "--allowedTools", "--allowed-tools", "--disallowedTools", "--disallowed-tools", "--add-dir":
			var values []string
			if assigned {
				values = []string{value}
			} else {
				for i+1 < len(original) && !strings.HasPrefix(original[i+1], "-") {
					i++
					values = append(values, original[i])
				}
			}
			if len(values) == 0 {
				return invalid()
			}
			for _, value := range values {
				if strings.HasPrefix(value, "-") || value == "" && key != "--tools" {
					return invalid()
				}
			}
			options = append(options, key)
			options = append(options, values...)
			continue
		case "--permission-mode", "--model", "--agent", "--effort", "--setting-sources", "--system-prompt-snapshot":
			if !assigned {
				i++
				if i >= len(original) {
					return invalid()
				}
				value = original[i]
			}
			if strings.HasPrefix(value, "-") || value == "" && key != "--setting-sources" {
				return invalid()
			}
			switch key {
			case "--permission-mode":
				switch value {
				case "bypassPermissions":
					continue
				case "acceptEdits", "auto", "manual", "dontAsk", "plan":
				default:
					return invalid()
				}
			case "--effort":
				switch value {
				case "low", "medium", "high", "xhigh", "max":
				default:
					return invalid()
				}
			case "--setting-sources":
				if value != "" {
					for _, source := range strings.Split(value, ",") {
						switch source {
						case "user", "project", "local":
						default:
							return invalid()
						}
					}
				}
			case "--system-prompt-snapshot":
				if value != "on" && value != "off" {
					return invalid()
				}
			}
			options = append(options, key, value)
		default:
			return invalid()
		}
	}
	return options, nil
}
