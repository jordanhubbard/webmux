package vt_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"tuios.dev/vt"
	"tuios.dev/vt/snapshot"
)

type underlineReferenceStyle struct {
	Flags     uint8  `json:"flags"`
	Visible   uint8  `json:"vu"`
	Underline uint8  `json:"u"`
	Attrs     uint8  `json:"a"`
	Fg        string `json:"fg"`
	Bg        string `json:"bg"`
	Color     string `json:"uc"`
	Link      string `json:"l"`
}
type underlineReferenceCell struct {
	Content string                  `json:"c"`
	Width   int                     `json:"w"`
	Style   underlineReferenceStyle `json:"s"`
}
type underlineReferenceState struct {
	Cursor [2]int                     `json:"cursor"`
	Alt    bool                       `json:"alt"`
	Pen    underlineReferenceStyle    `json:"pen"`
	Cells  [][]underlineReferenceCell `json:"cells"`
}

func underlineModelState(e *vt.Emulator) underlineReferenceState {
	state := snapshot.Capture(e, e.Width(), e.Height(), -1, 0)
	style := func(s snapshot.StyleState) underlineReferenceStyle {
		flags := s.UnderlineFlags
		if flags == 0 {
			flags = 1
			if s.Underline != 0 {
				flags++
			}
			if s.Underline != 0 || s.LinkURL != "" {
				flags += 2
			}
		}
		visible := uint8(0)
		if flags == 2 || flags == 4 {
			if flags < 3 {
				visible = 1
			} else if s.LinkURL != "" {
				visible = 5
			} else {
				visible = s.Underline
			}
		}
		return underlineReferenceStyle{Flags: flags, Visible: visible, Underline: s.Underline, Attrs: s.Attrs, Fg: s.FgColor, Bg: s.BgColor, Color: s.UlColor, Link: s.LinkURL}
	}
	result := underlineReferenceState{Cursor: [2]int{state.CursorX, state.CursorY}, Alt: state.IsAltScreen, Pen: style(*state.Pen), Cells: make([][]underlineReferenceCell, e.Height())}
	if state.PendingWrap && result.Cursor[0] == e.Width()-1 {
		result.Cursor[0]++
	}
	for y, row := range state.Screen {
		result.Cells[y] = make([]underlineReferenceCell, e.Width())
		for x, cell := range row {
			result.Cells[y][x] = underlineReferenceCell{cell.Content, cell.Width, style(cell.StyleState)}
		}
	}
	return result
}

func TestXtermUnderlineMatchesIndependentReference(t *testing.T) {
	data, err := os.ReadFile("testdata/webmux-xterm-underline.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Data, Future string
		Resizes            [][2]int
		Initial, Continued underlineReferenceState
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 126 {
		t.Fatalf("reference case count changed: %d", len(cases))
	}
	for _, tc := range cases {
		for _, split := range []bool{false, true} {
			name := tc.Name
			if split {
				name += "/byte-split"
			}
			t.Run(name, func(t *testing.T) {
				e := vt.NewEmulator(8, 4)
				defer e.Close()
				e.EnableXtermNoWrap()
				e.EnableXtermUnderline()
				feed := func(text string) {
					if split {
						for _, b := range []byte(text) {
							e.Write([]byte{b})
						}
					} else {
						e.WriteString(text)
					}
				}
				feed(tc.Data)
				for _, size := range tc.Resizes {
					e.Resize(size[0], size[1])
				}
				if actual := underlineModelState(e); !reflect.DeepEqual(actual, tc.Initial) {
					a, _ := json.Marshal(actual)
					b, _ := json.Marshal(tc.Initial)
					t.Fatalf("initial model differs from unmodified renderer\nactual %s\nreference %s", a, b)
				}
				feed(tc.Future)
				if actual := underlineModelState(e); !reflect.DeepEqual(actual, tc.Continued) {
					a, _ := json.Marshal(actual)
					b, _ := json.Marshal(tc.Continued)
					t.Fatalf("continued model differs from unmodified renderer\nactual %s\nreference %s", a, b)
				}
			})
		}
	}
}
