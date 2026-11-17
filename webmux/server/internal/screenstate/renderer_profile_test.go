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
			packet, token, err := underlineStatePacket(underlineRendererState{Underline: flags, Repeat: cluster})
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
			if err != nil || len(data) > 192 || len(encoded) > 256 {
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
		if _, _, err := underlineStatePacket(underlineRendererState{Underline: flags}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("accepted invalid underline flags", flags, err)
		}
	}
	for _, cluster := range []string{"\x00", "\x1b", "\x7f", "\u0085", "\xff", strings.Repeat("x", 65)} {
		if _, _, err := underlineStatePacket(underlineRendererState{Underline: 1, Repeat: cluster}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("accepted invalid repeat cluster %q: %v", cluster, err)
		}
	}
}

func TestUnderlineCursorTailBounds(t *testing.T) {
	for _, column := range []int{-1, 501} {
		for _, field := range []string{"active", "normal", "alternate"} {
			state := underlineRendererState{Underline: 1}
			switch field {
			case "active":
				state.X = &column
			case "normal":
				state.NormalX = &column
			case "alternate":
				state.AltX = &column
			}
			if _, _, err := underlineStatePacket(state); !errors.Is(err, ErrUnsupported) {
				t.Fatal("unbounded cursor accepted", field, column, err)
			}
		}
	}
	column := 500
	if _, _, err := underlineStatePacket(underlineRendererState{Underline: 4, Repeat: strings.Repeat("\\", 64), X: &column, NormalX: &column, AltX: &column}); err != nil {
		t.Fatal("bounded worst-case escaped tail refused", err)
	}
}

func TestUnderlineModelProfileIsOptInAndLegacyRemainsFailClosed(t *testing.T) {
	if _, err := NewWithProfile(8, 4, "unknown"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown model accepted", err)
	}
	s, err := NewWithProfile(8, 4, XtermUnderlineRendererProfile)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Feed([]byte("A\x1b7\x1b[4:3;58;5;9mB\x1b8")); err != nil {
		t.Fatal(err)
	}
	image, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(image.state)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for n := 0; n < 2; n++ {
		for _, old := range []string{"", XtermRendererProfile} {
			if _, err := image.BytesForProfile(old); !errors.Is(err, ErrUnsupported) {
				t.Fatal("independent flags reached an older renderer", err)
			}
		}
		encoded, err := image.BytesForProfile(XtermUnderlineRendererProfile)
		if err != nil || encoded.RequiredRenderer != XtermUnderlineRendererProfile || len(encoded.RendererToken) != 32 || seen[encoded.RendererToken] {
			t.Fatal("missing or reused v2 requirement", err)
		}
		seen[encoded.RendererToken] = true
		if strings.Count(string(encoded.Data), "\x1b]7777;webmux.replay.") != 1 || encoded.Data[len(encoded.Data)-1] != '\a' {
			t.Fatal("one final restoration packet required")
		}
	}
	after, err := json.Marshal(image.state)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("profile encoding mutated the captured image", err)
	}
}

func TestRendererCompatibilityDoesNotGuessFutureProfiles(t *testing.T) {
	profiles := []string{"", XtermRendererProfile, XtermUnderlineRendererProfile, "unknown"}
	expected := [4][4]bool{{true, false, false, false}, {true, true, false, false}, {true, true, true, false}, {false, false, false, false}}
	for i, offered := range profiles {
		for j, required := range profiles {
			if got := RendererSupports(offered, required); got != expected[i][j] {
				t.Fatalf("offered%q required%q: %v want%v", offered, required, got, expected[i][j])
			}
		}
	}
}
