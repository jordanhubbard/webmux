package recovery

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxHistoryBytes = 64 << 10

// History reads a bounded suffix of the named session-owned transcript. Missing
// or rotated logs are a recoverable absence, not a reason to discard metadata.
// Raw control sequences are never replayed from disk into a new terminal.
func History(directory, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || !strings.HasPrefix(name, "session-") || !strings.HasSuffix(name, ".log") {
		return "", errors.New("invalid transcript reference")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("transcript is not a regular file")
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return "", errors.New("transcript changed while opening")
	}
	start := max(int64(0), opened.Size()-MaxHistoryBytes)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxHistoryBytes))
	if err != nil {
		return "", err
	}
	if start > 0 {
		// Start at a complete line rather than inside UTF-8 or an ANSI packet.
		if line := strings.IndexByte(string(data), '\n'); line >= 0 {
			data = data[line+1:]
		} else {
			return "", nil
		}
	}
	// Terminal redraws contain long runs of cleared rows. Compact those in the
	// plain preview, while retaining the original transcript file unchanged.
	text := strings.ReplaceAll(PlainText(string(data)), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var lines []string
	blank := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Trim(line, " \t") == "" {
			blank++
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		lines = append(lines, line)
	}
	preview := strings.Join(lines, "\n")
	if len(preview) > MaxHistoryBytes {
		start := len(preview) - MaxHistoryBytes
		for start < len(preview) && !utf8.RuneStart(preview[start]) {
			start++
		}
		preview = preview[start:]
	}
	return preview, nil
}

// PlainText removes CSI and all string controls, including clipboard, browser
// and shell-integration packets. It tolerates truncated UTF-8 and ANSI tails.
func PlainText(input string) string {
	var out strings.Builder
	state := byte(0)
	for len(input) > 0 {
		r, size := utf8.DecodeRuneInString(input)
		input = input[size:]
		switch state {
		case 0:
			if r == '\x1b' {
				state = 'e'
			} else if r == 0x9b {
				state = 'c'
			} else if r == 0x90 || r == 0x98 || r == 0x9d || r == 0x9e || r == 0x9f {
				state = 's'
			} else if r == '\n' || r == '\r' || r == '\t' || r >= 32 && r != 127 && !(r >= 0x80 && r <= 0x9f) {
				out.WriteRune(r)
			}
		case 'e':
			switch r {
			case '[':
				state = 'c'
			case ']', 'P', '^', '_', 'X':
				state = 's'
			default:
				if r >= 0x20 && r <= 0x2f {
					state = 'g'
				} else {
					state = 0
				}
			}
		case 'g':
			if r >= 0x30 && r <= 0x7e {
				state = 0
			} else if r == '\x1b' {
				state = 'e'
			}
		case 'c':
			if r >= 0x40 && r <= 0x7e {
				state = 0
			} else if r == '\x1b' {
				state = 'e'
			}
		case 's':
			if r == '\a' || r == 0x9c {
				state = 0
			} else if r == '\x1b' {
				state = 't'
			}
		case 't':
			if r == '\\' {
				state = 0
			} else if r != '\x1b' {
				state = 's'
			}
		}
	}
	return out.String()
}
