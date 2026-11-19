package vt

import uv "github.com/charmbracelet/ultraviolet"

// EnableXtermUnderline preserves the independent underline and extended flag
// words used by xterm. Call before the first Write. This opt-in state requires
// a compatible snapshot encoder; ordinary ANSI rendering projects visible state.
func (e *Emulator) EnableXtermUnderline() { e.xtermUnderline = true }

// XtermUnderlineFlags returns explicit flags, deriving ordinary states when no
// extension is needed. Color alone does not enable xterm's extended flag.
func XtermUnderlineFlags(pen uv.Style, link uv.Link) uv.UnderlineFlags {
	if pen.UnderlineFlags.IsSet() {
		return pen.UnderlineFlags
	}
	return uv.NewUnderlineFlags(pen.Underline != uv.UnderlineNone, pen.Underline != uv.UnderlineNone || link.URL != "")
}

func setXtermUnderlineFlags(pen *uv.Style, link uv.Link, enabled, extended bool) {
	if enabled == (pen.Underline != uv.UnderlineNone) && extended == (pen.Underline != uv.UnderlineNone || link.URL != "") {
		pen.UnderlineFlags = uv.UnderlineFlags{}
	} else {
		pen.UnderlineFlags = uv.NewUnderlineFlags(enabled, extended)
	}
}

func (e *Emulator) updateXtermExtended(pen *uv.Style) {
	if e.xtermUnderline {
		enabled := XtermUnderlineFlags(*pen, e.scr.cur.Link).Enabled()
		setXtermUnderlineFlags(pen, e.scr.cur.Link, enabled, pen.Underline != uv.UnderlineNone || e.scr.cur.Link.URL != "")
	}
}

func (e *Emulator) updateXtermUnderline(pen *uv.Style) {
	if e.xtermUnderline {
		setXtermUnderlineFlags(pen, e.scr.cur.Link, pen.Underline != uv.UnderlineNone, pen.Underline != uv.UnderlineNone || e.scr.cur.Link.URL != "")
	}
}

// XtermRestoredPen restores the saved fg/bg flags but keeps the live extended
// style/color and link object, exactly as the locked renderer does on DECRC.
func XtermRestoredPen(saved uv.Style, savedLink uv.Link, live uv.Style, liveLink uv.Link) uv.Style {
	flags := XtermUnderlineFlags(saved, savedLink)
	saved.Underline, saved.UnderlineColor = live.Underline, live.UnderlineColor
	setXtermUnderlineFlags(&saved, liveLink, flags.Enabled(), flags.Extended())
	return saved
}

// SetXtermHyperlink applies an OSC8 link update without losing hidden underline
// flags. RestoreCursorPen is deliberately different: it restores exact state.
func (e *Emulator) SetXtermHyperlink(link uv.Link) {
	enabled := XtermUnderlineFlags(e.scr.cur.Pen, e.scr.cur.Link).Enabled()
	e.scr.cur.Link = link
	if e.xtermUnderline {
		setXtermUnderlineFlags(&e.scr.cur.Pen, link, enabled, e.scr.cur.Pen.Underline != uv.UnderlineNone || link.URL != "")
	}
}

// xtermPrintedCell mirrors BufferLine's storage boundary: without HAS_EXTENDED
// the cell carries no extended object. The active pen still retains that object.
func (e *Emulator) xtermPrintedCell(cell uv.Cell) uv.Cell {
	if !e.xtermUnderline {
		return cell
	}
	flags := XtermUnderlineFlags(cell.Style, cell.Link)
	if !flags.Extended() {
		cell.Style.Underline = uv.UnderlineNone
		cell.Style.UnderlineColor = nil
		cell.Link = uv.Link{}
		setXtermUnderlineFlags(&cell.Style, cell.Link, flags.Enabled(), false)
	}
	return cell
}
