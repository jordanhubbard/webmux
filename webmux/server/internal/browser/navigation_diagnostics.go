package browser

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const navigationDiagnosticLimit = 128

type NavigationEvent struct {
	Stage            string `json:"stage,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Disposition      string `json:"disposition,omitempty"`
	Initiator        string `json:"initiator,omitempty"`
	Redirected       bool   `json:"redirected,omitempty"`
	Milliseconds     int64  `json:"ms"`
	Sequence         uint64 `json:"sequence"`
	Event            string `json:"event"`
	Target           string `json:"target,omitempty"`
	Frame            string `json:"frame,omitempty"`
	Loader           string `json:"loader,omitempty"`
	Request          string `json:"request,omitempty"`
	ErrorClass       string `json:"errorClass,omitempty"`
	MatchesRequested *bool  `json:"matchesRequested,omitempty"`
	Status           int    `json:"status,omitempty"`
	Canceled         bool   `json:"canceled,omitempty"`
	Main             bool   `json:"main,omitempty"`
}

type NavigationDiagnostics struct {
	Events  []NavigationEvent `json:"events"`
	Dropped uint64            `json:"dropped"`
}

type navigationTrace struct {
	mu        sync.Mutex
	started   time.Time
	stage     string
	observed  map[string]bool
	sequence  uint64
	target    string
	requested [32]byte
	events    []NavigationEvent
	dropped   uint64
}

type navigationTraceKey struct{}

func newNavigationTrace() *navigationTrace {
	if os.Getenv("WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS") != "1" {
		return nil
	}
	return &navigationTrace{started: time.Now()}
}

// IDs are opaque CDP atoms. Never allow arbitrary error text or URLs in this
// diagnostic schema, including an unexpected browser-supplied identifier.
func navigationAtom(value string) string {
	if len(value) > 96 {
		return "invalid"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return "invalid"
		}
	}
	return value
}

func navigationErrorClass(text string, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case err != nil:
		return "protocol_error"
	case text == "net::ERR_ABORTED":
		return "aborted"
	case text == "net::ERR_BLOCKED_BY_CLIENT" || text == "net::ERR_BLOCKED_BY_RESPONSE":
		return "blocked"
	case text != "":
		return "network_error"
	default:
		return "accepted"
	}
}

// Startup is sequence zero. User requests retain their existing sequence numbers.
func (t *navigationTrace) startup(target string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.stage, t.target, t.requested = "startup", navigationAtom(target), sha256.Sum256([]byte("about:blank"))
	t.mu.Unlock()
	t.record(NavigationEvent{Event: "startup_blank_begin", Target: target}, "")
}

func (t *navigationTrace) startupResult(err error) {
	if t == nil {
		return
	}
	event := "startup_blank_ready"
	if err != nil {
		event = "startup_blank_error"
	}
	t.record(NavigationEvent{Event: event, ErrorClass: navigationErrorClass("", err)}, "")
}

func (t *navigationTrace) ready() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.stage = "ready"
	t.mu.Unlock()
	t.record(NavigationEvent{Event: "worker_ready"}, "")
}

func navigationReason(value string) string {
	switch value {
	case "", "anchorClick", "formSubmissionGet", "formSubmissionPost", "httpHeaderRefresh", "initialFrameNavigation", "metaTagRefresh", "other", "pageBlockInterstitial", "reload", "scriptInitiated", "remove", "swap":
		return value
	}
	return "other"
}
func navigationDisposition(value string) string {
	switch value {
	case "", "currentTab", "newTab", "newWindow", "download":
		return value
	}
	return "other"
}
func navigationInitiator(value string) string {
	switch value {
	case "", "parser", "script", "preload", "SignedExchange", "preflight", "FedCM", "other":
		return value
	}
	return "other"
}

func (t *navigationTrace) begin(target, url string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.sequence++
	t.stage = "navigation"
	t.target, t.requested = navigationAtom(target), sha256.Sum256([]byte(url))
	t.mu.Unlock()
	t.record(NavigationEvent{Event: "navigate_request", Target: target}, "")
}

func (t *navigationTrace) record(event NavigationEvent, url string) {
	if t == nil {
		return
	}
	event.Target = navigationAtom(event.Target)
	event.Frame = navigationAtom(event.Frame)
	event.Loader = navigationAtom(event.Loader)
	event.Request = navigationAtom(event.Request)
	event.Reason = navigationReason(event.Reason)
	event.Disposition = navigationDisposition(event.Disposition)
	event.Initiator = navigationInitiator(event.Initiator)
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.events) == navigationDiagnosticLimit {
		t.dropped++
		return
	}
	event.Milliseconds = time.Since(t.started).Milliseconds()
	event.Sequence = t.sequence
	event.Stage = t.stage
	if url != "" && event.Target == t.target {
		matches := sha256.Sum256([]byte(url)) == t.requested
		event.MatchesRequested = &matches
	}
	t.events = append(t.events, event)
}

func (t *navigationTrace) snapshot() *NavigationDiagnostics {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	events := append([]NavigationEvent{}, t.events...)
	for i := range events {
		if events[i].MatchesRequested != nil {
			value := *events[i].MatchesRequested
			events[i].MatchesRequested = &value
		}
	}
	return &NavigationDiagnostics{Events: events, Dropped: t.dropped}
}

func (t *navigationTrace) watch(ctx context.Context) {
	if t == nil {
		return
	}
	target := string(chromedp.FromContext(ctx).Target.TargetID)
	// Listen preserves cross-event ordering and observes events before the CDP
	// reply that follows them. The callback only copies bounded metadata; it
	// must never issue a command or wait for network/page work.
	t.mu.Lock()
	if t.observed == nil {
		t.observed = map[string]bool{}
	}
	if t.observed[target] {
		t.mu.Unlock()
		return
	}
	if len(t.observed) >= navigationDiagnosticLimit {
		t.dropped++
		t.mu.Unlock()
		return
	}
	t.observed[target] = true
	t.mu.Unlock()
	err := chromedp.Listen(ctx, func(event any) { t.observe(target, event) })
	if err != nil {
		t.mu.Lock()
		delete(t.observed, target)
		t.mu.Unlock()
		t.record(NavigationEvent{Event: "observer_error", Target: target, ErrorClass: "protocol_error"}, "")
	} else {
		t.record(NavigationEvent{Event: "observer_ready", Target: target}, "")
	}
}

func (t *navigationTrace) observe(target string, event any) {
	if t == nil {
		return
	}
	switch e := event.(type) {
	case *page.EventFrameRequestedNavigation:
		t.record(NavigationEvent{Event: "renderer_navigation", Target: target, Frame: string(e.FrameID), Reason: string(e.Reason), Disposition: string(e.Disposition)}, e.URL)
	case *page.EventFrameStartedLoading:
		t.record(NavigationEvent{Event: "frame_loading_started", Target: target, Frame: string(e.FrameID)}, "")
	case *page.EventFrameStoppedLoading:
		t.record(NavigationEvent{Event: "frame_loading_stopped", Target: target, Frame: string(e.FrameID)}, "")
	case *page.EventFrameDetached:
		t.record(NavigationEvent{Event: "frame_detached", Target: target, Frame: string(e.FrameID), Reason: string(e.Reason)}, "")
	case *page.EventFrameStartedNavigating:
		t.record(NavigationEvent{Event: "navigation_started", Target: target, Frame: string(e.FrameID), Loader: string(e.LoaderID)}, e.URL)
	case *page.EventFrameNavigated:
		if e.Frame != nil {
			t.record(NavigationEvent{Event: "navigation_committed", Target: target, Frame: string(e.Frame.ID), Loader: string(e.Frame.LoaderID), Main: e.Frame.ParentID == ""}, e.Frame.URL)
		}
	case *network.EventRequestWillBeSent:
		if e.Type == network.ResourceTypeDocument && e.Request != nil {
			initiator := ""
			if e.Initiator != nil {
				initiator = string(e.Initiator.Type)
			}
			t.record(NavigationEvent{Initiator: initiator, Redirected: e.RedirectResponse != nil, Event: "document_request", Target: target, Frame: string(e.FrameID), Loader: string(e.LoaderID), Request: string(e.RequestID)}, e.Request.URL)
		}
	case *network.EventResponseReceived:
		if e.Type == network.ResourceTypeDocument && e.Response != nil {
			t.record(NavigationEvent{Event: "document_response", Target: target, Frame: string(e.FrameID), Loader: string(e.LoaderID), Request: string(e.RequestID), Status: int(e.Response.Status)}, e.Response.URL)
		}
	case *network.EventLoadingFailed:
		if e.Type == network.ResourceTypeDocument {
			t.record(NavigationEvent{Event: "document_failed", Target: target, Request: string(e.RequestID), ErrorClass: navigationErrorClass(e.ErrorText, nil), Canceled: e.Canceled}, "")
		}
	}
}
