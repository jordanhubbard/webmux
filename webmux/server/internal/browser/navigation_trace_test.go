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
	EndElapsedMs    int64                       `json:"endElapsedMs"`
	EndAcknowledged bool                        `json:"endAcknowledged"`
	ErrorClass      string                      `json:"errorClass,omitempty"`
	Complete        bool                        `json:"complete"`
	DataLoss        bool                        `json:"dataLoss"`
	Bytes           int                         `json:"bytes"`
	Scanned         int                         `json:"scanned"`
	Dropped         int                         `json:"dropped"`
	MatchedEvents   int                         `json:"matchedEvents"`
	IdentityLimit   bool                        `json:"identityLimit,omitempty"`
	Events          []firstNavigationTraceEvent `json:"events"`
}

type firstNavigationTraceEvent struct {
	Name             string `json:"name"`
	Phase            string `json:"phase"`
	Microseconds     int64  `json:"relativeUs"`
	Navigation       int    `json:"navigation,omitempty"`
	Frame            int    `json:"frame,omitempty"`
	Flow             int    `json:"flow,omitempty"`
	MainFrame        *bool  `json:"mainFrame,omitempty"`
	URLClass         string `json:"urlClass,omitempty"`
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

func startFirstNavigationTrace(root context.Context, loading bool) (*firstNavigationTraceCapture, error) {
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
	categories := []string{"navigation"}
	if loading {
		categories = append(categories, "loading")
	}
	_, err = cdp.Call(ctx, chromedp.FromContext(root).Browser, tracing.Start, tracing.StartParams{
		TransferMode: tracing.StartTransferModeReportEvents,
		TraceConfig: &tracing.TraceConfig{
			RecordMode: tracing.TraceConfigRecordModeRecordUntilFull, TraceBufferSizeInKb: 1024,
			IncludedCategories: categories, ExcludedCategories: []string{"*"},
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
	// Chromium acknowledges End before asynchronously stopping Perfetto and
	// reading the trace (TracingHandler::End / OnTracingSessionStopped).
	// Keep this post-navigation collection deadline separate from navigation.
	started := time.Now()
	ctx, cancel := context.WithTimeout(root, 15*time.Second)
	defer cancel()
	_, err := cdp.Call(ctx, chromedp.FromContext(root).Browser, tracing.End, cdp.Empty{})
	endAcknowledged := err == nil
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
	result.EndElapsedMs = time.Since(started).Milliseconds()
	result.EndAcknowledged = endAcknowledged
	result.Complete, result.DataLoss = c.complete, c.lost
	result.Bytes, result.Scanned = c.bytes, c.scanned
	result.Dropped += c.dropped
	if err != nil || !result.Complete || result.DataLoss || result.Dropped != 0 || result.IdentityLimit {
		if err != nil {
			result.ErrorClass = navigationErrorClass("", err)
		} else {
			result.ErrorClass = "incomplete_or_truncated"
		}
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
	Frame            json.RawMessage `json:"frame_tree_node"`
}

type firstNavigationTraceFrame struct {
	ID   int64 `json:"frame_tree_node_id"`
	Main *bool `json:"is_main_frame"`
}

func projectFirstNavigationTrace(values []json.RawMessage, requested string) *firstNavigationTrace {
	result := &firstNavigationTrace{Events: []firstNavigationTraceEvent{}}
	aliases := map[int64]int{}
	frames := map[int64]int{}
	flows := map[string]int{}
	var origin int64
	for _, value := range values {
		var raw struct {
			Name  string          `json:"name"`
			Ph    string          `json:"ph"`
			Ts    float64         `json:"ts"`
			ID    json.RawMessage `json:"id"`
			PID   int64           `json:"pid"`
			Scope string          `json:"scope"`
			ID2   struct {
				Global json.RawMessage `json:"global"`
				Local  json.RawMessage `json:"local"`
			} `json:"id2"`
			Args struct {
				URL          string                   `json:"url"`
				Reason       json.RawMessage          `json:"reason"`
				Discard      json.RawMessage          `json:"navigation_discard_reason"`
				CurrentEvent *int                     `json:"current_event"`
				Frame        json.RawMessage          `json:"frame_tree_node"`
				FrameInfo    json.RawMessage          `json:"frame_tree_node_info"`
				FrameHost    json.RawMessage          `json:"render_frame_host"`
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
		case "CorsURLLoader::CorsURLLoader", "CreateNetworkContextInNetworkService", "CreateNetworkContextInternal", "CreateInProcessNetworkServiceOnThread", "CreateInProcessNetworkService", "CorsURLLoaderFactory::CreateLoaderAndStart", "ThrottlingURLLoader::StartNow", "NavigationRequest::Create", "NavigationRequest::NavigationRequest", "NavigationRequest::~NavigationRequest", "NavigationRequest::BeginNavigation", "NavigationRequest::BeginNavigationImpl", "NavigationRequest::StartNavigation", "NavigationRequest::WillStartRequest", "NavigationRequest::OnStartChecksComplete", "NavigationRequest::OnResponseStarted", "NavigationRequest::OnRequestFailed", "NavigationRequest::OnRequestFailedInternal", "NavigationRequest::OnFailureChecksComplete", "NavigationRequest::CancelDeferredNavigation", "NavigationRequest::ReadyToCommitNavigation", "NavigationRequest::CommitNavigation", "NavigationRequest::DidCommitNavigation", "Navigator::Navigate", "Navigator::CancelNavigation", "FrameTreeNode::ResetNavigationRequest", "FrameTreeNode::CancelRestartingBackForwardCacheNavigation", "FrameTreeNode::DidStopLoading", "RenderFrameHostImpl::NavigationRequestCancelled", "RenderFrameHostImpl::Stop", "RenderFrameHostImpl::ResetChildren", "NavigationThrottleRegistryImpl::RegisterNavigationThrottles", "NavigationThrottleRunner::ProcessInternal", "NavigationURLLoaderImpl::CreateInterceptors", "NavigationURLLoaderImpl::Restart", "NavigationURLLoaderImpl::CreateThrottlingLoaderAndStart", "NavigationURLLoaderImpl::CreateURLLoaderThrottles", "NavigationURLLoaderImpl::MaybeCreateLoaderForResponse", "NavigationURLLoaderImpl::NotifyRequestFailed", "NavigationURLLoaderImpl::Start", "NavigationURLLoaderImpl::OnComplete", "NavigationURLLoaderImpl::OnReceiveResponse", "WebContentsImpl::DidStartNavigation", "WebContentsImpl::DidFinishNavigation", "NavigationRequest":
		default:
			continue
		}
		// The network-service constructor exposes the requested URL. Retain
		// only its exact fixture match, with the URL projected to a boolean.
		if raw.Name == "CorsURLLoader::CorsURLLoader" && (raw.Ph != "X" || raw.Args.URL != requested) {
			continue
		}
		// Keep only the two handoff scopes from loading. Background requests
		// also emit constructor/response/destructor and flow bookkeeping; those
		// exhausted the projection in native Windows captures. Navigation
		// events and their flow edges remain unchanged.
		if (raw.Name == "ThrottlingURLLoader::StartNow" || raw.Name == "CorsURLLoaderFactory::CreateLoaderAndStart") && raw.Ph != "X" {
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
		// Flow identities are opaque, bounded, capture-local aliases. Preserve
		// explicit process-local versus global scope without exporting raw IDs.
		if phase == "s" || phase == "f" {
			key := traceFlowIdentifier(raw.ID)
			if global := traceFlowIdentifier(raw.ID2.Global); global != "" {
				key = "global:" + global
			} else if local := traceFlowIdentifier(raw.ID2.Local); local != "" {
				key = fmt.Sprintf("local:%d:%s", raw.PID, local)
			}
			if key != "" && len(raw.Scope) <= 64 {
				key += ":" + raw.Scope
				if flows[key] == 0 && len(flows) < 64 {
					flows[key] = len(flows) + 1
				}
				row.Flow = flows[key]
				if row.Flow == 0 {
					result.IdentityLimit = true
				}
			}
		}
		row.ThrottleEvent = traceBoundedInt(raw.Args.CurrentEvent)
		nav := raw.Args.Navigation
		if nav == nil {
			nav = raw.Args.Request
		}
		if nav == nil {
			nav = raw.Args.Handle
		}
		url := raw.Args.URL
		frame := firstNavigationFrame(raw.Args.Frame)
		if frame == nil {
			frame = firstNavigationFrame(raw.Args.FrameInfo)
		}
		if frame == nil {
			frame = firstNavigationFrame(raw.Args.FrameHost)
		}
		reason := raw.Args.Reason
		if len(raw.Args.Discard) != 0 {
			reason = raw.Args.Discard
		}
		if nav != nil {
			if navFrame := firstNavigationFrame(nav.Frame); navFrame != nil {
				frame = navFrame
			}
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
		if frame != nil {
			row.MainFrame = frame.Main
			if frame.ID > 0 && frame.ID < 1<<53 && (frames[frame.ID] != 0 || len(frames) < 64) {
				if frames[frame.ID] == 0 {
					frames[frame.ID] = len(frames) + 1
				}
				row.Frame = frames[frame.ID]
			} else if frame.ID > 0 && frame.ID < 1<<53 {
				result.IdentityLimit = true
			}
		}
		if url != "" && requested != "" {
			matches := url == requested
			row.MatchesRequested = &matches
			if matches {
				result.MatchedEvents++
			}
		}
		switch {
		case url == "":
		case requested != "" && url == requested:
			row.URLClass = "fixture"
		case url == "about:blank":
			row.URLClass = "about_blank"
		case strings.HasPrefix(url, "data:"):
			row.URLClass = "data"
		case strings.HasPrefix(url, "chrome://") || strings.HasPrefix(url, "chrome-extension://"):
			row.URLClass = "browser_internal"
		default:
			row.URLClass = "other"
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

// Chromium uses both object snapshots and opaque pointer strings for these
// optional fields. Only the object form contributes a local frame alias.
func firstNavigationFrame(value json.RawMessage) *firstNavigationTraceFrame {
	var frame firstNavigationTraceFrame
	if json.Unmarshal(value, &frame) != nil || (frame.ID == 0 && frame.Main == nil) {
		return nil
	}
	return &frame
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

// A trace ID can be an integer or a hex string. Reject structured/unbounded
// input; only the resulting local alias is ever included in a record.
func traceFlowIdentifier(raw json.RawMessage) string {
	if len(raw) == 0 || len(raw) > 80 {
		return ""
	}
	value := string(raw)
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return ""
		}
	}
	if len(value) == 0 || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' || r == 'x') {
			return ""
		}
	}
	return value
}
