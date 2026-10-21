// Package browser provides an ephemeral browser over private stdio pipes.
package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const Width, Height = 1100, 760

type Request struct {
	Action    string  `json:"action"`
	URL       string  `json:"url,omitempty"`
	Text      string  `json:"text,omitempty"`
	Key       string  `json:"key,omitempty"`
	Code      string  `json:"code,omitempty"`
	KeyCode   int64   `json:"keyCode,omitempty"`
	Modifiers int64   `json:"modifiers,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	DeltaY    float64 `json:"deltaY,omitempty"`
	Target    string  `json:"target,omitempty"`
}

type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Frame struct {
	Image  []byte `json:"image,omitempty"`
	URL    string `json:"url"`
	Target string `json:"target"`
	Tabs   []Tab  `json:"tabs"`
	Error  string `json:"error,omitempty"`
}

func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

// Worker is also run on SSH hosts. It exposes no listener and exits on stdin EOF.
func Worker(in io.Reader, out io.Writer) error {
	profile, err := os.MkdirTemp("", "webmux-browser-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profile)
	// Retain Chromium's normal isolation and safe-browsing defaults. The generic
	// automation defaults disable protections that are useful for an auth browser.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck, chromedp.Headless,
		chromedp.UserDataDir(profile), chromedp.WindowSize(Width, Height),
		chromedp.Flag("no-sandbox", false), chromedp.Flag("enable-automation", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	}
	if path := os.Getenv("WEBMUX_BROWSER_EXECUTABLE"); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	alloc, stop := chromedp.NewExecAllocator(context.Background(), opts...)
	defer stop()
	root, cancel := chromedp.NewContext(alloc)
	defer cancel()
	// Start outside an operation timeout: cancelling the first Run context would
	// otherwise tear down the browser after a successful first response.
	startup := time.AfterFunc(20*time.Second, cancel)
	err = chromedp.Do(root, chromedp.EmulateViewport(Width, Height), chromedp.Navigate("about:blank"))
	startup.Stop()
	if err != nil {
		_ = json.NewEncoder(out).Encode(Frame{Error: "Cannot start Chromium. Install Chrome/Chromium on the terminal host and run as a non-root user."})
		return err
	}
	tabs := map[target.ID]context.Context{chromedp.FromContext(root).Target.TargetID: root}
	current := root
	requests := make(chan Request)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(requests)
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var req Request
			if json.Unmarshal(scanner.Bytes(), &req) != nil {
				return
			}
			select {
			case requests <- req:
			case <-done:
				return
			}
		}
	}()
	idle := time.NewTimer(30 * time.Minute)
	defer idle.Stop()
	encoder := json.NewEncoder(out)
	for {
		select {
		case <-idle.C:
			return nil
		case req, ok := <-requests:
			if !ok {
				return nil
			}
			idle.Reset(30 * time.Minute)
			if req.Action == "tab" {
				infos, err := chromedp.Targets(root)
				if err != nil {
					return err
				}
				for _, info := range infos {
					if info.Type != "page" || string(info.TargetID) != req.Target {
						continue
					}
					ctx, exists := tabs[info.TargetID]
					if !exists {
						var release context.CancelFunc
						ctx, release = chromedp.NewContext(root, chromedp.WithTargetID(info.TargetID))
						defer release()
						if err := chromedp.Do(ctx, chromedp.EmulateViewport(Width, Height)); err != nil {
							return err
						}
						tabs[info.TargetID] = ctx
					}
					current = ctx
				}
			}
			ctx, finish := context.WithTimeout(current, 15*time.Second)
			frame := Frame{Target: string(chromedp.FromContext(current).Target.TargetID), Tabs: []Tab{}}
			err := perform(ctx, req)
			if err == nil {
				// A redirect can invalidate the execution context or stall a
				// screenshot. Bound frame capture separately so polling and queued
				// input recover without waiting for the full action timeout.
				frameCtx, stopFrame := context.WithTimeout(ctx, 3*time.Second)
				err = chromedp.Do(frameCtx, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
					var err error
					frame.URL, err = chromedp.Location()(ctx, target)
					if err != nil {
						return err
					}
					quality := int64(65)
					result, err := cdp.Call(ctx, target, page.CaptureScreenshot, page.CaptureScreenshotParams{
						Format: page.CaptureScreenshotFormatJpeg, Quality: &quality,
					})
					frame.Image = result.Data
					return err
				}))
				stopFrame()
			}
			finish()
			if err != nil {
				frame.Error = "Browser action failed. Check the URL or retry; end and reopen the browser if it has stopped."
			}
			infos, _ := chromedp.Targets(root)
			for _, info := range infos {
				if info.Type == "page" {
					frame.Tabs = append(frame.Tabs, Tab{ID: string(info.TargetID), Title: info.Title})
				}
			}
			if err := encoder.Encode(frame); err != nil {
				return err
			}
		}
	}
}

func protocolAction[P, R any](command cdp.Command[P, R], params P) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		_, err := cdp.Call(ctx, target, command, params)
		return err
	})
}

func perform(ctx context.Context, req Request) error {
	var actions []chromedp.Action[chromedp.Void]
	switch req.Action {
	case "frame", "tab":
		return nil
	case "navigate":
		if !ValidURL(req.URL) {
			return errors.New("only HTTP and HTTPS URLs are supported")
		}
		// Do not wait for navigation: OAuth callbacks may intentionally leave a
		// page loading while waiting for terminal-side work.
		actions = append(actions, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
			result, err := cdp.Call(ctx, target, page.Navigate, page.NavigateParams{URL: req.URL})
			if err == nil && result.ErrorText != "" {
				err = errors.New("navigation failed")
			}
			return err
		}))
	case "back":
		actions = append(actions, chromedp.NavigateBack())
	case "reload":
		actions = append(actions, protocolAction(page.Reload, page.ReloadParams{}))
	case "text":
		if len(req.Text) > 65536 {
			return errors.New("text too long")
		}
		actions = append(actions, protocolAction(input.InsertText, input.InsertTextParams{Text: req.Text}))
	case "key":
		if len(req.Text) > 8 || len(req.Key) > 64 || len(req.Code) > 64 || req.Modifiers < 0 || req.Modifiers > 15 {
			return errors.New("invalid key")
		}
		if req.Key == "Enter" {
			req.Text = "\r"
		}
		if runtime.GOOS != "darwin" && req.Modifiers&4 != 0 {
			req.Modifiers = req.Modifiers&^4 | 2
		}
		actions = append(actions,
			protocolAction(input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyDown, Text: req.Text, Key: req.Key, Code: req.Code, WindowsVirtualKeyCode: req.KeyCode, Modifiers: req.Modifiers}),
			protocolAction(input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyUp, Key: req.Key, Code: req.Code, WindowsVirtualKeyCode: req.KeyCode, Modifiers: req.Modifiers}))
	case "click", "scroll":
		if req.X < 0 || req.X > Width || req.Y < 0 || req.Y > Height || req.DeltaY < -3000 || req.DeltaY > 3000 {
			return errors.New("invalid pointer")
		}
		if req.Action == "scroll" {
			actions = append(actions, protocolAction(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseWheel, X: req.X, Y: req.Y, DeltaY: req.DeltaY}))
		} else {
			actions = append(actions,
				protocolAction(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMousePressed, X: req.X, Y: req.Y, Button: input.MouseButtonLeft, ClickCount: 1}),
				protocolAction(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseReleased, X: req.X, Y: req.Y, Button: input.MouseButtonLeft, ClickCount: 1}))
		}
	default:
		return errors.New("unknown browser action")
	}
	return chromedp.Do(ctx, actions...)
}
