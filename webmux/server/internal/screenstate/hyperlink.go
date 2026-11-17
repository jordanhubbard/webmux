package screenstate

import (
	"crypto/rand"
	"strconv"
	"strings"
	"unicode"

	uv "github.com/charmbracelet/ultraviolet"
	"tuios.dev/vt"
)

// Each anonymous opening is a separate xterm link even when its URI equals the
// preceding link. Give it an unpredictable, screen-local ID so repainting and
// restoring the active pen can join its cells across rows and later live output.
// Explicit IDs stay unchanged, including when live output reuses them later.
func installXtermHyperlinks(e *vt.Emulator) {
	namespace := "webmux-" + rand.Text() + "-"
	var serial uint64
	e.RegisterOscHandler(8, func(data []byte) bool {
		parts := strings.SplitN(string(data), ";", 3)
		if len(parts) != 3 {
			return true
		}
		params := strings.TrimFunc(parts[1], func(r rune) bool { return unicode.IsSpace(r) || r == 0xfeff })
		uri := parts[2]
		pen, _ := e.CursorPen()
		if uri == "" {
			// xterm ignores an empty URI with nonempty parameters.
			if params == "" {
				e.RestoreCursorPen(pen, uv.Link{})
			}
			return true
		}
		id := ""
		for _, part := range strings.Split(params, ":") {
			if strings.HasPrefix(part, "id=") {
				id = strings.TrimPrefix(part, "id=")
				break
			}
		}
		if id == "" {
			serial++
			id = namespace + strconv.FormatUint(serial, 10)
		}
		e.RestoreCursorPen(pen, uv.Link{URL: uri, Params: "id=" + id})
		return true
	})
}
