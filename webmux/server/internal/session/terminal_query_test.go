package session

import "testing"

func TestAgentDeckAttachDoesNotAnswerQueriesOrChangeInput(t *testing.T) {
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
	b.mu.Lock()
	r := b.entries[s.ID].run
	b.mu.Unlock()
	// Synchronously process both split and repeated native queries before
	// sending user input, so an unexpected synthetic reply cannot race the oracle.
	b.output(s.ID, r, "\x1b[3J\x1b]11;?\x1b\\")
	b.output(s.ID, r, "\x1b[6n")
	b.output(s.ID, r, "\x1b]11;?\x1b\\\x1b[6n")
	if err := b.Input("owner", s.ID, "hello\r"); err != nil {
		t.Fatal(err)
	}
	if got := <-attach.input; got != "hello\r" {
		t.Fatalf("query answered or input changed: %q", got)
	}
}
