package browseropen

import (
	"strings"
	"testing"
)

func TestEveryPacketBoundaryAndNoSensitiveOutput(t *testing.T) {
	url := "https://login.example/authorize?state=secret&redirect_uri=http://127.0.0.1:1234/callback"
	packet := Packet(url)
	for boundary := range len(packet) + 1 {
		var decoder Decoder
		a, first := decoder.Feed("before" + packet[:boundary])
		b, second := decoder.Feed(packet[boundary:] + "after")
		urls := append(first, second...)
		if a+b != "beforeafter" || len(urls) != 1 || urls[0] != url {
			t.Fatalf("boundary %d: %q, %v", boundary, a+b, urls)
		}
	}
}

func TestMalformedAndOversizedPacketsAreBoundedAndRemoved(t *testing.T) {
	for _, payload := range []string{Packet("file:///private"), Prefix + "not-base64!\a", Prefix + strings.Repeat("a", maxPacket+1) + "\a"} {
		var decoder Decoder
		var output string
		for _, char := range payload {
			text, urls := decoder.Feed(string(char))
			output += text
			if len(urls) != 0 || len(decoder.pending) > maxPacket+len(Prefix) {
				t.Fatal("invalid packet accepted or unbounded memory")
			}
		}
		text, urls := decoder.Feed("normal")
		if output+text != "normal" || len(urls) != 0 {
			t.Fatal("packet leaked or subsequent output lost")
		}
	}
}
