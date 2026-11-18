package browser

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	browsercdp "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

type firstNavigationEvidence struct {
	cleanupProcessIdentities map[uint32]uint64
	cleanupHandleProbe       func(error, string) *firstNavigationHandleReport

	Transport        string                           `json:"transport"`
	ProfileLocation  string                           `json:"profileLocation"`
	Mode             string                           `json:"mode"`
	Platform         string                           `json:"platform"`
	Architecture     string                           `json:"architecture"`
	BrowserProduct   string                           `json:"browserProduct,omitempty"`
	Loaded           bool                             `json:"loaded"`
	FixtureRequests  int64                            `json:"fixtureRequests"`
	NavigateCalls    int                              `json:"navigateCalls"`
	TargetSelection  string                           `json:"targetSelection,omitempty"`
	ExtraTargets     int                              `json:"extraTargets,omitempty"`
	CleanupFailed    bool                             `json:"cleanupFailed,omitempty"`
	CleanupRemoval   *firstNavigationRemovalError     `json:"cleanupRemoval,omitempty"`
	CleanupHandles   *firstNavigationHandleReport     `json:"cleanupHandles,omitempty"`
	CleanupHolders   *firstNavigationRemovalHolders   `json:"cleanupHolders,omitempty"`
	CleanupProcesses *firstNavigationCleanupProcesses `json:"cleanupProcesses,omitempty"`
	Failure          string                           `json:"failure,omitempty"`
	Diagnostics      *NavigationDiagnostics           `json:"diagnostics,omitempty"`
	NetLog           *firstNavigationNetLog           `json:"netLog,omitempty"`
	BrowserTrace     *firstNavigationTrace            `json:"browserTrace,omitempty"`
	NetworkSandbox   *firstNavigationNetworkSandbox   `json:"networkSandbox,omitempty"`
	ProcessOutput    *firstNavigationOutput           `json:"processOutput,omitempty"`
	TraceLoading     bool                             `json:"traceLoading,omitempty"`
	TraceStartStage  string                           `json:"traceStartStage,omitempty"`
}

// These are independent first loads in fresh private browser processes. A failed
// load is never replayed. The Node fixture matches the application's browser
// acceptance origin while the controls remove protocol/controller layers.
func TestNativeFirstNavigationComponents(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 for native Chromium component qualification")
	}
	if !filepath.IsAbs(os.Getenv("WEBMUX_BROWSER_EXECUTABLE")) {
		t.Fatal("native qualification requires an explicit Chromium executable")
	}
	t.Setenv("WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS", "1")
	site, hits := firstNavigationNodeFixture(t)
	for _, mode := range []string{"direct", "direct-screenshot", "direct-home", "direct-home-screenshot", "direct-websocket", "direct-home-websocket", "direct-child", "direct-child-original", "direct-home-child", "direct-home-child-original", "direct-netlog", "direct-home-netlog", "direct-trace", "direct-home-trace", "direct-trace-late", "direct-home-trace-late", "direct-trace-loading-late", "direct-home-trace-loading-late", "direct-trace-loading-log-late", "direct-home-trace-loading-log-late", "worker-subprocess-temporary", "worker-subprocess-saved"} {
		t.Run(mode, func(t *testing.T) {
			if strings.HasSuffix(mode, "-netlog") && runtime.GOOS != "windows" {
				t.Skip("Windows-only bounded anonymous NetLog capture; macOS sandbox refuses its scratch directory")
			}
			record := firstNavigationEvidence{Mode: mode, Platform: runtime.GOOS, Architecture: runtime.GOARCH, Transport: "pipe", ProfileLocation: "system-temporary"}
			if strings.Contains(mode, "websocket") {
				record.Transport = "websocket"
			}
			if strings.Contains(mode, "-home") || mode == "worker-subprocess-temporary" {
				record.ProfileLocation = "home-temporary"
			}
			if mode == "worker-subprocess-saved" {
				record.ProfileLocation = "home-saved"
			}
			before := hits.Load()
			// Registered before browser cleanup, so the final snapshot is written last.
			t.Cleanup(func() {
				record.FixtureRequests = hits.Load() - before
				dir := os.Getenv("WEBMUX_BROWSER_NAVIGATION_ARTIFACT_DIR")
				if dir == "" {
					return
				}
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Error(err)
					return
				}
				data, err := json.MarshalIndent(record, "", "  ")
				if err != nil {
					t.Error(err)
					return
				}
				if err = os.WriteFile(filepath.Join(dir, mode+".json"), append(data, '\n'), 0600); err != nil {
					t.Error(err)
				}
			})
			var call func(Request) Frame
			if strings.HasPrefix(mode, "worker-") {
				call = firstNavigationWorker(t, mode == "worker-subprocess-saved", &record)
			} else {
				call = firstNavigationDirect(t, firstNavigationDirectOptions{
					screenshot: strings.HasSuffix(mode, "screenshot"), profileAtHome: strings.Contains(mode, "-home"),
					websocket: strings.Contains(mode, "websocket"), createChild: strings.Contains(mode, "-child"),
					selectOriginal: strings.HasSuffix(mode, "-original"), netLog: strings.HasSuffix(mode, "-netlog"),
					browserTrace: strings.Contains(mode, "-trace"), traceLate: strings.HasSuffix(mode, "-late"), traceLoading: strings.Contains(mode, "-loading-"), processOutput: strings.Contains(mode, "-log-"),
				}, &record)
			}
			record.NavigateCalls++
			frame := call(Request{Action: "navigate", URL: site + "/check"})
			deadline := time.Now().Add(5 * time.Second)
			for {
				record.Diagnostics = frame.NavigationDiagnostics
				for _, tab := range frame.Tabs {
					if tab.ID == frame.Target && frame.URL == site+"/check" && tab.Title == "FIRST_NAVIGATION_OK" {
						record.Loaded = true
					}
				}
				if record.Loaded {
					break
				}
				if time.Now().After(deadline) {
					record.Failure = "fixture_document_not_loaded"
					t.Fatal("first navigation did not load the fixture within five seconds")
				}
				time.Sleep(50 * time.Millisecond)
				frame = call(Request{Action: "frame"})
			}
		})
	}
}

func firstNavigationNodeFixture(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required by native first-navigation qualification")
	}
	ctx, cancel := context.WithCancel(context.Background())
	script := `const http=require('node:http');const s=http.createServer((q,r)=>{if(q.url==='/check'||q.url==='/pending')process.stdout.write('{"hit":true}\n');if(q.url==='/pending')return;r.writeHead(200,{'Content-Type':'text/html; charset=utf-8'});r.end('<title>FIRST_NAVIGATION_OK</title>')});s.listen(0,'127.0.0.1',()=>process.stdout.write(JSON.stringify({port:s.address().port})+'\n'));process.stdin.resume();process.stdin.on('end',()=>s.close(()=>process.exit(0)));`
	cmd := exec.CommandContext(ctx, node, "-e", script)
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	var hits atomic.Int64
	port := make(chan int, 1)
	scanned := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		defer close(scanned)
		scan := bufio.NewScanner(out)
		scan.Buffer(make([]byte, 512), 4096)
		for scan.Scan() {
			var row struct {
				Port int  `json:"port"`
				Hit  bool `json:"hit"`
			}
			if json.Unmarshal(scan.Bytes(), &row) != nil {
				return
			}
			if row.Port > 0 {
				port <- row.Port
			}
			if row.Hit {
				hits.Add(1)
			}
		}
	}()
	go func() { <-scanned; done <- cmd.Wait() }()
	t.Cleanup(func() {
		in.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cancel()
			<-done
		}
		cancel()
	})
	select {
	case value := <-port:
		if value < 1024 || value > 65535 {
			t.Fatal("invalid Node fixture port")
		}
		site := "http://127.0.0.1:" + strconv.Itoa(value)
		response, err := (&http.Client{Timeout: 3 * time.Second}).Get(site + "/health")
		if err != nil {
			t.Fatal("Node fixture health unavailable")
		}
		response.Body.Close()
		return site, &hits
	case <-time.After(5 * time.Second):
		t.Fatal("Node fixture startup timed out")
	}
	return "", &hits
}

// The test executable accepts the production worker mode only for its own
// subprocess control; ordinary package tests retain the standard testing entry.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--browser-worker" {
		if Worker(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func firstNavigationWorker(t *testing.T, saved bool, record *firstNavigationEvidence) func(Request) Frame {
	t.Helper()
	first := Request{Action: "frame"}
	stopped := false
	if saved {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}
		nativeKey := hex.EncodeToString(key)
		first = Request{Action: "configure", ProfileKey: nativeKey}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		// Preserve the original execution roots. Remove only this random fixture's
		// profile after its worker has actually exited, never another stored profile.
		t.Cleanup(func() {
			if stopped {
				root := filepath.Join(home, "webmux-browser-profiles")
				if err := os.RemoveAll(filepath.Join(root, nativeKey)); err != nil {
					record.CleanupFailed = true
					t.Error("private saved profile cleanup failed")
				}
				if err := os.Remove(filepath.Join(root, nativeKey+".lock")); err != nil && !os.IsNotExist(err) {
					record.CleanupFailed = true
					t.Error("private saved profile lock cleanup failed")
				}
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := WorkerCommand(ctx, session.Session{Transport: "local"}, nil, "--browser-worker")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	send, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	receive, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		send.Close()
		select {
		case err := <-done:
			if err == nil {
				stopped = true
			} else {
				record.CleanupFailed = true
				t.Error("native worker did not close cleanly")
			}
		case <-time.After(10 * time.Second):
			record.CleanupFailed = true
			t.Error("native worker cleanup timed out")
			cancel()
			<-done
		}
		receive.Close()
		cancel()
	})
	decoder := json.NewDecoder(receive)
	call := func(req Request) Frame {
		t.Helper()
		frames := make(chan Frame, 1)
		failure := make(chan error, 1)
		go func() {
			if err := json.NewEncoder(send).Encode(req); err != nil {
				failure <- err
				return
			}
			var frame Frame
			if err := decoder.Decode(&frame); err != nil {
				failure <- err
				return
			}
			frames <- frame
		}()
		select {
		case frame := <-frames:
			record.Diagnostics = frame.NavigationDiagnostics
			if frame.Error != "" && frame.Error != "Page rendering was interrupted; retry or open the site in your browser" {
				record.Failure = "worker_action_error"
				t.Fatal("native worker action failed")
			}
			return frame
		case <-failure:
			record.Failure = "worker_protocol_error"
			t.Fatal("native worker protocol failed")
		case <-time.After(30 * time.Second):
			record.Failure = "worker_request_timeout"
			t.Fatal("native worker request timed out")
		}
		return Frame{}
	}
	frame := call(first)
	if frame.Protocol != Protocol {
		record.Failure = "worker_protocol_mismatch"
		t.Fatal("native worker protocol mismatch")
	}
	return call
}

type firstNavigationDirectOptions struct {
	screenshot, profileAtHome, websocket, createChild, selectOriginal, netLog, browserTrace, traceLate, traceLoading, processOutput bool
}

func firstNavigationDirect(t *testing.T, options firstNavigationDirectOptions, record *firstNavigationEvidence) func(Request) Frame {
	t.Helper()
	var profile string
	if options.profileAtHome {
		var err error
		profile, err = temporaryProfile()
		if err != nil {
			record.Failure = "direct_home_profile_error"
			t.Fatal("cannot create private home profile")
		}
		// Registered before browser cleanup: Chrome exits before profile removal.
		t.Cleanup(func() {
			err := os.RemoveAll(profile)
			if record.cleanupHandleProbe != nil {
				record.CleanupHandles = record.cleanupHandleProbe(err, profile)
			}
			if err != nil {
				record.CleanupFailed = true
				record.CleanupRemoval = classifyFirstNavigationRemoval(err, profile)
				record.CleanupHolders = queryFirstNavigationRemovalHolders(err, profile, record.cleanupProcessIdentities)
				t.Errorf("private home profile cleanup failed: %+v", record.CleanupRemoval)
			}
		})
	} else {
		profile = t.TempDir()
	}
	opts := workerAllocatorOptions(profile)
	var requested string
	if options.netLog {
		// Anonymous fixture browsers only. Raw logs stay in the owned profile;
		// upload only a bounded projection after a graceful browser shutdown.
		path := filepath.Join(profile, "native-netlog.json")
		opts = append(opts, chromedp.Flag("log-net-log", path), chromedp.Flag("net-log-capture-mode", "Default"), chromedp.Flag("net-log-max-size-mb", "4"))
		t.Cleanup(func() {
			var err error
			record.NetLog, err = readFirstNavigationNetLog(path, requested)
			if err != nil {
				if record.Failure == "" {
					record.Failure = "netlog_projection_error"
				}
				t.Error("cannot project anonymous native network diagnostics")
			}
		})
	}
	if options.websocket {
		// Fresh test profiles only. Keep the ephemeral diagnostic listener on loopback;
		// the actual worker continues using private pipes with its original flags.
		opts = append(opts, remote.WebSocket, chromedp.Flag("remote-debugging-address", "127.0.0.1"))
	}
	var output *firstNavigationOutputCapture
	if options.processOutput {
		output = &firstNavigationOutputCapture{}
		opts = append(opts, chromedp.CombinedOutput(output))
	}
	alloc, stop := chromedp.NewExecAllocator(context.Background(), opts...)
	root, cancel := chromedp.NewContext(alloc)
	t.Cleanup(func() {
		finishObservation := observeFirstNavigationCleanup(root, record)
		if options.profileAtHome && os.Getenv("WEBMUX_BROWSER_CLEANUP_PROCESSES") == "1" {
			record.cleanupHandleProbe = prepareFirstNavigationHandleProbe(record.cleanupProcessIdentities)
		}
		defer func() { record.CleanupProcesses = finishObservation() }()
		if options.netLog {
			closing, finish := context.WithTimeout(root, 5*time.Second)
			if chromedp.Cancel(closing) != nil {
				record.CleanupFailed = true
				t.Error("anonymous netlog browser did not close gracefully")
			}
			finish()
		}
		cancel()
		stop()
	})
	if output != nil {
		t.Cleanup(func() {
			snapshot := output.snapshot()
			record.ProcessOutput = &snapshot
			if runtime.GOOS == "windows" {
				record.NetworkSandbox = inspectFirstNavigationNetworkSandbox(root)
				if os.Getenv("WEBMUX_BROWSER_REQUIRE_NETWORK_APPCONTAINER") == "1" &&
					(record.NetworkSandbox == nil || !record.NetworkSandbox.Observed || !record.NetworkSandbox.AppContainer || snapshot.ExecutableDeniedReports != 0 || snapshot.RestartReports != 0) {
					if record.Failure == "" {
						record.Failure = "network_service_sandbox_unverified"
					}
					t.Error("protected network service did not pass native AppContainer verification")
				}
			}
			if snapshot.DroppedBytes != 0 || snapshot.DroppedLines != 0 {
				t.Error("anonymous browser output observation exceeded bounds")
			}
		})
	}
	trace := newNavigationTrace()
	startup := time.AfterFunc(20*time.Second, cancel)
	t.Cleanup(func() { startup.Stop() })
	beginCapture := func(stage string) {
		record.TraceStartStage = stage
		record.TraceLoading = options.traceLoading
		capture, err := startFirstNavigationTrace(root, options.traceLoading)
		if err != nil {
			record.Failure = "browser_trace_start_error"
			t.Fatal("anonymous navigation trace could not start")
		}
		t.Cleanup(func() {
			record.BrowserTrace, err = capture.finish(root, requested)
			if err != nil || record.BrowserTrace.MatchedEvents == 0 {
				if record.Failure == "" {
					record.Failure = "browser_trace_capture_error"
				}
				t.Error("anonymous navigation trace did not retain a matching navigation")
			}
		})
	}
	if options.browserTrace && !options.traceLate {
		beginCapture("before_target")
	}
	err := chromedp.Do(root, chromedp.EmulateViewport(Width, Height), chromedp.Func(func(_ context.Context, target *chromedp.Target) error {
		trace.startup(string(target.TargetID))
		trace.watch(root)
		return nil
	}), chromedp.Navigate("about:blank"))
	startup.Stop()
	trace.startupResult(err)
	if err != nil {
		record.Failure = "direct_startup_error"
		record.Diagnostics = trace.snapshot()
		t.Fatal("direct native Chromium startup failed")
	}
	if options.browserTrace && options.traceLate {
		beginCapture("after_blank")
	}
	active := root
	record.TargetSelection = "initial"
	if options.createChild {
		// Both members of each matched pair create and initialize a fresh target
		// in the original browser context. Only the selected navigation target
		// differs; extra setup is not exclusive to the child-target trial.
		child, closeChild := chromedp.NewContext(root)
		t.Cleanup(closeChild)
		childStartup := time.AfterFunc(20*time.Second, closeChild)
		err = chromedp.Do(child, chromedp.EmulateViewport(Width, Height), chromedp.Func(func(_ context.Context, target *chromedp.Target) error {
			trace.watch(child)
			return nil
		}), chromedp.Navigate("about:blank"))
		childStartup.Stop()
		if err != nil {
			record.Failure = "direct_child_startup_error"
			record.Diagnostics = trace.snapshot()
			t.Fatal("direct child target startup failed")
		}
		originalContext, childContext := chromedp.FromContext(root), chromedp.FromContext(child)
		if originalContext.Browser != childContext.Browser || originalContext.BrowserContextID != childContext.BrowserContextID || originalContext.Target.TargetID == childContext.Target.TargetID {
			record.Failure = "direct_child_identity_mismatch"
			t.Fatal("child must be a distinct target in the same browser and browser context")
		}
		record.ExtraTargets = 1
		record.TargetSelection = "initial-after-child"
		if !options.selectOriginal {
			active = child
			record.TargetSelection = "child"
		}
	}
	// Query version only after the trial, so metadata collection cannot warm
	// the browser before its first network navigation.
	t.Cleanup(func() {
		ctx, finish := context.WithTimeout(root, 2*time.Second)
		defer finish()
		version, err := cdp.Call(ctx, chromedp.FromContext(root).Browser, browsercdp.GetVersion, cdp.Empty{})
		if err != nil {
			return
		}
		for _, prefix := range []string{"Chrome/", "HeadlessChrome/", "Chromium/"} {
			if !strings.HasPrefix(version.Product, prefix) || len(version.Product) > 64 {
				continue
			}
			numeric := strings.TrimPrefix(version.Product, prefix)
			valid := numeric != ""
			for _, char := range numeric {
				if char != '.' && (char < '0' || char > '9') {
					valid = false
				}
			}
			if valid {
				record.BrowserProduct = version.Product
			}
		}
	})

	if options.screenshot {
		ctx, finish := context.WithTimeout(root, 3*time.Second)
		err = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
			_, err := cdp.Call(ctx, target, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatJpeg})
			return err
		}))
		finish()
		if err != nil {
			record.Failure = "direct_initial_screenshot_error"
			t.Fatal("direct initial screenshot failed")
		}
	}

	trace.ready()
	return func(req Request) Frame {
		t.Helper()
		ctx, finish := context.WithTimeout(active, 15*time.Second)
		defer finish()
		if req.Action == "navigate" {
			requested = req.URL
			if output != nil {
				output.setPhase("navigation")
				defer output.setPhase("after_navigation")
			}
			target := chromedp.FromContext(active).Target
			trace.begin(string(target.TargetID), req.URL)
			result, err := cdp.Call(ctx, target, page.Navigate, page.NavigateParams{URL: req.URL})
			trace.record(NavigationEvent{Event: "navigate_ack", Target: string(target.TargetID), Frame: string(result.FrameID), Loader: string(result.LoaderID), ErrorClass: navigationErrorClass(result.ErrorText, err)}, "")
			record.Diagnostics = trace.snapshot()
			if err != nil {
				record.Failure = "direct_protocol_error"
				t.Fatal("direct navigation protocol failed")
			}
		}
		infos, err := chromedp.Targets(ctx)
		if err != nil {
			record.Failure = "direct_target_query_error"
			t.Fatal("direct target query failed")
		}
		frame := Frame{Target: string(chromedp.FromContext(active).Target.TargetID), NavigationDiagnostics: trace.snapshot()}
		for _, info := range infos {
			if info.Type == "page" {
				frame.Tabs = append(frame.Tabs, Tab{ID: string(info.TargetID), Title: info.Title})
				if string(info.TargetID) == frame.Target {
					frame.URL = info.URL
				}
			}
		}
		return frame
	}
}
