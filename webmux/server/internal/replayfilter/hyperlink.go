package replayfilter

import (
	"strconv"
	"strings"
)

// Match the bounded link representation retained by the VT history ring. Raw
// Unicode stays intact: snapshots are delivered to the UTF-8 xterm frontend,
// whose hyperlink IDs distinguish Unicode from literal percent-encoded IDs.
const MaxLinkURL = 2083
const MaxLinkParams = 256

func hyperlinkOSC(data []byte, overflow bool) (string, bool) {
	parts := strings.SplitN(string(data), ";", 3)
	command, err := strconv.Atoi(parts[0])
	if err != nil || command != 8 {
		return "", false
	}
	if overflow {
		return "", true
	}
	if len(parts) != 3 {
		return "", false
	} // xterm ignores malformed OSC 8.
	params, uri := parts[1], parts[2]
	if len(params) > MaxLinkParams || len(uri) > MaxLinkURL {
		return "", true
	}
	for _, part := range []string{params, uri} {
		for _, r := range part {
			if r < 0x20 || r >= 0x7f && r <= 0x9f {
				return "", true
			}
		}
	}
	return "\x1b]8;" + params + ";" + uri + "\x1b\\", false
}
