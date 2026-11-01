package session

import (
	"strings"
	"testing"
)

func TestReplayScreenControlsAcrossEveryBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, data, prefix string
	}{
		{"tmux", "\x1b[?1049h", "\x1b[?1049h"},
		{"legacy", "\x1b[?47h", "\x1b[?1049h"},
		{"combined", "\x1b[?25;1047h", "\x1b[?1049h"},
		{"exit", "\x1b[?1049h\x1b[?1049l", ""},
		{"reset", "\x1b[?1049h\x1bc", ""},
		{"osc", "\x1b]0;\x1b[?1049h\a", ""},
		{"dcs", "\x1bP\x1b[?1049h\x1b\\", ""},
		{"cancel", "\x1b[?1049\x18h", ""},
		{"partial", "\x1b[?1049h\x1b[?1049", "\x1b[?1049h\x1b[?1049"},
		{"partial-string", "\x1b[?1049h\x1b]0;ignored\x1b", "\x1b[?1049h\x1b]\x1b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for split := 0; split <= len(tc.data); split++ {
				var screen replayScreen
				screen.feed(tc.data[:split])
				screen.feed(tc.data[split:])
				if got := screen.prefix(); got != tc.prefix {
					t.Fatalf("split %d: %q, want %q", split, got, tc.prefix)
				}
			}
		})
	}
}

func TestViewerRetainsAlternateScreenAfterScrollbackTruncation(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "\x1b[?1049h"+strings.Repeat("history line\r\n", 6000)+"\x1b[2J\x1b[HSTATUS")
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.HasSuffix(text, "STATUS")
	})
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(text, "1049h") {
		t.Fatal("fixture did not discard alternate-screen entry")
	}
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	event(t, v, "status")
	replay := event(t, v, "output")["data"].(string)
	if !strings.HasPrefix(replay, "\x1b[?1049h") || !strings.HasSuffix(replay, "STATUS") {
		t.Fatal("viewer lost screen selection or retained output")
	}
	// Screen selection must follow the discarded prefix, not the final state:
	// an exit still in the ring must execute after its alternate-screen contents.
	p.output(t, "\x1b[?1049lNORMAL")
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.HasSuffix(text, "NORMAL")
	})
	exited, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, exited, "viewer_join")
	event(t, exited, "status")
	if got := event(t, exited, "output")["data"].(string); !strings.HasPrefix(got, "\x1b[?1049h") || !strings.HasSuffix(got, "\x1b[?1049lNORMAL") {
		t.Fatalf("screen exit lost its ordering in replay: %q", got[:min(80, len(got))])
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	(<-launched).output(t, "fresh shell")
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return text == "fresh shell"
	})
	next, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, next, "viewer_join")
	event(t, next, "status")
	if got := event(t, next, "output")["data"]; got != "fresh shell" {
		t.Fatalf("new provider inherited screen state: %q", got)
	}
}
