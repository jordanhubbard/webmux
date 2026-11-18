package httpapi

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// The qualified runner resolves standard labels/defaults to an absolute socket.
// Empty fields retain the legacy, environment-selected attachment contract.
func rigTmuxServerArgs(flag, value string) ([]string, error) {
	if flag == "" && value == "" {
		return nil, nil
	}
	if flag != "-S" || value == "" || len(value) > 2048 || !filepath.IsAbs(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return nil, fmt.Errorf("Rig tmux server selection is invalid or unsupported; refresh the canonical inventory")
	}
	return []string{flag, value}, nil
}

var rigTmuxTarget = regexp.MustCompile(`^[a-zA-Z0-9_.:@-]{1,256}$`)
var rigTmuxSessionID = regexp.MustCompile(`^\$[0-9]+$`)

func rigTmuxArgs(flag, value, command, target string, args ...string) ([]string, error) {
	if !rigTmuxTarget.MatchString(target) {
		return nil, fmt.Errorf("Rig tmux attachment target is invalid or unsupported")
	}
	server, err := rigTmuxServerArgs(flag, value)
	if err != nil {
		return nil, err
	}
	exact := "=" + target
	if command == "display-message" {
		exact += ":"
	}
	return append(append(server, command, "-t", exact), args...), nil
}
