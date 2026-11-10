package session

import (
	"testing"
	"time"
)

func TestAgentDeckAttachArgv(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want bool
	}{
		{[]string{"/home/u/.local/bin/agent-deck", "session", "attach", "abc"}, true},
		{[]string{"agent-deck", "-p", "work", "session", "attach", "abc"}, true},
		{[]string{"agent-deck", "remote", "attach", "host", "abc"}, true},
		{[]string{"agent-deck", "session", "show", "abc"}, false},
		{[]string{"agent-deck", "attach"}, false},
		{[]string{"/usr/bin/tmux", "attach", "-t", "x"}, false},
		{[]string{"not-agent-deck", "session", "attach", "abc"}, false},
		{nil, false},
	} {
		if got := agentDeckAttachArgv(tc.argv); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.argv, got, tc.want)
		}
	}
}

func TestAttachProbeAnswersOnlyColorThenCursorQuery(t *testing.T) {
	start := time.Now()
	probe := newAttachProbe([]string{"agent-deck", "session", "attach", "x"}, start)
	if probe == nil {
		t.Fatal("agent-deck attach has no probe")
	}
	if newAttachProbe([]string{"bash"}, start) != nil {
		t.Fatal("ordinary command received a probe")
	}
	// A cursor query without the preceding OSC 11 query is not termenv's
	// background probe and must not be answered.
	if answer, done := probe.feed("prompt \x1b[6n", start); answer || done {
		t.Fatal("answered a cursor query without the colour query")
	}
	// Sequences split across PTY reads are still recognised.
	for _, chunk := range []string{"\x1b[2J\x1b]1", "1;?\x1b", "\\\x1b[", "6"} {
		if answer, done := probe.feed(chunk, start); answer || done {
			t.Fatalf("answered early at %q", chunk)
		}
	}
	if answer, done := probe.feed("n", start); !answer || !done {
		t.Fatalf("probe not answered: answer=%v done=%v", answer, done)
	}
}

func TestAttachProbeExpires(t *testing.T) {
	start := time.Now()
	probe := newAttachProbe([]string{"agent-deck", "session", "attach", "x"}, start)
	if answer, done := probe.feed("\x1b]11;?\x1b\\\x1b[6n", start.Add(attachProbeWindow+time.Millisecond)); answer || !done {
		t.Fatalf("expired probe: answer=%v done=%v", answer, done)
	}
}

func TestAgentDeckAttachRunAnswersStartupProbeOnce(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	ordinary := <-launched
	// An ordinary terminal never answers terminal queries on the viewer's behalf.
	ordinary.output(t, "\x1b]11;?\x1b\\\x1b[6n")
	eventually(t, func() bool { value, _ := b.Get("owner", s.ID); return value.State == "connected" })
	b.mu.Lock()
	b.entries[s.ID].value.ExecArgv = []string{"/opt/bin/agent-deck", "session", "attach", "deck-id"}
	b.mu.Unlock()
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	attach := <-launched
	select {
	case got := <-ordinary.input:
		t.Fatalf("ordinary terminal answered a query: %q", got)
	default:
	}
	attach.output(t, "\x1b[3J\x1b]11;?\x1b\\")
	attach.output(t, "\x1b[6n")
	select {
	case got := <-attach.input:
		if got != attachProbeCursorReply {
			t.Fatalf("reply: %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Agent Deck startup probe was not answered")
	}
	// User input still flows, and a later identical probe is not answered.
	attach.output(t, "\x1b]11;?\x1b\\\x1b[6n")
	if err := b.Input("owner", s.ID, "hello\r"); err != nil {
		t.Fatal(err)
	}
	if got := <-attach.input; got != "hello\r" {
		t.Fatalf("second probe answered or input changed: %q", got)
	}
}
