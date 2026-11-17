package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNavigationDiagnosticsAreOptInBoundedAndRedacted(t *testing.T) {
	t.Setenv("WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS", "")
	if newNavigationTrace() != nil {
		t.Fatal("navigation diagnostics enabled by default")
	}
	var disabled *navigationTrace
	disabled.begin("target", "https://private.invalid/?token=secret")
	disabled.record(NavigationEvent{Event: "navigate_ack"}, "")
	if disabled.snapshot() != nil {
		t.Fatal("disabled trace emitted data")
	}
	t.Setenv("WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS", "1")
	trace := newNavigationTrace()
	private := "https://private.invalid/password?token=secret"
	trace.begin("target", private)
	trace.record(NavigationEvent{Event: "document_request", Target: "target", Frame: "https://secret.invalid", Loader: strings.Repeat("s", 100), Request: "123.4"}, private)
	first := trace.snapshot()
	if first.Events[1].MatchesRequested == nil || !*first.Events[1].MatchesRequested {
		t.Fatal("lost request correlation")
	}
	*first.Events[1].MatchesRequested = false
	first.Events[0].Event = "mutated"
	if !*trace.snapshot().Events[1].MatchesRequested || trace.snapshot().Events[0].Event == "mutated" {
		t.Fatal("snapshot aliases the live trace")
	}
	for range navigationDiagnosticLimit + 8 {
		trace.record(NavigationEvent{Event: "navigate_ack", Target: "target", ErrorClass: navigationErrorClass(private, nil)}, "")
	}
	snapshot := trace.snapshot()
	if len(snapshot.Events) != navigationDiagnosticLimit || snapshot.Dropped != 10 {
		t.Fatalf("unbounded trace: %d events, %d dropped", len(snapshot.Events), snapshot.Dropped)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"https://", "password", "secret", "private.invalid"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("diagnostics leaked %q", secret)
		}
	}
}

func TestNavigationErrorClassificationDoesNotRetainProtocolText(t *testing.T) {
	for _, tc := range []struct {
		text string
		err  error
		want string
	}{
		{"", nil, "accepted"}, {"net::ERR_ABORTED", nil, "aborted"},
		{"net::ERR_BLOCKED_BY_CLIENT", nil, "blocked"},
		{"net::ERR_OTHER: https://user:password@private.invalid", nil, "network_error"},
		{"", errors.New("secret protocol response"), "protocol_error"},
		{"", context.Canceled, "canceled"}, {"", context.DeadlineExceeded, "timeout"},
	} {
		if got := navigationErrorClass(tc.text, tc.err); got != tc.want {
			t.Errorf("classification %q, want %q", got, tc.want)
		}
	}
}
