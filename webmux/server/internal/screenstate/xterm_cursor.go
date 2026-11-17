package screenstate

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"tuios.dev/vt"
)

type xtermCursorProfile struct{ unsupported bool }

// installXtermCursorProfile preserves the frontend's origin mode on cursor
// restore and clamps the saved position without re-arming its pending wrap.
// The dependency's generic VT restore reinstates both saved values instead.
func installXtermCursorProfile(e *vt.Emulator) *xtermCursorProfile {
	profile := &xtermCursorProfile{}
	origin := e.GetModes()[6]
	autoWrap := e.GetModes()[7]
	// DEC mode callbacks run after the dependency has restored its full pen.
	// Capture the global xterm pen before processing the mode batch instead.
	var modePen uv.Style
	var modeLink uv.Link
	e.RegisterCsiHandler(ansi.Command('?', 0, 'l'), func(ansi.Params) bool {
		modePen, modeLink = e.CursorPen()
		return false
	})
	restore := func(live uv.Style, liveLink uv.Link) {
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
		pen := saved.Pen
		savedExtended := pen.Underline != 0 || pen.UnderlineColor != nil || saved.Link.URL != ""
		liveExtended := live.Underline != 0 || live.UnderlineColor != nil || liveLink.URL != ""
		if savedExtended {
			// xterm saves foreground/background flags, but leaves the current
			// extended underline object in place. Both must agree on whether an
			// underline is enabled for vt's single underline field to represent it.
			if (pen.Underline != 0) != (live.Underline != 0) {
				profile.unsupported = true
			}
			pen.Underline, pen.UnderlineColor = live.Underline, live.UnderlineColor
		} else if liveExtended {
			// xterm hides this extended object without clearing it. Later SGR can
			// expose it again; the dependency cannot retain that hidden state.
			profile.unsupported = true
		}
		e.RestoreCursorPen(pen, liveLink)
		modePen, modeLink = pen, liveLink
		e.RestoreCursorProtected(saved.Protected)
		e.RestoreCharsets(saved.Charsets, saved.GL, saved.GR)
	}
	restoreLive := func() { pen, link := e.CursorPen(); restore(pen, link) }
	e.RegisterEscHandler('8', func() bool { restoreLive(); return true })
	e.RegisterCsiHandler('u', func(ansi.Params) bool { restoreLive(); return true })
	previous := e.GetCallbacks()
	callbacks := previous
	callbacks.EnableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeAutoWrap {
			// With wrapping off, vt clamps an addressed final-column cursor
			// and one advanced past the final cell to the same coordinate.
			// Re-enabling wrapping cannot recover that lost distinction. A
			// retained pending wrap is representable; otherwise refuse until
			// RIS rather than silently overwriting the final cell on replay.
			if !autoWrap && e.CursorPosition().X == e.Width()-1 && !e.CursorPendingWrap() {
				profile.unsupported = true
			}
			autoWrap = true
		}
		if mode == ansi.ModeOrigin {
			origin = true
		}
		if previous.EnableMode != nil {
			previous.EnableMode(mode)
		}
	}
	callbacks.DisableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeAutoWrap {
			autoWrap = false
		}
		if mode == ansi.ModeOrigin {
			origin = false
		}
		// These callbacks run after each individual mode operation. This keeps
		// batches such as ?6;1048l in order and selects the main buffer before
		// correcting a 1049 restore, without recursively feeding the parser.
		if mode == ansi.ModeSaveCursor || mode == ansi.ModeAltScreenSaveCursor {
			restore(modePen, modeLink)
		}
		if previous.DisableMode != nil {
			previous.DisableMode(mode)
		}
	}
	e.SetCallbacks(callbacks)
	e.RegisterEscHandler('c', func() bool {
		origin, autoWrap = false, true
		modePen = uv.Style{}
		modeLink = uv.Link{}
		profile.unsupported = false
		return false
	})
	e.RegisterCsiHandler(ansi.Command(0, '!', 'p'), func(ansi.Params) bool { origin = false; return false })
	return profile
}
