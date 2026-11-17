package agentdeckattach

import "testing"

func TestSupportedStableMinimum(t *testing.T) {
	for text, want := range map[string]bool{"Agent Deck v1.16.39": true, "Agent Deck v1.17.0 build": true, "Agent Deck v2.0.0": true, "Agent Deck v1.16.34": false, "Agent Deck v1.16.38": false, "Agent Deck v1.15.99": false, "Agent Deck v1.16.39-beta": false, "Other v9.9.9": false, "Agent Deck v1.16.39.1": false, "Agent Deck v99999999999999999999.0.0": false} {
		if got := supported(text); got != want {
			t.Fatalf("%q supported=%v", text, got)
		}
	}
}
func TestOnlyKnownAttachInvocationsMatch(t *testing.T) {
	for _, argv := range [][]string{{"/tool directory/agent-deck", "session", "attach", "seat"}, {"agent-deck", "-p", "work", "session", "attach", "seat"}, {"agent-deck", "remote", "attach", "host", "seat"}} {
		if !Match(argv) {
			t.Fatal(argv)
		}
	}
	for _, argv := range [][]string{nil, {"sh", "-c", "agent-deck session attach seat"}, {"agent-deck", "version"}, {"agent-deck", "session", "stop", "seat"}, {"agent-deck", "session", "attach"}, {"other", "session", "attach", "seat"}} {
		if Match(argv) {
			t.Fatal(argv)
		}
	}
}
