package screenstate

import (
	"github.com/charmbracelet/x/ansi"
	"tuios.dev/vt"
)

// installXtermCursorProfile preserves the frontend's origin mode on cursor
// restore and clamps the saved position without re-arming its pending wrap.
// The dependency's generic VT restore reinstates both saved values instead.
func installXtermCursorProfile(e *vt.Emulator) {
	origin := e.GetModes()[6]
	restore := func() {
		saved := e.SavedCursor(false)
		e.RestoreModes(map[int]bool{6: origin})
		low, high := 0, e.Height()-1
		if origin {
			region := e.ScrollRegion()
			low, high = region.Min.Y, region.Max.Y-1
		}
		// Keep the saved coordinates intact: a later restore after widening the
		// margins must still use the original position, not this clamped one.
		e.RestoreCursorPosition(min(max(saved.X, 0), e.Width()-1), min(max(saved.Y, low), high))
		e.RestoreCursorPendingWrap(false)
		e.RestoreCursorPen(saved.Pen, saved.Link)
		e.RestoreCursorProtected(saved.Protected)
		e.RestoreCharsets(saved.Charsets, saved.GL, saved.GR)
	}
	e.RegisterEscHandler('8', func() bool { restore(); return true })
	e.RegisterCsiHandler('u', func(ansi.Params) bool { restore(); return true })
	previous := e.GetCallbacks()
	callbacks := previous
	callbacks.EnableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeOrigin {
			origin = true
		}
		if previous.EnableMode != nil {
			previous.EnableMode(mode)
		}
	}
	callbacks.DisableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeOrigin {
			origin = false
		}
		// These callbacks run after each individual mode operation. This keeps
		// batches such as ?6;1048l in order and selects the main buffer before
		// correcting a 1049 restore, without recursively feeding the parser.
		if mode == ansi.ModeSaveCursor || mode == ansi.ModeAltScreenSaveCursor {
			restore()
		}
		if previous.DisableMode != nil {
			previous.DisableMode(mode)
		}
	}
	e.SetCallbacks(callbacks)
	e.RegisterEscHandler('c', func() bool { origin = false; return false })
	e.RegisterCsiHandler(ansi.Command(0, '!', 'p'), func(ansi.Params) bool { origin = false; return false })
}
