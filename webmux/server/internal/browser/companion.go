package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	browsercdp "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const MaxFileBytes = 10 << 20
const maxRetainedBytes = 50 << 20
const MaxMessageBytes = 24 << 20

type Upload struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}
type Download struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	State string  `json:"state"`
	Bytes float64 `json:"bytes"`
}
type Dialog struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Prompt  string `json:"prompt"`
}
type FileChooser struct {
	Multiple bool `json:"multiple"`
	node     cdp.BackendNodeID
}
type SelectOption struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
	Selected bool   `json:"selected"`
}
type SelectControl struct {
	Options []SelectOption `json:"options"`
}

type companion struct {
	root, current   context.Context
	tabs            map[target.ID]context.Context
	cancels         map[target.ID]context.CancelFunc
	profile         string
	width, height   int64
	mu              sync.Mutex
	dialogs         map[target.ID]*Dialog
	choosers        map[target.ID]*FileChooser
	downloads       map[string]*Download
	uploaded        int
	operation       context.CancelFunc
	operationTarget target.ID
}

func newCompanion(ctx context.Context, profile string) (*companion, error) {
	id := chromedp.FromContext(ctx).Target.TargetID
	b := &companion{root: ctx, current: ctx, tabs: map[target.ID]context.Context{id: ctx}, cancels: map[target.ID]context.CancelFunc{}, profile: profile, width: Width, height: Height, dialogs: map[target.ID]*Dialog{}, choosers: map[target.ID]*FileChooser{}, downloads: map[string]*Download{}}
	b.watchTab(ctx)
	directory := filepath.Join(profile, "downloads")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	begin := chromedp.BrowserEvents(ctx, browsercdp.DownloadWillBegin)
	progress := chromedp.BrowserEvents(ctx, browsercdp.DownloadProgress)
	go func() {
		for event, err := range begin {
			if err != nil {
				return
			}
			b.mu.Lock()
			overLimit := b.downloads[event.GUID] == nil && len(b.downloads) >= 20
			if !overLimit {
				if b.downloads[event.GUID] == nil {
					b.downloads[event.GUID] = &Download{ID: event.GUID, State: "inProgress"}
				}
				b.downloads[event.GUID].Name = filepath.Base(strings.ReplaceAll(event.SuggestedFilename, "\\", "/"))
			}
			b.mu.Unlock()
			if overLimit {
				b.cancelDownload(event.GUID)
			}
		}
	}()
	go func() {
		for event, err := range progress {
			if err != nil {
				return
			}
			b.mu.Lock()
			download := b.downloads[event.GUID]
			if download == nil && len(b.downloads) < 20 {
				download = &Download{ID: event.GUID, Name: event.GUID}
				b.downloads[event.GUID] = download
			}
			overLimit := event.TotalBytes > MaxFileBytes || event.ReceivedBytes > MaxFileBytes
			overLimit = overLimit || download == nil
			var total float64
			for _, d := range b.downloads {
				if d.ID != event.GUID && d.State != "canceled" {
					total += d.Bytes
				}
			}
			overLimit = overLimit || total+event.ReceivedBytes > maxRetainedBytes
			if download != nil {
				download.Bytes = event.ReceivedBytes
				download.State = string(event.State)
				if overLimit {
					download.State = "canceled"
				}
			}
			b.mu.Unlock()
			if overLimit {
				b.cancelDownload(event.GUID)
				_ = os.Remove(filepath.Join(directory, event.GUID))
				_ = os.Remove(filepath.Join(directory, event.GUID+".crdownload"))
			}
		}
	}()
	enabled := true
	_, err := cdp.Call(ctx, chromedp.FromContext(ctx).Browser, browsercdp.SetDownloadBehavior, browsercdp.SetDownloadBehaviorParams{Behavior: browsercdp.SetDownloadBehaviorBehaviorAllowAndName, DownloadPath: directory, EventsEnabled: &enabled})
	return b, err
}

func (b *companion) cancelDownload(id string) {
	ctx, cancel := context.WithTimeout(b.root, 2*time.Second)
	defer cancel()
	_, _ = cdp.Call(ctx, chromedp.FromContext(b.root).Browser, browsercdp.CancelDownload, browsercdp.CancelDownloadParams{GUID: id})
}

func (b *companion) watchTab(ctx context.Context) {
	id := chromedp.FromContext(ctx).Target.TargetID
	dialogs := chromedp.Events(ctx, page.JavascriptDialogOpening)
	choosers := chromedp.Events(ctx, page.FileChooserOpened)
	go func() {
		for event, err := range dialogs {
			if err != nil {
				return
			}
			b.mu.Lock()
			b.dialogs[id] = &Dialog{Type: string(event.Type), Message: event.Message, Prompt: event.DefaultPrompt}
			interrupt := b.operation
			active := b.operationTarget == id
			b.mu.Unlock()
			if interrupt != nil && active {
				interrupt()
			}
		}
	}()
	go func() {
		for event, err := range choosers {
			if err != nil {
				return
			}
			b.mu.Lock()
			b.choosers[id] = &FileChooser{Multiple: string(event.Mode) == "selectMultiple", node: event.BackendNodeID}
			interrupt := b.operation
			active := b.operationTarget == id
			b.mu.Unlock()
			if interrupt != nil && active {
				interrupt()
			}
		}
	}()
}

func (b *companion) selectTab(id target.ID) error {
	if ctx := b.tabs[id]; ctx != nil {
		if err := chromedp.Do(ctx, chromedp.EmulateViewport(b.width, b.height), protocolAction(page.BringToFront, cdp.Empty{})); err != nil {
			return err
		}
		b.current = ctx
		return nil
	}
	infos, err := chromedp.Targets(b.root)
	if err != nil {
		return err
	}
	found := false
	for _, info := range infos {
		if info.Type == "page" && info.TargetID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("This tab has closed; select another tab")
	}
	ctx, cancel := chromedp.NewContext(b.root, chromedp.WithTargetID(id), chromedp.WithDetachOnCancel())
	if err := chromedp.Do(ctx, chromedp.EmulateViewport(b.width, b.height), protocolAction(page.BringToFront, cdp.Empty{}), protocolAction(page.SetInterceptFileChooserDialog, page.SetInterceptFileChooserDialogParams{Enabled: true})); err != nil {
		cancel()
		return err
	}
	b.tabs[id] = ctx
	b.cancels[id] = cancel
	b.current = ctx
	b.watchTab(ctx)
	return nil
}

func (b *companion) step(ctx context.Context, req Request, frame *Frame) error {
	id := chromedp.FromContext(b.current).Target.TargetID
	switch req.Action {
	case "resize":
		if req.Width < 320 || req.Width > 2560 || req.Height < 240 || req.Height > 1600 {
			return errors.New("Browser viewport must be 320–2560 by 240–1600 pixels")
		}
		b.width, b.height = req.Width, req.Height
		return chromedp.Do(ctx, chromedp.EmulateViewport(b.width, b.height))
	case "dialog":
		b.mu.Lock()
		dialog := b.dialogs[id]
		b.mu.Unlock()
		if dialog == nil || req.Target != string(id) || len(req.Text) > 65536 {
			return errors.New("This browser dialog has closed or belongs to another tab")
		}
		b.mu.Lock()
		delete(b.dialogs, id)
		b.mu.Unlock()
		err := chromedp.Do(ctx, protocolAction(page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: req.Accept, PromptText: req.Text}))
		if err != nil {
			b.mu.Lock()
			if b.dialogs[id] == nil {
				b.dialogs[id] = dialog
			}
			b.mu.Unlock()
		}
		return err
	case "select_option", "cancel_select":
		if req.Target != string(id) || len(req.Text) > 65536 {
			return errors.New("This page selector belongs to another tab")
		}
		if err := perform(ctx, Request{Action: "key", Key: "Escape", Code: "Escape", KeyCode: 27}, b.width, b.height); err != nil {
			return err
		}
		encoded, _ := json.Marshal(req.Text)
		script := `(()=>{const e=document.activeElement;if(!e || e.tagName!=='SELECT')return false;`
		if req.Action == "select_option" {
			script += `const value=` + string(encoded) + `;const option=Array.from(e.options).find(o=>o.value===value && !o.disabled && !o.parentElement?.disabled);if(!option)return false;e.value=value;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));`
		}
		script += `e.blur();return true})()`
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
		if err == nil && !ok {
			return errors.New("The page selector has closed or that option is unavailable")
		}
		return err
	case "find":
		if req.Text == "" || len(req.Text) > 4096 {
			return errors.New("Find a nonempty phrase of at most 4096 bytes")
		}
		encoded, _ := json.Marshal(req.Text)
		direction := "false"
		if req.Backward {
			direction = "true"
		}
		found, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`window.find(`+string(encoded)+`,false,`+direction+`,true,false,false,false)`))
		frame.Found = &found
		return err
	case "copy":
		text, err := chromedp.Run(ctx, chromedp.Evaluate[string](`(()=>{const e=document.activeElement;return (e && typeof e.selectionStart==='number') ? e.value.slice(e.selectionStart,e.selectionEnd) : String(window.getSelection()||'')})()`))
		if len(text) > 128<<10 {
			return errors.New("Copy at most 128 KiB at a time")
		}
		frame.Clipboard = &text
		return err
	case "upload":
		b.mu.Lock()
		chooser := b.choosers[id]
		b.mu.Unlock()
		if chooser == nil || chooser.node == 0 {
			return errors.New("Click a file input on the page before uploading")
		}
		if len(req.Files) == 0 || len(req.Files) > 8 || (!chooser.Multiple && len(req.Files) != 1) {
			return errors.New("Choose files matching this page's file input (at most eight)")
		}
		total := 0
		for _, f := range req.Files {
			if f.Name == "" || f.Name == "." || f.Name == ".." || strings.ContainsAny(f.Name, "/\\\x00\r\n") || len(f.Name) > 255 {
				return errors.New("Invalid upload filename")
			}
			total += len(f.Data)
		}
		if total > MaxFileBytes || b.uploaded+total > maxRetainedBytes {
			return errors.New("Upload at most 10 MiB per request and 50 MiB per browser")
		}
		directory, err := os.MkdirTemp(b.profile, "upload-")
		if err != nil {
			return err
		}
		paths := make([]string, 0, len(req.Files))
		names := map[string]bool{}
		for _, f := range req.Files {
			if names[f.Name] {
				_ = os.RemoveAll(directory)
				return errors.New("Upload filenames must be distinct")
			}
			names[f.Name] = true
			p := filepath.Join(directory, f.Name)
			if err := os.WriteFile(p, f.Data, 0600); err != nil {
				_ = os.RemoveAll(directory)
				return err
			}
			paths = append(paths, p)
		}
		err = chromedp.Do(ctx, protocolAction(dom.SetFileInputFiles, dom.SetFileInputFilesParams{Files: paths, BackendNodeID: chooser.node}))
		if err != nil {
			_ = os.RemoveAll(directory)
			return err
		}
		b.uploaded += total
		b.mu.Lock()
		delete(b.choosers, id)
		b.mu.Unlock()
		return nil
	case "cancel_upload":
		b.mu.Lock()
		delete(b.choosers, id)
		b.mu.Unlock()
		return nil
	case "download":
		if !regexp.MustCompile(`^[a-fA-F0-9-]{36}$`).MatchString(req.Target) {
			return errors.New("Invalid download")
		}
		b.mu.Lock()
		d := b.downloads[req.Target]
		var name, state string
		if d != nil {
			name, state = d.Name, d.State
		}
		b.mu.Unlock()
		if state != "completed" {
			return errors.New("Download is not complete")
		}
		p := filepath.Join(b.profile, "downloads", req.Target)
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
			return errors.New("Download is unavailable or exceeds 10 MiB")
		}
		data, err := os.ReadFile(p)
		if err == nil {
			frame.File = &Upload{Name: name, Data: data}
		}
		return err
	default:
		return perform(ctx, req, b.width, b.height)
	}
}

func (b *companion) tabAction(req Request) error {
	switch req.Action {
	case "tab":
		return b.selectTab(target.ID(req.Target))
	case "new_tab":
		infos, err := chromedp.Targets(b.root)
		if err != nil {
			return err
		}
		pages := 0
		for _, info := range infos {
			if info.Type == "page" {
				pages++
			}
		}
		if pages >= 16 {
			return errors.New("Close a tab before opening another (limit 16)")
		}
		result, err := cdp.Call(b.root, chromedp.FromContext(b.root).Browser, target.CreateTarget, target.CreateTargetParams{URL: "about:blank"})
		if err != nil {
			return err
		}
		return b.selectTab(result.TargetID)
	case "close_tab":
		id := chromedp.FromContext(b.current).Target.TargetID
		if req.Target != string(id) {
			return errors.New("Select the tab before closing it")
		}
		infos, err := chromedp.Targets(b.root)
		if err != nil {
			return err
		}
		var next target.ID
		for _, info := range infos {
			if info.Type == "page" && info.TargetID != id {
				next = info.TargetID
				break
			}
		}
		if next == "" {
			return errors.New("Keep one tab open, or use End browser")
		}
		_, err = cdp.Call(b.root, chromedp.FromContext(b.root).Browser, target.CloseTarget, target.CloseTargetParams{TargetID: id})
		if err != nil {
			return err
		}
		if cancel := b.cancels[id]; cancel != nil {
			cancel()
			delete(b.cancels, id)
		}
		delete(b.tabs, id)
		return b.selectTab(next)
	}
	return nil
}

func (b *companion) snapshot(req Request, actionErr error) Frame {
	id := chromedp.FromContext(b.current).Target.TargetID
	frame := Frame{Protocol: 2, Target: string(id), Tabs: []Tab{}, Width: b.width, Height: b.height, Downloads: []Download{}}
	infos, _ := chromedp.Targets(b.root)
	for _, info := range infos {
		if info.Type == "page" {
			frame.Tabs = append(frame.Tabs, Tab{ID: string(info.TargetID), Title: info.Title})
			if info.TargetID == id {
				frame.URL = info.URL
			}
		}
	}
	sort.Slice(frame.Tabs, func(i, j int) bool { return frame.Tabs[i].ID < frame.Tabs[j].ID })
	b.mu.Lock()
	frame.Dialog = b.dialogs[id]
	if c := b.choosers[id]; c != nil {
		frame.FileChooser = &FileChooser{Multiple: c.Multiple}
	}
	for _, d := range b.downloads {
		frame.Downloads = append(frame.Downloads, *d)
	}
	b.mu.Unlock()
	sort.Slice(frame.Downloads, func(i, j int) bool { return frame.Downloads[i].ID < frame.Downloads[j].ID })
	if actionErr != nil && !(errors.Is(actionErr, context.Canceled) && (frame.Dialog != nil || frame.FileChooser != nil)) {
		frame.Error = fmt.Sprintf("Browser action failed: %s", actionErr)
		return frame
	}
	if frame.Dialog != nil {
		return frame
	}
	ctx, cancel := context.WithTimeout(b.current, 3*time.Second)
	defer cancel()
	switch req.Action {
	case "frame", "mouse_up", "key":
		control, _ := chromedp.Run(ctx, chromedp.Evaluate[*SelectControl](`(()=>{const e=document.activeElement;if(!e || e.tagName!=='SELECT' || e.multiple || e.size>1)return null;return {options:Array.from(e.options).slice(0,500).map(o=>({value:o.value,label:o.label,selected:o.selected,disabled:o.disabled || !!o.parentElement?.disabled}))}})()`))
		frame.Select = control
	}
	_ = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		history, err := cdp.Call(ctx, t, page.GetNavigationHistory, cdp.Empty{})
		if err == nil {
			frame.CanBack = history.CurrentIndex > 0
			frame.CanForward = history.CurrentIndex < int64(len(history.Entries)-1)
		}
		return err
	}))
	switch req.Action {
	case "frame", "navigate", "tab", "new_tab", "close_tab", "resize", "reload", "back", "forward", "dialog", "upload", "select_option", "cancel_select", "find", "stop":
		quality := int64(75)
		err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			result, err := cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatJpeg, Quality: &quality})
			frame.Image = result.Data
			return err
		}))
		if err != nil {
			frame.Error = "Page rendering was interrupted; retry or open the site in your browser"
		}
	}
	return frame
}
