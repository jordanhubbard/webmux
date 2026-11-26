//go:build windows

package browser

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/systeminfo"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/windows"
)

type browserExitIdentity struct {
	pid     uint32
	created uint64
	image   string
}

func processExitIdentity(pid uint32) (browserExitIdentity, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return browserExitIdentity{}, err
	}
	defer windows.CloseHandle(process)
	return readBrowserExitIdentity(process, pid)
}

func readBrowserExitIdentity(process windows.Handle, pid uint32) (browserExitIdentity, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return browserExitIdentity{}, err
	}
	var image [32768]uint16
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(process, 0, &image[0], &size); err != nil {
		return browserExitIdentity{}, err
	}
	return browserExitIdentity{pid: pid, created: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), image: windows.UTF16ToString(image[:size])}, nil
}

func prepareBrowserProcessExit(ctx context.Context) (func(context.Context) error, error) {
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return nil, nil
	} // No allocated browser to join.
	if c.Browser.Process() == nil {
		return nil, errBrowserExitIdentity
	}
	pid := c.Browser.Process().Pid
	if pid <= 0 || uint64(pid) > 0xffffffff {
		return nil, errBrowserExitIdentity
	}
	parent, err := processExitIdentity(uint32(pid))
	if err != nil {
		return nil, errBrowserExitIdentity
	}
	info, err := cdp.Call(ctx, c.Browser, systeminfo.GetProcessInfo, cdp.Empty{})
	if err != nil || len(info.ProcessInfo) == 0 || len(info.ProcessInfo) > 64 {
		return nil, errBrowserExitIdentity
	}
	identities := []browserExitIdentity{}
	parentReported := false
	for _, p := range info.ProcessInfo {
		if p.ID <= 0 || p.ID > 0xffffffff {
			return nil, errBrowserExitIdentity
		}
		current, err := processExitIdentity(uint32(p.ID))
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			continue
		} // Already exited before a handle could be retained.
		if err != nil || current.created < parent.created || !strings.EqualFold(current.image, parent.image) {
			return nil, errBrowserExitIdentity
		}
		if current.pid == parent.pid {
			if current.created != parent.created {
				return nil, errBrowserExitIdentity
			}
			parentReported = true
		}
		identities = append(identities, current)
	}
	if !parentReported {
		return nil, errBrowserExitIdentity
	}
	return retainBrowserExitHandles(identities)
}

// The caller supplies a previously proven private browser process set. Recheck
// every immutable creation identity before retaining SYNCHRONIZE handles. No
// bare PID is used for waiting after cancellation; PID reuse is refused.
func retainBrowserExitHandles(identities []browserExitIdentity) (func(context.Context) error, error) {
	if len(identities) == 0 || len(identities) > 64 {
		return nil, errBrowserExitIdentity
	}
	handles := []windows.Handle{}
	closeAll := func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	}
	seen := map[uint32]bool{}
	for _, expected := range identities {
		if expected.pid == 0 || expected.created == 0 || expected.image == "" || seen[expected.pid] {
			closeAll()
			return nil, errBrowserExitIdentity
		}
		seen[expected.pid] = true
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, expected.pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			continue
		}
		if err != nil {
			closeAll()
			return nil, errBrowserExitIdentity
		}
		actual, err := readBrowserExitIdentity(handle, expected.pid)
		if err != nil || actual.created != expected.created || !strings.EqualFold(actual.image, expected.image) {
			windows.CloseHandle(handle)
			closeAll()
			return nil, errBrowserExitIdentity
		}
		handles = append(handles, handle)
	}
	var once sync.Once
	var result error
	return func(ctx context.Context) error {
		once.Do(func() {
			defer closeAll()
			if len(handles) == 0 {
				return
			}
			for {
				if ctx.Err() != nil {
					result = errBrowserExitWait
					return
				}
				// Event waits, not a sleep or repeated removal. Short bounded waits also
				// honor cancellation when the caller's deadline is shortened.
				status, err := windows.WaitForMultipleObjects(handles, true, 20)
				if err == nil && status == windows.WAIT_OBJECT_0 {
					return
				}
				if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
					result = errBrowserExitWait
					return
				}
			}
		})
		return result
	}, nil
}
