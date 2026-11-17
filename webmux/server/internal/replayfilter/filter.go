// Package replayfilter keeps terminal output requests out of retained replay.
// Live output must still be delivered unchanged. This filter is only for the
// history copy: it is not a screen emulator or an authorization boundary for
// input from a running terminal.
package replayfilter

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Filter accepts arbitrary read boundaries and retains at most one UTF-8 rune
// and bounded escape/OSC state. String controls are omitted; the Xterm profile
// retains normalized color overrides separately as metadata. Clipboard/browser/
// recovery OSC and DCS queries never enter replay. Queries must not be
// replayed: the viewer's replies would become fresh input to the surviving job.
type Filter struct {
	// Xterm matches the browser's xterm display profile. Its unsupported DEC
	// mode 69 must remain off so CSI s saves a cursor, rather than setting
	// horizontal margins. This affects the mirror only, never live output.
	Xterm              bool
	state              byte
	sequence           []byte
	partial            []byte
	osc                bool
	oscData            []byte
	oscOverflow        bool
	unsupportedDisplay bool
	colors             ColorState
	repeatable         bool
	beforeControl      bool
}

// AtBoundary reports whether a new viewer can begin the following live stream
// without inheriting a partial character, escape sequence or private payload.
func (f *Filter) AtBoundary() bool { return f.state == 0 && len(f.partial) == 0 }

// CanRepeat reports whether xterm would accept REP at this exact boundary.
// Complete controls clear that state even when omitted from the mirror copy.
func (f *Filter) CanRepeat() bool { return f.Xterm && f.repeatable && f.AtBoundary() }

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
			cancelled := r == 0x18 || r == 0x1a
			finished := r == 0x9c || (r == 7 && f.osc) || (f.state == 't' && r == '\\')
			if finished || cancelled {
				if finished && !cancelled && f.Xterm && f.osc {
					updates, unsupported := renderingOSC(f.oscData, f.oscOverflow)
					for _, update := range updates {
						f.colors.apply(update)
					}
					f.unsupportedDisplay = f.unsupportedDisplay || unsupported
				}
				f.state, f.oscData, f.oscOverflow = 0, nil, false
			} else if r == 0x1b {
				if f.state == 't' {
					f.appendOSC(0x1b)
				}
				f.state = 't'
			} else {
				if f.state == 't' {
					f.appendOSC(0x1b)
				}
				f.appendOSC(r)
				f.state = 's'
			}
			continue
		}
		if r == 0x18 || r == 0x1a {
			f.state, f.sequence = 0, nil
			f.repeatable, f.beforeControl = false, false
			continue
		}
		if r == 0x1b {
			f.beforeControl, f.repeatable = f.repeatable, false
			f.state, f.sequence = 'e', []byte{0x1b}
			continue
		}
		if r == 0x9b {
			f.beforeControl, f.repeatable = f.repeatable, false
			f.state, f.sequence = 'c', []byte{0x1b, '['}
			continue
		}
		if r == 0x90 || r == 0x98 || r == 0x9d || r == 0x9e || r == 0x9f {
			f.repeatable, f.beforeControl = false, false
			f.state, f.sequence, f.osc = 's', nil, r == 0x9d
			continue
		}
		if f.state != 0 && (r < 0x20 || r == 0x7f) {
			// C0 executes inside ESC/CSI without ending the sequence; DEL is
			// simply ignored. Keep the boundary incomplete until its final byte.
			if r != 0x7f {
				f.beforeControl = false
				if r >= 8 && r <= 15 {
					out.WriteRune(r)
				}
			}
			continue
		}
		switch f.state {
		case 0:
			if r != 0x7f { // DEL is ignored by xterm without clearing REP.
				f.repeatable = r >= 0x20 && (r < 0x7f || r >= 0xa0)
			}
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
				if r == 'c' {
					f.unsupportedDisplay = false
					f.colors = ColorState{}
				}
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
					sequence := f.sequence
					if f.Xterm {
						sequence = xtermCSI(sequence, r)
						if r == 'b' && !f.beforeControl {
							sequence = nil
						}
					}
					if len(sequence) > 0 {
						out.Write(sequence)
						out.WriteRune(r)
					}
				}
				f.state, f.sequence = 0, nil
				f.repeatable, f.beforeControl = false, false
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

func xtermCSI(sequence []byte, final rune) []byte {
	if final == 'm' {
		return xtermSGR(sequence)
	}
	if final != 'h' && final != 'l' || !bytes.HasPrefix(sequence, []byte("\x1b[?")) {
		return sequence
	}
	params := strings.Split(string(sequence[3:]), ";")
	kept := make([]string, 0, len(params))
	for _, p := range params {
		value, _, _ := strings.Cut(p, ":")
		number, err := strconv.Atoi(value)
		if err != nil || number != 69 {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(params) {
		return sequence
	}
	if len(kept) == 0 {
		return nil
	}
	return []byte("\x1b[?" + strings.Join(kept, ";"))
}

func renderingCSI(sequence []byte, final rune) bool {
	// Do not retain device/status/mode queries or window operations. Only
	// display editing, movement, modes and rendition pass this allowlist.
	parameters := string(sequence[2:])
	if final == 'p' {
		return parameters == "!"
	} // DEC soft reset
	if final == 'q' {
		if parameters == "\"" || parameters == "0\"" || parameters == "1\"" || parameters == "2\"" {
			return true // DECSCA affects subsequent selective erase.
		}
		return strings.HasSuffix(parameters, " ") && strings.Trim(parameters, "0123456789 ") == ""
	} // cursor style
	if !strings.ContainsRune("@ABCDEFGHIJKLMPSTXZ`abdefghlmrsu", final) {
		return false
	}
	return strings.Trim(parameters, "0123456789;:?>") == ""
}

// The pinned frontend stores SGR 59's -1 underline-color sentinel in a
// 26-bit extended-color field. While extended attributes are enabled, that
// renders as RGB white rather than the foreground default. Mirror the actual
// renderer, including subsequent SGR 4, instead of the generic VT interpretation.
func xtermSGR(sequence []byte) []byte {
	params := strings.Split(string(sequence[2:]), ";")
	var out []string
	changed := false
	for i := 0; i < len(params); i++ {
		p := params[i]
		code, _, subparams := strings.Cut(p, ":")
		number, err := strconv.Atoi(code)
		if err != nil {
			number = -1
		}
		switch number {
		case 59:
			out = append(out, "58", "2", "255", "255", "255")
			changed = true
		case 38, 48, 58:
			out = append(out, p)
			// Semicolon colors consume their numeric payloads, even when one of
			// the channels is 59. Colon colors are a single entry and pass intact.
			if !subparams && i+1 < len(params) {
				n := 0
				mode, _ := strconv.Atoi(params[i+1])
				switch mode {
				case 2:
					n = 4
				case 5:
					n = 2
				}
				n = min(n, len(params)-i-1)
				out = append(out, params[i+1:i+1+n]...)
				i += n
			}
		default:
			out = append(out, p)
		}
	}
	if !changed {
		return sequence
	}
	return []byte("\x1b[" + strings.Join(out, ";"))
}

// UnsupportedDisplay reports a display change outside the bounded OSC color
// profile. Refuse a new screen attachment until RIS repairs the mirror.
func (f *Filter) UnsupportedDisplay() bool { return f.unsupportedDisplay }

const maxOSCBytes = 8192

func (f *Filter) appendOSC(r rune) {
	if !f.Xterm || !f.osc || f.oscOverflow {
		return
	}
	if len(f.oscData)+utf8.RuneLen(r) > maxOSCBytes {
		f.oscOverflow = true
		return
	}
	f.oscData = utf8.AppendRune(f.oscData, r)
}
