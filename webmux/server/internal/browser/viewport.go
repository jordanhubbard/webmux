package browser

import (
	"context"
	"errors"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

func (b *companion) resizeViewport(ctx context.Context, id target.ID, width, height int64) error {
	if width < 320 || width > 2560 || height < 240 || height > 1600 {
		return errors.New("Browser viewport must be 320–2560 by 240–1600 pixels")
	}
	// Only the latest layout matters. Returning to the actual size also cancels
	// a queued intermediate size, without sending CDP work into a modal page.
	b.pendingWidth, b.pendingHeight = width, height
	if b.width == width && b.height == height {
		b.pendingWidth, b.pendingHeight = 0, 0
		return nil
	}
	return b.applyViewport(ctx, id)
}

func (b *companion) applyViewport(ctx context.Context, id target.ID) error {
	if b.pendingWidth == 0 {
		return nil
	}
	if b.hasDialog(id) {
		// Chromium can hold EmulateViewport until its JavaScript dialog closes.
		// Waiting here would prevent the same serial worker from handling that
		// dialog. Keep one bounded latest size and report the actual old size.
		return nil
	}
	width, height := b.pendingWidth, b.pendingHeight
	if err := chromedp.Do(ctx, chromedp.EmulateViewport(width, height)); err != nil {
		return err
	}
	b.width, b.height = width, height
	b.pendingWidth, b.pendingHeight = 0, 0
	return nil
}

func (b *companion) hasDialog(id target.ID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dialogs[id] != nil
}
