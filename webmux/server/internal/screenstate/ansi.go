// ANSI painting adapted from tuios.dev/vt v0.3.2 snapshot/replay.go.
// Local changes restore state used by subsequent output.
//
// MIT License
//
// Copyright (c) 2025 Gaurav Gosain
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package screenstate

import (
	"bytes"
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"

	"tuios.dev/vt"
	"tuios.dev/vt/snapshot"
)

// replayScreen paints a captured state on a fresh xterm-compatible viewer.
// It restores display/input modes and hidden state used by later output:
// margins, saved cursor/pen/charsets, pending wraps and protected cells.
// The owner supplies only complete filtered states; private strings and mirror
// replies are never emitted. REP state and filtered OSC display features still
// need separate qualification. The capture is not modified.
func replayScreen(st *snapshot.State, main *underlay, repeat bool) ([]byte, error) {
	if st == nil {
		return nil, ErrIncomplete
	}
	cp := *st
	if err := cp.Unpack(); err != nil {
		return nil, err
	}
	return replay(&cp, main, repeat)
}

// replay is Replay for a state that is not packed.
func replay(st *snapshot.State, under *underlay, repeat bool) ([]byte, error) {
	if st == nil {
		return nil, ErrIncomplete
	}
	var b bytes.Buffer
	b.Grow(st.Width * (st.Height + len(st.Scrollback)) * 2)
	w := &vtWriter{b: &b, width: st.Width}

	// Autowrap on and the pen reset, whatever the client was left at.
	b.WriteString("\x1b[0m\x1b[?7h\x1b[H")

	// The rows from the top: history, then the screen the history belongs
	// to. Under an alternate screen that is the main screen, whose wrap flags
	// the state does not carry.
	main, mainWraps := st.Screen, wrapFlags(st.ScreenWraps, len(st.Screen))
	if st.IsAltScreen && st.MainScreen != nil {
		main, mainWraps = st.MainScreen, make([]bool, len(st.MainScreen))
		if under != nil {
			mainWraps = under.wraps
		}
	}
	rows := make([][]snapshot.CellState, 0, len(st.Scrollback)+len(main))
	rows = append(rows, st.Scrollback...)
	rows = append(rows, main...)
	wraps := append(wrapFlags(st.ScrollbackWraps, len(st.Scrollback)), mainWraps...)
	mainProtected := st.Protected
	if st.IsAltScreen {
		mainProtected = st.MainProtected
	}
	mainProtection := protectionRows(st.Width, len(main), mainProtected)
	paintProtection := make([][]bool, len(st.Scrollback))
	paintProtection = append(paintProtection, mainProtection...)
	w.rows(rows, wraps, false, paintProtection)

	if st.IsAltScreen {
		// 1049 saves the cursor on the main screen, which is where leaving
		// the program puts it back.
		mainTop, mainLeft := 0, 0
		if under != nil {
			mainTop, mainLeft = w.margins(under.margins, st.Modes[69])
		}
		if c := st.MainSavedCursor; c != nil {
			w.savedCursor(c, main, mainProtection, mainTop, mainLeft)
			if !st.Modes[1049] {
				b.WriteString("\x1b7")
			}
		}
		w.kittyStack(st.KittyKbdMainStack)
		// Entered the way the guest entered it, so leaving it later does
		// what the guest expects.
		alt := "1049"
		switch {
		case st.Modes[1047]:
			alt = "1047"
		case st.Modes[47]:
			alt = "47"
		}
		b.WriteString("\x1b[?")
		b.WriteString(alt)
		b.WriteString("h\x1b[?6l\x1b[?69l\x1b[r\x1b[0m")
		charsets(&b, []int{'B', 'B', 'B', 'B', 0, 0})
		w.cur, w.set = snapshot.StyleState{}, false
		w.protect(false)
		w.rows(st.Screen, wrapFlags(st.ScreenWraps, len(st.Screen)), true, protectionRows(st.Width, st.Height, st.Protected))
	}

	if len(st.TabStops) > 0 {
		// HTS sets a stop at the cursor, so the table goes before the
		// cursor is placed.
		b.WriteString("\x1b[3g")
		for _, x := range tabStopCols(st.TabStops) {
			if x < st.Width {
				w.cup(x, 0)
				b.WriteString("\x1bH")
			}
		}
	}

	top, left := w.margins(st.Margins, st.Modes[69])

	activeProtection := protectionRows(st.Width, st.Height, st.Protected)
	if st.SavedCursor != nil {
		w.savedCursor(st.SavedCursor, st.Screen, activeProtection, top, left)
		b.WriteString("\x1b7")
	}
	w.mode(6, st.Modes[6])

	// Origin mode homes the cursor too, and from here on a cursor position
	// counts from the top margin.
	cx, cy := st.CursorX, st.CursorY
	if st.Modes[6] {
		cy -= top
		cx -= left
	}

	// The cursor, with its pending wrap: printing the cell under it again
	// leaves the cursor where it is with the wrap armed, which no cursor
	// movement can do.
	w.cup(cx, cy)
	if st.PendingWrap {
		w.reprint(st.Screen, activeProtection, st.CursorX, st.CursorY, cx, cy)
		w.resetPen()
	}

	charsets(&b, st.Charsets)
	decModes(&b, st.Modes)
	// The active screen's stack. Under the alternate screen the main
	// screen's was pushed before the switch.
	w.kittyStack(st.KittyKbdStack)
	if st.ModifyOtherKeysKnown && st.ModifyOtherKeys > 0 {
		b.WriteString("\x1b[>4;")
		b.WriteString(strconv.Itoa(st.ModifyOtherKeys))
		b.WriteString("m")
	}
	if st.CursorShape > 0 {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(st.CursorShape))
		b.WriteString(" q")
	}
	w.protect(st.PenProtected)
	if st.Pen != nil {
		w.pen(*st.Pen)
	}
	if st.TitlesKnown {
		b.WriteString("\x1b]2;")
		b.WriteString(st.Title)
		b.WriteString("\x1b\\")
	}
	// After everything printed above, which insert mode would push along
	// the row.
	for _, m := range []int{4, 20} {
		if on, ok := st.ANSIModes[m]; ok {
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(m))
			if on {
				b.WriteByte('h')
			} else {
				b.WriteByte('l')
			}
		}
	}
	if len(st.PendingInput) <= vt.MaxPendingInput {
		b.Write(st.PendingInput)
	}
	if repeat {
		// Xterm clears REP after every control. Reprint the last cluster only
		// after every mode/pen/cursor sequence, leaving no following control
		// which could clear it again. This also re-arms a pending wrap.
		x, y := st.CursorX-1, st.CursorY
		if st.PendingWrap {
			x = st.CursorX
		}
		if y < 0 || y >= len(st.Screen) || x < 0 || x >= len(st.Screen[y]) || st.LastPrinted == "" {
			return nil, ErrIncomplete
		}
		if st.Screen[y][x].Width == 0 && x > 0 {
			x--
		}
		addressX, addressY := x, y
		if st.Modes[6] {
			addressX -= left
			addressY -= top
		}
		w.cup(addressX, addressY)
		if st.ANSIModes[4] {
			// Delete the cell before re-inserting it so IRM does not shift
			// the captured row a second time.
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(max(st.Screen[y][x].Width, 1)))
			b.WriteByte('P')
		}
		b.WriteString(st.LastPrinted)
	}
	return b.Bytes(), nil
}

// vtWriter writes rows of cells, keeping track of the pen and the link it
// last set so a run of cells painted the same way costs one SGR.
type vtWriter struct {
	b         *bytes.Buffer
	width     int
	cur       snapshot.StyleState
	set       bool // cur was written; false means the pen is the default
	protected bool
}

// rows writes rows from the cursor down. absolute places each row with a
// cursor movement instead of a newline, for a screen that must not scroll.
func (w *vtWriter) rows(rows [][]snapshot.CellState, wraps []bool, absolute bool, protection [][]bool) {
	for i, row := range rows {
		if absolute {
			w.resetPen()
			w.cup(0, i)
		}
		// A wrapped row is printed through its last column and the next row
		// carries on from it. It counts as wrapped only when the next row
		// prints something, or the newline after that row would land on the
		// wrong line.
		wrapped := !absolute && i < len(wraps) && wraps[i] && i+1 < len(rows) && len(rows[i+1]) > 0
		var protected []bool
		if i < len(protection) {
			protected = protection[i]
		}
		w.row(row, wrapped, !absolute && i > 0 && i-1 < len(wraps) && wraps[i-1], protected)
		if absolute || i == len(rows)-1 || wrapped {
			continue
		}
		w.resetPen()
		w.b.WriteString("\r\n")
	}
	w.resetPen()
}

// row writes one row's cells. full prints every column, for a wrapped row;
// otherwise trailing blanks are left out. lead prints at least the first
// cell, so a row a wrapped row carries on into starts on the next line.
func (w *vtWriter) row(row []snapshot.CellState, full, lead bool, protection []bool) {
	n := min(len(row), w.width)
	end := n
	if !full {
		for end > 0 && blankCell(row[end-1]) && !(end-1 < len(protection) && protection[end-1]) {
			end--
		}
		if lead && end == 0 && n > 0 {
			end = 1
		}
	}
	for x := 0; x < end; x++ {
		c := row[x]
		if c.Width == 0 && c.Content == "" && x > 0 && row[x-1].Width > 1 {
			// The second column of a wide character.
			continue
		}
		if c.Width > 1 && x+c.Width > w.width {
			// A wide character that no longer fits the width. Print a blank
			// in its place rather than wrap it onto the next row.
			c = snapshot.CellState{Content: " ", Width: 1, StyleState: c.StyleState}
		}
		w.protect(x < len(protection) && protection[x])
		w.cell(c)
	}
}

// cell prints one cell with its pen.
func (w *vtWriter) cell(c snapshot.CellState) {
	if !w.set || c.StyleState != w.cur {
		w.pen(c.StyleState)
	}
	if c.Content == "" {
		w.b.WriteByte(' ')
		return
	}
	w.b.WriteString(c.Content)
}

// blankCell reports whether a cell prints nothing and paints nothing.
func blankCell(c snapshot.CellState) bool {
	return (c.Content == "" || c.Content == " ") && c.StyleState == (snapshot.StyleState{})
}

// cup moves the cursor to column x, row y, counted from zero.
func (w *vtWriter) cup(x, y int) {
	w.b.WriteString("\x1b[")
	w.b.WriteString(strconv.Itoa(y + 1))
	w.b.WriteString(";")
	w.b.WriteString(strconv.Itoa(x + 1))
	w.b.WriteString("H")
}

// resetPen puts the pen back to the default and closes an open link.
func (w *vtWriter) resetPen() {
	if !w.set {
		return
	}
	if w.cur.LinkURL != "" {
		w.b.WriteString("\x1b]8;;\x1b\\")
	}
	w.b.WriteString("\x1b[0m")
	w.cur, w.set = snapshot.StyleState{}, false
}

// pen sets the pen to s: one SGR from a reset, and the link when it changed.
func (w *vtWriter) pen(s snapshot.StyleState) {
	if s.LinkURL != w.cur.LinkURL || s.LinkParams != w.cur.LinkParams {
		w.b.WriteString("\x1b]8;")
		w.b.WriteString(s.LinkParams)
		w.b.WriteString(";")
		w.b.WriteString(s.LinkURL)
		w.b.WriteString("\x1b\\")
	}
	w.b.WriteString("\x1b[0")
	attrs := []struct {
		bit  uint8
		code string
	}{
		{uv.AttrBold, "1"}, {uv.AttrFaint, "2"}, {uv.AttrItalic, "3"},
		{uv.AttrBlink, "5"}, {uv.AttrRapidBlink, "6"}, {uv.AttrReverse, "7"},
		{uv.AttrConceal, "8"}, {uv.AttrStrikethrough, "9"},
	}
	for _, a := range attrs {
		if s.Attrs&a.bit != 0 {
			w.b.WriteString(";")
			w.b.WriteString(a.code)
		}
	}
	switch {
	case s.Underline == 1:
		w.b.WriteString(";4")
	case s.Underline > 1:
		w.b.WriteString(";4:")
		w.b.WriteString(strconv.Itoa(int(s.Underline)))
	}
	sgrColor(w.b, s.FgColor, 30, 90, "38")
	sgrColor(w.b, s.BgColor, 40, 100, "48")
	sgrColor(w.b, s.UlColor, -1, -1, "58")
	w.b.WriteByte('m')
	w.cur, w.set = s, true
}

// sgrColor appends one colour in the wire form colorToWire writes: a<n> for
// one of the 16 palette colours, i<n> for the 256-colour palette, #rrggbb
// for truecolour. base and bright are the SGR codes of palette colour 0 and
// 8; -1 means the colour has only the extended form, as the underline
// colour does.
func sgrColor(b *bytes.Buffer, wire string, base, bright int, ext string) {
	if len(wire) < 2 {
		return
	}
	switch wire[0] {
	case 'a', 'i':
		n, err := strconv.Atoi(wire[1:])
		if err != nil || n < 0 || n > 255 {
			return
		}
		switch {
		case wire[0] == 'a' && base >= 0 && n < 8:
			b.WriteString(";")
			b.WriteString(strconv.Itoa(base + n))
		case wire[0] == 'a' && bright >= 0 && n < 16:
			b.WriteString(";")
			b.WriteString(strconv.Itoa(bright + n - 8))
		default:
			b.WriteString(";")
			b.WriteString(ext)
			b.WriteString(";5;")
			b.WriteString(strconv.Itoa(n))
		}
	case '#':
		if len(wire) != 7 {
			return
		}
		b.WriteString(";")
		b.WriteString(ext)
		b.WriteString(";2")
		for i := range 3 {
			hi, lo := hexNibble(wire[1+2*i]), hexNibble(wire[2+2*i])
			if hi < 0 || lo < 0 {
				return
			}
			b.WriteString(";")
			b.WriteString(strconv.Itoa(hi<<4 | lo))
		}
	}
}

// kittyStack pushes the kitty keyboard flags, base entry first.
func (w *vtWriter) kittyStack(stack []int) {
	// The state's stack starts with the base entry, which every emulator
	// already has. It is set in place, and only the rest are pushed over it:
	// pushing it too left a fresh emulator with one entry more than the
	// guest's.
	for i, flags := range stack {
		if i == 0 {
			w.b.WriteString("\x1b[=")
			w.b.WriteString(strconv.Itoa(flags))
			w.b.WriteString(";1u")
			continue
		}
		w.b.WriteString("\x1b[>")
		w.b.WriteString(strconv.Itoa(flags))
		w.b.WriteString("u")
	}
}

// charsets designates G0 to G3 and shifts GL, in the layout of
// snapshot.State.Charsets. US ASCII in G0 shifted into GL is the default and
// writes nothing.
func charsets(b *bytes.Buffer, cs []int) {
	if len(cs) != 6 {
		return
	}
	intro := [4]string{"(", ")", "*", "+"}
	for i := range 4 {
		if c := cs[i]; c > ' ' && c < 0x7f {
			b.WriteString("\x1b")
			b.WriteString(intro[i])
			b.WriteString(string(rune(c)))
		}
	}
	switch cs[4] {
	case 0:
		b.WriteByte(0x0f)
	case 1:
		b.WriteByte(0x0e) // SO
	case 2:
		b.WriteString("\x1bn") // LS2
	case 3:
		b.WriteString("\x1bo") // LS3
	}
}

// snapshotModes are the DEC private modes a SNAP frame sets, in the order it
// sets them: what changes the bytes the client sends, and the two that
// change how the screen shows (autowrap and the cursor's visibility).
// Autowrap comes after the rows, which are painted with it on.
var snapshotModes = []int{
	1,    // DECCKM: application cursor keys
	7,    // DECAWM: autowrap
	25,   // DECTCEM: the cursor is shown
	9,    // X10 mouse
	1000, // mouse button tracking
	1002, // mouse button and drag tracking
	1003, // mouse any-motion tracking
	1004, // focus reports
	1005, // UTF-8 mouse encoding
	1006, // SGR mouse encoding
	1015, // urxvt mouse encoding
	1016, // SGR pixel mouse encoding
	1007, // alternate scroll
	2004, // bracketed paste
}

// decModes writes the snapshot's DEC modes. The keypad mode is mode 66 in the
// state and is written as DECKPAM or DECKPNM, which every emulator knows.
func decModes(b *bytes.Buffer, modes map[int]bool) {
	// Resets first, then sets: in ghostty the mouse tracking modes share one
	// state, so resetting 1002 after setting 1000 would turn mouse reports
	// off.
	for _, want := range []bool{false, true} {
		for _, m := range snapshotModes {
			on, ok := modes[m]
			if !ok || on != want {
				continue
			}
			b.WriteString("\x1b[?")
			b.WriteString(strconv.Itoa(m))
			if on {
				b.WriteByte('h')
			} else {
				b.WriteByte('l')
			}
		}
	}
	if on, ok := modes[66]; ok {
		if on {
			b.WriteString("\x1b=")
		} else {
			b.WriteString("\x1b>")
		}
	}
}

func tabStopCols(bits []byte) []int {
	cols := []int{}
	for i, b := range bits {
		for k := range 8 {
			if b&(1<<k) != 0 {
				cols = append(cols, i*8+k)
			}
		}
	}
	return cols
}

func wrapFlags(bits []byte, n int) []bool {
	out := make([]bool, max(n, 0))
	for i := range out {
		if i/8 < len(bits) {
			out[i] = bits[i/8]&(1<<(i%8)) != 0
		}
	}
	return out
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// protectionRows expands only protected rows; the dimensions are validated by Screen.
func protectionRows(width, height int, runs []int) [][]bool {
	rows := make([][]bool, height)
	for i := 0; i+2 < len(runs); i += 3 {
		y, x, n := runs[i], runs[i+1], runs[i+2]
		if y < 0 || y >= height || x < 0 || n < 1 || x >= width {
			continue
		}
		if rows[y] == nil {
			rows[y] = make([]bool, width)
		}
		for col := x; col < min(width, x+n); col++ {
			rows[y][col] = true
		}
	}
	return rows
}
func protectedAt(rows [][]bool, y, x int) bool {
	return y >= 0 && y < len(rows) && x >= 0 && x < len(rows[y]) && rows[y][x]
}
func (w *vtWriter) protect(on bool) {
	if w.protected == on {
		return
	}
	w.protected = on
	if on {
		w.b.WriteString("\x1b[1\"q")
	} else {
		w.b.WriteString("\x1b[0\"q")
	}
}
func (w *vtWriter) mode(number int, on bool) {
	w.b.WriteString("\x1b[?")
	w.b.WriteString(strconv.Itoa(number))
	if on {
		w.b.WriteByte('h')
	} else {
		w.b.WriteByte('l')
	}
}

// savedCursor sets the live cursor before DECSC or 1049 saves it. The caller
// restores the active cursor afterwards. Addressing follows the saved origin.
func (w *vtWriter) savedCursor(c *snapshot.SavedCursorState, grid [][]snapshot.CellState, protection [][]bool, top, left int) {
	w.mode(6, c.Origin)
	x, y := c.X, c.Y
	if c.Origin {
		x -= left
		y -= top
	}
	w.cup(max(x, 0), max(y, 0))
	if c.PendingWrap {
		w.reprint(grid, protection, c.X, c.Y, x, y)
	}

	if c.Pen != nil {
		w.pen(*c.Pen)
	} else {
		w.pen(snapshot.StyleState{})
	}
	w.protect(c.Protected)
	charsets(w.b, c.Charsets)
}

func (w *vtWriter) reprint(grid [][]snapshot.CellState, protection [][]bool, x, y, addressX, addressY int) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	col := x
	if grid[y][col].Width == 0 && col > 0 && grid[y][col-1].Width == 2 {
		col--
	}
	w.cup(max(addressX+col-x, 0), max(addressY, 0))
	charsets(w.b, []int{'B', 'B', 'B', 'B', 0, 0})
	w.protect(protectedAt(protection, y, col))
	w.cell(grid[y][col])
}

// margins precede saved/current cursor restoration because both operations
// address relative to them under origin mode. Full regions reset inherited state.
func (w *vtWriter) margins(m []int, horizontal bool) (top, left int) {
	w.mode(6, false)
	if len(m) == 4 && m[2] > 0 && m[3] > 0 {
		top = m[1]
		w.b.WriteString("\x1b[")
		w.b.WriteString(strconv.Itoa(top + 1))
		w.b.WriteByte(';')
		w.b.WriteString(strconv.Itoa(top + m[3]))
		w.b.WriteByte('r')
	} else {
		w.b.WriteString("\x1b[r")
	}
	w.mode(69, horizontal)
	if horizontal && len(m) == 4 && m[2] > 0 {
		left = m[0]
		w.b.WriteString("\x1b[")
		w.b.WriteString(strconv.Itoa(left + 1))
		w.b.WriteByte(';')
		w.b.WriteString(strconv.Itoa(left + m[2]))
		w.b.WriteByte('s')
	}
	return
}
