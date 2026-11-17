package screenstate

import (
	"bytes"
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
