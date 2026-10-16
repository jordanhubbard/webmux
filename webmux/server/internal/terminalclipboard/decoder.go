// Package terminalclipboard recognizes write-only OSC 52 requests. Requests are
// stripped before logging/replay; clipboard reads never receive a response.
package terminalclipboard

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 128 * 1024
const prefix = "\x1b]52;"
const maxPacket = MaxBytes*4/3 + 32

type Decoder struct {
	pending  string
	dropping bool
}

func terminator(s string) (int, int) {
	bel, st := strings.IndexByte(s, '\a'), strings.Index(s, "\x1b\\")
	if bel >= 0 && (st < 0 || bel < st) {
		return bel, 1
	}
	if st >= 0 {
		return st, 2
	}
	return -1, 0
}
func (d *Decoder) Feed(chunk string) (string, *string) {
	data := d.pending + chunk
	d.pending = ""
	var visible strings.Builder
	var clipboard *string
	for data != "" {
		if d.dropping {
			end, n := terminator(data)
			if end < 0 {
				if strings.HasSuffix(data, "\x1b") {
					d.pending = "\x1b"
				}
				break
			}
			d.dropping = false
			data = data[end+n:]
			continue
		}
		at := strings.Index(data, prefix)
		if at < 0 {
			keep := 0
			for n := min(len(data), len(prefix)-1); n > 0; n-- {
				if strings.HasSuffix(data, prefix[:n]) {
					keep = n
					break
				}
			}
			visible.WriteString(data[:len(data)-keep])
			d.pending = data[len(data)-keep:]
			break
		}
		visible.WriteString(data[:at])
		data = data[at+len(prefix):]
		end, n := terminator(data)
		if end < 0 {
			if len(data) > maxPacket {
				d.dropping = true
				if strings.HasSuffix(data, "\x1b") {
					d.pending = "\x1b"
				}
			} else {
				d.pending = prefix + data
			}
			break
		}
		if end <= maxPacket {
			selection, encoded, ok := strings.Cut(data[:end], ";")
			if ok && (selection == "" || selection == "c" || selection == "s" || selection == "p") && encoded != "?" {
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err == nil && len(decoded) <= MaxBytes && utf8.Valid(decoded) {
					text := string(decoded)
					clipboard = &text
				}
			}
		}
		data = data[end+n:]
	}
	return visible.String(), clipboard
}
