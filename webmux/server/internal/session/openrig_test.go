package session

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestOpenRigAttachmentIsSharedPerOwnerAndSeparateFromGrid(t *testing.T) {
	b, store, launched := fixture(t)
	var requests []terminal.LaunchRequest
	b.launch = func(request terminal.LaunchRequest, password string) (process, error) {
		requests = append(requests, request)
		p := fake()
		launched <- p
		return p, nil
	}
	var wg sync.WaitGroup
	results := make(chan Session, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := b.EnsureOpenRig("owner", "/test/rig")
			if err != nil {
				t.Error(err)
				return
			}
			results <- value
		}()
	}
	wg.Wait()
	close(results)
	var id string
	for result := range results {
		if id != "" && id != result.ID {
			t.Fatal("concurrent views created duplicate dashboard clients")
		}
		id = result.ID
		if result.Persistent || result.Workspace != "openrig" || result.Companion != "openrig" {
			t.Fatalf("unexpected dashboard state: %+v", result)
		}
	}
	if len(requests) != 1 || !slices.Equal(requests[0].ExecArgv, []string{"/test/rig", "tui"}) {
		t.Fatalf("unexpected launches: %+v", requests)
	}
	if len(b.List("owner")) != 0 || b.OpenRigSession("other") != nil {
		t.Fatal("dashboard leaked into terminal grid or another owner")
	}
	if _, err := b.Get("other", id); !errors.Is(err, ErrNotFound) {
		t.Fatal("another owner could access the dashboard")
	}
	var layout struct {
		Layout struct {
			Tiles []any `yaml:"tiles"`
		} `yaml:"layout"`
	}
	if err := store.ReadConfig("layout.yaml", &layout); err != nil || len(layout.Layout.Tiles) != 0 {
		t.Fatalf("dashboard consumed a grid tile: %+v, %v", layout, err)
	}
	p := <-launched
	if err := b.Delete("owner", id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("detach did not stop the viewer client")
	}
	if b.OpenRigSession("owner") != nil {
		t.Fatal("detached viewer remained discoverable")
	}
}

func TestOpenRigRequiresExplicitReconnectAfterServerRestart(t *testing.T) {
	b, store, _ := fixture(t)
	first, err := b.EnsureOpenRig("owner", "/test/rig")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	launches := 0
	restored, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { launches++; return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatal("restoring WebMux launched OpenRig")
	}
	if got := restored.OpenRigSession("owner"); got == nil || got.State != "disconnected" {
		t.Fatalf("unexpected restored status: %+v", got)
	}
	second, err := restored.EnsureOpenRig("owner", "/test/rig")
	if err != nil || first.ID != second.ID || launches != 1 {
		t.Fatalf("reconnect: %+v %v, launches=%d", second, err, launches)
	}
}

func TestOpenRigPersistenceFailureDoesNotLeaveOrphanAttachment(t *testing.T) {
	b, store, launched := fixture(t)
	file := filepath.Join(store.Home, "data/sessions/sessions.yaml")
	if err := os.MkdirAll(file, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(file); err != nil {
			t.Error(err)
		}
	}()
	if _, err := b.EnsureOpenRig("owner", "/test/rig"); err == nil {
		t.Fatal("attachment succeeded without persisted state")
	}
	if b.OpenRigSession("owner") != nil {
		t.Fatal("failed attachment retained a record")
	}
	select {
	case <-(<-launched).done:
	case <-time.After(time.Second):
		t.Fatal("failed attachment leaked its client")
	}
}

func TestOpenRigViewsReuseOnlyTheirOwnerAndView(t *testing.T) {
	b, _, _ := fixture(t)
	dashboard, err := b.EnsureOpenRig("owner", "/test/rig")
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := b.EnsureOpenRigView("owner", "alpha", "Claude", []string{"tmux", "attach-session", "-t", "=proof-alpha@team"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := b.EnsureOpenRigView("owner", "alpha", "Claude", []string{"tmux", "attach-session", "-t", "=proof-alpha@team"})
	if err != nil || again.ID != alpha.ID || alpha.ID == dashboard.ID {
		t.Fatalf("view collision: %+v %+v %v", alpha, again, err)
	}
	other, err := b.EnsureOpenRigView("other", "alpha", "Claude", []string{"tmux", "attach-session", "-t", "=other"})
	if err != nil || other.ID == alpha.ID {
		t.Fatalf("owner collision: %+v %v", other, err)
	}
	if len(b.OpenRigViews("owner")) != 2 || len(b.OpenRigViews("other")) != 1 || len(b.List("owner")) != 0 {
		t.Fatal("incorrect owner/grid isolation")
	}
	if err := b.Delete("owner", alpha.ID); err != nil {
		t.Fatal(err)
	}
	if b.OpenRigSession("owner").ID != dashboard.ID {
		t.Fatal("detaching agent stopped dashboard")
	}
}

func TestManagedRigAgentsUseOrdinaryPersistentGridTiles(t *testing.T) {
	b, store, launched := fixture(t)
	argv := []string{"/test/tmux", "attach-session", "-t", "=central-seat"}
	first, err := b.EnsureRigTerminal("owner", "central-seat", "reviewer · remote", argv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.EnsureRigTerminal("owner", "central-seat", "reviewer · remote", argv)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(b.List("owner")) != 1 {
		t.Fatal("duplicate or missing ordinary tile")
	}
	if first.Companion != "" || first.Workspace != "" || !first.Persistent {
		t.Fatalf("not an ordinary persistent tile: %+v", first)
	}
	if len(b.List("other")) != 0 {
		t.Fatal("agent exposed to another owner")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	<-launched
	launches := 0
	restored, err := newBroker(store, nil, func(request terminal.LaunchRequest, _ string) (process, error) {
		launches++
		if !slices.Equal(request.ExecArgv, argv) {
			t.Fatalf("changed verified attach argv: %v", request.ExecArgv)
		}
		return fake(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	if launches != 1 || len(restored.List("owner")) != 1 {
		t.Fatal("managed terminal did not reattach after restart")
	}
}
