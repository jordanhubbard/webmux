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
		Widths     []int
	}
	if err := json.Unmarshal(data, &cases); err != nil || len(cases) != 19 {
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
					for x := range 8 {
						cell := e.CellAt(x, y)
						width := 1
						if cell == nil {
							line.WriteByte(' ')
						} else {
							width = cell.Width
							if cell.Content == "" && width == 1 {
								line.WriteByte(' ')
							} else {
								line.WriteString(cell.Content)
							}
						}
						if y == 1 {
							widths = append(widths, width)
						}
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
