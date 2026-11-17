// Package agentdeckattach validates the optional coordinator before attaching.
package agentdeckattach

import (
	"bytes"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

const Helper = "--agent-deck-attach-worker"
const Required = "1.16.39"

func Match(argv []string) bool {
	if len(argv) < 4 || filepath.Base(argv[0]) != "agent-deck" {
		return false
	}
	args := argv[1:]
	if len(args) >= 2 && args[0] == "-p" {
		args = args[2:]
	}
	return len(args) >= 3 && ((args[0] == "session" && args[1] == "attach") || (len(args) >= 4 && args[0] == "remote" && args[1] == "attach"))
}

func supported(output string) bool {
	fields := strings.Fields(output)
	if len(fields) < 3 || fields[0] != "Agent" || fields[1] != "Deck" || !strings.HasPrefix(fields[2], "v") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(fields[2], "v"), ".")
	if len(parts) != 3 {
		return false
	}
	var have [3]uint64
	for i, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return false
		}
		have[i] = n
	}
	need := [3]uint64{1, 16, 39}
	for i := range have {
		if have[i] != need[i] {
			return have[i] > need[i]
		}
	}
	return true
}

// A named buffer keeps io.Copy's optional fast paths from bypassing this limit.
type versionOutput struct{ buffer bytes.Buffer }

func (w *versionOutput) Write(p []byte) (int, error) {
	if len(p) > 256-w.buffer.Len() {
		return 0, errors.New("coordinator version output exceeds limit")
	}
	return w.buffer.Write(p)
}
