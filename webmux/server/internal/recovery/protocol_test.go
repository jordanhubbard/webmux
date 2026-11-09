package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPacketsSurviveEveryReadBoundaryWithoutEnteringOutput(t *testing.T) {
	want := Event{Kind: "command", Instance: "shell-1", Cwd: "/workspace/a b", Command: "printf 'hello 😀'"}
	packet, err := Packet(want)
	if err != nil {
		t.Fatal(err)
	}
	for split := 0; split <= len(packet); split++ {
		var d Decoder
		first, a := d.Feed("before" + packet[:split])
		second, b := d.Feed(packet[split:] + "after")
		all := append(a, b...)
		if first+second != "beforeafter" || len(all) != 1 || all[0].Command != want.Command || all[0].Cwd != want.Cwd {
			t.Fatalf("split %d: %q %#v", split, first+second, all)
		}
	}
}

func TestMalformedAndOversizedPacketsAreBoundedAndDiscarded(t *testing.T) {
	var d Decoder
	visible, events := d.Feed("a" + Prefix + strings.Repeat("x", MaxPacket+1))
	if visible != "a" || len(events) != 0 || len(d.pending) != 0 {
		t.Fatal("unterminated packet was retained")
	}
	visible, events = d.Feed("not a command\aafter")
	if visible != "after" || len(events) != 0 {
		t.Fatal("oversized payload escaped")
	}
	visible, events = d.Feed(Prefix + "not-json\aend")
	if visible != "end" || len(events) != 0 {
		t.Fatal("malformed payload escaped")
	}
	if _, err := Packet(Event{Kind: "cli", Cwd: "/tmp/\x1b]52;clipboard"}); err == nil {
		t.Fatal("accepted control characters in native metadata")
	}
}

func TestPlainHistoryDropsCharsetDesignatorsAndC1StringControls(t *testing.T) {
	input := "before\x1b(Bafter\u009d52;c;private clipboard\u009cend\u009b31mred\u009b0m\u0090private DCS\u009c"
	if got := PlainText(input); got != "beforeafterendred" {
		t.Fatalf("control payload entered history: %q", got)
	}
}

func TestMalformedUTF8HistoryStaysBoundedAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	name := "session-malformed.log"
	data := []byte(strings.Repeat("\xff\n", MaxHistoryBytes/2))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	text, err := History(dir, name)
	if err != nil || len(text) > MaxHistoryBytes || !utf8.ValidString(text) {
		t.Fatalf("unbounded or malformed preview: bytes=%d, error=%v", len(text), err)
	}
	original, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(original) != string(data) {
		t.Fatal("preview changed the original transcript")
	}
}

func TestHistoryIsBoundedPlainTextAndRejectsLinks(t *testing.T) {
	dir := t.TempDir()
	name := "session-owned.log"
	packet, _ := Packet(Event{Kind: "command", Command: "hidden command"})
	data := strings.Repeat("old line\n", MaxHistoryBytes) + "\x1b[31mprior output 😀\x1b[0m\n\x1b]52;c;secret\a\x1b]777;webmux-browser;aHR0cHM6Ly9leGFtcGxlLmNvbQ\a" + packet + "visible\n\x1b]unterminated"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := History(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > MaxHistoryBytes || !strings.Contains(got, "prior output 😀\nvisible\n") || strings.ContainsAny(got, "\x1b\a") || strings.Contains(got, "secret") || strings.Contains(got, "hidden command") {
		t.Fatalf("unsafe history suffix: %q", got[max(0, len(got)-200):])
	}
	if _, err := History(dir, "../session-owned.log"); err == nil {
		t.Fatal("accepted traversal")
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("other owner's data"), 0600); err != nil {
		t.Fatal(err)
	}
	link := "session-link.log"
	if err := os.Symlink(outside, filepath.Join(dir, link)); err == nil {
		if _, err := History(dir, link); err == nil {
			t.Fatal("followed transcript symlink")
		}
	}
	if _, err := History(dir, "session-missing.log"); !os.IsNotExist(err) {
		t.Fatalf("missing log: %v", err)
	}
}

func TestStateCopiesDoNotShareMutableRecoveryEvidence(t *testing.T) {
	s := &State{Activity: &Activity{Argv: []string{"codex", "resume", "original"}}, Transcripts: []string{"session-first.log"}}
	c := s.Clone()
	c.Activity.Argv[2] = "different"
	c.Transcripts[0] = "session-other.log"
	if s.Activity.Argv[2] != "original" || s.Transcripts[0] != "session-first.log" {
		t.Fatal("shared state escaped")
	}
}
