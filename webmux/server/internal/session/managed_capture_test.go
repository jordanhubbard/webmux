package session

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func testRigCapturedIdentity() RigIdentity {
	return RigIdentity{Target: "coder@project", Host: "localhost", Runtime: "claude-code", Workspace: "/project", ManagerProfile: "default", NativeSource: "rig-snapshot", NativeSessionID: "08bbfe0e-d31c-4cb6-9d18-546eea56a08c", NativeStateFile: "/native/original.jsonl", ManagerRigID: "01M71ZM6DE3RTCXHPJRXJ6XKPE", ManagerNodeID: "01M71ZM6TZ91BH689AGCFP2WCN", ManagerLogicalID: "agents.coder", ManagerSnapshotID: "01M71ZM6QSB0G9GNTTXX0242KJ", ManagerSnapshotHash: strings.Repeat("a", 64), ManagerPolicyHash: strings.Repeat("b", 64), ManagerRecordIdentity: strings.Repeat("c", 64)}
}

func TestRigCaptureRespectsOwnerLoggingAndNativeReferences(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	original := testRigCapturedIdentity()
	blank := original
	blank.NativeSessionID, blank.NativeStateFile, blank.NativeSource = "", "", ""
	clearRigRecoveryReference(&blank)
	value, err := b.EnsureRigTerminal("owner", blank.Target, "coder", []string{"attach"}, blank)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	capture := func(context.Context, RigIdentity) (RigIdentity, error) { calls++; return original, nil }
	if err := b.CaptureManagedRecovery(context.Background(), "foreign", value.ID, capture); !errors.Is(err, ErrNotFound) || calls != 0 {
		t.Fatal("foreign owner reached native metadata", err, calls)
	}
	if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, capture); err != nil {
		t.Fatal(err)
	}
	saved, err := b.Get("owner", value.ID)
	if err != nil || saved.Rig.NativeSource != "rig-snapshot" || saved.Rig.ManagerSnapshotID != original.ManagerSnapshotID || saved.Rig.NativeObservedAt == "" {
		t.Fatal("native references not captured", saved.Rig, err)
	}
	before := *saved.Rig
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, capture); err != nil || calls != 1 {
		t.Fatal("paused logging queried or mutated the manager", err, calls)
	}
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.ManagerSnapshotHash = strings.Repeat("d", 64)
	if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) { return changed, nil }); err != nil {
		t.Fatal(err)
	}
	saved, _ = b.Get("owner", value.ID)
	if saved.Rig.ManagerSnapshotHash == before.ManagerSnapshotHash {
		t.Fatal("new snapshot observation was discarded")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	retained, err := restored.Get("owner", value.ID)
	if err != nil || *retained.Rig != *saved.Rig {
		t.Fatal("native references did not survive reconstruction", retained.Rig, err)
	}
}

func TestRigCaptureRejectsChangedDestinationsAndInvalidReferences(t *testing.T) {
	b, store, _ := fixture(t)
	enableLogging(t, store)
	original := testRigCapturedIdentity()
	value, err := b.EnsureRigTerminal("owner", original.Target, "coder", []string{"attach"}, original)
	if err != nil {
		t.Fatal(err)
	}
	before := *value.Rig
	for _, mutate := range []func(*RigIdentity){
		func(v *RigIdentity) { v.Workspace = "/other" }, func(v *RigIdentity) { v.Host = "foreign" }, func(v *RigIdentity) { v.Target = "other" }, func(v *RigIdentity) { v.ManagerProfile = "other" },
		func(v *RigIdentity) { v.ManagerSnapshotID = "--force" }, func(v *RigIdentity) { v.ManagerPolicyHash = "" }, func(v *RigIdentity) { v.NativeStateFile = "relative" }, func(v *RigIdentity) { v.NativeSource = "agent-deck-inventory" },
	} {
		changed := original
		mutate(&changed)
		if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) { return changed, nil }); err == nil {
			t.Fatal("invalid native capture accepted", changed)
		}
		current, _ := b.Get("owner", value.ID)
		if *current.Rig != before {
			t.Fatal("rejected capture changed retained references")
		}
	}
	if err := b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) {
		return original, errors.New("manager unavailable")
	}); err == nil {
		t.Fatal("native failure hidden")
	}
}
