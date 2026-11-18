package screenstate

import (
	"crypto/rand"

	"tuios.dev/vt/snapshot"
)

func wireUnderlineFlags(s snapshot.StyleState) uint8 {
	if s.UnderlineFlags != 0 {
		return s.UnderlineFlags
	}
	flags := uint8(1)
	if s.Underline != 0 {
		flags++
	}
	if s.Underline != 0 || s.LinkURL != "" {
		flags += 2
	}
	return flags
}

// Explicit flags are required only where the raw style/link no longer derives
// the renderer's foreground/background words. Legacy ANSI must refuse them.
func hasIndependentUnderline(st *snapshot.State) bool {
	for _, pen := range []*snapshot.StyleState{st.Pen, savedPen(st.SavedCursor), savedPen(st.MainSavedCursor)} {
		if pen != nil && pen.UnderlineFlags != 0 {
			return true
		}
	}
	for _, grid := range [][][]snapshot.CellState{st.Screen, st.MainScreen, st.Scrollback} {
		for _, row := range grid {
			for _, cell := range row {
				if cell.UnderlineFlags != 0 {
					return true
				}
			}
		}
	}
	return false
}

func savedPen(c *snapshot.SavedCursorState) *snapshot.StyleState {
	if c == nil {
		return nil
	}
	return c.Pen
}

// This scratch save is legal only while painting, before the true saved slot
// is installed. The final active pen uses plainPen plus one negotiated tail.
func (w *vtWriter) independentPen(s snapshot.StyleState) {
	flags := wireUnderlineFlags(s)
	enabled, extended := (flags-1)&1 != 0, (flags-1)&2 != 0
	seed := s
	seed.UnderlineFlags, seed.Underline, seed.UlColor = 0, 0, ""
	seed.LinkURL, seed.LinkParams = "", ""
	if enabled {
		seed.Underline = 1
	}
	if !enabled && extended {
		// Never print with this transient link. Reuse one image-local ID
		// across rows so repeated pen changes retain at most one entry.
		seed.LinkURL = "https://webmux.invalid/snapshot-pen"
		if w.scratchParams == "" {
			w.scratchParams = "id=webmux-snapshot-" + rand.Text()
		}
		seed.LinkParams = w.scratchParams
	}
	w.plainPen(seed)
	if enabled && !extended {
		w.b.WriteString("\x1b7\x1b[0m\x1b8\x1b[59m")
	}
	w.b.WriteString("\x1b7")
	w.plainPen(s)
	w.b.WriteString("\x1b8")
	w.cur, w.set = s, true
}

func rendererColumn(x, width int, pending bool) int {
	if pending && x < width {
		x++
	}
	return x
}

func underlineTail(st *snapshot.State, repeat bool) (string, string, error) {
	x := rendererColumn(st.CursorX, st.Width, st.PendingWrap)
	state := underlineRendererState{Underline: 1, X: &x}
	if st.Pen != nil {
		state.Underline = wireUnderlineFlags(*st.Pen)
	}
	if repeat {
		state.Repeat = st.LastPrinted
	}
	if c := st.SavedCursor; c != nil {
		x := rendererColumn(c.X, st.Width, c.PendingWrap)
		if st.IsAltScreen {
			state.AltX = &x
		} else {
			state.NormalX = &x
		}
	}
	if c := st.MainSavedCursor; st.IsAltScreen && c != nil {
		x := rendererColumn(c.X, st.Width, c.PendingWrap)
		state.NormalX = &x
	}
	return underlineStatePacket(state)
}
