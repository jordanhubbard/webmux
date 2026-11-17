package replayfilter

import (
	"fmt"
	"strconv"
	"strings"
)

// Only numeric color setters/resets enter the mirror. Queries and all other
// string controls remain omitted; normalized setters cannot produce replies or
// carry clipboard/browser/recovery requests. Live output never uses this copy.
func renderingOSC(data []byte, overflow bool) ([]colorUpdate, bool) {
	parts := strings.Split(string(data), ";")
	if parts[0] == "" || strings.Trim(parts[0], "0123456789") != "" {
		return nil, false
	}
	command, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, false
	}
	switch command {
	case 4, 10, 11, 12, 104, 110, 111, 112:
	default:
		return nil, false
	}
	if overflow {
		return nil, true
	}
	var updates []colorUpdate
	color := func(command, index int, spec string) bool {
		if spec == "?" {
			return true
		} // Never reissue a query at attachment.
		normalized, ok := xtermColor(spec)
		if !ok {
			return false
		}
		updates = append(updates, colorUpdate{command, index, normalized})
		return true
	}
	index := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil && s != "" && strings.Trim(s, "0123456789") == "" && n >= 0 && n < 256
	}
	switch command {
	case 4:
		if len(parts) < 3 || len(parts)%2 != 1 {
			return nil, true
		}
		for i := 1; i < len(parts); i += 2 {
			n, ok := index(parts[i])
			if !ok {
				return nil, true
			}
			if !color(4, n, parts[i+1]) {
				return nil, true
			}
		}
	case 10, 11, 12:
		if len(parts) < 2 || len(parts) > 14-command {
			return nil, true
		}
		for i, spec := range parts[1:] {
			if !color(command+i, -1, spec) {
				return nil, true
			}
		}
	case 104:
		if len(parts) == 1 {
			updates = append(updates, colorUpdate{104, -1, ""})
			break
		}
		for _, p := range parts[1:] {
			n, ok := index(p)
			if !ok {
				return nil, true
			}
			updates = append(updates, colorUpdate{104, n, ""})
		}
	case 110, 111, 112:
		if len(parts) != 1 {
			return nil, true
		}
		updates = append(updates, colorUpdate{command, -1, ""})
	}
	return updates, false
}

// ColorState retains only normalized guest RGB overrides. Keeping it outside
// vt prevents its themed SGR lookup from baking mutable palette entries into
// fixed RGB cells; xterm keeps those cells indexed when a palette later changes.
type ColorState struct {
	Palette        [256]string
	Fg, Bg, Cursor string
}
type colorUpdate struct {
	command, index int
	color          string
}

func (s *ColorState) apply(u colorUpdate) {
	switch u.command {
	case 4:
		s.Palette[u.index] = u.color
	case 104:
		if u.index < 0 {
			s.Palette = [256]string{}
		} else {
			s.Palette[u.index] = ""
		}
	case 10:
		s.Fg = u.color
	case 11:
		s.Bg = u.color
	case 12:
		s.Cursor = u.color
	case 110:
		s.Fg = ""
	case 111:
		s.Bg = ""
	case 112:
		s.Cursor = ""
	}
}
func (f *Filter) Colors() ColorState { return f.colors }

// Match the pinned frontend's XParseColor profile. rgb: scales and rounds its
// components, while #RGB shifts nibbles without repeating them (so #fff is
// #f0f0f0, whereas rgb:f/f/f is #ffffff). Named colors are outside this profile.
func xtermColor(spec string) (string, bool) {
	spec = strings.ToLower(spec)
	var parts []string
	scaled := strings.HasPrefix(spec, "rgb:")
	if scaled {
		parts = strings.Split(spec[4:], "/")
	} else if strings.HasPrefix(spec, "#") && len(spec) > 1 && (len(spec)-1)%3 == 0 {
		n := (len(spec) - 1) / 3
		parts = []string{spec[1 : 1+n], spec[1+n : 1+2*n], spec[1+2*n:]}
	}
	if len(parts) != 3 {
		return "", false
	}
	n := len(parts[0])
	if n < 1 || n > 4 {
		return "", false
	}
	values := [3]uint64{}
	for i, p := range parts {
		if len(p) != n || strings.Trim(p, "0123456789abcdef") != "" {
			return "", false
		}
		value, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return "", false
		}
		if scaled {
			base := uint64(1)<<(4*n) - 1
			value = (value*255 + base/2) / base
		} else if n == 1 {
			value <<= 4
		} else {
			value >>= 4 * (n - 2)
		}
		values[i] = value
	}
	return fmt.Sprintf("#%02x%02x%02x", values[0], values[1], values[2]), true
}
