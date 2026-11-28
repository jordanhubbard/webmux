package session

import "path/filepath"

// ExternalTmuxAttach reports whether argv only reattaches a tmux session that
// already exists. WebMux may run that command again after an upgrade. It does
// not match Agent Deck or OpenRig launchers, SSH wrappers, or any tmux command
// that creates or destroys a session.
func ExternalTmuxAttach(argv []string) bool {
	if len(argv) < 2 || filepath.Base(argv[0]) != "tmux" {
		return false
	}
	attach := false
	for _, arg := range argv[1:] {
		switch arg {
		case "kill-server", "kill-session", "kill-window", "kill-pane",
			"new-session", "new", "new-window", "neww",
			"respawn-pane", "respawn-window", "respawnp", "respawnw",
			"send-keys", "send",
			"split-window", "splitw",
			"killp", "killw":
			return false
		case "attach-session", "attach", "a":
			attach = true
		}
	}
	return attach
}
