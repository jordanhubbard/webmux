package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func nextBrowserControlRequest(t *testing.T, v *Viewer) Event {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-v.Ready():
			if value := v.Take(); value != nil && value["type"] == "browser_open" {
				return value
			}
		case <-deadline.C:
			t.Fatal("browser control request was not delivered")
			return nil
		}
	}
}

func TestBrowserControlPendingReconnectAndDurableOwnerAcknowledgement(t *testing.T) {
	b, store, launched := fixture(t)
	s := create(t, b, "owner")
	<-launched
	b.mu.Lock()
	e := b.entries[s.ID]
	key := e.value.RecoveryKey
	b.mu.Unlock()
	raw := "https://login.example/authorize?state=private-browser-control"
	pending, err := recovery.SealBrowserRequest(s.ID, key, raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.applyRecoverySnapshotLocked(e, e.run, recovery.RPCResponse{Revision: 1, BrowserRequest: pending, BrowserObservedAt: time.Now().UnixMilli()})
	b.mu.Unlock()
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := nextBrowserControlRequest(t, v)
	if request["url"] != raw || request["request_id"] != pending.ID {
		t.Fatal("wrong pending launch")
	}
	b.Leave("owner", s.ID, v.ID)
	v, err = b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nextBrowserControlRequest(t, v)["request_id"] != pending.ID {
		t.Fatal("unacknowledged request lost on reconnect")
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	<-launched
	b.mu.Lock()
	b.applyRecoverySnapshotLocked(e, e.run, recovery.RPCResponse{Revision: 1, BrowserRequest: pending, BrowserObservedAt: time.Now().UnixMilli()})
	b.mu.Unlock()
	if nextBrowserControlRequest(t, v)["request_id"] != pending.ID {
		t.Fatal("transport reconnect lost pending launch")
	}
	if err := b.AcknowledgeBrowser("foreign", s.ID, pending.ID); err == nil {
		t.Fatal("foreign owner acknowledged launch")
	}
	if err := b.AcknowledgeBrowser("owner", s.ID, "old-id"); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	stillPending := e.browserRequest != nil
	b.mu.Unlock()
	if !stillPending {
		t.Fatal("different acknowledgement removed launch")
	}
	if err := b.AcknowledgeBrowser("owner", s.ID, pending.ID); err != nil {
		t.Fatal(err)
	}
	// A new controller reads durable acknowledgement without a graceful close
	// or another successful host RPC. An old pending host reply cannot replay it.
	next, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	next.mu.Lock()
	entry := next.entries[s.ID]
	next.applyRecoverySnapshotLocked(entry, &run{}, recovery.RPCResponse{Revision: 1, BrowserRequest: pending, BrowserObservedAt: time.Now().UnixMilli()})
	ack, replayed := entry.value.LastBrowserAck, entry.browserRequest != nil
	next.mu.Unlock()
	if ack != pending.ID || replayed {
		t.Fatal("acknowledged launch replayed after controller reconstruction")
	}
	text, _ := b.Scrollback("owner", s.ID)
	bytes, err := os.ReadFile(filepath.Join(store.Home, "data", "sessions", "sessions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "private-browser-control") || strings.Contains(string(bytes), "private-browser-control") {
		t.Fatal("URL entered terminal history or durable session metadata")
	}
}

func TestBrowserControlPersistenceFailureDoesNotClaimAcknowledgement(t *testing.T) {
	b, store, launched := fixture(t)
	s := create(t, b, "owner")
	<-launched
	b.mu.Lock()
	e := b.entries[s.ID]
	pending, err := recovery.SealBrowserRequest(s.ID, e.value.RecoveryKey, "https://login.example/pending", time.Now())
	if err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	b.applyBrowserControlLocked(e, e.run, pending, time.Now().UnixMilli())
	b.mu.Unlock()
	path := filepath.Join(store.Home, "data", "sessions", "sessions.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	err = b.AcknowledgeBrowser("owner", s.ID, pending.ID)
	if cleanup := os.Remove(path); cleanup != nil {
		t.Fatal(cleanup)
	}
	if cleanup := os.WriteFile(path, before, 0600); cleanup != nil {
		t.Fatal(cleanup)
	}
	if err == nil {
		t.Fatal("failed acknowledgement was reported successful")
	}
	b.mu.Lock()
	ack, stillPending := e.value.LastBrowserAck, e.browserRequest != nil
	b.mu.Unlock()
	if ack != "" || !stillPending {
		t.Fatal("failed persistence discarded pending request")
	}
}

type browserRPCProcess struct {
	*fakeProcess
	request recovery.RPCRequest
	pending *recovery.BrowserRequest
}

func (p *browserRPCProcess) RecoveryRPC(_ context.Context, request recovery.RPCRequest) recovery.RPCResponse {
	p.request = request
	if p.pending != nil && p.pending.ID == request.BrowserAck {
		p.pending = nil
	}
	return recovery.RPCResponse{Protocol: 1, Revision: 1, BrowserRequest: p.pending, BrowserObservedAt: time.Now().UnixMilli()}
}

func TestBrowserControlUsesExistingPolicyRPCWithRecordingOff(t *testing.T) {
	b, _, _ := fixture(t)
	p := &browserRPCProcess{fakeProcess: fake()}
	b.launch = func(terminal.LaunchRequest, string) (process, error) { return p, nil }
	s, err := b.Create("owner", CreateRequest{Transport: "mosh", Hostname: "worker.example", Username: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	e := b.entries[s.ID]
	r := e.run
	key := e.value.RecoveryKey
	b.mu.Unlock()
	p.pending, err = recovery.SealBrowserRequest(s.ID, key, "https://login.example/control", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.syncRecovery(e, r)
	if p.request.Operation != "policy" || p.request.Recording == nil || *p.request.Recording {
		t.Fatal("browser launch required recording or another RPC")
	}
	if err := b.AcknowledgeBrowser("owner", s.ID, p.pending.ID); err != nil {
		t.Fatal(err)
	}
	b.syncRecovery(e, r)
	if p.pending != nil || p.request.BrowserAck == "" {
		t.Fatal("acknowledgement not included in existing synchronization")
	}
}

func TestBrowserControlExpiryUsesTheExecutionHostsClock(t *testing.T) {
	for _, skew := range []time.Duration{-24 * time.Hour, 24 * time.Hour} {
		t.Run(skew.String(), func(t *testing.T) {
			b, _, launched := fixture(t)
			s := create(t, b, "owner")
			<-launched
			hostNow := time.Now().Add(skew)
			b.mu.Lock()
			e := b.entries[s.ID]
			pending, err := recovery.SealBrowserRequest(s.ID, e.value.RecoveryKey, "https://login.example/clock", hostNow)
			if err != nil {
				b.mu.Unlock()
				t.Fatal(err)
			}
			b.applyBrowserControlLocked(e, e.run, pending, hostNow.UnixMilli())
			visible, remaining := e.browserRequest != nil, time.Until(e.browserExpires)
			b.applyBrowserControlLocked(e, e.run, nil, hostNow.Add(recovery.BrowserRequestLifetime).UnixMilli())
			cleared := e.browserRequest == nil
			b.mu.Unlock()
			if !visible || remaining < recovery.BrowserRequestLifetime-time.Second || remaining > recovery.BrowserRequestLifetime {
				t.Fatal("clock skew changed pending lifetime", visible, remaining)
			}
			if !cleared {
				t.Fatal("host expiry left a request available for reconnect")
			}
		})
	}
}
