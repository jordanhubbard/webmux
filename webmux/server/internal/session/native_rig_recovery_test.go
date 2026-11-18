package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/rigobservation"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func nativeRigFixture(t *testing.T) (*Broker, *storage.Store, Session, RecoveryRequest) {
	t.Helper()
	b, store, _ := fixture(t)
	enableLogging(t, store)
	root := t.TempDir()
	ref := NativeRigRecoveryReference{Contract: "webmux-rig-native-codex-recovery/v1", RigID: "01JQPWV4E7QP26M799M3S0MR6E", NodeID: "01JQPWV4E7QP26M799M3S0MR6F", Pod: "agents", Member: "alpha", Cwd: root, SessionName: "agents-alpha@qualified", SnapshotID: "01JQPWV4E7QP26M799M3S0MR6G", HistorySessionID: "01JQPWV4E7QP26M799M3S0MR6H", NativeSessionID: "01a1eff2-f58a-7082-be6b-69f9eca4a07c", NativeStateFile: filepath.Join(root, "original.jsonl"), NativeRecordIdentity: strings.Repeat("a", 64), SnapshotSHA256: strings.Repeat("b", 64), PlanFingerprint: strings.Repeat("c", 64)}
	encoded, _ := json.Marshal(ref)
	identity := RigIdentity{Target: ref.SessionName, AttachmentTarget: ref.SessionName, SeatID: "local-rig/" + (rigobservation.Seat{RigID: ref.RigID, LogicalID: "agents.alpha"}).Key(), Host: "localhost", Runtime: "codex", Workspace: root, TmuxServerFlag: "-S", TmuxServerValue: filepath.Join(root, "tmux.sock"), NativeRigRecovery: string(encoded), NativeSessionID: ref.NativeSessionID, NativeStateFile: ref.NativeStateFile, NativeSource: "rig-native-codex"}
	blank := identity
	clearRigRecoveryReference(&blank)
	blank.NativeSessionID, blank.NativeStateFile, blank.NativeSource = "", "", ""
	value, err := b.EnsureRigTerminal("owner", identity.Target, "coder", []string{"attach"}, blank)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) { return identity, nil }); err != nil {
		t.Fatal(err)
	}
	value, err = b.Get("owner", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := NativeRigReference(*value.Rig); !ok {
		t.Fatal("capture did not retain native reference", value.Rig)
	}
	input := previewAction(t, b, value.ID, "managed")
	input.Confirm = true
	return b, store, value, input
}

func TestNativeRigRecoveryBanksBeforeDispatchAndCompletesOnlyOriginalAcrossRestart(t *testing.T) {
	b, store, value, input := nativeRigFixture(t)
	prepares, posts := 0, 0
	operation := func(_ context.Context, owner string, saved RigIdentity, request RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if owner != "owner" || saved.NativeRigRecovery != value.Rig.NativeRigRecovery || request.ID != input.ID {
			t.Fatal("wrong banked identity")
		}
		if prepare {
			prepares++
			return NativeRigOperationResult{State: "prepared"}, nil
		}
		bytes, err := os.ReadFile(filepath.Join(store.Home, "data/sessions/sessions.yaml"))
		if err != nil || !strings.Contains(string(bytes), "managed_request:") || !strings.Contains(string(bytes), "outcome: requested") {
			t.Fatal("dispatch preceded durable exact request", err)
		}
		posts++
		return NativeRigOperationResult{State: "uncertain", Code: "response_unavailable"}, nil
	}
	result, err := b.RecoverRigNative(context.Background(), "owner", value.ID, input, operation)
	if err != nil || result.Outcome != "uncertain" {
		t.Fatal(result, err)
	}
	if _, err = b.RecoverRigNative(context.Background(), "owner", value.ID, input, operation); err != nil || posts != 1 || prepares != 1 {
		t.Fatal("ordinary duplicate dispatched", err, posts, prepares)
	}
	view, err := b.Recovery("owner", value.ID)
	if err != nil || view.ManagedOriginalRequest == nil || !view.ManagedOriginalRequest.Inspect || view.ManagedOriginalRequest.ID != input.ID || view.CanResumeManaged {
		t.Fatal("original completion was not retained", view, err)
	}
	calls := 0
	if err = b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) { calls++; return RigIdentity{}, nil }); err != nil || calls != 0 {
		t.Fatal("pending conversation was recaptured", err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newBroker(store, nil, b.launch)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if result, err = restored.RecoverRigNative(context.Background(), "owner", value.ID, input, operation); err != nil || result.Outcome != "uncertain" || posts != 1 {
		t.Fatal("restart retried original", result, err)
	}
	inspection := *view.ManagedOriginalRequest
	complete := func(_ context.Context, _ string, saved RigIdentity, request RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if prepare || !request.Inspect || request.ID != input.ID || request.Revision != input.Revision || saved.NativeRigRecovery != value.Rig.NativeRigRecovery {
			t.Fatal("inspection changed banked request")
		}
		posts++
		return NativeRigOperationResult{State: "confirmed", Argv: []string{"fresh-attach"}}, nil
	}
	result, err = restored.RecoverRigNative(context.Background(), "owner", value.ID, inspection, complete)
	if err != nil || result.Outcome != "consumed" || posts != 2 {
		t.Fatal(result, err)
	}
	if _, err = restored.RecoverRigNative(context.Background(), "owner", value.ID, inspection, complete); err != nil || posts != 2 {
		t.Fatal("consumed inspection dispatched", err)
	}
}

func TestNativeRigRecoveryRefusesChangedOrUnapprovedRequestsAndNewIDsWhilePending(t *testing.T) {
	b, _, value, input := nativeRigFixture(t)
	refuse := func(context.Context, string, RigIdentity, RecoveryRequest, bool) (NativeRigOperationResult, error) {
		t.Fatal("invalid request reached provider")
		return NativeRigOperationResult{}, nil
	}
	for _, mutate := range []func(*RecoveryRequest){func(v *RecoveryRequest) { v.Confirm = false }, func(v *RecoveryRequest) { v.Context = "transcript" }, func(v *RecoveryRequest) { v.Kind = "native" }, func(v *RecoveryRequest) { v.Instance = "foreign" }, func(v *RecoveryRequest) { v.Revision++ }, func(v *RecoveryRequest) { v.HostRevision = 1 }, func(v *RecoveryRequest) { v.Inspect = true }} {
		changed := input
		mutate(&changed)
		if _, err := b.RecoverRigNative(context.Background(), "owner", value.ID, changed, refuse); err == nil {
			t.Fatal("accepted invalid action", changed)
		}
	}
	if _, err := b.RecoverRigNative(context.Background(), "foreign", value.ID, input, refuse); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	pending := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		if prepare {
			return NativeRigOperationResult{State: "prepared"}, nil
		}
		return NativeRigOperationResult{State: "uncertain"}, nil
	}
	if _, err := b.RecoverRigNative(context.Background(), "owner", value.ID, input, pending); err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.ID = "9aa1753a-1d61-4420-af42-e6b6fa591c47"
	if _, err := b.RecoverRigNative(context.Background(), "owner", value.ID, changed, refuse); err == nil {
		t.Fatal("new ID bypassed pending original")
	}
	changed = input
	changed.Inspect = true
	changed.Revision++
	if _, err := b.RecoverRigNative(context.Background(), "owner", value.ID, changed, refuse); err == nil {
		t.Fatal("changed original revision accepted")
	}
}

func TestNativeRigRecoveryPersistsNothingAndDoesNotDispatchWhenPreparationOrSaveFails(t *testing.T) {
	for _, mode := range []string{"prepare", "save"} {
		t.Run(mode, func(t *testing.T) {
			b, store, value, input := nativeRigFixture(t)
			path := filepath.Join(store.Home, "data/sessions/sessions.yaml")
			operation := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
				if !prepare {
					t.Fatal("unbanked request dispatched")
				}
				if mode == "prepare" {
					return NativeRigOperationResult{}, errors.New("native review unavailable")
				}
				if err := os.Rename(path, path+".retained"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				return NativeRigOperationResult{State: "prepared"}, nil
			}
			if _, err := b.RecoverRigNative(context.Background(), "owner", value.ID, input, operation); err == nil {
				t.Fatal("preparation/save failure ignored")
			}
			if mode == "save" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".retained", path); err != nil {
					t.Fatal(err)
				}
			}
			view, err := b.Recovery("owner", value.ID)
			if err != nil || view.State.Intent != nil || view.State.Revision != input.Revision {
				t.Fatal("failed request changed public state", view, err)
			}
		})
	}
}

func TestNativeRigReferenceRejectsForeignMirrorsAndUnknownSchemaFields(t *testing.T) {
	_, _, value, _ := nativeRigFixture(t)
	valid := *value.Rig
	for _, mutate := range []func(*RigIdentity){func(v *RigIdentity) { v.Target = "foreign" }, func(v *RigIdentity) { v.AttachmentTarget = "foreign" }, func(v *RigIdentity) { v.Host = "remote" }, func(v *RigIdentity) { v.Runtime = "claude-code" }, func(v *RigIdentity) { v.SeatID = "local-rig/" + strings.Repeat("f", 64) }, func(v *RigIdentity) { v.NativeSessionID = "foreign" }, func(v *RigIdentity) { v.NativeStateFile = "relative" }, func(v *RigIdentity) { v.NativeSource = "rig-snapshot" }, func(v *RigIdentity) { v.TmuxServerValue = "relative" }, func(v *RigIdentity) {
		v.NativeRigRecovery = strings.TrimSuffix(v.NativeRigRecovery, "}") + `,"unexpected":true}`
	}} {
		changed := valid
		mutate(&changed)
		if _, ok := NativeRigReference(changed); ok {
			t.Fatal("foreign reference accepted", changed)
		}
	}
}

func TestNativeRigRecoveryReattachesSurvivingOriginalWithoutNativeDispatch(t *testing.T) {
	b, _, value, input := nativeRigFixture(t)
	calls := 0
	operation := func(_ context.Context, _ string, _ RigIdentity, _ RecoveryRequest, prepare bool) (NativeRigOperationResult, error) {
		calls++
		if !prepare {
			t.Fatal("survivor caused Resume dispatch")
		}
		return NativeRigOperationResult{State: "surviving", Argv: []string{"existing-original-attach"}}, nil
	}
	for range 2 {
		result, err := b.RecoverRigNative(context.Background(), "owner", value.ID, input, operation)
		if err != nil || result.Outcome != "consumed" {
			t.Fatal(result, err)
		}
	}
	if calls != 1 {
		t.Fatal("survivor duplicate reached provider", calls)
	}
	current, err := b.Get("owner", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := *current.Rig
	ref, _ := NativeRigReference(changed)
	ref.SnapshotID = "01JQPWV4E7QP26M799M3S0MR6J"
	encoded, _ := json.Marshal(ref)
	changed.NativeRigRecovery = string(encoded)
	if err = b.CaptureManagedRecovery(context.Background(), "owner", value.ID, func(context.Context, RigIdentity) (RigIdentity, error) { return changed, nil }); err != nil {
		t.Fatal(err)
	}
	if result, err := b.RecoverRigNative(context.Background(), "owner", value.ID, input, operation); err != nil || result.Outcome != "consumed" || calls != 1 {
		t.Fatal("historical receipt became new launch after recapture", result, err)
	}
}
