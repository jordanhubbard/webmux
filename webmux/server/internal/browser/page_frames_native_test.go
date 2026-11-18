package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	browsercdp "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/inspector"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// Uses the production allocator, including its sandbox and site-isolation
// settings. Fixture JavaScript is test setup, never a worker/HTTP action input.
func TestNativePageFrameControls(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 to run installed Chromium")
	}
	var mu sync.Mutex
	hits := map[string]int{}
	typed := map[string]string{}
	loaded := map[string]bool{}
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hit" {
			mu.Lock()
			hits[r.URL.Query().Get("name")]++
			mu.Unlock()
			return
		}
		if r.URL.Path == "/loaded" {
			mu.Lock()
			loaded[r.URL.Query().Get("document")] = true
			mu.Unlock()
			return
		}
		if r.URL.Path == "/typed" {
			mu.Lock()
			typed[r.URL.Query().Get("name")] = r.URL.Query().Get("value")
			mu.Unlock()
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<script>addEventListener('load',()=>fetch('/loaded?document='+encodeURIComponent(location.pathname+location.search),{method:'POST'}))</script>`)
		name := strings.TrimPrefix(r.URL.Path, "/")
		fmt.Fprintf(w, `<title>Fixture</title><button id="action" onclick="fetch('/hit?name=%s',{method:'POST'})">%s action</button><input id="entry" aria-label="%s field" value="private-field-value" oninput="fetch('/typed?name=%s&amp;value='+encodeURIComponent(this.value),{method:'POST'})">`, name, name, name, name)
		switch name {
		case "root":
			fmt.Fprintf(w, `<iframe sandbox="allow-scripts" src="/sandbox"></iframe><iframe sandbox src="/sandbox-no-scripts"></iframe><iframe id="same" src="/same"></iframe><iframe id="cross" name="identical" src="%s/cross"></iframe><div id="host"></div><script>const s=host.attachShadow({mode:'closed'});s.innerHTML='<button onclick="fetch(\'/hit?name=shadow\',{method:\'POST\'})">shadow action</button><input aria-label="shadow field" oninput="fetch(\'/typed?name=shadow&amp;value=\'+encodeURIComponent(this.value),{method:\'POST\'})">';</script>`, strings.Replace(base, "127.0.0.1", "localhost", 1))
		case "sandbox":
			fmt.Fprint(w, `<button onclick="try{top.location='/escaped'}catch(e){fetch('/hit?name=sandbox-blocked',{method:'POST'})}">sandbox navigation</button>`)
		case "same":
			fmt.Fprint(w, `<iframe src="/nested-same"></iframe>`)
		case "cross":
			fmt.Fprintf(w, `<iframe src="/nested-cross"></iframe><iframe src="%s/returned"></iframe>`, base)
		case "unrelated":
			fmt.Fprintf(w, `<iframe name="identical" src="%[1]s/cross"></iframe><iframe src="%[1]s/foreign"></iframe>`, strings.Replace(base, "127.0.0.1", "localhost", 1))
		}
	}))
	defer server.Close()
	base = server.URL
	allocator, stop := chromedp.NewExecAllocator(context.Background(), workerAllocatorOptions(t.TempDir())...)
	defer stop()
	ctx, cancel := chromedp.NewContext(allocator)
	defer cancel()
	ctx, deadline := context.WithTimeout(ctx, 90*time.Second)
	defer deadline()
	navigate := func(t *testing.T) {
		t.Helper()
		if err := chromedp.Do(ctx, chromedp.Navigate(base+"/root")); err != nil {
			t.Fatal(err)
		}
	}
	navigate(t)
	browser := chromedp.FromContext(ctx).Browser
	command, err := cdp.Call(ctx, browser, browsercdp.GetBrowserCommandLine, cdp.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range command.Arguments {
		if arg == "--no-sandbox" || arg == "--disable-site-isolation-trials" || strings.Contains(arg, "IsolateOrigins") || strings.Contains(arg, "SitePerProcess") {
			t.Fatal("production browser protections disabled")
		}
	}
	version, err := cdp.Call(ctx, browser, browsercdp.GetVersion, cdp.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native iframe controls: %s; production allocator, sandbox and site isolation retained", version.Product)
	b, err := newCompanion(ctx, t.TempDir(), &navigationTrace{})
	if err != nil {
		t.Fatal(err)
	}
	rootID := string(chromedp.FromContext(ctx).Target.TargetID)
	// Matching URL/name/browser context in a separate user tab is not ancestry.
	other, closeOther := chromedp.NewContext(ctx)
	defer closeOther()
	if err := chromedp.Do(other, chromedp.Navigate(base+"/unrelated")); err != nil {
		t.Fatal(err)
	}
	targets := func(t *testing.T) map[target.ID]bool {
		t.Helper()
		out := map[target.ID]bool{}
		err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			result, err := cdp.Call(ctx, t, target.GetTargets, target.GetTargetsParams{})
			for _, info := range result.TargetInfos {
				if info.Type == "page" || info.Type == "iframe" {
					out[info.TargetID] = true
				}
			}
			return err
		}))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := targets(t)
	read := func(t *testing.T) Frame {
		t.Helper()
		var f Frame
		if err := b.readPage(ctx, Request{Target: rootID}, &f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	find := func(t *testing.T, name string) string {
		t.Helper()
		f := read(t)
		encoded, _ := json.Marshal(f.PageText)
		if strings.Contains(string(encoded), "private-field-value") || strings.Contains(string(encoded), "foreign action") || strings.Contains(string(encoded), "unrelated action") || strings.Contains(string(encoded), "http://") {
			t.Fatalf("private/unrelated data in text: %s", encoded)
		}
		count := 0
		for _, n := range f.PageText.Nodes {
			if n.Name == "cross action" && n.Element != "" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected one owned cross-frame control, got %d", count)
		}
		for _, n := range f.PageText.Nodes {
			if n.Name == name && n.Element != "" {
				return n.Element
			}
		}
		t.Fatalf("missing %q in %+v", name, f.PageText)
		return ""
	}
	action := func(t *testing.T, name, kind string) error {
		return b.actOnElement(ctx, Request{Action: kind, Target: rootID, Element: find(t, name)})
	}
	await := func(t *testing.T, check func() bool) {
		t.Helper()
		until := time.Now().Add(3 * time.Second)
		for !check() {
			if time.Now().After(until) {
				t.Fatal("fixture observation timed out")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	for _, name := range []string{"root", "same", "nested-same", "cross", "nested-cross", "returned", "shadow", "sandbox"} {
		t.Run(name, func(t *testing.T) {
			ref := find(t, name+" action")
			req := Request{Action: "activate_element", Target: rootID, Element: ref}
			if err := b.actOnElement(ctx, req); err != nil {
				t.Fatal(err)
			}
			if err := b.actOnElement(ctx, req); !errors.Is(err, errPageElementChanged) {
				t.Fatalf("duplicate: %v", err)
			}
			await(t, func() bool { mu.Lock(); defer mu.Unlock(); return hits[name] == 1 })
			if err := action(t, name+" field", "focus_element"); err != nil {
				t.Fatal(err)
			}
			if err := chromedp.Do(ctx, chromedp.KeyEvent("x")); err != nil {
				t.Fatal(err)
			}
			await(t, func() bool { mu.Lock(); defer mu.Unlock(); return strings.Contains(typed[name], "x") })
		})
	}
	t.Run("sandbox-policy", func(t *testing.T) {
		if err := action(t, "sandbox navigation", "activate_element"); err != nil {
			t.Fatal(err)
		}
		await(t, func() bool { mu.Lock(); defer mu.Unlock(); return hits["sandbox-blocked"] == 1 })
		if err := action(t, "sandbox-no-scripts action", "activate_element"); err != nil {
			t.Fatal(err)
		}
		location, err := chromedp.Run(ctx, chromedp.Evaluate[string]("location.href"))
		if err != nil || location != base+"/root" {
			t.Fatal("sandbox escaped", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if hits["sandbox-no-scripts"] != 0 {
			t.Fatal("sandbox script restriction bypassed")
		}
	})
	if after := targets(t); len(after) != len(before) {
		t.Fatalf("read/action attachments changed target count: %d -> %d", len(before), len(after))
	} else {
		for id := range before {
			if !after[id] {
				t.Fatal("read/action closed a user target")
			}
		}
	}
	t.Run("read-replaces-handles", func(t *testing.T) {
		ref := find(t, "cross action")
		read(t)
		if err := b.actOnElement(ctx, Request{Action: "activate_element", Target: rootID, Element: ref}); !errors.Is(err, errPageElementChanged) {
			t.Fatal(err)
		}
	})
	t.Run("cancelled-operations-consume-references-and-preserve-targets", func(t *testing.T) {
		ref := find(t, "cross action")
		stopped, stop := context.WithCancel(ctx)
		stop()
		if err := b.actOnElement(stopped, Request{Action: "activate_element", Target: rootID, Element: ref}); err == nil {
			t.Fatal("cancelled action succeeded")
		}
		if b.elements != nil {
			t.Fatal("cancelled action retained handles")
		}
		var frame Frame
		if err := b.readPage(stopped, Request{Target: rootID}, &frame); err == nil {
			t.Fatal("cancelled read succeeded")
		}
		after := targets(t)
		if len(before) != len(after) {
			t.Fatal("cancelled operation changed targets")
		}
		for id := range before {
			if !after[id] {
				t.Fatal("cancelled operation closed a user target")
			}
		}
	})

	t.Run("cancel-after-attachment", func(t *testing.T) {
		op, stop := context.WithCancel(ctx)
		var frames *pageFrames
		err := chromedp.Do(op, chromedp.Func(func(c context.Context, target *chromedp.Target) error {
			var err error
			frames, err = discoverPageFrames(c, target)
			return err
		}))
		stop()
		if frames != nil {
			frames.close()
		}
		if err != nil {
			t.Fatal(err)
		}
		after := targets(t)
		if len(before) != len(after) {
			t.Fatal("attachment cancellation changed targets")
		}
		for id := range before {
			if !after[id] {
				t.Fatal("attachment cancellation closed a user target")
			}
		}
		find(t, "cross action")
	})

	eval := func(t *testing.T, js string) {
		t.Helper()
		if _, err := chromedp.Run(ctx, chromedp.Evaluate[any](js)); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the dispatch boundary itself: mutate after validation without a
	// fresh frame discovery that could incidentally reject the old handle first.
	for _, tc := range []struct{ name, control, js string }{
		{"adopt-after-validation", "same action", `document.body.appendChild(document.adoptNode(same.contentDocument.querySelector('#action')))`},
		{"adopt-root-after-validation", "root action", `same.contentDocument.body.appendChild(same.contentDocument.adoptNode(document.querySelector('#action')))`},
		{"remove-after-validation", "same action", `same.contentDocument.querySelector('#action').remove()`},
		{"navigate-after-validation", "same action", `same.src='/same?dispatch-race'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			navigate(t)
			ref := find(t, tc.control)
			element := b.elements[ref]
			var frames *pageFrames
			var selected *pageFrame
			err := chromedp.Do(ctx, chromedp.Func(func(c context.Context, target *chromedp.Target) error {
				var err error
				frames, err = discoverPageFrames(c, target)
				if err != nil {
					return err
				}
				for _, f := range frames.frames {
					if f.document == element.document {
						selected = f
						break
					}
				}
				if selected == nil {
					return errPageElementChanged
				}
				if _, err := selected.isolatedWorld(c); err != nil {
					return err
				}
				return selected.validate(c)
			}))
			if frames != nil {
				defer frames.close()
			}
			if err != nil {
				t.Fatal(err)
			}
			eval(t, tc.js)
			if tc.name == "navigate-after-validation" {
				await(t, func() bool { mu.Lock(); defer mu.Unlock(); return loaded["/same?dispatch-race"] })
			}
			if err := b.dispatchPageElement(ctx, selected, element, true); err == nil {
				t.Fatal("dispatch admitted a changed native document/node")
			}
		})
	}

	for _, tc := range []struct{ name, control, js string }{
		{"adopt-child-to-parent", "same action", `document.body.appendChild(document.adoptNode(same.contentDocument.querySelector('#action')))`},
		{"adopt-parent-to-child", "root action", `same.contentDocument.body.appendChild(same.contentDocument.adoptNode(document.querySelector('#action')))`},
		{"child-navigation", "same action", `same.src='/same?new-document'`},
		{"cross-navigation", "cross action", `cross.src=cross.src+'?new-document'`},
		{"ancestor-navigation", "nested-same action", `same.src='/same?new-ancestor'`},
		{"remove-frame", "cross action", `cross.remove()`},
		{"move-frame", "same action", `document.querySelector('#host').appendChild(same)`},
		{"replace-frame", "same action", `same.outerHTML='<iframe id="same" src="/same?replacement"></iframe>'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			navigate(t)
			ref := find(t, tc.control)
			eval(t, tc.js)
			want := map[string]string{"child-navigation": "/same?new-document", "cross-navigation": "/cross?new-document", "ancestor-navigation": "/same?new-ancestor", "replace-frame": "/same?replacement"}[tc.name]
			if want != "" {
				await(t, func() bool { mu.Lock(); defer mu.Unlock(); return loaded[want] })
			}

			if err := b.actOnElement(ctx, Request{Action: "activate_element", Target: rootID, Element: ref}); !errors.Is(err, errPageElementChanged) {
				t.Fatalf("stale frame action: %v", err)
			}
		})
	}
	t.Run("renderer-loss-after-validation", func(t *testing.T) {
		navigate(t)
		ref := find(t, "cross action")
		element := b.elements[ref]
		var frames *pageFrames
		var selected *pageFrame
		err := chromedp.Do(ctx, chromedp.Func(func(c context.Context, target *chromedp.Target) error {
			var err error
			frames, err = discoverPageFrames(c, target)
			if err != nil {
				return err
			}
			for _, f := range frames.frames {
				if f.document == element.document {
					selected = f
					break
				}
			}
			if selected == nil {
				return errPageElementChanged
			}
			if _, err := selected.isolatedWorld(c); err != nil {
				return err
			}
			return selected.validate(c)
		}))
		if frames != nil {
			defer frames.close()
		}
		if err != nil {
			t.Fatal(err)
		}
		crashed := make(chan bool, 1)
		events := chromedp.Events(selected.ctx, inspector.TargetCrashed)
		go func() {
			for _, err := range events {
				crashed <- err == nil
				return
			}
		}()
		stopped, stop := context.WithTimeout(ctx, 2*time.Second)
		_, _ = cdp.Call(stopped, selected.target, page.Crash, cdp.Empty{})
		stop()
		select {
		case ok := <-crashed:
			if !ok {
				t.Fatal("renderer crash event failed")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("renderer crash was not observed")
		}
		action, finish := context.WithTimeout(ctx, 2*time.Second)
		defer finish()
		if err := b.dispatchPageElement(action, selected, element, true); err == nil {
			t.Fatal("action survived renderer loss")
		}
		// The inspector session must not take ownership of the root tab.
		location, err := chromedp.Run(ctx, chromedp.Evaluate[string]("location.href"))
		if err != nil || location != base+"/root" {
			t.Fatal("lost root page after child renderer failure", err)
		}
	})

}
