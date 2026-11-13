// Package replayfilter keeps terminal output requests out of retained replay.
// Live output must still be delivered unchanged. This filter is only for the
// history copy: it is not a screen emulator or an authorization boundary for
// input from a running terminal.
package replayfilter

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Filter accepts arbitrary read boundaries and retains at most one UTF-8 rune
// and a bounded escape sequence. All string controls are omitted, including
// clipboard/browser/recovery OSC packets and DCS queries. Queries must not be
// replayed: the viewer's replies would become fresh input to the surviving job.
type Filter struct {
	state    byte
	sequence []byte
	partial  []byte
	osc      bool
}

// AtBoundary reports whether a new viewer can begin the following live stream
// without inheriting a partial character, escape sequence or private payload.
func (f *Filter) AtBoundary() bool { return f.state == 0 && len(f.partial) == 0 }

func (f *Filter) Feed(data []byte) []byte {
	if len(f.partial) > 0 {
		data = append(append([]byte{}, f.partial...), data...)
		f.partial = nil
	}
	var out bytes.Buffer
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			f.partial = append(f.partial[:0], data...)
			break
		}
		r, n := utf8.DecodeRune(data)
		data = data[n:]
		if f.state == 's' || f.state == 't' {
			if r == 0x9c || r == 0x18 || r == 0x1a || (r == 7 && f.osc) || (f.state == 't' && r == '\\') {
				f.state = 0
			} else if r == 0x1b {
				f.state = 't'
			} else {
				f.state = 's'
			}
			continue
		}
		if r == 0x18 || r == 0x1a {
			f.state, f.sequence = 0, nil
			continue
		}
		if r == 0x1b {
			f.state, f.sequence = 'e', []byte{0x1b}
			continue
		}
		if r == 0x9b {
			f.state, f.sequence = 'c', []byte{0x1b, '['}
			continue
		}
		if r == 0x90 || r == 0x98 || r == 0x9d || r == 0x9e || r == 0x9f {
			f.state, f.sequence, f.osc = 's', nil, r == 0x9d
			continue
		}
		switch f.state {
		case 0:
			// Omit BEL/ENQ and unrecognized C0/C1 controls. Retain ordinary
			// display movement and character-set shifts, as well as Unicode.
			if r >= 0x20 && (r < 0x7f || r >= 0xa0) || r >= 8 && r <= 15 {
				out.WriteRune(r)
			}
		case 'e':
			switch {
			case r == '[':
				f.state, f.sequence = 'c', append(f.sequence, '[')
			case strings.ContainsRune("]P^_X", r):
				f.state, f.sequence, f.osc = 's', nil, r == ']'
			case r >= 0x20 && r <= 0x2f:
				f.state, f.sequence = 'i', append(f.sequence, byte(r))
			default:
				if strings.ContainsRune("78DEHMc=>", r) {
					out.Write(f.sequence)
					out.WriteRune(r)
				}
				f.state, f.sequence = 0, nil
			}
		case 'i':
			// Character-set selection and DEC line rendering. Other ESC
			// intermediates (for example ESC SP G switching C1 encoding)
			// do not belong in replay.
			if len(f.sequence) == 2 && strings.ContainsRune("()*+-./#%", rune(f.sequence[1])) && r >= 0x30 && r <= 0x7e {
				out.Write(f.sequence)
				out.WriteRune(r)
			}
			f.state, f.sequence = 0, nil
		case 'c', 'd':
			if r >= 0x40 && r <= 0x7e {
				if f.state == 'c' && renderingCSI(f.sequence, r) {
					out.Write(f.sequence)
					out.WriteRune(r)
				}
				f.state, f.sequence = 0, nil
			} else if r >= 0x20 && r <= 0x3f {
				if len(f.sequence) < 128 && f.state == 'c' {
					f.sequence = append(f.sequence, byte(r))
				} else {
					f.state, f.sequence = 'd', nil
				}
			} else {
				f.state, f.sequence = 0, nil
			}
		}
	}
	return out.Bytes()
}

func renderingCSI(sequence []byte, final rune) bool {
	// Do not retain device/status/mode queries or window operations. Only
	// display editing, movement, modes and rendition pass this allowlist.
	parameters := string(sequence[2:])
	if final == 'p' {
		return parameters == "!"
	} // DEC soft reset
	if final == 'q' {
		return strings.HasSuffix(parameters, " ") && strings.Trim(parameters, "0123456789 ") == ""
	} // cursor style
	if !strings.ContainsRune("@ABCDEFGHIJKLMPSTXZ`abdefghlmrsu", final) {
		return false
	}
	return strings.Trim(parameters, "0123456789;:?>") == ""
}
