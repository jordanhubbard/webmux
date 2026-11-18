package browser

import (
	"context"
	"os"
	"testing"
	"time"

	browsercdp "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func TestFirstNavigationTraceReleasePreservesBrowserWithoutOpeningTarget(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 to run installed Chromium")
	}
	alloc, stop := chromedp.NewExecAllocator(context.Background(), workerAllocatorOptions(t.TempDir())...)
	root, cancel := chromedp.NewContext(alloc)
	t.Cleanup(func() {
		if err := closeBrowserProcesses(root, cancel, stop, false); err != nil {
			t.Errorf("trace owner cleanup failed: %v", err)
		}
	})
	capture, err := startFirstNavigationTrace(root, false)
	if err != nil {
		t.Fatal("trace did not start before target allocation")
	}
	c := chromedp.FromContext(root)
	if c.Target != nil || c.Browser == nil || c.Browser.Process() == nil {
		t.Fatal("trace startup changed target/allocation scope")
	}
	originalPID := c.Browser.Process().Pid
	if result, err := capture.finish(root, ""); err != nil || !result.Complete {
		t.Fatal("trace did not finish on its owner")
	}
	if c.Target != nil {
		t.Fatal("releasing trace created a target")
	}
	// The trace's cancellable listener has ended. Its browser owner must still
	// serve new protocol work and permit the first target to attach, with no
	// allocation restart or navigation added to the pre-target trace mode.
	ctx, finish := context.WithTimeout(root, 5*time.Second)
	defer finish()
	if _, err := cdp.Call(ctx, c.Browser, browsercdp.GetVersion, cdp.Empty{}); err != nil {
		t.Fatal("releasing the first listener stopped its browser")
	}
	if err := chromedp.Do(ctx, chromedp.Func(func(context.Context, *chromedp.Target) error { return nil })); err != nil {
		t.Fatal("browser could not attach its first target after trace release")
	}
	if c.Browser.Process().Pid != originalPID {
		t.Fatal("browser was replaced after trace release")
	}
}
