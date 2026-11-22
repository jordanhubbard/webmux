package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestCoordinationSnapshotDoesNotWaitForOtherTerminalLaunch(t *testing.T) {
	b, _, launched := fixture(t)
	existing := create(t, b, "owner")
	<-launched
	entered, release := make(chan struct{}), make(chan struct{})
	b.mu.Lock()
	b.launch = func(terminal.LaunchRequest, string) (process, error) {
		close(entered)
		<-release
		return fake(), nil
	}
	b.mu.Unlock()
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
		t.Fatal("coordination blocked behind another terminal's provider launch")
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

type launchResizeProcess struct {
	*fakeProcess
	resized chan [2]int
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
