package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func TestFirstNavigationTraceProjection(t *testing.T) {
	values := []json.RawMessage{
		json.RawMessage(`{"name":"NavigationRequest::NavigationRequest","ph":"X","ts":1000,"args":{"navigation":{"url":"http://private.invalid/check","navigation_id":987654,"state":0,"net_error":0,"browser_initiated":true,"has_committed":false,"frame_tree_node":{"current_frame_host":{"origin":"secret-origin"}}}}}`),
		json.RawMessage(`{"name":"WebContentsImpl::DidFinishNavigation","ph":"secret-phase","ts":1005,"args":{"navigation_handle":{"url":"http://private.invalid/check","navigation_id":987654,"net_error":-3,"discard_reason":"kExplicitCancellation"},"headers":"secret-cookie"}}`),
		json.RawMessage(`{"name":"Navigator::CancelNavigation","ph":"X","ts":1006,"args":{"reason":"secret arbitrary reason"}}`),
		json.RawMessage(`{"name":"Navigation: http://private.invalid/check","ph":"X","ts":1007,"args":{}}`),
	}
	r := projectFirstNavigationTrace(values, "http://private.invalid/check")
	if len(r.Events) != 3 || r.MatchedEvents != 2 || r.Events[1].Navigation != 1 || r.Events[1].Reason != "explicit_cancellation" || r.Events[1].Phase != "other" || *r.Events[1].NetError != -3 || r.Events[1].Microseconds != 5 || r.Events[2].Reason != "other" {
		t.Fatal("trace projection lost navigation or cancellation metadata", r)
	}
	encoded, _ := json.Marshal(r)
	for _, private := range []string{"secret", "987654", "http", "private.invalid", "origin", "headers"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private trace content escaped: %q", private)
		}
	}
}

func TestFirstNavigationTraceBounds(t *testing.T) {
	c := &firstNavigationTraceCapture{}
	c.appendValue(make([]byte, 2*1024*1024+1))
	if len(c.values) != 0 || c.dropped != 1 {
		t.Fatal("raw byte limit not enforced")
	}
	for i := 0; i < 8193; i++ {
		c.appendValue([]byte(`{}`))
	}
	if len(c.values) != 8192 || c.dropped != 2 {
		t.Fatal("raw event limit not enforced")
	}
	var values []json.RawMessage
	for i := 1; i <= 300; i++ {
		values = append(values, json.RawMessage(fmt.Sprintf(`{"name":"NavigationRequest","ph":"b","ts":1000,"args":{"navigation":{"navigation_id":%d,"url":"fixture"}}}`, i)))
	}
	r := projectFirstNavigationTrace(values, "fixture")
	if len(r.Events) != 256 || r.Dropped != 44 || !r.IdentityLimit || r.Events[63].Navigation != 64 || r.Events[64].Navigation != 0 {
		t.Fatal("projection bounds not enforced", r)
	}
	invalid := projectFirstNavigationTrace([]json.RawMessage{json.RawMessage(`{`), json.RawMessage(`{"name":"NavigationRequest","ts":-1}`)}, "fixture")
	if invalid.Dropped != 2 || len(invalid.Events) != 0 {
		t.Fatal("invalid trace events accepted")
	}
	if firstNavigationDiscardReason("NavigationDiscardReason::kNewOtherNavigationRendererInitiated") != "new_renderer_navigation" || firstNavigationDiscardReason("INTERNAL_CANCELLATION") != "internal_cancellation" {
		t.Fatal("known reason spelling lost")
	}
}

// This intentionally canceled load is a capture control, separate from the
// first-load acceptance matrix. It never retries a failed navigation.
func TestNativeNavigationTraceCancellation(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 for native cancellation capture")
	}
	if !filepath.IsAbs(os.Getenv("WEBMUX_BROWSER_EXECUTABLE")) {
		t.Fatal("explicit Chromium executable required")
	}
	site, hits := firstNavigationNodeFixture(t)
	var trace *firstNavigationTrace
	issued, confirmed := false, false
	navigateCalls := 0
	t.Cleanup(func() {
		dir := os.Getenv("WEBMUX_BROWSER_NAVIGATION_ARTIFACT_DIR")
		if dir == "" {
			return
		}
		record := struct {
			Mode                    string                `json:"mode"`
			Platform                string                `json:"platform"`
			Architecture            string                `json:"architecture"`
			IntentionalCancellation bool                  `json:"intentionalCancellation"`
			StopIssued              bool                  `json:"stopIssued"`
			ControlConfirmed        bool                  `json:"controlConfirmed"`
			NavigateCalls           int                   `json:"navigateCalls"`
			FixtureRequests         int64                 `json:"fixtureRequests"`
			BrowserTrace            *firstNavigationTrace `json:"browserTrace,omitempty"`
		}{"controlled-stop-trace", runtime.GOOS, runtime.GOARCH, true, issued, confirmed, navigateCalls, hits.Load(), trace}
		data, err := json.MarshalIndent(record, "", "  ")
		if err == nil {
			err = os.MkdirAll(dir, 0700)
		}
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, "controlled-stop-trace.json"), append(data, '\n'), 0600)
		}
		if err != nil {
			t.Error("cannot retain bounded cancellation-control record")
		}
	})
	alloc, stop := chromedp.NewExecAllocator(context.Background(), workerAllocatorOptions(t.TempDir())...)
	root, cancel := chromedp.NewContext(alloc)
	t.Cleanup(func() { cancel(); stop() })
	startup := time.AfterFunc(20*time.Second, cancel)
	t.Cleanup(func() { startup.Stop() })
	capture, err := startFirstNavigationTrace(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var err error
		trace, err = capture.finish(root, site+"/pending")
		if err != nil {
			t.Error(err)
		}
		for _, event := range trace.Events {
			if event.MatchesRequested != nil && *event.MatchesRequested && event.NetError != nil && *event.NetError == -3 {
				confirmed = err == nil && issued && hits.Load() == 1
			}
		}
		if !confirmed {
			t.Error("known requested cancellation was not captured")
		}
	})
	err = chromedp.Do(root, chromedp.EmulateViewport(Width, Height), chromedp.Navigate("about:blank"))
	startup.Stop()
	if err != nil {
		t.Fatal("cancellation-control browser startup failed")
	}
	ctx, finish := context.WithTimeout(root, 15*time.Second)
	defer finish()
	target := chromedp.FromContext(root).Target
	done := make(chan error, 1)
	navigateCalls++
	go func() {
		_, err := cdp.Call(ctx, target, page.Navigate, page.NavigateParams{URL: site + "/pending"})
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hits.Load() != 1 {
		t.Fatal("hanging fixture request was not received; controlled stop not established")
	}
	issued = true
	if _, err := cdp.Call(ctx, target, page.StopLoading, cdp.Empty{}); err != nil {
		t.Fatal("controlled stop failed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("controlled navigation protocol failed")
		}
	case <-ctx.Done():
		t.Fatal("controlled navigation did not finish")
	}
}
