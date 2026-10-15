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
			if first == nil {
				first = second
			}
			if first == nil || *first != "desktop 😀\ntext" {
				t.Fatalf("boundary %d lost copy", split)
			}
		}
	}
}
func TestQueriesInvalidAndOversizeAreStripped(t *testing.T) {
	for _, payload := range []string{"c;?", "c;%%%", "c;" + base64.StdEncoding.EncodeToString([]byte{0xff}), "c;" + strings.Repeat("Y", maxPacket+10)} {
		var d Decoder
		visible, copied := d.Feed("a" + prefix + payload + "\a" + "b")
		if visible != "ab" || copied != nil {
			t.Fatalf("invalid packet accepted: %q", visible)
		}
	}
	var d Decoder
	d.Feed(prefix + "c;" + strings.Repeat("Y", maxPacket+20) + "\x1b")
	visible, copied := d.Feed("\\back")
	if visible != "back" || copied != nil {
		t.Fatalf("oversize packet did not recover: %q", visible)
	}
}
