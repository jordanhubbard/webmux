//go:build darwin

package discovery

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/sys/unix"
)

type fakeNativeRegistration struct {
	reader, writer int
	released       int
}
type fakeNativeDNS struct {
	mu             sync.Mutex
	entries        map[uintptr]*fakeNativeRegistration
	indexes        []uint32
	acknowledgment int32
	runtimeError   atomic.Int32
	noAck          bool
	failAt         int
}

var fakeNativeID atomic.Uint64

func (f *fakeNativeDNS) api(t *testing.T) *nativeDNS {
	t.Helper()
	f.entries = map[uintptr]*fakeNativeRegistration{}
	return &nativeDNS{
		register: func(ref *uintptr, flags, index uint32, name, kind, domain string, host uintptr, port, length uint16, txt *byte, callback, opaque uintptr) int32 {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.indexes = append(f.indexes, index)
			if flags != 8 || host != 0 || domain != "local." {
				t.Error("registration must preserve identity and delegate native host ownership")
			}
			if f.failAt > 0 && len(f.indexes) == f.failAt {
				return -65563
			}
			descriptors := make([]int, 2)
			if err := unix.Pipe(descriptors); err != nil {
				t.Fatal(err)
			}
			*ref = uintptr(fakeNativeID.Add(1))
			f.entries[*ref] = &fakeNativeRegistration{reader: descriptors[0], writer: descriptors[1]}
			if !f.noAck {
				if _, err := unix.Write(descriptors[1], []byte{1}); err != nil {
					t.Fatal(err)
				}
			}
			return 0
		},
		socket: func(ref uintptr) int32 { f.mu.Lock(); defer f.mu.Unlock(); return int32(f.entries[ref].reader) },
		process: func(ref uintptr) int32 {
			f.mu.Lock()
			entry := f.entries[ref]
			f.mu.Unlock()
			var event [1]byte
			if _, err := unix.Read(entry.reader, event[:]); err != nil {
				return -65563
			}
			if code := f.runtimeError.Load(); code != 0 {
				return code
			}
			nativeRegistrationReply(ref, 0, f.acknowledgment, 0, 0, 0, 0)
			return 0
		},
		release: func(ref uintptr) {
			f.mu.Lock()
			defer f.mu.Unlock()
			entry := f.entries[ref]
			entry.released++
			if entry.released != 1 {
				t.Error("native reference released more than once")
				return
			}
			_ = unix.Close(entry.reader)
			_ = unix.Close(entry.writer)
		},
	}
}

func (f *fakeNativeDNS) requireReleased(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for ref, entry := range f.entries {
		if entry.released != 1 {
			t.Errorf("native reference release count %d", entry.released)
		}
		if _, ok := nativeRegistrations.Load(ref); ok {
			t.Error("released callback routing entry retained")
		}
	}
}
func nativeFixtureServices(t *testing.T) []mdns.ServiceInstance {
	t.Helper()
	service, err := nodeService(NodeOptions{ID: strings.Repeat("a", 32), Host: "fixture", Port: 19311, TLS: true})
	if err != nil {
		t.Fatal(err)
	}
	return []mdns.ServiceInstance{service}
}
func TestNativeAnnouncementsCancellationAndScope(t *testing.T) {
	fake := &fakeNativeDNS{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := make(chan error, 1)
	a, err := startNativeAnnouncementsWithAPI(ctx, fake.api(t), nativeFixtureServices(t), []net.Interface{{Index: 5}, {Index: 7}, {Index: 5}}, func(err error) { failed <- err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if len(fake.indexes) != 2 || fake.indexes[0] != 5 || fake.indexes[1] != 7 {
		t.Fatal("explicit interface scope was not preserved", fake.indexes)
	}
	cancel()
	select {
	case <-a.done:
	case <-time.After(time.Second):
		t.Fatal("owned registrations did not close after parent cancellation")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failed:
		t.Fatal("intentional cancellation reported as failure", err)
	default:
	}
	fake.requireReleased(t)
}
func TestNativeAnnouncementsStartupFailuresReleaseOwnedReferences(t *testing.T) {
	for _, test := range []struct {
		name    string
		fake    *fakeNativeDNS
		timeout time.Duration
		want    string
	}{
		{name: "daemon rejects registration", fake: &fakeNativeDNS{acknowledgment: -65548}, timeout: time.Second, want: "-65548"},
		{name: "partial start failure", fake: &fakeNativeDNS{failAt: 2}, timeout: time.Second, want: "-65563"},
		{name: "no acknowledgment", fake: &fakeNativeDNS{noAck: true}, timeout: 40 * time.Millisecond, want: "deadline exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.timeout)
			defer cancel()
			fake := test.fake
			a, err := startNativeAnnouncementsWithAPI(ctx, fake.api(t), nativeFixtureServices(t), []net.Interface{{Index: 5}, {Index: 7}}, func(error) { t.Error("startup failure escaped as runtime callback") })
			if a != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected startup result: %v %v", a, err)
			}
			fake.requireReleased(t)
		})
	}
}
func TestNativeAnnouncementsRuntimeFailureStopsBrowserAndReleases(t *testing.T) {
	fake := &fakeNativeDNS{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	browser := &NodeBrowser{cancel: cancel}
	failed := make(chan error, 1)
	a, err := startNativeAnnouncementsWithAPI(ctx, fake.api(t), nativeFixtureServices(t), nil, func(err error) { browser.fail(err); failed <- err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	fake.runtimeError.Store(-65563)
	fake.mu.Lock()
	for _, entry := range fake.entries {
		if _, err := unix.Write(entry.writer, []byte{2}); err != nil {
			t.Fatal(err)
		}
	}
	fake.mu.Unlock()
	select {
	case err := <-failed:
		if !strings.Contains(err.Error(), "-65563") {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime failure not reported")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("browser continued after terminal advertisement failure")
	}
	if browser.Err() == nil || !strings.Contains(browser.Err().Error(), "-65563") {
		t.Fatal("terminal discovery status was lost")
	}
	select {
	case <-a.done:
	case <-time.After(time.Second):
		t.Fatal("runtime failure leaked native registrations")
	}
	if err := a.Close(); err == nil {
		t.Fatal("terminal transport error lost at close")
	}
	fake.requireReleased(t)
	// Preserve the first cause when multiple transports report cancellation noise.
	browser.fail(errors.New("later cancellation noise"))
	if !strings.Contains(browser.Err().Error(), "-65563") {
		t.Fatal("first failure overwritten")
	}
}
