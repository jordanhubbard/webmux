package terminalclipboard

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSplitBoundariesAndTerminators(t *testing.T) {
	for _, ending := range []string{"\a", "\x1b\\"} {
		packet := prefix + "c;" + base64.StdEncoding.EncodeToString([]byte("desktop 😀\ntext")) + ending
		for split := 0; split <= len(packet); split++ {
			var d Decoder
			a, first := d.Feed("before" + packet[:split])
			b, second := d.Feed(packet[split:] + "after")
			if a+b != "beforeafter" {
				t.Fatalf("boundary %d: %q", split, a+b)
			}
			if len(first) == 0 {
				first = second
			}
			if len(first) != 1 || first[0].Text != "desktop 😀\ntext" {
				t.Fatalf("boundary %d lost copy", split)
			}
		}
	}
}
func TestQueriesInvalidAndOversizeAreStripped(t *testing.T) {
	for _, payload := range []string{"c;%%%", "c;" + base64.StdEncoding.EncodeToString([]byte{0xff}), "c;" + strings.Repeat("Y", maxPacket+10)} {
		var d Decoder
		visible, copied := d.Feed("a" + prefix + payload + "\a" + "b")
		if visible != "ab" || len(copied) != 0 {
			t.Fatalf("invalid packet accepted: %q", visible)
		}
	}
	var d Decoder
	d.Feed(prefix + "c;" + strings.Repeat("Y", maxPacket+20) + "\x1b")
	visible, copied := d.Feed("\\back")
	if visible != "back" || len(copied) != 0 {
		t.Fatalf("oversize packet did not recover: %q", visible)
	}
}

func TestClipboardQueriesPreserveSelectionAtEveryBoundary(t *testing.T) {
	for _, selection := range []string{"", "c", "p", "s"} {
		packet := prefix + selection + ";?\x1b\\"
		for split := 0; split <= len(packet); split++ {
			var d Decoder
			a, first := d.Feed("before" + packet[:split])
			b, second := d.Feed(packet[split:] + "after")
			requests := append(first, second...)
			if a+b != "beforeafter" || len(requests) != 1 || !requests[0].Read || requests[0].Selection != selection {
				t.Fatalf("selection %q split %d: %q %#v", selection, split, a+b, requests)
			}
		}
	}
	var d Decoder
	_, requests := d.Feed(prefix + "c;Y29weQ==\a" + prefix + "c;?\a")
	if len(requests) != 2 || requests[0].Text != "copy" || !requests[1].Read {
		t.Fatalf("lost ordered requests: %#v", requests)
	}
}
