package browser

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/target"
)

func TestModalViewportDefersLatestSizeAndReportsActualDimensions(t *testing.T) {
	id := target.ID("modal")
	b := companion{width: 1100, height: 760, dialogs: map[target.ID]*Dialog{id: {Type: "confirm"}}}
	// No Chrome context exists: a modal resize must return without CDP I/O.
	for _, width := range []int64{900, 1000} {
		if err := b.resizeViewport(context.Background(), id, width, 500); err != nil {
			t.Fatal(err)
		}
		if b.pendingWidth != width || b.pendingHeight != 500 || b.width != 1100 || b.height != 760 {
			t.Fatal("lost pending or actual size", b.pendingWidth, b.pendingHeight, b.width, b.height)
		}
	}
	if err := b.resizeViewport(context.Background(), id, 1100, 760); err != nil || b.pendingWidth != 0 || b.pendingHeight != 0 {
		t.Fatal("return to original layout did not cancel pending resize", err)
	}
	if err := b.resizeViewport(context.Background(), id, 99999, 500); err == nil || b.width != 1100 || b.pendingWidth != 0 {
		t.Fatal("invalid resize changed state", err)
	}
}
