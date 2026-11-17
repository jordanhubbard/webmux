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
	Name              string   `json:"name"`
	Cols              int      `json:"cols"`
	Rows              int      `json:"rows"`
	Original          string   `json:"original"`
	Snapshot          string   `json:"snapshot"`
	Future            string   `json:"future"`
	Recaptured        string   `json:"recaptured"`
	BeforeCols        int      `json:"beforeCols,omitempty"`
	BeforeRows        int      `json:"beforeRows,omitempty"`
	Resizes           [][2]int `json:"resizes,omitempty"`
	SnapshotError     string   `json:"snapshotError,omitempty"`
	RecapturedError   string   `json:"recapturedError,omitempty"`
	SnapshotProfile   string   `json:"snapshotProfile,omitempty"`
	SnapshotToken     string   `json:"snapshotToken,omitempty"`
	RecapturedProfile string   `json:"recapturedProfile,omitempty"`
	RecapturedToken   string   `json:"recapturedToken,omitempty"`
}

func main() {
	cases := []struct{ name, before, after string }{
		{"wrap-toggle-pending", "\x1b[4;80HX\x1b[?7l\x1b[?7h", "Y"},
		{"wrap-toggle-resolved", "\x1b[?7l\x1b[4;80HX\rSAFE\x1b[?7h", "Y"},
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
		fixtures = append(fixtures, fixture{Name: tc.name, Cols: 80, Rows: 24, Original: tc.before, Snapshot: string(before), Future: tc.after, Recaptured: string(after)})
	}
	var noWrap []struct{ Name, Data string }
	data, err := os.ReadFile("internal/thirdparty/vt/testdata/webmux-xterm-nowrap.json")
	check("read no-wrap reference", err)
	check("decode no-wrap reference", json.Unmarshal(data, &noWrap))
	noWrap = append(noWrap, []struct{ Name, Data string }{
		{"rep-space", "\x1b[?7l\x1b[2;6H \x1b[2;8HX日"},
		{"rep-ascii", "\x1b[?7l\x1b[2;6HV\x1b[2;8HX日"},
		{"rep-wide-placeholder", "\x1b[?7l\x1b[2;5H日\x1b[2;8HX日"},
		{"rep-colored-null", "\x1b[?7l\x1b[41m\x1b[2;6H\x1b[X\x1b[0m\x1b[2;8HX日"},
		{"soft-reset-focus", "\x1b[?1004h\x1b[?7l\x1b[2;8HX\x1b[!p"},
		{"soft-reset-paste", "\x1b[?2004h\x1b[?7l\x1b[2;8HX\x1b[!p"},
		{"soft-reset-sync", "\x1b[?2026h\x1b[?7l\x1b[2;8HX\x1b[!p"},
		{"region-bottom-wrap", "\x1b[2;1HTOP\x1b[3;1HBOTTOM\x1b[2;3r\x1b[?7l\x1b[3;8HX\x1b[?7hY"},
		{"region-top-reverse-index", "\x1b[2;1HTOP\x1b[3;1HBOTTOM\x1b[2;3r\x1b[?7l\x1b[2;8HX\x1bM"},
		{"region-bottom-index", "\x1b[2;1HTOP\x1b[3;1HBOTTOM\x1b[2;3r\x1b[?7l\x1b[3;8HX\n"},
		{"region-saved-restore", "\x1b[?7l\x1b[4;8HX\x1b7\x1b[2;3r\x1b[?6h\x1b8"},
	}...)
	for _, tc := range noWrap {
		for _, split := range []bool{false, true} {
			name := "nowrap-" + tc.Name
			if split {
				name += "-byte-split"
			}
			s, err := screenstate.New(8, 4)
			check(name, err)
			if split {
				for _, b := range []byte(tc.Data) {
					check(name, s.Feed([]byte{b}))
				}
			} else {
				check(name, s.Feed([]byte(tc.Data)))
			}
			image, err := s.Capture()
			check(name, err)
			before, err := image.Bytes()
			check(name, err)
			future := "\x1b[2b\x1b[?7hZ"
			check(name, s.Feed([]byte(future)))
			image, err = s.Capture()
			check(name, err)
			after, err := image.Bytes()
			check(name, err)
			check(name, s.Close())
			fixtures = append(fixtures, fixture{Name: name, Cols: 8, Rows: 4, Original: tc.Data, Snapshot: string(before), Future: future, Recaptured: string(after)})
		}
	}
	for _, tc := range []struct{ name, before, after string }{
		{"address", "\x1b[?7l\x1b[2;8H", "Z\x1b[?7hQ"},
		{"advanced", "\x1b[?7l\x1b[2;8HX", "Z\x1b[?7hQ"},
		{"wide", "\x1b[?7l\x1b[2;7H日", "Z\x1b[?7hQ"},
		{"overlap", "\x1b[?7l\x1b[2;7H日Z", "\x1b[2bQ\x1b[?7hR"},
		{"overlap-cleared", "\x1b[?7l\x1b[2;7H日Z\x1b[0m", "\x1b[2bQ\x1b[?7hR"},
		{"saved", "\x1b[?7l\x1b[2;8HX\x1b7\x1b[H", "\x1b8Z\x1b[?7hQ"},
		{"saved-wide", "\x1b[?7l\x1b[2;7H日\x1b7\x1b[H", "\x1b8Z\x1b[?7hQ"},
		{"alternate", "\x1b[?7l\x1b[2;8HX\x1b[?1049hALT", "\x1b[?1049lZ\x1b[?7hQ"},
	} {
		for _, plan := range []struct {
			name  string
			sizes [][2]int
		}{
			{"grow", [][2]int{{12, 4}}},
			{"shrink", [][2]int{{6, 4}}},
			{"shrink-grow", [][2]int{{6, 4}, {12, 4}}},
		} {
			for _, split := range []bool{false, true} {
				name := "nowrap-resize-" + tc.name + "-" + plan.name
				if split {
					name += "-byte-split"
				}
				s, err := screenstate.New(8, 4)
				check(name, err)
				if split {
					for _, b := range []byte(tc.before) {
						check(name, s.Feed([]byte{b}))
					}
				} else {
					check(name, s.Feed([]byte(tc.before)))
				}
				for _, size := range plan.sizes {
					check(name, s.Resize(size[0], size[1]))
				}
				image, err := s.Capture()
				check(name, err)
				before, beforeError, beforeProfile, beforeToken := qualifiedBytes(name, image)
				check(name, s.Feed([]byte(tc.after)))
				image, err = s.Capture()
				check(name, err)
				after, afterError, afterProfile, afterToken := qualifiedBytes(name, image)
				check(name, s.Close())
				last := plan.sizes[len(plan.sizes)-1]
				fixtures = append(fixtures, fixture{Name: name, Cols: last[0], Rows: last[1], Original: tc.before, Snapshot: string(before), Future: tc.after, Recaptured: string(after), BeforeCols: 8, BeforeRows: 4, Resizes: plan.sizes, SnapshotError: beforeError, RecapturedError: afterError, SnapshotProfile: beforeProfile, SnapshotToken: beforeToken, RecapturedProfile: afterProfile, RecapturedToken: afterToken})
			}
		}
	}
	if path := os.Getenv("WEBMUX_SCREEN_REFERENCE_INPUTS"); path != "" {
		file, err := os.Stat(path)
		check("recorded reference", err)
		if file.Size() > 16<<20 {
			check("recorded reference", fmt.Errorf("input exceeds 16MiB"))
		}
		data, err := os.ReadFile(path)
		check("recorded reference", err)
		var inputs []struct {
			Name             string
			Cols, Rows       int
			Original, Future string
		}
		check("recorded reference", json.Unmarshal(data, &inputs))
		if len(inputs) > 512 {
			check("recorded reference", fmt.Errorf("more than 512 inputs"))
		}
		for _, tc := range inputs {
			name := "recorded-conpty-" + tc.Name
			s, err := screenstate.New(tc.Cols, tc.Rows)
			check(name, err)
			check(name, s.Feed([]byte(tc.Original)))
			image, err := s.Capture()
			check(name, err)
			before, err := image.Bytes()
			check(name, err)
			check(name, s.Feed([]byte(tc.Future)))
			image, err = s.Capture()
			check(name, err)
			after, err := image.Bytes()
			check(name, err)
			check(name, s.Close())
			fixtures = append(fixtures, fixture{Name: name, Cols: tc.Cols, Rows: tc.Rows, Original: tc.Original, Future: tc.Future, Snapshot: string(before), Recaptured: string(after)})
		}
	}
	check("encode", json.NewEncoder(os.Stdout).Encode(fixtures))
}
func check(name string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, name+": "+err.Error())
		os.Exit(1)
	}
}

func qualifiedBytes(name string, image screenstate.Image) (data []byte, legacyError, profile, token string) {
	data, err := image.Bytes()
	if err == nil {
		return data, "", "", ""
	}
	legacyError = err.Error()
	encoded, err := image.BytesForProfile(screenstate.XtermRendererProfile)
	check(name, err)
	if encoded.RequiredRenderer != screenstate.XtermRendererProfile || len(encoded.RendererToken) != 32 {
		check(name, fmt.Errorf("missing explicit renderer capability"))
	}
	return encoded.Data, legacyError, encoded.RequiredRenderer, encoded.RendererToken
}
