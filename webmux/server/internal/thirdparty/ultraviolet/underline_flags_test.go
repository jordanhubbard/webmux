package uv

import (
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestRetainedUnderlineFlagsAreValuesAndHaveBoundedCodes(t *testing.T) {
	original := Cell{Content: "X", Width: 1, Style: Style{Underline: UnderlineCurly, UnderlineFlags: NewUnderlineFlags(false, false)}}
	copied := original.Clone()
	original.Style.UnderlineFlags = NewUnderlineFlags(true, true)
	if copied.Style.UnderlineFlags.Enabled() || copied.Style.UnderlineFlags.Extended() || copied.Equal(&original) {
		t.Fatal("cell copy lost independent flags")
	}
	for code := 0; code < 256; code++ {
		f, ok := UnderlineFlagsFromCode(uint8(code))
		if ok != (code <= 4) {
			t.Fatalf("wire code %d acceptance=%v", code, ok)
		}
		if ok && f.Code() != uint8(code) {
			t.Fatalf("wire code %d changed", code)
		}
	}
}

func TestRetainedUnderlineProjectsVisibleRenditionAndLink(t *testing.T) {
	for _, tc := range []struct {
		name              string
		enabled, extended bool
		style             Underline
		want              Underline
		link              bool
	}{
		{"hidden", false, false, UnderlineCurly, UnderlineNone, false},
		{"extended-but-disabled", false, true, UnderlineDashed, UnderlineNone, true},
		{"single-with-hidden-object", true, false, UnderlineCurly, UnderlineSingle, false},
		{"contradictory-zero-style", true, true, UnderlineNone, UnderlineNone, true},
		{"visible-curly", true, true, UnderlineCurly, UnderlineCurly, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Style{Underline: tc.style, UnderlineColor: ansi.Red, UnderlineFlags: NewUnderlineFlags(tc.enabled, tc.extended)}
			visible := s.VisibleStyle()
			if visible.Underline != tc.want || visible.UnderlineFlags.IsSet() {
				t.Fatal("wrong visible underline", visible)
			}
			if (visible.UnderlineColor != nil) != tc.extended {
				t.Fatal("hidden color leaked")
			}
			c := Cell{Content: "X", Width: 1, Style: s, Link: Link{URL: "https://example.test/a"}}
			if (c.VisibleLink().URL != "") != tc.link {
				t.Fatal("wrong visible link")
			}
			if s.String() != visible.String() || StyleDiff(&Style{}, &s) != StyleDiff(&Style{}, &visible) {
				t.Fatal("generic rendering used hidden state")
			}
			if tc.want == UnderlineNone && strings.Contains(s.String(), "4:3") {
				t.Fatal("hidden curly style rendered")
			}
		})
	}
	ordinary := Style{Underline: UnderlineCurly, UnderlineColor: ansi.Red}
	if ordinary.VisibleStyle() != ordinary {
		t.Fatal("ordinary style changed")
	}
}

func TestUnderlineFlagJSONPreservesOrdinaryStyles(t *testing.T) {
	data, err := json.Marshal(Style{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "UnderlineFlags") {
		t.Fatal("ordinary style JSON changed", string(data))
	}
	for code := uint8(0); code <= 4; code++ {
		flags, _ := UnderlineFlagsFromCode(code)
		data, err := json.Marshal(flags)
		if err != nil {
			t.Fatal(err)
		}
		var decoded UnderlineFlags
		if err := json.Unmarshal(data, &decoded); err != nil || decoded != flags {
			t.Fatal("flag JSON changed", err)
		}
	}
	for _, data := range []string{"5", "-1", "null", "{}", "256", "1.5"} {
		var flags UnderlineFlags
		if err := json.Unmarshal([]byte(data), &flags); err == nil {
			t.Fatal("invalid flag JSON accepted", data)
		}
	}
}
