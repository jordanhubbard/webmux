//go:build linux

package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/systeminfo"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/unix"
)

type browserExitIdentity struct {
	pid     int
	created uint64
	image   string
	uid     uint32
}

func processExitIdentity(pid int) (browserExitIdentity, error) {
	root := fmt.Sprintf("/proc/%d", pid)
	info, err := os.Stat(root)
	if err != nil {
		return browserExitIdentity{}, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return browserExitIdentity{}, errBrowserExitIdentity
	}
	data, err := os.ReadFile(root + "/stat")
	if err != nil {
		return browserExitIdentity{}, err
	}
	// comm may contain spaces and parentheses. Fields after its final ')' start
	// at field 3; starttime is field 22 and is immutable for this process.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return browserExitIdentity{}, errBrowserExitIdentity
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return browserExitIdentity{}, errBrowserExitIdentity
	}
	created, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || created == 0 {
		return browserExitIdentity{}, errBrowserExitIdentity
	}
	image, err := os.Readlink(root + "/exe")
	if err != nil {
		return browserExitIdentity{}, err
	}
	return browserExitIdentity{pid: pid, created: created, image: image, uid: owner.Uid}, nil
}

func prepareBrowserProcessExit(ctx context.Context) (func(context.Context) error, error) {
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return nil, nil
	}
	if c.Browser.Process() == nil || c.Browser.Process().Pid <= 0 {
		return nil, errBrowserExitIdentity
	}
	parent, err := processExitIdentity(c.Browser.Process().Pid)
	if err != nil {
		return nil, errBrowserExitIdentity
	}
	info, err := cdp.Call(ctx, c.Browser, systeminfo.GetProcessInfo, cdp.Empty{})
	if err != nil || len(info.ProcessInfo) == 0 || len(info.ProcessInfo) > 64 {
		return nil, errBrowserExitIdentity
	}
	var identities []browserExitIdentity
	parentReported := false
	for _, p := range info.ProcessInfo {
		if p.ID <= 0 || int64(int(p.ID)) != p.ID {
			return nil, errBrowserExitIdentity
		}
		current, err := processExitIdentity(int(p.ID))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ESRCH) {
			continue // It exited before its identity could be retained.
		}
		if err != nil || current.created < parent.created || current.image != parent.image || current.uid != parent.uid {
			return nil, errBrowserExitIdentity
		}
		if current.pid == parent.pid {
			if current != parent {
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

// Open pidfds before cancellation and recheck immutable identities. Waiting on
// these descriptors never follows a reused PID or signals another process.
// Unsupported kernels or unverifiable processes fail closed, retaining files.
func retainBrowserExitHandles(identities []browserExitIdentity) (func(context.Context) error, error) {
	if len(identities) == 0 || len(identities) > 64 {
		return nil, errBrowserExitIdentity
	}
	var handles []unix.PollFd
	closeAll := func() {
		for _, handle := range handles {
			_ = unix.Close(int(handle.Fd))
		}
	}
	seen := map[int]bool{}
	for _, expected := range identities {
		if expected.pid <= 0 || expected.created == 0 || expected.image == "" || expected.uid != uint32(os.Getuid()) || seen[expected.pid] {
			closeAll()
			return nil, errBrowserExitIdentity
		}
		seen[expected.pid] = true
		fd, err := unix.PidfdOpen(expected.pid, 0)
		if errors.Is(err, unix.ESRCH) {
			continue
		}
		if err != nil {
			closeAll()
			return nil, errBrowserExitIdentity
		}
		actual, err := processExitIdentity(expected.pid)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ESRCH) {
			_ = unix.Close(fd)
			continue
		}
		if err != nil || actual != expected {
			_ = unix.Close(fd)
			closeAll()
			return nil, errBrowserExitIdentity
		}
		handles = append(handles, unix.PollFd{Fd: int32(fd), Events: unix.POLLIN})
	}
	var once sync.Once
	var result error
	return func(ctx context.Context) error {
		once.Do(func() {
			defer closeAll()
			pending := append([]unix.PollFd(nil), handles...)
			for len(pending) != 0 {
				if ctx.Err() != nil {
					result = errBrowserExitWait
					return
				}
				budget := 20 * time.Millisecond
				if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < budget {
					budget = max(time.Until(deadline), time.Nanosecond)
				}
				_, err := unix.Poll(pending, int((budget+time.Millisecond-1)/time.Millisecond))
				if errors.Is(err, unix.EINTR) {
					continue
				}
				if err != nil {
					result = errBrowserExitWait
					return
				}
				remaining := pending[:0]
				for _, handle := range pending {
					if handle.Revents & ^int16(unix.POLLIN|unix.POLLHUP) != 0 {
						result = errBrowserExitWait
						return
					}
					if handle.Revents&(unix.POLLIN|unix.POLLHUP) == 0 {
						remaining = append(remaining, handle)
					}
				}
				pending = remaining
			}
		})
		return result
	}, nil
}
