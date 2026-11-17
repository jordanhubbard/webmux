package screenstate

import (
	"bytes"
	"errors"
	"testing"
)

func TestHyperlinkCaptureKeepsUnicodeIdentityAndAnonymousGroups(t *testing.T) {
	s := newScreen(t, 80, 24)
	s.Feed([]byte("\x1b]8;id=术;https://example.test/术\aA\x1b]8;id=%E6%9C%AF;https://example.test/术\aB\x1b]8;;https://example.test/a\aC\x1b]8;;https://example.test/a\aD"))
	first, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	st := first.state
	if st.Screen[0][0].LinkParams != "id=术" || st.Screen[0][1].LinkParams != "id=%E6%9C%AF" {
		t.Fatal("Unicode and literal escapes were conflated")
	}
	if st.Screen[0][2].LinkParams == st.Screen[0][3].LinkParams {
		t.Fatal("separate anonymous groups collapsed")
	}
	if st.Pen.LinkParams != st.Screen[0][3].LinkParams {
		t.Fatal("active anonymous pen lost its cell group")
	}
	before, err := first.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s.Feed([]byte("\x1b]8;;\aCLOSED\x1bcRESET"))
	after, err := first.Bytes()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("capture changed with the live mirror", err)
	}
}

func TestHiddenSavedHyperlinkRefusesNewSnapshot(t *testing.T) {
	for _, tc := range []struct{ save, restore string }{{"\x1b7", "\x1b8"}, {"\x1b[?1048h", "\x1b[?1048l"}, {"\x1b[?1049h", "\x1b[?1049l"}} {
		s := newScreen(t, 80, 24)
		s.Feed([]byte("A" + tc.save + "\x1b]8;id=hidden;https://example.test/a\aB"))
		before, err := s.Capture()
		if err != nil {
			t.Fatal(err)
		}
		original, err := before.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		s.Feed([]byte(tc.restore + "C"))
		unsafe, err := s.Capture()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := unsafe.Bytes(); !errors.Is(err, ErrUnsupported) {
			t.Fatal("hidden extended link state was accepted", err)
		}
		if data, err := before.Bytes(); err != nil || !bytes.Equal(data, original) {
			t.Fatal("earlier image was changed", err)
		}
		s.Feed([]byte("\x1bcRESET"))
		if _, err := s.Capture(); err != nil {
			t.Fatal(err)
		}
		imageBytes(t, s)
	}
}
