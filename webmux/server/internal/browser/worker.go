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
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const Width, Height = 1100, 760

type Request struct {
	Backward    bool     `json:"backward,omitempty"`
	IgnoreCache bool     `json:"ignoreCache,omitempty"`
	Action      string   `json:"action"`
	URL         string   `json:"url,omitempty"`
	Text        string   `json:"text,omitempty"`
	Key         string   `json:"key,omitempty"`
	Code        string   `json:"code,omitempty"`
	KeyCode     int64    `json:"keyCode,omitempty"`
	Modifiers   int64    `json:"modifiers,omitempty"`
	X           float64  `json:"x,omitempty"`
	Y           float64  `json:"y,omitempty"`
	DeltaY      float64  `json:"deltaY,omitempty"`
	Target      string   `json:"target,omitempty"`
	Width       int64    `json:"width,omitempty"`
	Height      int64    `json:"height,omitempty"`
	DeltaX      float64  `json:"deltaX,omitempty"`
	Button      string   `json:"button,omitempty"`
	Buttons     int64    `json:"buttons,omitempty"`
	ClickCount  int64    `json:"clickCount,omitempty"`
	Accept      bool     `json:"accept,omitempty"`
	Files       []Upload `json:"files,omitempty"`
}

type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Frame struct {
	Found       *bool          `json:"found,omitempty"`
	Protocol    int            `json:"protocol"`
	Image       []byte         `json:"image,omitempty"`
	URL         string         `json:"url"`
	Target      string         `json:"target"`
	Tabs        []Tab          `json:"tabs"`
	Error       string         `json:"error,omitempty"`
	Select      *SelectControl `json:"select,omitempty"`
	Width       int64          `json:"width"`
	Height      int64          `json:"height"`
	CanBack     bool           `json:"canBack"`
	CanForward  bool           `json:"canForward"`
	Dialog      *Dialog        `json:"dialog,omitempty"`
	FileChooser *FileChooser   `json:"fileChooser,omitempty"`
	Downloads   []Download     `json:"downloads"`
	File        *Upload        `json:"file,omitempty"`
	Clipboard   *string        `json:"clipboard,omitempty"`
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
	browser, err := newCompanion(root, profile)
	if err != nil {
		return err
	}
	if err := chromedp.Do(root, protocolAction(page.SetInterceptFileChooserDialog, page.SetInterceptFileChooserDialogParams{Enabled: true})); err != nil {
		return err
	}
	requests := make(chan Request)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(requests)
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), MaxMessageBytes)
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
			err := browser.tabAction(req)
			var extra Frame
			if err == nil && req.Action != "tab" && req.Action != "new_tab" && req.Action != "close_tab" {
				ctx, finish := context.WithTimeout(browser.current, 15*time.Second)
				browser.mu.Lock()
				browser.operation = finish
				browser.operationTarget = chromedp.FromContext(browser.current).Target.TargetID
				browser.mu.Unlock()
				err = browser.step(ctx, req, &extra)
				browser.mu.Lock()
				browser.operation = nil
				browser.mu.Unlock()
				finish()
			}
			frame := browser.snapshot(req, err)
			frame.Clipboard, frame.File, frame.Found = extra.Clipboard, extra.File, extra.Found

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

func perform(ctx context.Context, req Request, width, height int64) error {
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
			if err == nil && result.ErrorText != "" && result.ErrorText != "net::ERR_ABORTED" {
				err = errors.New("navigation failed")
			}
			return err
		}))
	case "back", "forward":
		actions = append(actions, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			history, err := cdp.Call(ctx, t, page.GetNavigationHistory, cdp.Empty{})
			if err != nil {
				return err
			}
			index := history.CurrentIndex - 1
			if req.Action == "forward" {
				index = history.CurrentIndex + 1
			}
			if index < 0 || index >= int64(len(history.Entries)) {
				return errors.New("No navigation history in that direction")
			}
			_, err = cdp.Call(ctx, t, page.NavigateToHistoryEntry, page.NavigateToHistoryEntryParams{EntryID: history.Entries[index].ID})
			return err
		}))
	case "stop":
		actions = append(actions, protocolAction(page.StopLoading, cdp.Empty{}))
	case "reload":
		actions = append(actions, protocolAction(page.Reload, page.ReloadParams{IgnoreCache: &req.IgnoreCache}))
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
		var commands []string
		primary := int64(2)
		if runtime.GOOS == "darwin" {
			primary = 4
		}
		if req.Modifiers&primary != 0 && req.Modifiers&1 == 0 {
			switch strings.ToLower(req.Key) {
			case "a":
				commands = []string{"selectAll"}
			case "z":
				if req.Modifiers&8 != 0 {
					commands = []string{"redo"}
				} else {
					commands = []string{"undo"}
				}
			case "y":
				commands = []string{"redo"}
			}
		}
		actions = append(actions,
			protocolAction(input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyDown, Text: req.Text, Key: req.Key, Code: req.Code, WindowsVirtualKeyCode: req.KeyCode, Modifiers: req.Modifiers, Commands: commands}),
			protocolAction(input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyUp, Key: req.Key, Code: req.Code, WindowsVirtualKeyCode: req.KeyCode, Modifiers: req.Modifiers}))
	case "click", "scroll", "mouse_down", "mouse_up", "mouse_move":
		if req.X < 0 || req.X > float64(width) || req.Y < 0 || req.Y > float64(height) || req.DeltaY < -3000 || req.DeltaY > 3000 || req.DeltaX < -3000 || req.DeltaX > 3000 || req.Modifiers < 0 || req.Modifiers > 15 || req.Buttons < 0 || req.Buttons > 7 {
			return errors.New("invalid pointer")
		}
		if req.Action == "scroll" {
			actions = append(actions, protocolAction(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseWheel, X: req.X, Y: req.Y, DeltaY: req.DeltaY, DeltaX: req.DeltaX, Modifiers: req.Modifiers}))
		} else if req.Action != "click" {
			button := input.MouseButton(req.Button)
			if button != "left" && button != "right" && button != "middle" && button != "none" {
				return errors.New("invalid mouse button")
			}
			if req.ClickCount < 0 || req.ClickCount > 3 {
				return errors.New("invalid click count")
			}
			kind := input.DispatchMouseEventTypeMouseMoved
			if req.Action == "mouse_down" {
				kind = input.DispatchMouseEventTypeMousePressed
			}
			if req.Action == "mouse_up" {
				kind = input.DispatchMouseEventTypeMouseReleased
			}
			actions = append(actions, protocolAction(input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: kind, X: req.X, Y: req.Y, Button: button, Buttons: req.Buttons, ClickCount: req.ClickCount, Modifiers: req.Modifiers}))
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
