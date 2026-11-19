package browser

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
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

func TestStartupNavigationDiagnosticsKeepStagesAndSafeInitiators(t *testing.T) {
	t.Setenv("WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS", "1")
	trace := newNavigationTrace()
	private := "https://user:password@private.invalid/path?token=secret"
	trace.startup("target")
	trace.observe("target", &page.EventFrameStartedLoading{FrameID: "frame"})
	trace.observe("target", &page.EventFrameNavigated{Frame: &cdp.Frame{ID: "frame", LoaderID: "startup-loader", URL: "about:blank"}})
	trace.startupResult(nil)
	trace.ready()
	trace.begin("target", private)
	trace.observe("target", &page.EventFrameRequestedNavigation{FrameID: "frame", URL: private, Reason: page.ClientNavigationReasonScriptInitiated, Disposition: page.ClientNavigationDispositionCurrentTab})
	trace.observe("target", &network.EventRequestWillBeSent{FrameID: "frame", LoaderID: "request-loader", RequestID: "request", Type: network.ResourceTypeDocument, Request: &network.Request{URL: private}, Initiator: &network.Initiator{Type: network.InitiatorTypeScript, URL: private}, RedirectResponse: &network.Response{URL: private}})
	trace.observe("target", &page.EventFrameStoppedLoading{FrameID: "frame"})
	trace.observe("target", &page.EventFrameDetached{FrameID: "frame", Reason: page.FrameDetachedReasonSwap})
	trace.record(NavigationEvent{Event: "future_enum", Stage: private, Reason: private, Disposition: private, Initiator: private}, "")
	snapshot := trace.snapshot()
	if snapshot.Events[0].Stage != "startup" || snapshot.Events[0].Sequence != 0 || snapshot.Events[3].Event != "startup_blank_ready" || snapshot.Events[4].Stage != "ready" {
		t.Fatalf("lost startup boundary: %+v", snapshot.Events)
	}
	renderer, request := snapshot.Events[6], snapshot.Events[7]
	if renderer.Stage != "navigation" || renderer.Sequence != 1 || renderer.Reason != "scriptInitiated" || renderer.Disposition != "currentTab" || renderer.MatchesRequested == nil || !*renderer.MatchesRequested {
		t.Fatalf("lost renderer provenance: %+v", renderer)
	}
	if request.Initiator != "script" || !request.Redirected || request.Request != "request" || request.Loader != "request-loader" {
		t.Fatalf("lost document provenance: %+v", request)
	}
	last := snapshot.Events[len(snapshot.Events)-1]
	if last.Reason != "other" || last.Disposition != "other" || last.Initiator != "other" || last.Stage != "navigation" {
		t.Fatalf("unrecognized protocol strings escaped: %+v", last)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"https://", "password", "private.invalid", "token=", "secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("diagnostic leaked %q", secret)
		}
	}
}

func TestStartupNavigationDiagnosticsRemainNilSafe(t *testing.T) {
	var trace *navigationTrace
	trace.startup("target")
	trace.startupResult(context.Canceled)
	trace.ready()
	trace.observe("target", &page.EventFrameStoppedLoading{})
	if trace.snapshot() != nil {
		t.Fatal("disabled startup diagnostics emitted data")
	}
}
