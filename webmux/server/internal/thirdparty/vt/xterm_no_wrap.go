package vt

import uv "github.com/charmbracelet/ultraviolet"

// EnableXtermNoWrap preserves xterm's logical cursor one column past the right
// edge after a print with DECAWM disabled. Call before the first Write. This is
// an opt-in WebMux extension; NewEmulator's upstream behavior is unchanged.
func (e *Emulator) EnableXtermNoWrap() { e.xtermNoWrap = true }

func (e *Emulator) setPrintedCursor(x, y int) {
	if !e.xtermNoWrap || e.autoWrapMode() || x < e.scr.Width() {
		e.scr.setCursor(x, y, false)
		return
	}
	old := e.scr.cur.Position
	e.scr.cur.Position = uv.Pos(e.scr.Width(), min(max(y, 0), e.scr.Height()-1))
	if e.cb.CursorPosition != nil && old != e.scr.cur.Position {
		e.cb.CursorPosition(old, e.scr.cur.Position)
	}
}

func (e *Emulator) restrictXtermCursor() {
	if e.xtermNoWrap && e.scr.cur.X >= e.scr.Width() {
		e.scr.setCursor(e.scr.Width()-1, e.scr.cur.Y, false)
	}
}
