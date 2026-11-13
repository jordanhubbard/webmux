package screenstate

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"tuios.dev/vt"
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
		{"\x1b[\a", "6nVISIBLE"},
		{"\x1b[\n", "6nVISIBLE"},
		{"\x1b[\x7f", "6nVISIBLE"},
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

func TestContinuationPreservesHiddenDisplayState(t *testing.T) {
	for _, tc := range []struct{ name, before, after string }{
		{"saved-origin", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[31m\x1b7\x1b[?6l\x1b[10;20H\x1b[32m", "\x1b8X"},
		{"saved-pending-wrap", "\x1b[4;80HX\x1b7\x1b[10;20H", "\x1b8Y"},
		{"saved-wide-pending-wrap", "\x1b[4;79H日\x1b7\x1b[10;20H", "\x1b8Y"},
		{"active-wide-pending-wrap", "\x1b[4;79H日", "Y"},
		{"protected-wide", "\x1b[1\"q日本\x1b[0\"qTEXT\x1b[H", "\x1b[?2K"},
		{"protected-main", "\x1b[1\"qKEEP\x1b[0\"q\x1b[?1049hAPP", "\x1b[?1049l\x1b[?2K"},
		{"protected-blank", "\x1b[41m\x1b[1\"q    \x1b[0\"q\x1b[0m\x1b[H", "\x1b[?2K"},
		{"saved-cursor", "\x1b[4;8H\x1b[31m\x1b7\x1b[10;20H\x1b[32m", "\x1b8X"},
		{"saved-alt-cursor", "SHELL\x1b[?1049h\x1b[4;8H\x1b[31m\x1b7\x1b[10;20H\x1b[32m", "\x1b8X"},
		{"main-origin-after-return", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[31mSHELL\x1b[?1049h\x1b[32mAPP", "\x1b[?1049l\x1b[1;1HX"},
		{"saved-main-origin", "\x1b[3;20r\x1b[?6h\x1b[4;8H\x1b[31mSHELL\x1b[?1049h\x1b[32mAPP", "\x1b[?1049lX"},
		{"saved-main-pen", "\x1b[31mSHELL\x1b[?1049h\x1b[32mAPP", "\x1b[?1049lX"},
		{"saved-charset", "\x1b(0\x1b[4;8H\x1b7\x1b(B\x1b[10;20H", "\x1b8q"},
		{"protected-cells", "\x1b[1\"qKEEP\x1b[0\"q\x1b[H", "\x1b[?2K"},
		{"protected-pen", "\x1b[1\"q", "KEEP\x1b[H\x1b[?2K"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen := newScreen(t, 80, 24)
			reference := vt.NewEmulator(80, 24)
			defer reference.Close()
			restored := vt.NewEmulator(80, 24)
			defer restored.Close()
			if err := screen.Feed([]byte(tc.before)); err != nil {
				t.Fatal(err)
			}
			reference.WriteString(tc.before)
			restored.Write(imageBytes(t, screen))
			for _, part := range []string{"", tc.after} {
				reference.WriteString(part)
				restored.WriteString(part)
				if err := screen.Feed([]byte(part)); err != nil {
					t.Fatal(err)
				}
				projected := vt.NewEmulator(80, 24)
				projected.Write(imageBytes(t, screen))
				if projected.String() != reference.String() || projected.CursorPosition() != reference.CursorPosition() {
					t.Error("capture changed the continuing mirror")
				}
				projected.Close()
				if reference.String() != restored.String() || reference.CursorPosition() != restored.CursorPosition() {
					t.Fatalf("continuing output diverged: %q\nreference=%q\nrestored=%q", part, reference.String(), restored.String())
				}
				p, _ := reference.CursorPen()
				q, _ := restored.CursorPen()
				if !reflect.DeepEqual(p, q) {
					t.Fatalf("continuing pen diverged: %+v %+v", p, q)
				}
			}
		})
	}
}

func TestMirrorMatchesXtermHorizontalModeHandling(t *testing.T) {
	s := newScreen(t, 80, 24)
	// xterm ignores private mode 69. CSI 5;15s therefore saves the cursor
	// after READY, rather than setting margins and homing it.
	if err := s.Feed([]byte("READY\x1b[?25;69;1004h\x1b[5;15s\x1b[1;14H")); err != nil {
		t.Fatal(err)
	}
	restored := vt.NewEmulator(80, 24)
	defer restored.Close()
	restored.Write(imageBytes(t, s))
	restored.WriteString("ABCDE\x1b[?1049lX")
	if got := restored.CursorPosition(); got.X != 6 || got.Y != 0 {
		t.Fatal("xterm cursor-save semantics were lost", got)
	}
	if !strings.HasPrefix(restored.String(), "READYX") {
		t.Fatal("xterm restored at the wrong cell", restored.String())
	}
	if !restored.GetModes()[25] || !restored.GetModes()[1004] {
		t.Fatal("discarded supported batched modes")
	}
	if restored.GetModes()[69] {
		t.Fatal("enabled a mode xterm ignores")
	}
}

func TestNoWrapFinalColumnRefusesAmbiguousCursorWithoutChangingScreen(t *testing.T) {
	for _, text := range []string{"\x1b[4;79HX", "\x1b[4;80HX", "\x1b[4;79HXX", "\x1b[4;80H"} {
		s := newScreen(t, 80, 24)
		if err := s.Feed([]byte("\x1b[?7l" + text)); err != nil {
			t.Fatal(err)
		}
		image, err := s.Capture()
		if err != nil {
			t.Fatal(err)
		}
		if data, err := image.Bytes(); !errors.Is(err, ErrUnsupported) || data != nil {
			t.Fatal("ambiguous no-wrap cursor was replayed", err)
		}
		// A later unambiguous cursor movement is still accepted by the same mirror.
		if err := s.Feed([]byte("\x1b[HSAFE")); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(imageBytes(t, s), []byte("SAFE")) {
			t.Fatal("refusal damaged the continuing screen")
		}
	}
}
