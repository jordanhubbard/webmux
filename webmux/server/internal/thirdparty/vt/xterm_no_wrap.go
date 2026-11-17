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
	e.restoreXtermCursor(x, y)
}

// restoreXtermCursor keeps an already valid logical position across operations
// such as SU, SD and DECSTR that preserve it, even after DECAWM is enabled.
func (e *Emulator) restoreXtermCursor(x, y int) {
	if !e.xtermNoWrap || x < e.scr.Width() {
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

// xterm.js keeps a wide lead at the right edge when a no-wrap single-cell
// print overwrites its trailing cell. Preserve that overlapping representation
// instead of the generic grid operation's wide-character repair.
func (e *Emulator) setXtermPrintedCell(x, y int, cell *uv.Cell) {
	var lead uv.Cell
	keep := false
	if e.xtermNoWrap && !e.autoWrapMode() && x == e.Width()-1 && x > 0 && cell.Width == 1 {
		if previous := e.scr.CellAt(x-1, y); previous != nil && previous.Width == 2 {
			lead, keep = *previous, true
		}
	}
	e.scr.SetCell(x, y, cell)
	if keep {
		e.scr.buf.rows[y][x-1] = lead
	}
}
