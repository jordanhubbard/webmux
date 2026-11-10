package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	identity := RigIdentity{Target: "central-seat", Project: "demo-release", Name: "reviewer", Host: "remote", Runtime: "codex"}
	first, err := b.EnsureRigTerminal("owner", "central-seat", "reviewer · remote", argv, identity)
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
	if first.Rig == nil || *first.Rig != identity || second.Rig == nil || *second.Rig != identity {
		t.Fatal("explicit identity missing")
	}
	first.Rig.Name = "client mutation"
	if b.entries[first.ID].value.Rig.Name != identity.Name {
		t.Fatal("returned identity aliases broker state")
	}
	values := restored.List("owner")
	if len(values) != 1 || values[0].Rig == nil || *values[0].Rig != identity {
		t.Fatal("identity did not survive persistence")
	}
	if launches != 0 || len(restored.List("owner")) != 1 {
		t.Fatal("managed terminal reused saved arguments before provider validation")
	}
	resolve := func(_ context.Context, owner string, saved RigIdentity) ([]string, error) {
		if owner != "owner" || saved != identity {
			t.Fatal("restoration resolved another owner or provider", owner, saved)
		}
		return argv, nil
	}
	if err := restored.RestoreManaged(context.Background(), resolve); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreManaged(context.Background(), resolve); err != nil || launches != 1 {
		t.Fatal("managed restoration did not reattach once", launches, err)
	}
}

func TestManagedRigOpenRestoresMinimizedTileWithoutOverlapOrDuplicateViewer(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore", true: "full grid"}[full], func(t *testing.T) {
			b, store, launched := fixture(t)
			argv := []string{"/test/tmux", "attach-session", "-t", "=seat"}
			first, err := b.EnsureRigTerminal("owner", "seat", "coder", argv)
			if err != nil {
				t.Fatal(err)
			}
			other, err := b.EnsureRigTerminal("owner", "other", "reviewer", argv)
			if err != nil {
				t.Fatal(err)
			}
			minimized := true
			if _, err := b.Patch("owner", first.ID, Patch{Minimized: &minimized}); err != nil {
				t.Fatal(err)
			}
			if _, err := b.Patch("owner", other.ID, Patch{Row: &first.Row, Col: &first.Col}); err != nil {
				t.Fatal(err)
			}
			if full {
				if err := store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"terminal_grid": map[string]any{"max_cols": 1, "max_rows": 1}}}); err != nil {
					t.Fatal(err)
				}
			}
			reopened, err := b.EnsureRigTerminal("owner", "seat", "coder", argv)
			if full {
				if err == nil {
					t.Fatal("restored terminal bypassed a full grid")
				}
				unchanged, getErr := b.Get("owner", first.ID)
				if getErr != nil || !unchanged.Minimized || unchanged.Row != first.Row || unchanged.Col != first.Col {
					t.Fatalf("failed restore changed saved state: %+v, %v", unchanged, getErr)
				}
			} else {
				if err != nil || reopened.ID != first.ID || reopened.Minimized {
					t.Fatalf("terminal was not restored: %+v, %v", reopened, err)
				}
				if reopened.Row == first.Row && reopened.Col == first.Col {
					t.Fatal("restored tile overlaps the reviewer")
				}
				repeated, err := b.EnsureRigTerminal("owner", "seat", "coder", argv)
				if err != nil || repeated.ID != reopened.ID {
					t.Fatalf("repeated open changed the viewer: %+v, %v", repeated, err)
				}
			}
			if len(b.List("owner")) != 2 || len(launched) != 2 {
				t.Fatal("opening the minimized terminal duplicated a viewer")
			}
		})
	}
}

func TestManagedRigPlacementPreservesSeatAndRejectsOccupiedCell(t *testing.T) {
	b, _, launched := fixture(t)
	argv := []string{"/test/tmux", "attach-session", "-t", "=seat"}
	row, col := 2, 1
	first, err := b.EnsureRigTerminalAt("owner", "seat", "coder", argv, &row, &col)
	if err != nil || first.Row != row || first.Col != col {
		t.Fatalf("requested placement: %+v, %v", first, err)
	}
	other, err := b.EnsureRigTerminal("owner", "other", "reviewer", argv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureRigTerminalAt("owner", "seat", "coder", argv, &other.Row, &other.Col); err == nil {
		t.Fatal("occupied cell accepted")
	}
	unchanged, err := b.Get("owner", first.ID)
	if err != nil || unchanged.Row != row || unchanged.Col != col {
		t.Fatalf("collision moved viewer: %+v, %v", unchanged, err)
	}
	newRow, newCol := 3, 1
	moved, err := b.EnsureRigTerminalAt("owner", "seat", "coder", argv, &newRow, &newCol)
	if err != nil || moved.ID != first.ID || moved.Row != newRow || moved.Col != newCol {
		t.Fatalf("viewer move: %+v, %v", moved, err)
	}
	if len(b.List("owner")) != 2 || len(launched) != 2 {
		t.Fatal("viewer move relaunched a seat")
	}
}

func TestManagedReconnectRequiresFreshOwnerBoundAttachment(t *testing.T) {
	b, _, _ := fixture(t)
	var requests []terminal.LaunchRequest
	var processes []*fakeProcess
	b.launch = func(request terminal.LaunchRequest, _ string) (process, error) {
		requests = append(requests, request)
		p := fake()
		processes = append(processes, p)
		return p, nil
	}
	identity := RigIdentity{Provider: "agent-deck", Target: "agent-deck:remote:seat", Host: "remote", Workspace: "/project"}
	original := []string{"/old/agent-deck", "remote", "attach", "remote", "seat"}
	value, err := b.EnsureRigTerminal("owner", identity.Target, "coder", original, identity)
	if err != nil {
		t.Fatal(err)
	}
	fresh := []string{"/current/agent-deck", "remote", "attach", "remote", "seat"}
	if _, err := b.Reconnect("owner", value.ID, ""); err == nil {
		t.Fatal("managed reconnect reused stale saved arguments")
	}
	stale := identity
	stale.Host = "different-host"
	if _, err := b.ReconnectManaged("owner", value.ID, stale, fresh); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("stale destination was accepted", err)
	}
	if _, err := b.ReconnectManaged("other", value.ID, identity, fresh); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner could reconnect the managed viewer", err)
	}
	if len(requests) != 1 {
		t.Fatal("rejected requests created another attachment")
	}
	select {
	case <-processes[0].done:
		t.Fatal("rejected request disconnected the original viewer")
	default:
	}
	result, err := b.ReconnectManaged("owner", value.ID, identity, fresh)
	if err != nil || len(requests) != 2 || !slices.Equal(requests[1].ExecArgv, fresh) || result.Rig == nil || *result.Rig != identity || result.ID != value.ID {
		t.Fatal("fresh attachment lost the original viewer identity", result, requests, err)
	}
	fresh[0] = "caller mutation"
	result, _ = b.Get("owner", value.ID)
	if result.ExecArgv[0] != "/current/agent-deck" {
		t.Fatal("saved attach arguments alias the caller")
	}
}

func TestManagedRestorePreservesHistoryWhenManagerIsUnavailable(t *testing.T) {
	b, store, launched := fixture(t)
	enableLogging(t, store)
	identity := RigIdentity{Provider: "agent-deck", Target: "agent-deck:remote:seat", Host: "remote", Workspace: "/project"}
	value, err := b.EnsureRigTerminal("owner", identity.Target, "coder", []string{"old-attach"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	p := <-launched
	if _, err := p.writer.Write([]byte("MANAGED_RETAINED_HISTORY\n")); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		v, _ := b.Recovery("owner", value.ID)
		return strings.Contains(v.History, "MANAGED_RETAINED_HISTORY")
	})
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	launches := 0
	restored, err := newBroker(store, nil, func(request terminal.LaunchRequest, _ string) (process, error) {
		launches++
		if !slices.Equal(request.ExecArgv, []string{"fresh-attach"}) {
			t.Fatal("used unvalidated saved arguments", request.ExecArgv)
		}
		return fake(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Restore(); err != nil || launches != 0 {
		t.Fatal("ordinary restore launched a managed viewer", launches, err)
	}
	if err := restored.RestoreManaged(context.Background(), func(context.Context, string, RigIdentity) ([]string, error) {
		return nil, errors.New("\x1b]52;c;hidden\aCoordinator offline " + strings.Repeat("x", 2000))
	}); err != nil {
		t.Fatal("unavailable manager prevented startup", err)
	}
	view, err := restored.Recovery("owner", value.ID)
	if err != nil || launches != 0 || view.State.Disposition != "managed-unavailable" || view.Hostname != "remote" || view.ManagerTarget != identity.Target || view.ManagerDirectory != identity.Workspace || !strings.Contains(view.History, "MANAGED_RETAINED_HISTORY") || !strings.Contains(view.Message, "Coordinator offline") || strings.Contains(view.Message, "hidden") || len(view.State.ManagerMessage) > 1024 {
		t.Fatal("lost retained evidence or exposed unsafe manager output", view, launches, err)
	}
	if err := restored.RestoreManaged(context.Background(), func(context.Context, string, RigIdentity) ([]string, error) { return []string{"fresh-attach"}, nil }); err != nil {
		t.Fatal(err)
	}
	view, _ = restored.Recovery("owner", value.ID)
	if launches != 1 || view.State.Pending || view.State.Disposition != "reattached" || view.State.ManagerMessage != "" || !strings.Contains(view.History, "MANAGED_RETAINED_HISTORY") {
		t.Fatal("successful reattachment lost history or kept a stale error", view, launches)
	}
}

func TestCanceledManagedRestoreDoesNotLaunchAfterResolution(t *testing.T) {
	b, store, _ := fixture(t)
	identity := RigIdentity{Target: "seat", Host: "localhost"}
	_, err := b.EnsureRigTerminal("owner", identity.Target, "coder", []string{"attach"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) {
		t.Fatal("canceled provider resolution launched a viewer")
		return nil, errors.New("unexpected launch")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = restored.RestoreManaged(ctx, func(context.Context, string, RigIdentity) ([]string, error) {
		cancel()
		return []string{"must-not-launch"}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was lost", err)
	}
}
