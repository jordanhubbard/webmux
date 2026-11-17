package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestRigLogicalViewerRebindPreservesLayoutAndOwner(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	var requests []terminal.LaunchRequest
	var processes []*fakeProcess
	b.launch = func(request terminal.LaunchRequest, _ string) (process, error) {
		requests = append(requests, request)
		p := fake()
		processes = append(processes, p)
		return p, nil
	}
	identity := RigIdentity{SeatID: "local-rig/" + strings.Repeat("a", 64), Target: "first", Project: "project", Name: "worker", Runtime: "codex"}
	row, col := 2, 1
	first, err := b.EnsureRigTerminalAt("owner", identity.Target, "worker", []string{"attach", "first"}, &row, &col, identity)
	if err != nil {
		t.Fatal(err)
	}
	processes[0].output(t, "RETAINED_LOGICAL_HISTORY\n")
	eventually(t, func() bool {
		view, _ := b.Recovery("owner", first.ID)
		return strings.Contains(view.History, "RETAINED_LOGICAL_HISTORY")
	})
	// A rebind may never carry a coding-conversation or process reference to
	// the new attachment merely because the logical role is unchanged.
	b.mu.Lock()
	b.entries[first.ID].value.Rig.NativeSessionID = "old-conversation"
	b.entries[first.ID].value.Rig.ManagerProcess = "old-process-reference"
	b.mu.Unlock()
	identity.Target = "second"
	second, err := b.EnsureRigTerminal("owner", identity.Target, "worker", []string{"attach", "second"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Row != row || second.Col != col || second.Rig.Target != "second" || len(requests) != 2 || !slices.Equal(requests[1].ExecArgv, []string{"attach", "second"}) {
		t.Fatal("rebind changed logical viewer or attachment", first, second, requests)
	}
	if second.Rig.NativeSessionID != "" || second.Rig.ManagerProcess != "" {
		t.Fatal("stale native evidence survived rebind", second.Rig)
	}
	view, err := b.Recovery("owner", second.ID)
	if err != nil || !strings.Contains(view.History, "RETAINED_LOGICAL_HISTORY") {
		t.Fatal("rebind discarded retained history", view, err)
	}
	select {
	case <-processes[0].done:
	default:
		t.Fatal("old attachment client remained live")
	}
	other, err := b.EnsureRigTerminal("other", identity.Target, "worker", []string{"attach", "second"}, identity)
	if err != nil || other.ID == second.ID {
		t.Fatal("logical key crossed owner boundary", other, err)
	}
	if _, err := b.ReconnectManaged("owner", second.ID, *first.Rig, []string{"stale"}); err == nil {
		t.Fatal("old attachment reference authorized reconnect")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, func(terminal.LaunchRequest, string) (process, error) { return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	value, err := restored.Get("owner", first.ID)
	if err != nil || value.Rig.SeatID != identity.SeatID || value.Rig.Target != "second" || value.Row != row || value.Col != col {
		t.Fatal("persisted logical identity changed", value, err)
	}
}

func TestRigLogicalRebindRejectsOccupiedPlacementBeforeDetaching(t *testing.T) {
	b, _, launched := fixture(t)
	identity := RigIdentity{SeatID: "local-rig/" + strings.Repeat("b", 64), Target: "first"}
	first, err := b.EnsureRigTerminal("owner", "first", "worker", []string{"attach", "first"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	client := <-launched
	occupied, err := b.EnsureRigTerminal("owner", "occupant", "other", []string{"attach", "other"})
	if err != nil {
		t.Fatal(err)
	}
	identity.Target = "second"
	if _, err := b.EnsureRigTerminalAt("owner", "second", "worker", []string{"attach", "second"}, &occupied.Row, &occupied.Col, identity); err == nil {
		t.Fatal("occupied cell accepted")
	}
	select {
	case <-client.done:
		t.Fatal("failed placement detached the original viewer")
	default:
	}
	value, err := b.Get("owner", first.ID)
	if err != nil || value.Rig.Target != "first" {
		t.Fatal("failed placement changed identity", value, err)
	}
}

func TestRigLogicalRebindChecksNativeAddressWhenManagerAliasIsUnchanged(t *testing.T) {
	b, _, launched := fixture(t)
	identity := RigIdentity{SeatID: "local-rig/" + strings.Repeat("c", 64), Target: "manager-alias", AttachmentTarget: "first"}
	first, err := b.EnsureRigTerminal("owner", identity.Target, "worker", []string{"attach", "first"}, identity)
	if err != nil {
		t.Fatal(err)
	}
	old := <-launched
	identity.AttachmentTarget = "second"
	second, err := b.EnsureRigTerminal("owner", identity.Target, "worker", []string{"attach", "second"}, identity)
	if err != nil || second.ID != first.ID || second.Rig.AttachmentTarget != "second" || !slices.Equal(second.ExecArgv, []string{"attach", "second"}) {
		t.Fatal("unchanged manager alias concealed a native address change", second, err)
	}
	select {
	case <-old.done:
	default:
		t.Fatal("old native attachment survived explicit Open")
	}
}

func TestRigLogicalIdentityUpgradesExactAddressViewer(t *testing.T) {
	b, _, launched := fixture(t)
	old := RigIdentity{Provider: "rig", Target: "current-address", Host: "localhost"}
	first, err := b.EnsureRigTerminal("owner", old.Target, "worker", []string{"attach", old.Target}, old)
	if err != nil {
		t.Fatal(err)
	}
	identity := old
	identity.Provider = ""
	identity.SeatID = "local-rig/" + strings.Repeat("d", 64)
	identity.AttachmentTarget = old.Target
	second, err := b.EnsureRigTerminal("owner", old.Target, "worker", []string{"attach", old.Target}, identity)
	if err != nil || first.ID != second.ID || second.OpenRigView != "managed:"+identity.SeatID || len(launched) != 1 {
		t.Fatal("exact-address compatibility duplicated an existing viewer", second, err)
	}
}
