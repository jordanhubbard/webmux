package session

import "strings"

// replayScreen remembers screen selection in the discarded prefix of the raw
// output ring. Without it, a fresh viewer replays tmux redraws in the normal
// buffer, where resizing reflows fixed status bars into ordinary scrollback.
// This is a bounded control parser, not a screen emulator. String controls are
// skipped so escape-looking text in an OSC/DCS payload cannot select a screen.
type replayScreen struct {
	alternate bool
	state     byte
	pending   string
}

func (e *entry) appendReplay(data string) {
	previous := e.scrollback
	e.scrollback, e.scrollbackUnits = appendScrollback(previous, e.scrollbackUnits, data)
	discarded := len(previous) + len(data) - len(e.scrollback)
	if discarded > 0 {
		n := min(discarded, len(previous))
		e.replayScreen.feed(previous[:n])
		e.replayScreen.feed(data[:discarded-n])
	}
}

func (s *replayScreen) feed(data string) {
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch s.state {
		case 0:
			if c == '\x1b' {
				s.state, s.pending = 'e', "\x1b"
			}
		case 'e':
			switch c {
			case '[':
				s.state, s.pending = 'c', "\x1b["
			case ']', 'P', '^', '_', 'X':
				s.state, s.pending = 's', "\x1b"+string(c)
			case '\x1b':
			case 'c':
				s.alternate = false
				s.state, s.pending = 0, ""
			default:
				s.state, s.pending = 0, ""
			}
		case 'c':
			if c == '\x1b' {
				s.state, s.pending = 'e', "\x1b"
				continue
			}
			if c == '\x18' || c == '\x1a' {
				s.state, s.pending = 0, ""
				continue
			}
			if c >= 0x40 && c <= 0x7e {
				if (c == 'h' || c == 'l') && strings.HasPrefix(s.pending, "\x1b[?") {
					for _, mode := range strings.Split(s.pending[3:], ";") {
						if mode == "47" || mode == "1047" || mode == "1049" {
							s.alternate = c == 'h'
						}
					}
				}
				s.state, s.pending = 0, ""
			} else if len(s.pending) < 128 {
				s.pending += string(c)
			} else {
				s.state, s.pending = 0, ""
			}
		case 's':
			if c == '\x07' && s.pending == "\x1b]" {
				s.state, s.pending = 0, ""
			} else if c == '\x1b' {
				s.state = 't'
			}
		case 't':
			if c == '\\' {
				s.state, s.pending = 0, ""
			} else if c != '\x1b' {
				s.state = 's'
			}
		}
	}
}

func (s *replayScreen) prefix() string {
	prefix := ""
	if s.alternate {
		prefix = "\x1b[?1049h"
	}
	// Complete a control split by the retention boundary in the fresh parser.
	prefix += s.pending
	if s.state == 't' {
		prefix += "\x1b"
	}
	return prefix
}
