package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func rigStopLedgerFixture(t *testing.T) (string, rigStopPreview) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native Rig Stop requires Unix directory sync; Windows routes refuse it")
	}
	home := t.TempDir()
	review := rigStopReview{Contract: "webmux-rig-stop/v1", RigID: "01M71ZM6DE3RTCXHPJRXJ6XKPE", LogicalID: "agents.worker", SessionName: "worker@project", TmuxServerFlag: "-S", TmuxServerValue: "/tmp/private.sock", Port: 43210, Runtime: "terminal", ManagerPID: 123, ManagerIdentity: strings.Repeat("a", 64), Expected: rigStopExpected{NodeID: "01M71ZM6TZ91BH689AGCFP2WCN", SessionID: "01M71ZM6QSB0G9GNTTXX0242KJ"}}
	preview, err := newRigStopPreview(home, "owner", localRigSeatID(review.RigID, review.LogicalID), "Owner reviewed local seat effect", review)
	if err != nil {
		t.Fatal(err)
	}
	return home, preview
}

func rigStopTestResult(state, code, phase string) rigStopReceipt {
	posted := true
	return rigStopReceipt{State: state, Code: code, StopPhase: phase, Posted: &posted}
}

func TestRigStopReviewEligibilityRequiresNativeOwnershipAndQualifiedAttachment(t *testing.T) {
	base := rigCanonicalSeat{RigID: "rig", LogicalID: "agents.worker", CanonicalSessionName: "seat", TmuxTarget: "seat", TmuxServerFlag: "-S", TmuxServerValue: "/tmp/socket", Openable: true, CheckedStop: true, SessionStatus: "running", rigEnrollment: rigEnrollment{EnrollmentOrigin: "launched", EnrollmentSource: "rig-session-registry", EnrollmentID: "01M71ZM6QSB0G9GNTTXX0242KJ", EnrollmentObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	for _, row := range []struct {
		name     string
		change   func(*rigCanonicalSeat)
		eligible bool
	}{
		{"native launched", func(*rigCanonicalSeat) {}, true},
		{"private launch record is not ownership", func(s *rigCanonicalSeat) { s.Managed = true; s.EnrollmentOrigin = "claimed" }, false},
		{"unverified lineage", func(s *rigCanonicalSeat) { s.EnrollmentID = "invalid" }, false},
		{"legacy version", func(s *rigCanonicalSeat) { s.CheckedStop = false }, false},
		{"missing socket", func(s *rigCanonicalSeat) { s.TmuxServerFlag, s.TmuxServerValue = "", "" }, false},
		{"stopped", func(s *rigCanonicalSeat) { s.SessionStatus = "exited" }, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			seat := base
			row.change(&seat)
			status := rigCoordinationStatus(rigStatusEnvelope{Seats: []rigCanonicalSeat{seat}})
			if len(status.Seats) != 1 || status.Seats[0].CanReviewStop != row.eligible {
				t.Fatal("Stop eligibility conflated launch records, lineage or attachment", status)
			}
		})
	}
}

func TestRigStopDurableClaimConsumesConcurrentAndReconstructedRequests(t *testing.T) {
	home, preview := rigStopLedgerFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	dispatch := func(_ context.Context, value rigStopPreview) (rigStopReceipt, error) {
		if value != preview {
			t.Error("dispatch changed the retained review")
		}
		calls.Add(1)
		close(started)
		<-release
		return rigStopTestResult("stopped", "stopped", "kill_attempted"), nil
	}
	var result rigStopReceipt
	var dispatchErr error
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		result, dispatchErr = consumeRigStop(context.Background(), home, "owner", preview.ID, dispatch)
	}()
	<-started
	for range 8 {
		duplicate, err := consumeRigStop(context.Background(), home, "owner", preview.ID, dispatch)
		if err != nil || duplicate.State != "uncertain" || duplicate.StopPhase != "unknown" {
			t.Fatal("in-flight request was retryable", duplicate, err)
		}
	}
	close(release)
	worker.Wait()
	if dispatchErr != nil || result.State != "stopped" || calls.Load() != 1 {
		t.Fatal(result, dispatchErr, calls.Load())
	}
	// No in-memory operation registry is supplied to this reconstructed call.
	consumed, err := consumeRigStop(context.Background(), home, "owner", preview.ID, dispatch)
	if err != nil || consumed.State != "stopped" || calls.Load() != 1 {
		t.Fatal("reconstruction redispatched", consumed, err, calls.Load())
	}
	_, directory := rigStopRoot(home, "owner")
	info, err := os.Stat(filepath.Join(directory, preview.ID+".result.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("receipt permissions", info, err)
	}
}

func TestRigStopLostResponseBlocksNewReviewsAcrossOwnersAndPortChanges(t *testing.T) {
	home, preview := rigStopLedgerFixture(t)
	calls := 0
	dispatch := func(context.Context, rigStopPreview) (rigStopReceipt, error) {
		calls++
		return rigStopReceipt{}, errors.New("response lost after native effect")
	}
	result, err := consumeRigStop(context.Background(), home, "owner", preview.ID, dispatch)
	if err != nil || result.State != "uncertain" || result.Posted != nil || calls != 1 {
		t.Fatal(result, err, calls)
	}
	for _, owner := range []string{"owner", "other-admin"} {
		review := preview.Review
		review.Port++
		fresh, err := newRigStopPreview(home, owner, preview.Target, "Another explicit review", review)
		if err != nil {
			t.Fatal(err)
		}
		result, err := consumeRigStop(context.Background(), home, owner, fresh.ID, dispatch)
		if err != nil || result.State != "uncertain" || calls != 1 {
			t.Fatal("unknown outcome bypassed by owner or endpoint", result, err, calls)
		}
	}
}

func TestRigStopInterruptedDispatchRemainsUncertainAfterPreviewExpires(t *testing.T) {
	home, preview := rigStopLedgerFixture(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = consumeRigStop(context.Background(), home, "owner", preview.ID, func(context.Context, rigStopPreview) (rigStopReceipt, error) {
			// Exit after the durable claim, before any completion can be saved.
			runtime.Goexit()
			return rigStopReceipt{}, nil
		})
	}()
	<-done
	_, directory := rigStopRoot(home, "owner")
	preview.ExpiresAt = time.Now().Add(-time.Hour)
	_ = os.Remove(filepath.Join(directory, preview.ID+".review.json"))
	if err := writeRigStopRecord(home, filepath.Join(directory, preview.ID+".review.json"), preview); err != nil {
		t.Fatal(err)
	}
	result, err := consumeRigStop(context.Background(), home, "owner", preview.ID, func(context.Context, rigStopPreview) (rigStopReceipt, error) {
		t.Fatal("expired review bypassed an interrupted dispatch")
		return rigStopReceipt{}, nil
	})
	if err != nil || result.State != "uncertain" {
		t.Fatal("interruption was mistaken for an unused expired preview", result, err)
	}
}

func TestRigStopConfirmedNoEffectAllowsOnlyANewExplicitReview(t *testing.T) {
	home, preview := rigStopLedgerFixture(t)
	calls := 0
	dispatch := func(context.Context, rigStopPreview) (rigStopReceipt, error) {
		calls++
		return rigStopTestResult("refused", "typing_guard_enabled", "none"), nil
	}
	for range 2 {
		result, err := consumeRigStop(context.Background(), home, "owner", preview.ID, dispatch)
		if err != nil || result.State != "refused" || calls != 1 {
			t.Fatal("same request was reused", result, err, calls)
		}
	}
	fresh, err := newRigStopPreview(home, "owner", preview.Target, "Fresh owner confirmation", preview.Review)
	if err != nil {
		t.Fatal(err)
	}
	result, err := consumeRigStop(context.Background(), home, "owner", fresh.ID, dispatch)
	if err != nil || result.State != "refused" || calls != 2 {
		t.Fatal("confirmed no-effect refusal prevented a deliberate new review", result, err, calls)
	}
}

func TestRigStopOwnerExpiryStorageAndPartialReceiptRefusals(t *testing.T) {
	for _, variant := range []string{"owner", "expired", "storage", "partial-result"} {
		t.Run(variant, func(t *testing.T) {
			home, preview := rigStopLedgerFixture(t)
			root, directory := rigStopRoot(home, "owner")
			owner := "owner"
			calls := 0
			if variant == "owner" {
				owner = "foreign"
			} else if variant == "expired" {
				preview.ExpiresAt = time.Now().Add(-time.Minute)
				_ = os.Remove(filepath.Join(directory, preview.ID+".review.json"))
				if err := writeRigStopRecord(home, filepath.Join(directory, preview.ID+".review.json"), preview); err != nil {
					t.Fatal(err)
				}
			} else if variant == "storage" {
				if err := os.WriteFile(filepath.Join(root, "claims"), []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			dispatch := func(context.Context, rigStopPreview) (rigStopReceipt, error) {
				calls++
				if variant == "partial-result" {
					if err := os.WriteFile(filepath.Join(directory, preview.ID+".result.json"), []byte(`{"state":`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return rigStopTestResult("stopped", "stopped", "kill_attempted"), nil
			}
			result, err := consumeRigStop(context.Background(), home, owner, preview.ID, dispatch)
			if variant != "partial-result" {
				if err == nil || calls != 0 {
					t.Fatal("refusal reached dispatch", result, err, calls)
				}
			} else {
				if err != nil || result.State != "uncertain" || calls != 1 {
					t.Fatal(result, err, calls)
				}
				result, err = consumeRigStop(context.Background(), home, owner, preview.ID, dispatch)
				if err != nil || result.State != "uncertain" || calls != 1 {
					t.Fatal("partial receipt allowed retry", result, err, calls)
				}
			}
		})
	}
}
