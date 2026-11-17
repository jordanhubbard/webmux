package vt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"tuios.dev/vt"
	"tuios.dev/vt/snapshot"
)

func TestXtermUnderlineStateAndImmutableSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, data        string
		style             uv.Underline
		enabled, extended bool
	}{
		{"hidden-curly", "A\x1b7\x1b[4:3;58;5;9mB\x1b8X", uv.UnderlineCurly, false, false},
		{"contradictory-reset", "\x1b[4:3mA\x1b7\x1b[0mB\x1b8X", uv.UnderlineNone, true, true},
		{"contradictory-disable", "\x1b[4:3mA\x1b7\x1b[24mB\x1b8X", uv.UnderlineNone, true, true},
		{"hidden-color-only", "A\x1b7\x1b[58;5;9mB\x1b8X", uv.UnderlineNone, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := vt.NewEmulator(12, 4)
			defer e.Close()
			e.EnableXtermNoWrap()
			e.EnableXtermUnderline()
			e.WriteString(tc.data)
			pen, link := e.CursorPen()
			flags := vt.XtermUnderlineFlags(pen, link)
			if pen.Underline != tc.style || flags.Enabled() != tc.enabled || flags.Extended() != tc.extended {
				t.Fatalf("wrong pen: %+v flags=%v", pen, flags)
			}
			c := e.CellAt(1, 0)
			wantCellStyle := tc.style
			if !tc.extended {
				wantCellStyle = uv.UnderlineNone
			}
			flags = vt.XtermUnderlineFlags(c.Style, c.Link)
			if c.Content != "X" || c.Style.Underline != wantCellStyle || flags.Enabled() != tc.enabled || flags.Extended() != tc.extended {
				t.Fatalf("printed cell lost raw flags: %+v", c)
			}
			image := snapshot.Capture(e, 12, 4, -1, 0)
			before, err := json.Marshal(image)
			if err != nil {
				t.Fatal(err)
			}
			var wire snapshot.State
			if err := json.Unmarshal(before, &wire); err != nil {
				t.Fatal(err)
			}
			copy := vt.NewEmulator(12, 4)
			defer copy.Close()
			copy.EnableXtermNoWrap()
			copy.EnableXtermUnderline()
			if err := snapshot.Apply(copy, &wire); err != nil {
				t.Fatal(err)
			}
			if !e.CellAt(1, 0).Equal(copy.CellAt(1, 0)) {
				t.Fatalf("wire roundtrip lost initial cell: source=%+v restored=%+v", e.CellAt(1, 0), copy.CellAt(1, 0))
			}
			restoredPen, restoredLink := copy.CursorPen()
			if !reflect.DeepEqual(pen, restoredPen) || link != restoredLink {
				t.Fatal("wire roundtrip lost pen")
			}
			for _, part := range []string{"\x1b[31m", "\x1b[59m", "Y\x1b[4:5mZ\x1b8Q"} {
				e.WriteString(part)
				copy.WriteString(part)
			}
			a := snapshot.Capture(e, 12, 4, -1, 0)
			b := snapshot.Capture(copy, 12, 4, -1, 0)
			if !reflect.DeepEqual(a, b) {
				t.Fatal("continued wire image diverged")
			}
			// Applying a received wire copy may pack it. The captured source image
			// must remain unchanged by either emulator's continued output.
			after, err := json.Marshal(image)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("captured image aliased later writes")
			}
		})
	}
}

func TestXtermUnderlineIsOptInAndSurvivesScreenSwitches(t *testing.T) {
	const data = "\x1b[4:3mA\x1b7\x1b[0mB\x1b8X"
	ordinary := vt.NewEmulator(12, 4)
	defer ordinary.Close()
	ordinary.WriteString(data)
	pen, _ := ordinary.CursorPen()
	if pen.Underline != uv.UnderlineCurly || pen.UnderlineFlags.IsSet() {
		t.Fatal("upstream profile changed")
	}
	for _, mode := range []string{"47", "1047", "1049"} {
		e := vt.NewEmulator(12, 4)
		e.EnableXtermUnderline()
		e.WriteString("\x1b[4:3mA\x1b[?" + mode + "h\x1b[4:5mB\x1b[?" + mode + "lX")
		pen, link := e.CursorPen()
		flags := vt.XtermUnderlineFlags(pen, link)
		if pen.Underline != uv.UnderlineDashed || !flags.Enabled() || !flags.Extended() {
			t.Fatalf("global extended pen lost across %s", mode)
		}
		e.Close()
	}
}

func TestUnknownUnderlineWireFlagsRefuseBeforeTerminalMutation(t *testing.T) {
	for _, where := range []string{"cell", "pen", "saved", "main-saved"} {
		t.Run(where, func(t *testing.T) {
			e := vt.NewEmulator(8, 4)
			defer e.Close()
			e.EnableXtermUnderline()
			e.WriteString("KEEP")
			before, _ := json.Marshal(snapshot.Capture(e, 8, 4, -1, 0))
			bad := snapshot.Capture(e, 8, 4, -1, 0)
			switch where {
			case "cell":
				bad.Screen[0][0].UnderlineFlags = 255
			case "pen":
				bad.Pen.UnderlineFlags = 255
			case "saved":
				bad.SavedCursor = &snapshot.SavedCursorState{Pen: &snapshot.StyleState{UnderlineFlags: 255}}
			case "main-saved":
				bad.MainSavedCursor = &snapshot.SavedCursorState{Pen: &snapshot.StyleState{UnderlineFlags: 255}}
			}
			if err := snapshot.Apply(e, bad); err == nil {
				t.Fatal("unknown flag code accepted")
			}
			after, _ := json.Marshal(snapshot.Capture(e, 8, 4, -1, 0))
			if string(before) != string(after) {
				t.Fatal("refusal mutated terminal")
			}
		})
	}
}
