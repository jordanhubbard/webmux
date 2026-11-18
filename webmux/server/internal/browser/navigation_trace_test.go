package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/tracing"
	"github.com/chromedp/chromedp"
)

type firstNavigationTrace struct {
	Complete      bool                        `json:"complete"`
	DataLoss      bool                        `json:"dataLoss"`
	Bytes         int                         `json:"bytes"`
	Scanned       int                         `json:"scanned"`
	Dropped       int                         `json:"dropped"`
	MatchedEvents int                         `json:"matchedEvents"`
	IdentityLimit bool                        `json:"identityLimit,omitempty"`
	Events        []firstNavigationTraceEvent `json:"events"`
}

type firstNavigationTraceEvent struct {
	Name             string `json:"name"`
	Phase            string `json:"phase"`
	Microseconds     int64  `json:"relativeUs"`
	Navigation       int    `json:"navigation,omitempty"`
	MatchesRequested *bool  `json:"matchesRequested,omitempty"`
	State            *int   `json:"state,omitempty"`
	NetError         *int   `json:"netError,omitempty"`
	Committed        *bool  `json:"committed,omitempty"`
	BrowserInitiated *bool  `json:"browserInitiated,omitempty"`
	Reason           string `json:"reason,omitempty"`
	NumericReason    *int   `json:"numericReason,omitempty"`
	ThrottleEvent    *int   `json:"throttleEvent,omitempty"`
}

type firstNavigationTraceCapture struct {
	mu       sync.Mutex
	values   []json.RawMessage
	bytes    int
	scanned  int
	dropped  int
	complete bool
	lost     bool
	done     chan struct{}
	once     sync.Once
	unlisten context.CancelFunc
}

func startFirstNavigationTrace(root context.Context) (*firstNavigationTraceCapture, error) {
	c := &firstNavigationTraceCapture{done: make(chan struct{})}
	listen, unlisten := context.WithCancel(root)
	c.unlisten = unlisten
	err := chromedp.ListenBrowser(listen, func(event any) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch e := event.(type) {
		case *tracing.EventDataCollected:
			for _, value := range e.Value {
				c.appendValue(value)
			}
		case *tracing.EventTracingComplete:
			c.complete, c.lost = true, e.DataLossOccurred
			c.once.Do(func() { close(c.done) })
		}
	})
	if err != nil {
		unlisten()
		return nil, fmt.Errorf("cannot observe anonymous browser tracing")
	}
	ctx, finish := context.WithTimeout(root, 5*time.Second)
	defer finish()
	_, err = cdp.Call(ctx, chromedp.FromContext(root).Browser, tracing.Start, tracing.StartParams{
		TransferMode: tracing.StartTransferModeReportEvents,
		TraceConfig: &tracing.TraceConfig{
			RecordMode: tracing.TraceConfigRecordModeRecordUntilFull, TraceBufferSizeInKb: 1024,
			IncludedCategories: []string{"navigation"}, ExcludedCategories: []string{"*"},
		},
	})
	if err != nil {
		unlisten()
		return nil, fmt.Errorf("cannot start anonymous browser tracing")
	}
	return c, nil
}

// Caller holds mu. Copy bounded raw buckets in the CDP callback; project only
// after tracingComplete, outside the browser's message-dispatch callback.
func (c *firstNavigationTraceCapture) appendValue(value []byte) {
	c.scanned++
	if c.bytes+len(value) > 2*1024*1024 || len(c.values) == 8192 {
		c.dropped++
		return
	}
	c.values = append(c.values, append(json.RawMessage{}, value...))
	c.bytes += len(value)
}

func (c *firstNavigationTraceCapture) finish(root context.Context, requested string) (*firstNavigationTrace, error) {
	defer c.unlisten()
	ctx, cancel := context.WithTimeout(root, 5*time.Second)
	defer cancel()
	_, err := cdp.Call(ctx, chromedp.FromContext(root).Browser, tracing.End, cdp.Empty{})
	if err == nil {
		select {
		case <-c.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := projectFirstNavigationTrace(c.values, requested)
	result.Complete, result.DataLoss = c.complete, c.lost
	result.Bytes, result.Scanned = c.bytes, c.scanned
	result.Dropped += c.dropped
	if err != nil || !result.Complete || result.DataLoss || result.Dropped != 0 || result.IdentityLimit {
		return result, fmt.Errorf("anonymous navigation trace incomplete or truncated")
	}
	return result, nil
}

type firstNavigationTraceNav struct {
	URL              string          `json:"url"`
	ID               int64           `json:"navigation_id"`
	State            *int            `json:"state"`
	NetError         *int            `json:"net_error"`
	Committed        *bool           `json:"has_committed"`
	BrowserInitiated *bool           `json:"browser_initiated"`
	Discard          json.RawMessage `json:"navigation_discard_reason"`
	Reason           json.RawMessage `json:"discard_reason"`
}

func projectFirstNavigationTrace(values []json.RawMessage, requested string) *firstNavigationTrace {
	result := &firstNavigationTrace{Events: []firstNavigationTraceEvent{}}
	aliases := map[int64]int{}
	var origin int64
	for _, value := range values {
		var raw struct {
			Name string  `json:"name"`
			Ph   string  `json:"ph"`
			Ts   float64 `json:"ts"`
			Args struct {
				URL          string                   `json:"url"`
				Reason       json.RawMessage          `json:"reason"`
				Discard      json.RawMessage          `json:"navigation_discard_reason"`
				CurrentEvent *int                     `json:"current_event"`
				Navigation   *firstNavigationTraceNav `json:"navigation"`
				Request      *firstNavigationTraceNav `json:"navigation_request"`
				Handle       *firstNavigationTraceNav `json:"navigation_handle"`
			} `json:"args"`
		}
		if json.Unmarshal(value, &raw) != nil || raw.Ts < 0 || raw.Ts > 1e16 {
			result.Dropped++
			continue
		}
		switch raw.Name {
		case "NavigationRequest::Create", "NavigationRequest::NavigationRequest", "NavigationRequest::~NavigationRequest", "NavigationRequest::BeginNavigation", "NavigationRequest::BeginNavigationImpl", "NavigationRequest::StartNavigation", "NavigationRequest::WillStartRequest", "NavigationRequest::OnStartChecksComplete", "NavigationRequest::OnResponseStarted", "NavigationRequest::OnRequestFailed", "NavigationRequest::OnRequestFailedInternal", "NavigationRequest::OnFailureChecksComplete", "NavigationRequest::CancelDeferredNavigation", "NavigationRequest::ReadyToCommitNavigation", "NavigationRequest::CommitNavigation", "NavigationRequest::DidCommitNavigation", "Navigator::Navigate", "Navigator::CancelNavigation", "FrameTreeNode::ResetNavigationRequest", "FrameTreeNode::CancelRestartingBackForwardCacheNavigation", "FrameTreeNode::DidStopLoading", "RenderFrameHostImpl::NavigationRequestCancelled", "RenderFrameHostImpl::Stop", "RenderFrameHostImpl::ResetChildren", "NavigationThrottleRegistryImpl::RegisterNavigationThrottles", "NavigationThrottleRunner::ProcessInternal", "NavigationURLLoaderImpl::Start", "NavigationURLLoaderImpl::OnComplete", "NavigationURLLoaderImpl::OnReceiveResponse", "WebContentsImpl::DidStartNavigation", "WebContentsImpl::DidFinishNavigation", "NavigationRequest":
		default:
			continue
		}
		if len(result.Events) == 256 {
			result.Dropped++
			continue
		}
		if len(result.Events) == 0 {
			origin = int64(raw.Ts)
		}
		phase := raw.Ph
		switch phase {
		case "B", "E", "X", "I", "i", "b", "e", "n", "s", "f":
		default:
			phase = "other"
		}
		row := firstNavigationTraceEvent{Name: raw.Name, Phase: phase, Microseconds: int64(raw.Ts) - origin}
		row.ThrottleEvent = traceBoundedInt(raw.Args.CurrentEvent)
		nav := raw.Args.Navigation
		if nav == nil {
			nav = raw.Args.Request
		}
		if nav == nil {
			nav = raw.Args.Handle
		}
		url := raw.Args.URL
		reason := raw.Args.Reason
		if len(raw.Args.Discard) != 0 {
			reason = raw.Args.Discard
		}
		if nav != nil {
			if nav.ID > 0 && nav.ID < 1<<53 && (aliases[nav.ID] != 0 || len(aliases) < 64) {
				if aliases[nav.ID] == 0 {
					aliases[nav.ID] = len(aliases) + 1
				}
				row.Navigation = aliases[nav.ID]
			} else if nav.ID > 0 && nav.ID < 1<<53 {
				result.IdentityLimit = true
			}
			url = nav.URL
			row.State, row.NetError = traceBoundedInt(nav.State), traceBoundedInt(nav.NetError)
			row.Committed, row.BrowserInitiated = nav.Committed, nav.BrowserInitiated
			if len(nav.Discard) != 0 {
				reason = nav.Discard
			} else if len(nav.Reason) != 0 {
				reason = nav.Reason
			}
		}
		if url != "" && requested != "" {
			matches := url == requested
			row.MatchesRequested = &matches
			if matches {
				result.MatchedEvents++
			}
		}
		if len(reason) != 0 && string(reason) != "null" {
			var number int
			var text string
			if json.Unmarshal(reason, &number) == nil {
				row.NumericReason = traceBoundedInt(&number)
			} else if json.Unmarshal(reason, &text) == nil {
				row.Reason = firstNavigationDiscardReason(text)
			}
		}
		result.Events = append(result.Events, row)
	}
	return result
}

func traceBoundedInt(value *int) *int {
	if value == nil || *value < -100000 || *value > 100000 {
		return nil
	}
	return value
}

func firstNavigationDiscardReason(value string) string {
	value = strings.TrimPrefix(value, "NavigationDiscardReason::")
	value = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(value, "k"), "_", ""))
	switch value {
	case "newreloadnavigation":
		return "new_reload"
	case "newhistorynavigation":
		return "new_history"
	case "newothernavigationbrowserinitiated":
		return "new_browser_navigation"
	case "newothernavigationrendererinitiated":
		return "new_renderer_navigation"
	case "willremoveframe":
		return "frame_removed"
	case "explicitcancellation":
		return "explicit_cancellation"
	case "internalcancellation":
		return "internal_cancellation"
	case "committednavigation":
		return "committed_navigation"
	case "renderprocessgone":
		return "renderer_gone"
	case "renderframehostdestruction":
		return "frame_host_destroyed"
	case "neverstarted":
		return "never_started"
	case "failedsecuritycheck":
		return "failed_security_check"
	case "newduplicatenavigation":
		return "duplicate_navigation"
	default:
		return "other"
	}
}
