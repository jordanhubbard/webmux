// This fixture emits real Go snapshots for the independent browser renderer test.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"os"
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
		{"immediate", "Q", "\x1b[3b"},
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
