package browser

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestControllerLeaseIsolationAndExpiry(t *testing.T) {
	now := time.Unix(123, 0)
	l := controllerLease{now: func() time.Time { return now }}
	a, b := strings.Repeat("a", 32), strings.Repeat("b", 32)
	first, err := l.control(a, "", true, false, false)
	if err != nil || !first.Owned || first.Token == "" {
		t.Fatal(first, err)
	}
	observer, err := l.control(b, "", true, false, false)
	if err != nil || observer.Owned || observer.Available || observer.Token != "" {
		t.Fatal(observer, err)
	}
	if err := l.authorize(b, first.Token); !errors.Is(err, ErrControl) {
		t.Fatal("token crossed viewers", err)
	}
	if _, err := l.control(b, "", false, false, true); !errors.Is(err, ErrControl) {
		t.Fatal("observer released lease", err)
	}
	taken, err := l.control(b, "", true, true, false)
	if err != nil || !taken.Owned || taken.Token == first.Token {
		t.Fatal(taken, err)
	}
	if err := l.authorize(a, first.Token); !errors.Is(err, ErrControl) {
		t.Fatal("stale token survived takeover", err)
	}
	now = now.Add(ControlDuration)
	if err := l.authorize(b, taken.Token); !errors.Is(err, ErrControl) {
		t.Fatal("expired lease authorized", err)
	}
	renewed, err := l.control(a, "", true, false, false)
	if err != nil || !renewed.Owned {
		t.Fatal(renewed, err)
	}
	released, err := l.control(a, renewed.Token, false, false, true)
	if err != nil || released.Owned || !released.Available || released.Token != "" {
		t.Fatal(released, err)
	}
}

func TestQueuedBrowserMutationRechecksLeaseAtDispatch(t *testing.T) {
	now := time.Unix(100, 0)
	c := &client{done: make(chan struct{}), stopped: make(chan struct{}), control: controllerLease{now: func() time.Time { return now }}}
	a := strings.Repeat("a", 32)
	grant, _ := c.control.control(a, "", true, false, false)
	m := Manager{clients: map[string]*client{"session": c}}
	c.mu.Lock()
	result := make(chan error, 1)
	go func() {
		_, err := m.CallControlled(context.Background(), "session", Request{Action: "navigate", URL: "https://example.com", Viewer: a, ControlToken: grant.Token})
		result <- err
	}()
	// Expire while the operation waits on the worker mutex. No worker is present:
	// authorization must fail before trying any pipe I/O.
	now = now.Add(ControlDuration)
	c.mu.Unlock()
	select {
	case err := <-result:
		if !errors.Is(err, ErrControl) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stale request reached worker")
	}
}
