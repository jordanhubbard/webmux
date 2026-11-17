package replayfilter

import (
	"bytes"
	"strings"
	"testing"
)

func TestReplayFilterAcrossEveryReadBoundary(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"unicode", "α🦀日本\r\n", "α🦀日本\r\n"},
		{"drawing", "\x1b[?1049h\x1b[2J\x1b[H\x1b[31mTEXT\x1b[0m\x1b[?1049l", "\x1b[?1049h\x1b[2J\x1b[H\x1b[31mTEXT\x1b[0m\x1b[?1049l"},
		{"modes", "\x1b[?2004h\x1b[4h\x1b[?25l\x1b[2 q\x1b[!p", "\x1b[?2004h\x1b[4h\x1b[?25l\x1b[2 q\x1b[!p"},
		{"charset", "\x1b(0q\x1b(B\x1b7\x1b8\x1bM", "\x1b(0q\x1b(B\x1b7\x1b8\x1bM"},
		{"protected-cells", "\x1b[1\"qKEEP\x1b[0\"q\x1b[?2K", "\x1b[1\"qKEEP\x1b[0\"q\x1b[?2K"},
		{"invalid-protection", "A\x1b[3\"q\x1b[?1\"qB", "AB"},
		{"csi-with-c0", "Q\x1b[\n31mZ", "Q\n\x1b[31mZ"},
		{"csi-with-del", "Q\x1b[\x7f31mZ", "Q\x1b[31mZ"},
		{"requests", "A\x1b[6n\x1b[c\x1b[>c\x1b[?2004$p\x1b[18t\x1bZB", "AB"},
		{"osc", "A\x1b]52;c;clipboard\aB\x1b]777;webmux-browser;private\x1b\\C\x1b]777;webmux-recovery;private\aD", "ABCD"},
		{"strings", "A\x1bP$qcursor\x1b\\B\x1b_hidden\x1b\\C\x1b^private\x1b\\D\x1bXhidden\x1b\\E", "ABCDE"},
		{"string-nested-escape", "A\x1b]52;c;\x1b[?1049hsecret\aB", "AB"},
		{"c1", "A\u009d52;c;clipboard\u009cB\u0090$qquery\u009cC\u009b6nD\u009b31mE", "ABCD\x1b[31mE"},
		{"bells", "A\a\x05B", "AB"},
		{"cancel", "A\x1b[?1049\x18B\x1b]private\x1aC", "ABC"},
		{"oversized-csi", "A\x1b[" + strings.Repeat("1;", 100) + "mB", "AB"},
		{"unterminated", "A\x1b]" + strings.Repeat("secret", 10000), "A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Every split for bounded controls; the large unclosed string is
			// additionally tested byte by byte below without quadratic work.
			for split := 0; split <= min(len(tc.input), 300); split++ {
				var f Filter
				got := append(f.Feed([]byte(tc.input[:split])), f.Feed([]byte(tc.input[split:]))...)
				if string(got) != tc.want {
					t.Fatalf("split %d: %q, want %q", split, got, tc.want)
				}
			}
			var f Filter
			var got []byte
			for _, b := range []byte(tc.input) {
				got = append(got, f.Feed([]byte{b})...)
				if len(f.sequence) > 128 || len(f.partial) > 3 {
					t.Fatal("unbounded parser state")
				}
			}
			if string(got) != tc.want {
				t.Fatalf("single-byte reads: %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplayFilterCannotReleaseBufferedRequestAsText(t *testing.T) {
	var f Filter
	for range 1000 {
		if got := f.Feed([]byte("\x1b]777;webmux-recovery;")); len(got) != 0 {
			t.Fatal("released string payload")
		}
		if got := f.Feed(bytes.Repeat([]byte("private"), 1000)); len(got) != 0 {
			t.Fatal("released oversized string")
		}
	}
	if got := f.Feed([]byte("\aVISIBLE")); string(got) != "VISIBLE" {
		t.Fatalf("failed to recover after string: %q", got)
	}
}

func TestXtermProfilePreservesSupportedModesAcrossReadBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[?69hTEXT\x1b[?69l", "TEXT"},
		{"\x1b[?25;69;1004h", "\x1b[?25;1004h"},
		{"\x1b[?0069;2004h", "\x1b[?2004h"},
		{"\x1b[?69:1h", ""},
	} {
		for split := 0; split <= len(tc.input); split++ {
			f := Filter{Xterm: true}
			input := []byte(tc.input)
			original := bytes.Clone(input)
			got := append(f.Feed(input[:split]), f.Feed(input[split:])...)
			if string(got) != tc.want || !bytes.Equal(input, original) {
				t.Fatalf("profile split %d: %q wanted %q", split, got, tc.want)
			}
		}
	}
}

func TestXtermRepeatAcrossEveryReadBoundary(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"Q\x1b[3b", "Q\x1b[3b"},
		{"Q\x1b[\a3b", "Q"},
		{"Q\x1b[\x7f3b", "Q\x1b[3b"},
		{"日\x1b[3b", "日\x1b[3b"},
		{"Q\x1b[2J\x1b[3b", "Q\x1b[2J"},
		{"Q\x1b[31m\x1b[3b", "Q\x1b[31m"},
		{"Q\a\x1b[3b", "Q"},
		{"Q\x1b]777;private\a\x1b[3b", "Q"},
		{"Q\x1b[2b\x1b[3b", "Q\x1b[2b"},
		{"Q\x7f\x1b[3b", "Q\x1b[3b"},
		{"Q\x1b[?69h\x1b[3b", "Q"},
		{"Q\x1b[6n\x1b[3b", "Q"},
	} {
		for split := 0; split <= len(tc.input); split++ {
			f := Filter{Xterm: true}
			input := []byte(tc.input)
			original := bytes.Clone(input)
			got := append(f.Feed(input[:split]), f.Feed(input[split:])...)
			if string(got) != tc.want || !bytes.Equal(input, original) {
				t.Fatalf("split %d: %q wanted %q", split, got, tc.want)
			}
		}
	}
}

func TestXtermUnderlineColorReset(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[59m", "\x1b[58;2;255;255;255m"},
		{"\x1b[4:3;59;31m", "\x1b[4:3;58;2;255;255;255;31m"},
		{"\x1b[38;2;59;59;59;59m", "\x1b[38;2;59;59;59;58;2;255;255;255m"},
		{"\x1b[48;5;59m", "\x1b[48;5;59m"},
		{"\x1b[58;2;59;59;59m", "\x1b[58;2;59;59;59m"},
		{"\x1b[38:2::59:59:59;59m", "\x1b[38:2::59:59:59;58;2;255;255;255m"},
	} {
		for split := 0; split <= len(tc.input); split++ {
			f := Filter{Xterm: true}
			got := append(f.Feed([]byte(tc.input[:split])), f.Feed([]byte(tc.input[split:]))...)
			if string(got) != tc.want {
				t.Fatalf("split %d: %q, want %q", split, got, tc.want)
			}
			var plain Filter
			if got := string(plain.Feed([]byte(tc.input))); got != tc.input {
				t.Fatal("changed generic history profile", got)
			}
		}
	}
}

func TestXtermOSCColorsAcrossEveryReadBoundary(t *testing.T) {
	for _, input := range []string{
		"\x1b]4;1;#123456\a", "\x1b]4;1;#123456;2;#abcdef\x1b\\",
		"\x1b]4;1;?;2;#123456\a", "\x1b]10;?;#123456;?\a",
		"\x1b]104;1;2\a", "\x1b]110\a", "\x1b]52;c;#123456\a",
		"\x1b]777;webmux-browser;private\a",
	} {
		complete := Filter{Xterm: true}
		complete.Feed([]byte(input))
		for split := 0; split <= len(input); split++ {
			f := Filter{Xterm: true}
			got := append(f.Feed([]byte(input[:split])), f.Feed([]byte(input[split:]))...)
			if len(got) != 0 || f.UnsupportedDisplay() || !f.AtBoundary() || f.Colors() != complete.Colors() {
				t.Fatalf("split %d: %q", split, got)
			}
		}
	}
	f := Filter{Xterm: true}
	f.Feed([]byte("\x1b]4;1;#123456;2;#abcdef\a\x1b]10;#234567;#345678;#456789\a"))
	c := f.Colors()
	if c.Palette[1] != "#123456" || c.Palette[2] != "#abcdef" || c.Fg != "#234567" || c.Bg != "#345678" || c.Cursor != "#456789" {
		t.Fatal(c)
	}
	f.Feed([]byte("\x1b]104;1\a\x1b]111\a"))
	c = f.Colors()
	if c.Palette[1] != "" || c.Palette[2] != "#abcdef" || c.Bg != "" || c.Fg != "#234567" {
		t.Fatal(c)
	}
	f.Feed([]byte("\x1bc"))
	if f.Colors() != (ColorState{}) {
		t.Fatal("RIS retained color overrides")
	}
}

func TestUnsupportedOSCColorsRefuseAndRemainBounded(t *testing.T) {
	for _, payload := range []string{"4;1;" + strings.Repeat("f", maxOSCBytes), "4;999;#123456", "10;not-a-color"} {
		f := Filter{Xterm: true}
		if got := f.Feed([]byte("\x1b]" + payload)); len(got) != 0 || f.AtBoundary() {
			t.Fatal("released partial color")
		}
		if len(f.oscData) > maxOSCBytes {
			t.Fatal("unbounded OSC")
		}
		if got := f.Feed([]byte("\a")); len(got) != 0 || !f.UnsupportedDisplay() {
			t.Fatal("accepted unsupported color")
		}
		f.Feed([]byte("\x1bc"))
		if f.UnsupportedDisplay() {
			t.Fatal("RIS did not repair profile")
		}
	}
	f := Filter{Xterm: true}
	f.Feed([]byte("\x1b]10;not-a-color\x18"))
	if f.UnsupportedDisplay() {
		t.Fatal("cancelled OSC changed display")
	}
}

func TestXtermColorFormats(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"#fa3", "#f0a030"}, {"rgb:f/a/3", "#ffaa33"}, {"RGB:1234/5678/9ABC", "#12569a"}, {"#123456789", "#124578"},
	} {
		got, ok := xtermColor(tc.input)
		if !ok || got != tc.want {
			t.Fatalf("%s: %q, want %q", tc.input, got, tc.want)
		}
	}
	for _, input := range []string{"red", "rgb:f/aa/333", "#1234", "#ggg", "#1;2;3", "\x1b]52;c;private"} {
		if color, ok := xtermColor(input); ok {
			t.Fatal("accepted invalid color", input, color)
		}
	}
}
