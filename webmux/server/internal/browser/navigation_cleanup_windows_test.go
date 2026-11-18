//go:build windows

package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/systeminfo"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/windows"
)

// Observe only processes reported by this fixture's private CDP browser. Keep
// read-only native handles across shutdown, so later PID reuse cannot change
// the exit observation. No PID, image path, command line or token is exported.
func observeFirstNavigationCleanup(root context.Context) func() *firstNavigationCleanupProcesses {
	if os.Getenv("WEBMUX_BROWSER_CLEANUP_PROCESSES") != "1" {
		return func() *firstNavigationCleanupProcesses { return nil }
	}
	r := &firstNavigationCleanupProcesses{State: "unavailable", ParentState: "unavailable"}
	var parent windows.Handle
	var children []windows.Handle
	finish := func() *firstNavigationCleanupProcesses {
		if parent != 0 {
			status, err := windows.WaitForSingleObject(parent, 0)
			if err == nil && status == windows.WAIT_OBJECT_0 {
				r.ParentState = "exited"
			} else if err == nil && status == uint32(windows.WAIT_TIMEOUT) {
				r.ParentState = "running"
			}
			windows.CloseHandle(parent)
		}
		for _, process := range children {
			status, err := windows.WaitForSingleObject(process, 0)
			if err == nil && status == windows.WAIT_OBJECT_0 {
				r.ExitedChildren++
			} else if err == nil && status == uint32(windows.WAIT_TIMEOUT) {
				r.RunningChildren++
			} else {
				r.Unqueryable++
			}
			windows.CloseHandle(process)
		}
		return r
	}
	c := chromedp.FromContext(root)
	if c == nil || c.Browser == nil || c.Browser.Process() == nil {
		return finish
	}
	identity := func(process windows.Handle) (string, uint64, error) {
		var created, exited, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
			return "", 0, err
		}
		var image [1024]uint16
		size := uint32(len(image))
		if err := windows.QueryFullProcessImageName(process, 0, &image[0], &size); err != nil {
			return "", 0, err
		}
		return windows.UTF16ToString(image[:size]), uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
	}
	var err error
	parent, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(c.Browser.Process().Pid))
	if err != nil {
		return finish
	}
	parentImage, parentCreated, err := identity(parent)
	if err != nil {
		return finish
	}
	ctx, cancel := context.WithTimeout(root, 2*time.Second)
	defer cancel()
	info, err := cdp.Call(ctx, c.Browser, systeminfo.GetProcessInfo, cdp.Empty{})
	if err != nil {
		return finish
	}
	if len(info.ProcessInfo) > 64 {
		r.State = "process_limit"
		return finish
	}
	parentReported := false
	for _, p := range info.ProcessInfo {
		parentReported = parentReported || p.ID == int64(c.Browser.Process().Pid)
	}
	if !parentReported {
		r.State = "parent_not_reported"
		return finish
	}
	r.State = "observed"
	for _, p := range info.ProcessInfo {
		if p.ID == int64(c.Browser.Process().Pid) {
			continue
		}
		r.ReportedChildren++
		if p.ID <= 0 || p.ID > 0xffffffff {
			r.Unqueryable++
			continue
		}
		process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.ID))
		if err != nil {
			r.Unqueryable++
			continue
		}
		image, created, err := identity(process)
		if err != nil || !strings.EqualFold(image, parentImage) || created < parentCreated {
			windows.CloseHandle(process)
			r.Unqueryable++
			continue
		}
		children = append(children, process)
		r.ObservedChildren++
	}
	return finish
}

func TestFirstNavigationCleanupHeldFileReportsNativeSharingViolation(t *testing.T) {
	profile := t.TempDir()
	if err := os.Mkdir(filepath.Join(profile, "Default"), 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(profile, "Default", "Cookies")
	if err := os.WriteFile(name, []byte("owned synthetic marker"), 0600); err != nil {
		t.Fatal(err)
	}
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	removeErr := os.RemoveAll(profile)
	closeErr := windows.CloseHandle(handle)
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if removeErr == nil {
		t.Fatal("retained native file handle did not refuse profile removal")
	}
	r := classifyFirstNavigationRemoval(removeErr, profile)
	if r.Class != "sharing_violation" || r.OSCode != 32 || r.FileCategory != "cookie_store" {
		t.Fatalf("wrong native removal classification: %+v", r)
	}
	if err := os.RemoveAll(profile); err != nil {
		t.Fatalf("owned profile remained after native handle close: %+v", classifyFirstNavigationRemoval(err, profile))
	}
}
