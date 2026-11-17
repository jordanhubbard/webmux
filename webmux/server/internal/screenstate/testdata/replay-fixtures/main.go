// This fixture emits real Go snapshots for the independent browser renderer test.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"os"
	"strings"
)

type fixture struct {
	Name       string `json:"name"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	Original   string `json:"original"`
	Snapshot   string `json:"snapshot"`
	Future     string `json:"future"`
	Recaptured string `json:"recaptured"`
}

func main() {
	cases := []struct{ name, before, after string }{
		{"link-named", "\x1b]8;id=one;https://example.test/a\aA", "B\x1b]8;;\aC"},
		{"link-anonymous", "\x1b]8;;https://example.test/a\aA\x1b]8;;https://example.test/a\aB", "C\x1b]8;;\aD"},
		{"link-replace", "\x1b]8;id=a;https://example.test/a\aA\x1b]8;id=b;https://example.test/b\aB", "C"},
		{"link-unicode", "\x1b]8;id=术;https://example.test/术?q=😀\aA\x1b]8;id=%E6%9C%AF;https://example.test/术?q=😀\aB", "C"},
		{"link-semicolon", "\x1b]8;id=a:unused=yes;https://example.test/a;b?x=y;z\x1b\\A", "B"},
		{"link-wrapped", "\x1b]8;;https://example.test/a\a" + strings.Repeat("A", 81), "B"},
		{"link-id-reuse", "\x1b]8;id=same;https://example.test/a\aA\x1b]8;;\aB", "\x1b]8;id=same;https://example.test/a\aC"},
		{"link-save", "\x1b]8;id=saved;https://example.test/a\aA\x1b7\x1b]8;id=live;https://example.test/b\aB", "\x1b8C"},
		{"link-alternate", "\x1b]8;id=main;https://example.test/a\aA\x1b[?1049h\x1b]8;id=alt;https://example.test/b\aB", "\x1b[?1049lC"},
		{"link-close-params", "\x1b]8;id=one;https://example.test/a\aA\x1b]8;id=ignored;\aB", "C\x1b]8;;\aD"},

		{"palette-short-hex", "\x1b]4;1;#fa3\a\x1b[31mA", "\x1b]4;1;#123456\aB"},
		{"palette-short-rgb", "\x1b]4;1;rgb:f/a/3\a\x1b[38;5;1mA", "\x1b]4;1;#123456\aB"},
		{"palette-long-rgb", "\x1b]4;1;RGB:1234/5678/9ABC\a\x1b[31mA", "B"},
		{"palette-long-hex", "\x1b]4;1;#123456789\a\x1b[31mA", "B"},
		{"palette-single", "\x1b]4;1;#123456\a\x1b[31mA", "B"},
		{"palette-batch", "\x1b]4;1;#123456;2;rgb:ab/cd/ef\x1b\\\x1b[31mA\x1b[32mB", "C"},
		{"palette-query", "\x1b]4;1;#123456\a\x1b]4;1;?\a\x1b[31mA", "B"},
		{"palette-reset", "\x1b]4;1;#123456\a\x1b[31mA", "\x1b]104;1\aB"},
		{"dynamic-colors", "\x1b]10;#123456;#345678;#abcdef\aA", "B"},
		{"dynamic-query", "\x1b]10;?;#345678;?\aA", "B"},
		{"dynamic-reset", "\x1b]10;#123456;#345678;#abcdef\aA", "\x1b]110\a\x1b]111\a\x1b]112\aB"},

		{"saved-underline-csi", "\x1b[4:3;58;5;2mA\x1b[s\x1b[4:4;58;5;9mB", "\x1b[uX"},
		{"saved-underline-single", "\x1b[4mA\x1b7\x1b[4:5;58;5;9mB", "\x1b8X"},
		{"saved-underline-color", "\x1b[58;5;2mA\x1b7\x1b[58;5;9mB", "\x1b8X"},
		{"saved-underline-color-reset", "\x1b[4:3;58;5;2mA\x1b7\x1b[59mB", "\x1b8X"},
		{"saved-underline-batched", "\x1b[4:3;58;5;2mA\x1b[?1048h\x1b[4:5;58;5;9mB", "\x1b[?6;1048lX"},
		{"saved-underline-twice", "\x1b[4:3;58;5;2mA\x1b7\x1b[4:5;58;5;9mB", "\x1b8X\x1b[4:4;58;5;6m\x1b8Y"},
		{"saved-underline", "\x1b[4:3;58;2;30;210;60mA\x1b7\x1b[4:5;58;5;9mB", "\x1b8X"},
		{"saved-underline-1048", "\x1b[4:3;58;5;2mA\x1b[?1048h\x1b[4:5;58;5;9mB", "\x1b[?1048lX"},
		{"saved-underline-1049", "\x1b[4:3;58;5;2mA\x1b[?1049h\x1b[4:5;58;5;9mB", "\x1b[?1049lX"},
		{"alternate-wrapped-row", "SHELL\x1b[?1049h\x1b[H" + strings.Repeat("A", 80) + "B", "C"},
		{"alternate-wrapped-blank", "SHELL\x1b[?1049h\x1b[H" + strings.Repeat("A", 80) + " ", "Y"},
		{"origin-clamp-top", "\x1b[H\x1b7\x1b[3;20r\x1b[?6h", "\x1b8X"},
		{"origin-clamp-bottom", "\x1b[24;1H\x1b7\x1b[3;20r\x1b[?6h", "\x1b8X"},
		{"origin-clamp-then-widen", "\x1b[24;1H\x1b7\x1b[3;20r\x1b[?6h", "\x1b8X\x1b[?6l\x1b8Y"},
		{"origin-batched-reset", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[?1048h", "\x1b[?6;1048lX"},
		{"origin-batched-reset-reverse", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[?1048h", "\x1b[?1048;6lX"},
		{"origin-active-at-restore", "\x1b[H\x1b[?1048h\x1b[3;20r\x1b[?6h", "\x1b[?1048lX"},
		{"origin-after-ris", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b7\x1bc", "\x1b8X"},
		{"origin-after-soft-reset", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b7\x1b[!p", "\x1b8X"},
		{"saved-origin", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[31m\x1b7\x1b[?6l\x1b[10;20H\x1b[32m", "\x1b8X"},
		{"saved-pending-wrap", "\x1b[4;80HX\x1b7\x1b[10;20H", "\x1b8Y"},
		{"saved-origin-csi", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[s\x1b[?6l\x1b[10;20H", "\x1b[uTEXT"},
		{"saved-origin-1048", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[?1048h\x1b[?6l\x1b[10;20H", "\x1b[?1048lTEXT"},
		{"saved-pending-1049", "\x1b[4;80HX\x1b[?1049hAPP", "\x1b[?1049lY"},
		{"immediate", "Q", "\x1b[3b"},
		{"colored-underline", "\x1b[1;3;4:3;38;2;220;40;20;48;2;12;24;36;58;2;30;210;60mQ", "\x1b[3b"},
		{"unicode", "日", "\x1b[3b"},
		{"combining", "e\u0301", "\x1b[3b"},
		{"pending-wrap", "\x1b[4;80HX", "\x1b[3b"},
		{"wide-pending-wrap", "\x1b[4;79H日", "\x1b[3b"},
		{"insert-mode", "ABCDE\r\x1b[4hQ", "\x1b[3b"},
		{"after-sgr", "Q\x1b[31m", "\x1b[3b"},
		{"after-bell", "Q\a", "\x1b[3b"},
		{"after-private-osc", "Q\x1b]777;private\a", "\x1b[3b"},
		{"after-query", "Q\x1b[6n", "\x1b[3b"},
		{"after-repeat", "Q\x1b[2b", "\x1b[3b"},
		{"c0-inside-csi", "Q\x1b[\a3b", "\x1b[3b"},
		{"del-inside-csi", "Q\x1b[\x7f3b", "Z"},
		{"after-del", "Q\x7f", "\x1b[3b"},
		{"after-erased-character", "Q\x1b[2J\x1b[10;20H", "\x1b[3b"},
	}
	var fixtures []fixture
	for _, tc := range cases {
		s, err := screenstate.New(80, 24)
		check(tc.name, err)
		check(tc.name, s.Feed([]byte(tc.before)))
		image, err := s.Capture()
		check(tc.name, err)
		before, err := image.Bytes()
		check(tc.name, err)
		check(tc.name, s.Feed([]byte(tc.after)))
		image, err = s.Capture()
		check(tc.name, err)
		after, err := image.Bytes()
		check(tc.name, err)
		check(tc.name, s.Close())
		fixtures = append(fixtures, fixture{tc.name, 80, 24, tc.before, string(before), tc.after, string(after)})
	}
	check("encode", json.NewEncoder(os.Stdout).Encode(fixtures))
}
func check(name string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, name+": "+err.Error())
		os.Exit(1)
	}
}
