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
	err = chromedp.Run(root, chromedp.EmulateViewport(Width, Height), chromedp.Navigate("about:blank"))
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
						if err := chromedp.Run(ctx, chromedp.EmulateViewport(Width, Height)); err != nil {
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
				err = chromedp.Run(ctx, chromedp.Location(&frame.URL), chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					frame.Image, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(65).Do(ctx)
					return err
				}))
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

func perform(ctx context.Context, req Request) error {
	var actions []chromedp.Action
	switch req.Action {
	case "frame", "tab":
		return nil
	case "navigate":
		if !ValidURL(req.URL) {
			return errors.New("only HTTP and HTTPS URLs are supported")
		}
		// Do not wait for navigation: OAuth callbacks may intentionally leave a
		// page loading while waiting for terminal-side work.
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, errorText, _, err := page.Navigate(req.URL).Do(ctx)
			if err == nil && errorText != "" {
				err = errors.New("navigation failed")
			}
			return err
		}))
	case "back":
		actions = append(actions, chromedp.NavigateBack())
	case "reload":
		actions = append(actions, page.Reload())
	case "text":
		if len(req.Text) > 65536 {
			return errors.New("text too long")
		}
		actions = append(actions, input.InsertText(req.Text))
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
			input.DispatchKeyEvent(input.KeyDown).WithText(req.Text).WithKey(req.Key).WithCode(req.Code).WithWindowsVirtualKeyCode(req.KeyCode).WithModifiers(input.Modifier(req.Modifiers)),
			input.DispatchKeyEvent(input.KeyUp).WithKey(req.Key).WithCode(req.Code).WithWindowsVirtualKeyCode(req.KeyCode).WithModifiers(input.Modifier(req.Modifiers)))
	case "click", "scroll":
		if req.X < 0 || req.X > Width || req.Y < 0 || req.Y > Height || req.DeltaY < -3000 || req.DeltaY > 3000 {
			return errors.New("invalid pointer")
		}
		if req.Action == "scroll" {
			actions = append(actions, input.DispatchMouseEvent(input.MouseWheel, req.X, req.Y).WithDeltaY(req.DeltaY))
		} else {
			actions = append(actions,
				input.DispatchMouseEvent(input.MousePressed, req.X, req.Y).WithButton(input.Left).WithClickCount(1),
				input.DispatchMouseEvent(input.MouseReleased, req.X, req.Y).WithButton(input.Left).WithClickCount(1))
		}
	default:
		return errors.New("unknown browser action")
	}
	return chromedp.Run(ctx, actions...)
}
