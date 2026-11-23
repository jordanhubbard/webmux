package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestCoordinationSnapshotDoesNotWaitForOtherTerminalLaunch(t *testing.T) {
	coordinationSnapshotDuringLaunch(t, false)
}

func TestCoordinationSnapshotDoesNotWaitForInterfaceProvisioning(t *testing.T) {
	coordinationSnapshotDuringLaunch(t, true)
}

func coordinationSnapshotDuringLaunch(t *testing.T, provisioning bool) {
	t.Helper()
	b, _, launched := fixture(t)
	existing := create(t, b, "owner")
	<-launched
	entered, release := make(chan struct{}), make(chan struct{})
	if provisioning {
		b.SetCoordinationLaunch(func([]Session, Session) (*terminal.CoordinationAccess, error) {
			close(entered)
			<-release
			return nil, nil
		})
	} else {
		b.mu.Lock()
		b.launch = func(terminal.LaunchRequest, string) (process, error) {
			close(entered)
			<-release
			return fake(), nil
		}
		b.mu.Unlock()
	}
	// Release before fixture cleanup even when the control fails.
	created := make(chan struct{})
	defer func() { close(release); <-created }()
	go func() {
		defer close(created)
		_, _ = b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "blocked"})
	}()
	<-entered
	snapshots := make(chan []Session, 1)
	go func() {
		_ = b.WithCoordinationSnapshot(func(all []Session) error { snapshots <- all; return nil })
	}()
	select {
	case all := <-snapshots:
		found := false
		for _, s := range all {
			if s.ID == existing.ID {
				found = s.CoordinationLive
			}
			if s.ID != existing.ID && s.CoordinationLive {
				t.Error("pending launch granted live authority")
			}
		}
		if !found {
			t.Fatal("existing terminal lost live authority during another launch")
		}
	case <-time.After(time.Second):
		t.Fatalf("coordination blocked behind another terminal's startup (provisioning=%v)", provisioning)
	}
}

func TestCoordinationProvisioningUsesFreshSerializedSnapshot(t *testing.T) {
	b, _, launched := fixture(t)
	existing := create(t, b, "owner")
	<-launched
	entered, release := make(chan Session, 1), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	observed := make(chan []Session, 1)
	b.SetCoordinationLaunch(func(all []Session, current Session) (*terminal.CoordinationAccess, error) {
		if current.ExecCommand == "first" {
			entered <- current
			<-release
		} else {
			observed <- all
		}
		return nil, nil
	})
	firstDone := make(chan struct{})
	var firstErr error
	go func() {
		defer close(firstDone)
		_, firstErr = b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "first"})
	}()
	defer func() { unblock(); <-firstDone }()
	var first Session
	select {
	case first = <-entered:
	case <-firstDone:
		t.Fatalf("first creation failed before provisioning: %v", firstErr)
	case <-time.After(5 * time.Second):
		t.Fatal("first provisioner did not start")
	}
	secondDone := make(chan struct{})
	var second Session
	var secondErr error
	go func() {
		defer close(secondDone)
		second, secondErr = b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "second"})
	}()
	defer func() { unblock(); <-secondDone }()
	// Wait until the second launch has registered, without letting a regression
	// that holds b.mu strand the test before its cleanup can release the barrier.
	eventually(t, func() bool {
		result := make(chan []Session, 1)
		go func() { _ = b.WithCoordinationSnapshot(func(all []Session) error { result <- all; return nil }) }()
		select {
		case all := <-result:
			return len(all) == 3
		case <-time.After(5 * time.Second):
			t.Fatal("queued provisioner blocked broker inventory")
			return false
		}
	})
	select {
	case <-observed:
		t.Fatal("provisioning callbacks ran concurrently")
	default:
	}
	if err := b.Delete("owner", first.ID); err != nil {
		t.Fatal(err)
	}
	unblock()
	for _, done := range []chan struct{}{firstDone, secondDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("queued startup did not finish")
		}
	}
	if !errors.Is(firstErr, ErrNotFound) || secondErr != nil {
		t.Fatalf("deleted=%v queued=%v", firstErr, secondErr)
	}
	all := <-observed
	if len(all) != 2 {
		t.Fatalf("queued provisioner received stale inventory: %v", all)
	}
	for _, s := range all {
		if s.ID != existing.ID && s.ID != second.ID {
			t.Fatal("deleted launch remained in queued provisioner's snapshot")
		}
	}
	if len(launched) != 1 {
		t.Fatal("a provider was launched for the deleted pending entry")
	}
}

func TestPendingTerminalLaunchCannotSurviveDeletionOrShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "delete"
		if shutdown {
			name = "shutdown"
		}
		t.Run(name, func(t *testing.T) {
			b, _, _ := fixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			provider := fake()
			b.mu.Lock()
			b.launch = func(terminal.LaunchRequest, string) (process, error) {
				close(entered)
				<-release
				return provider, nil
			}
			b.mu.Unlock()
			result := make(chan error, 1)
			go func() {
				_, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "blocked"})
				result <- err
			}()
			<-entered
			var pending Session
			if err := b.WithCoordinationSnapshot(func(all []Session) error { pending = all[0]; return nil }); err != nil {
				t.Fatal(err)
			}
			// A caller cannot turn an unpublished launch into an active participant.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := b.WaitCoordinationLaunch(ctx, pending.Owner, pending.ID); !errors.Is(err, context.Canceled) {
				t.Errorf("pending launch ignored caller cancellation: %v", err)
			}
			closed := make(chan error, 1)
			if shutdown {
				go func() { closed <- b.Close() }()
				eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.closed })
				select {
				case <-closed:
					t.Error("shutdown returned before pending provider cleanup")
				default:
				}
			} else {
				if _, err := b.Reconnect(pending.Owner, pending.ID, ""); !errors.Is(err, ErrRecoveryConflict) {
					t.Errorf("duplicate pending launch was admitted: %v", err)
				}
				if err := b.Delete(pending.Owner, pending.ID); err != nil {
					t.Error(err)
				}
			}
			close(release)
			select {
			case err := <-result:
				if shutdown && !errors.Is(err, ErrClosed) {
					t.Errorf("late creation after shutdown: %v", err)
				}
				if !shutdown && !errors.Is(err, ErrNotFound) {
					t.Errorf("late creation after deletion: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("late provider was not reaped")
			}
			select {
			case <-provider.done:
			default:
				t.Error("discarded provider was not closed")
			}
			if shutdown {
				if err := <-closed; err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestShutdownDuringCoordinationProvisioningStartsNoProvider(t *testing.T) {
	b, _, launched := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	b.SetCoordinationLaunch(func([]Session, Session) (*terminal.CoordinationAccess, error) {
		close(entered)
		<-release
		return nil, nil
	})
	created := make(chan error, 1)
	go func() {
		_, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "blocked"})
		created <- err
	}()
	defer unblock()
	select {
	case <-entered:
	case err := <-created:
		t.Fatalf("creation failed before provisioning: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("provisioner did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.closed })
	select {
	case <-closed:
		t.Fatal("shutdown returned before its provisioner finished")
	default:
	}
	unblock()
	select {
	case err := <-created:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("late creation after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("creation did not observe shutdown")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown retained a finished provisioner")
	}
	if len(launched) != 0 {
		t.Fatal("provider started after shutdown during provisioning")
	}
}

type launchResizeProcess struct {
	*fakeProcess
	resized chan [2]int
}

func TestRevokedAgentDuringProvisioningStartsNoProvider(t *testing.T) {
	b, store, launched := fixture(t)
	configureAgents(t, store, true)
	// Isolate the synchronous launch guard from the periodic policy sweep.
	b.mu.Lock()
	b.policyStarted = true
	b.mu.Unlock()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	b.SetCoordinationLaunch(func([]Session, Session) (*terminal.CoordinationAccess, error) {
		close(entered)
		<-release
		return nil, nil
	})
	type outcome struct {
		session Session
		err     error
	}
	done, result := make(chan struct{}), make(chan outcome, 1)
	go func() {
		defer close(done)
		value, _, err := b.EnsureAgentAttach("owner", "alpha", "task", 120, 40)
		result <- outcome{value, err}
	}()
	defer func() { unblock(); <-done }()
	select {
	case <-entered:
	case got := <-result:
		t.Fatalf("attach failed before provisioning: %v", got.err)
	case <-time.After(5 * time.Second):
		t.Fatal("agent provisioner did not start")
	}
	configureAgents(t, store, false)
	unblock()
	select {
	case got := <-result:
		// New-agent creation preserves its established error-state response;
		// the authoritative condition is no live or apparently pending launch.
		if got.session.State != "error" {
			t.Fatalf("revoked agent remained live/pending: state=%q err=%v", got.session.State, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent startup did not observe revocation")
	}
	if len(launched) != 0 {
		t.Fatal("provider started after agent policy was revoked during provisioning")
	}
}

func (p *launchResizeProcess) Resize(cols, rows int) error {
	p.resized <- [2]int{cols, rows}
	return nil
}

func TestPendingTerminalLaunchKeepsLatestGeometry(t *testing.T) {
	b, _, _ := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	provider := &launchResizeProcess{fakeProcess: fake(), resized: make(chan [2]int, 1)}
	b.mu.Lock()
	b.launch = func(terminal.LaunchRequest, string) (process, error) {
		close(entered)
		<-release
		return provider, nil
	}
	b.mu.Unlock()
	result := make(chan error, 1)
	go func() {
		_, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "alice", Transport: "exec", ExecCommand: "blocked"})
		result <- err
	}()
	<-entered
	var pending Session
	_ = b.WithCoordinationSnapshot(func(all []Session) error { pending = all[0]; return nil })
	if err := b.Resize(pending.Owner, pending.ID, 132, 43); err != nil {
		t.Error(err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-provider.resized:
		if size != [2]int{132, 43} {
			t.Fatal("stale startup geometry", size)
		}
	default:
		t.Fatal("pending geometry was not applied to the published provider")
	}
	if err := b.WaitCoordinationLaunch(context.Background(), pending.Owner, pending.ID); err != nil {
		t.Fatal(err)
	}
}
