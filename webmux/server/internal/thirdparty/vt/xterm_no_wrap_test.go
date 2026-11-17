package vt

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestWebMuxXtermNoWrapReference(t *testing.T) {
	data, err := os.ReadFile("testdata/webmux-xterm-nowrap.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Data string
		Cursor     [2]int
		Lines      []string
		Cells      [][]string
		Widths     []int
	}
	if err := json.Unmarshal(data, &cases); err != nil || len(cases) != 138 {
		t.Fatal("missing independent xterm reference", err)
	}
	for _, tc := range cases {
		for _, split := range []bool{false, true} {
			name := tc.Name
			if split {
				name += "-byte-split"
			}
			t.Run(name, func(t *testing.T) {
				e := NewEmulator(8, 4)
				e.EnableXtermNoWrap()
				if split {
					for _, b := range []byte(tc.Data) {
						_, _ = e.Write([]byte{b})
					}
				} else {
					_, _ = e.Write([]byte(tc.Data))
				}
				position := e.CursorPosition()
				if e.CursorPendingWrap() && position.X == e.Width()-1 {
					position.X++
				}
				if got := [2]int{position.X, position.Y}; got != tc.Cursor {
					t.Errorf("cursor got %v, xterm %v", got, tc.Cursor)
				}
				var lines []string
				var widths []int
				for y := range 4 {
					var line strings.Builder
					var contents []string
					skipUntil := 0
					for x := range 8 {
						cell := e.CellAt(x, y)
						content := " "
						width := 1
						if cell == nil {
							if x >= skipUntil {
								line.WriteByte(' ')
							}
						} else {
							width = cell.Width
							if cell.Content != "" {
								content = cell.Content
							}
							if cell.Content == "" && width == 1 {
								if x >= skipUntil {
									line.WriteByte(' ')
								}
							} else {
								if x >= skipUntil {
									line.WriteString(cell.Content)
								}
							}
						}
						contents = append(contents, content)
						skipUntil = max(skipUntil, x+width)
						if y == 1 {
							widths = append(widths, width)
						}
					}
					if !slices.Equal(contents, tc.Cells[y]) {
						t.Errorf("row %d cells got %q; xterm %q", y, contents, tc.Cells[y])
					}
					lines = append(lines, line.String())
				}
				if !slices.Equal(lines, tc.Lines) || !slices.Equal(widths, tc.Widths) {
					t.Errorf("grid got %q widths %v; xterm %q widths %v", lines, widths, tc.Lines, tc.Widths)
				}
			})
		}
	}
}

func TestWebMuxNoWrapExtensionIsOptIn(t *testing.T) {
	e := NewEmulator(8, 4)
	_, _ = e.Write([]byte("\x1b[?7l\x1b[2;8HX"))
	if e.CursorPosition().X != 7 {
		t.Fatal("changed upstream default cursor behavior")
	}
}

func TestWebMuxXtermNoWrapRepeatReadsLiteralSourceCell(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		wantX        int
		last         string
	}{
		{"null", "", 7, "X"},
		{"space", "\x1b[2;6H ", 8, " "},
		{"text", "\x1b[2;6HV", 8, "V"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(8, 4)
			e.EnableXtermNoWrap()
			_, _ = e.Write([]byte("\x1b[?7l" + tc.prefix + "\x1b[2;8HX日\x1b[2b"))
			if got := e.CursorPosition().X; got != tc.wantX {
				t.Fatalf("cursor %d, want %d", got, tc.wantX)
			}
			if got := e.CellAt(7, 1).Content; got != tc.last {
				t.Fatalf("last cell %q, want %q", got, tc.last)
			}
		})
	}
}
