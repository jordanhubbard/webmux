package screenstate

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func newScreen(t *testing.T, cols, rows int) *Screen {
	t.Helper()
	s, err := New(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func imageBytes(t *testing.T, s *Screen) []byte {
	t.Helper()
	image, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	data, err := image.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSnapshotRetainsUnchangedCellsAfterOutputTailEviction(t *testing.T) {
	s := newScreen(t, 80, 24)
	initial := []byte("SHELL_BEHIND_APP\r\n\x1b[?1049h\x1b[2J\x1b[H\x1b[31mHEADER_DRAWN_ONCE\x1b[5;4H\x1b[32m日本語 α\x1b[?25l\x1b[?2004h")
	if err := s.Feed(initial); err != nil {
		t.Fatal(err)
	}
	output := bytes.Repeat([]byte("\x1b[10;1HSTATUS_UPDATE"), 100000)
	if bytes.Contains(output[len(output)-(1<<20):], []byte("HEADER_DRAWN_ONCE")) {
		t.Fatal("fixture did not evict header")
	}
	for len(output) > 0 {
		n := min(7919, len(output))
		if err := s.Feed(output[:n]); err != nil {
			t.Fatal(err)
		}
		output = output[n:]
	}
	image, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	before, err := image.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"HEADER_DRAWN_ONCE", "STATUS_UPDATE", "SHELL_BEHIND_APP", "日本語"} {
		if !bytes.Contains(before, []byte(text)) {
			t.Fatalf("snapshot omitted %s", text)
		}
	}
	if len(before) > 4096 {
		t.Fatal("snapshot retained the output flood", len(before))
	}
	if err := s.Feed([]byte("\x1b[HNEW_CONTENT")); err != nil {
		t.Fatal(err)
	}
	after, err := image.Bytes()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("captured image aliased the continuing job", err)
	}
}

func TestPartialControlsCannotBecomeReplayRequests(t *testing.T) {
	for _, tc := range []struct{ prefix, suffix string }{
		{"\x1b]52;c;private-clipboard", "\aVISIBLE"},
		{"\x1b]777;webmux-recovery;private-identity", "\aVISIBLE"},
		{"\x1b]777;webmux-browser;private-browser", "\x1b\\VISIBLE"},
		{"\x1bPprivate-dcs", "\x1b\\VISIBLE"},
		{"\x1b[", "6nVISIBLE"},
		{"\x1b[?2026hPARTIAL", "VISIBLE\x1b[?2026l"},
		{"\xe6\x97", "\xa5VISIBLE"},
	} {
		s := newScreen(t, 80, 24)
		input := []byte(tc.prefix)
		copyInput := bytes.Clone(input)
		if err := s.Feed(input); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(input, copyInput) {
			t.Fatal("mirror altered live bytes")
		}
		if _, err := s.Capture(); !errors.Is(err, ErrIncomplete) {
			t.Fatalf("partial control was attachable: %q %v", tc.prefix, err)
		}
		if err := s.Feed([]byte(tc.suffix)); err != nil {
			t.Fatal(err)
		}
		data := imageBytes(t, s)
		if !bytes.Contains(data, []byte("VISIBLE")) || bytes.Contains(data, []byte("private-")) || bytes.Contains(data, []byte("\x1b[6n")) || bytes.Contains(data, []byte("\x1b]52;")) {
			t.Fatalf("unsafe or incomplete replay: %q", data)
		}
	}
}

func TestSnapshotRefusesOversizeAndInvalidGeometry(t *testing.T) {
	for _, size := range [][2]int{{0, 24}, {80, 0}, {501, 24}, {80, 201}} {
		if s, err := New(size[0], size[1]); err == nil {
			_ = s.Close()
			t.Fatal("accepted invalid size", size)
		}
	}
	s := newScreen(t, 200, 200)
	cluster := "x" + strings.Repeat("\u0301", 30)
	input := []byte(strings.Repeat(cluster, 200*200))
	for len(input) > 0 {
		n := min(7919, len(input))
		if err := s.Feed(input[:n]); err != nil {
			t.Fatal(err)
		}
		input = input[n:]
	}
	image, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	if data, err := image.Bytes(); !errors.Is(err, ErrTooLarge) || data != nil {
		t.Fatal("oversized viewport was attachable", len(data), err)
	}
	if err := s.Resize(0, 24); err == nil {
		t.Fatal("accepted invalid resize")
	}
}

func TestUnattendedStringsAndCombiningFloodStayBounded(t *testing.T) {
	s := newScreen(t, 80, 24)
	if err := s.Feed([]byte("\x1b]777;")); err != nil {
		t.Fatal(err)
	}
	for range 200 {
		if err := s.Feed(bytes.Repeat([]byte("private"), 1000)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Capture(); !errors.Is(err, ErrIncomplete) {
		t.Fatal(err)
	}
	if err := s.Feed([]byte("\aSAFE\r\n" + "x" + strings.Repeat("\u0301", 10000))); err != nil {
		t.Fatal(err)
	}
	data := imageBytes(t, s)
	if len(data) > 4096 || bytes.Contains(data, []byte("private")) {
		t.Fatal("retained unbounded string or cluster", len(data))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Feed([]byte("late")); err == nil {
		t.Fatal("closed mirror accepted output")
	}
}
