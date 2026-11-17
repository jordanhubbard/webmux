package screenstate

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRendererProfileIsExplicitAndImageStaysImmutable(t *testing.T) {
	s := newScreen(t, 8, 4)
	if err := s.Feed([]byte("\x1b[?7l\x1b[2;7H日Z")); err != nil {
		t.Fatal(err)
	}
	if err := s.Resize(12, 4); err != nil {
		t.Fatal(err)
	}
	image, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		encoded, err := image.BytesForProfile(XtermRendererProfile)
		if err != nil || encoded.RequiredRenderer != XtermRendererProfile || len(encoded.RendererToken) != 32 {
			t.Fatalf("missing negotiated profile: %+v %v", encoded, err)
		}
		expected := "\x1b]7777;webmux.replay.v1;" + encoded.RendererToken + ";Wg==\a"
		if !bytes.HasSuffix(encoded.Data, []byte(expected)) {
			t.Fatal("restoration packet must be the final operation")
		}
		if data, err := image.Bytes(); data != nil || !errors.Is(err, ErrUnsupported) {
			t.Fatal("profile use changed legacy refusal", err)
		}
	}
	if _, err := image.BytesForProfile("unknown"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown profile accepted", err)
	}
	ordinary := newScreen(t, 8, 4)
	if err := ordinary.Feed([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	normal, err := ordinary.Capture()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := normal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := normal.BytesForProfile(XtermRendererProfile)
	if err != nil || encoded.RequiredRenderer != "" || encoded.RendererToken != "" || !bytes.Equal(legacy, encoded.Data) {
		t.Fatal("ordinary ANSI unexpectedly requires a profile", err)
	}
}

func TestRepeatStatePacketBoundsAndNonce(t *testing.T) {
	for _, invalid := range []string{"", "\x00", "\x1b", "\x7f", "\u0085", "\xff", strings.Repeat("x", 65)} {
		if _, _, err := repeatStatePacket(invalid); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("accepted invalid cluster %q: %v", invalid, err)
		}
	}
	seen := map[string]bool{}
	for _, valid := range []string{"Z", "日", "e\u0301", strings.Repeat("x", 64), "Z"} {
		packet, token, err := repeatStatePacket(valid)
		if err != nil || len(token) != 32 || seen[token] || !strings.HasSuffix(packet, "\a") {
			t.Fatal("invalid or reused snapshot token", err)
		}
		seen[token] = true
	}
}

func TestUnderlineStatePacketCombinesFlagsAndRepeatWithinBounds(t *testing.T) {
	seen := map[string]bool{}
	for flags := uint8(1); flags <= 4; flags++ {
		for _, cluster := range []string{"", "Z", "日", "e\u0301", strings.Repeat("\\", 64), strings.Repeat("\u2028", 21)} {
			packet, token, err := underlineStatePacket(flags, cluster)
			if err != nil || len(token) != 32 || seen[token] {
				t.Fatal("missing or reused combined-state nonce", err)
			}
			seen[token] = true
			if _, err := hex.DecodeString(token); err != nil {
				t.Fatal(err)
			}
			prefix := "\x1b]7777;webmux.replay.v2;" + token + ";"
			if !strings.HasPrefix(packet, prefix) || !strings.HasSuffix(packet, "\a") || strings.Count(packet, "\x1b]") != 1 {
				t.Fatal("state must be one v2 packet")
			}
			encoded := strings.TrimSuffix(strings.TrimPrefix(packet, prefix), "\a")
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(data) > 160 || len(encoded) > 216 {
				t.Fatal("payload exceeded renderer bounds", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(data, &payload); err != nil || len(payload) != 2 {
				t.Fatal("invalid schema", err)
			}
			var gotFlags uint8
			var gotCluster string
			if json.Unmarshal(payload["u"], &gotFlags) != nil || json.Unmarshal(payload["r"], &gotCluster) != nil || gotFlags != flags || gotCluster != cluster {
				t.Fatal("state was not preserved", string(data))
			}
		}
	}
	for _, flags := range []uint8{0, 5, 255} {
		if _, _, err := underlineStatePacket(flags, ""); !errors.Is(err, ErrUnsupported) {
			t.Fatal("accepted invalid underline flags", flags, err)
		}
	}
	for _, cluster := range []string{"\x00", "\x1b", "\x7f", "\u0085", "\xff", strings.Repeat("x", 65)} {
		if _, _, err := underlineStatePacket(1, cluster); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("accepted invalid repeat cluster %q: %v", cluster, err)
		}
	}
}
