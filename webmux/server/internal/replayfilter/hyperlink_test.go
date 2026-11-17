package replayfilter

import (
	"bytes"
	"strings"
	"testing"
)

func TestXtermHyperlinksAtEveryReadBoundary(t *testing.T) {
	for _, wire := range []string{"\x1b]8;id=术;https://example.test/术;q=😀\aLABEL\x1b]8;;\x1b\\", "\u009d8;;https://example.test/a\u009cLABEL\x1b]8;;\a"} {
		full := Filter{Xterm: true}
		want := full.Feed([]byte(wire))
		if !bytes.Contains(want, []byte("https://example.test/")) || full.UnsupportedDisplay() {
			t.Fatal("link lost", string(want))
		}
		for i := 0; i <= len(wire); i++ {
			f := Filter{Xterm: true}
			got := append(f.Feed([]byte(wire[:i])), f.Feed([]byte(wire[i:]))...)
			if !bytes.Equal(got, want) || !f.AtBoundary() || f.UnsupportedDisplay() {
				t.Fatalf("boundary %d: %q", i, got)
			}
		}
		legacy := Filter{}
		if bytes.Contains(legacy.Feed([]byte(wire)), []byte("https://")) {
			t.Fatal("legacy history must omit string controls")
		}
	}
}
func TestUnsafeHyperlinksRefuseReplayUntilReset(t *testing.T) {
	for _, payload := range []string{"8;;https://example.test/" + strings.Repeat("x", MaxLinkURL), "8;id=" + strings.Repeat("x", MaxLinkParams) + ";https://example.test", "8;;https://example.test/\x00private", "8;;https://example.test/\u009d52;clipboard", "8;;https://example.test/\x1b]777;private"} {
		f := Filter{Xterm: true}
		got := f.Feed([]byte("\x1b]" + payload + "\a"))
		if len(got) != 0 || !f.UnsupportedDisplay() {
			t.Fatal("unsafe link entered mirror", payload)
		}
		f.Feed([]byte("\x1b]8;;\a"))
		if !f.UnsupportedDisplay() {
			t.Fatal("close hid unsupported state")
		}
		f.Feed([]byte("\x1bc"))
		if f.UnsupportedDisplay() {
			t.Fatal("reset did not repair mirror")
		}
	}
}
func TestHyperlinkCannotAdmitOtherOSCRequests(t *testing.T) {
	f := Filter{Xterm: true}
	got := f.Feed([]byte("\x1b]52;c;clipboard\a\x1b]777;open;https://private.invalid\a\x1b]8;;https://example.test\aA\x1b]8;;\a"))
	if bytes.Contains(got, []byte("clipboard")) || bytes.Contains(got, []byte("private.invalid")) || !bytes.Contains(got, []byte("https://example.test")) {
		t.Fatalf("wrong passive OSC admission: %q", got)
	}
}
