package coordination

import (
	"errors"
	bolt "go.etcd.io/bbolt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func sharedFixture(t *testing.T) *remoteHarness {
	t.Helper()
	r := prepareClaims(t)
	return r
}
func submitShared(t *testing.T, r *remoteHarness, request Request) (ReservationIntent, Result) {
	t.Helper()
	result, err := r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, &r.route)
	if err != nil || result.Shared == nil {
		t.Fatal(result, err)
	}
	intent, _, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil {
		t.Fatal(err)
	}
	return intent, result
}
func applySharedFixture(t *testing.T, r *remoteHarness, intent ReservationIntent, operation string) ReservationReply {
	t.Helper()
	reply, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, operation, intent.Envelope)
	if err != nil || !reply.Valid(intent.Envelope) {
		t.Fatal(reply, err)
	}
	if err = r.source.s.ApplySharedReply(testNode, intent.Envelope, reply, r.source.now); err != nil {
		t.Fatal(err)
	}
	return reply
}
func TestSharedReservationLostReplyAndOriginalFenceAfterReopen(t *testing.T) {
	r := sharedFixture(t)
	resource := "node:" + r.node + ":heavy"
	request := Request{ID: "lost-shared-reserve", Action: "reserve", Resource: resource, LeaseSeconds: 120}
	intent, pending := submitShared(t, r, request)
	if pending.Status != "reservation-pending" || pending.Lease != nil {
		t.Fatal(pending)
	}
	allowed, err := r.source.s.BeginSharedAttempt(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	// The receiver commits, then both authorities lose their process before the
	// source records the reply. The original ID must recover exactly one fence.
	committed, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, "admit", intent.Envelope)
	if err != nil || committed.Result.Status != "reserved" || committed.Result.Fence != 1 {
		t.Fatal(committed, err)
	}
	receiverHome := r.receiver.home
	if err = r.receiver.Close(); err != nil {
		t.Fatal(err)
	}
	r.receiver = New(receiverHome)
	r.receiver.Now = func() time.Time { return r.source.now }
	if err = r.source.s.Close(); err != nil {
		t.Fatal(err)
	}
	r.source.s = New(r.source.home)
	r.source.s.Now = func() time.Time { return r.source.now }
	recovered, _, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || !recovered.Attempted || recovered.Resolved {
		t.Fatal(recovered, err)
	}
	reply := applySharedFixture(t, r, recovered, "status")
	if reply.Result.Fence != 1 || !reply.Current {
		t.Fatal(reply)
	}
	_, granted, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || granted.Lease == nil || !granted.Lease.Current || granted.Status != "reserved" || granted.Fence != 1 {
		t.Fatal(granted, err)
	}
	duplicate, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, "admit", intent.Envelope)
	if err != nil || !reflect.DeepEqual(duplicate.Result, committed.Result) {
		t.Fatal(duplicate, err)
	}
	// Renew with a new ID, same original record/fence. An old reserve receipt may
	// never inherit that extension when queried after its original deadline.
	r.source.now = r.source.now.Add(90 * time.Second)
	renewal := Request{ID: "renew-shared-original", Action: "renew-reservation", Resource: resource, Record: granted.Record, Fence: granted.Fence, LeaseSeconds: 120}
	renewalIntent, _ := submitShared(t, r, renewal)
	applySharedFixture(t, r, renewalIntent, "admit")
	r.source.now = r.source.now.Add(31 * time.Second)
	old := applySharedFixture(t, r, intent, "status")
	if old.Current {
		t.Fatal("old reserve inherited renewed expiry")
	}
	_, oldResult, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || oldResult.Lease.Current || oldResult.Expires.After(r.source.now) {
		t.Fatal(oldResult, err)
	}
	renewed := applySharedFixture(t, r, renewalIntent, "status")
	if !renewed.Current {
		t.Fatal("renewed original fence lost")
	}
	// Releasing after changing project remains allowed, and cannot permit a late
	// old success to revive the source-side lease.
	release := Request{ID: "release-shared-original", Action: "release-reservation", Resource: resource, Record: granted.Record, Fence: granted.Fence}
	r.source.inputs[0].Project = nil
	releaseIntent, _ := submitShared(t, r, release)
	applySharedFixture(t, r, releaseIntent, "admit")
	r.source.inputs[0].Project = membersFixture()[0].Project
	if err = r.source.s.ApplySharedReply(testNode, renewalIntent.Envelope, renewed, r.source.now); err != nil {
		t.Fatal(err)
	}
	_, afterRelease, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, renewal)
	if err != nil || afterRelease.Lease.Current {
		t.Fatal("late reply revived released lease", afterRelease, err)
	}
	next, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "local-after-shared", Action: "reserve", Resource: resource})
	if err != nil || next.Status != "reserved" || next.Fence != 2 {
		t.Fatal(next, err)
	}
	stale := Request{ID: "stale-shared-renew", Action: "renew-reservation", Resource: resource, Record: granted.Record, Fence: 1}
	r.source.inputs[0].Project = membersFixture()[0].Project
	staleIntent, _ := submitShared(t, r, stale)
	lost := applySharedFixture(t, r, staleIntent, "admit")
	if lost.Result.Status != "lease-lost" {
		t.Fatal(lost)
	}
}
func TestSharedAndLocalReservationsContendInOneAuthority(t *testing.T) {
	r := sharedFixture(t)
	resource := "node:" + r.node + ":heavy"
	intent, _ := submitShared(t, r, Request{ID: "contended-shared-request", Action: "reserve", Resource: resource})
	type outcome struct {
		result Result
		err    error
	}
	out := make(chan outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		reply, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, "admit", intent.Envelope)
		if reply.Result == nil {
			out <- outcome{err: err}
			return
		}
		out <- outcome{*reply.Result, err}
	}()
	go func() {
		defer wg.Done()
		result, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "contended-local-request", Action: "reserve", Resource: resource})
		out <- outcome{result, err}
	}()
	wg.Wait()
	close(out)
	wins, busy := 0, 0
	for o := range out {
		if o.err != nil {
			t.Fatal(o.err)
		}
		switch o.result.Status {
		case "reserved":
			wins++
			if o.result.Fence != 1 {
				t.Fatal(o.result)
			}
		case "busy":
			busy++
		default:
			t.Fatal(o.result)
		}
	}
	if wins != 1 || busy != 1 {
		t.Fatal(wins, busy)
	}
}
func TestSharedReservationClockOffsetsRealmIsolationAndHistory(t *testing.T) {
	r := sharedFixture(t)
	r.receiver.Now = func() time.Time { return r.source.now.Add(24 * time.Hour) }
	request := Request{ID: "skewed-shared-reserve", Action: "reserve", Resource: "node:" + r.node + ":heavy", LeaseSeconds: 120}
	intent, _ := submitShared(t, r, request)
	reply := applySharedFixture(t, r, intent, "admit")
	_, result, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || !result.Lease.Current || !result.Expires.Equal(r.source.now.Add(120*time.Second)) {
		t.Fatal(result, err)
	}
	if reply.Result.Expires.Equal(*result.Expires) {
		t.Fatal("remote clock used as local deadline")
	}
	// The receiving account owns visibility, even when the remote source account
	// happens to share a name with a different local account.
	foreign := r.inputs[0]
	foreign.Ref.Owner = "owner"
	foreign.Ref.Session = "foreign-local"
	inputs := append(append([]MemberInput{}, r.inputs...), foreign)
	view, err := r.receiver.OwnerView(r.node, "owner", inputs)
	if err != nil || !view.Resources[0].Occupied || view.Resources[0].Holder != nil {
		t.Fatal(view.Resources, err)
	}
	ownerView, err := r.receiver.OwnerView(r.node, "receiver", inputs)
	if err != nil || ownerView.Resources[0].Holder == nil || *ownerView.Resources[0].Holder != intent.Envelope.Actor {
		t.Fatal(ownerView.Resources, err)
	}
	wrong := r.scope
	wrong.ReceiverOwner = "owner"
	e := intent.Envelope
	e.Owner = "owner"
	isolated, err := r.receiver.ExchangeReservation(r.node, wrong, inputs, "status", e)
	if err != nil || isolated.State != "absent" || isolated.Result != nil {
		t.Fatal("another receiving account read the original outcome", isolated, err)
	}
	e.Request = Request{ID: "wrong-realm-release", Action: "release-reservation", Resource: request.Resource, Record: result.Record, Fence: result.Fence}
	e.ID = requestKey(e.Actor, e.Request.ID)
	isolated, err = r.receiver.ExchangeReservation(r.node, wrong, inputs, "admit", e)
	if err != nil || isolated.Result == nil || isolated.Result.Status != "lease-lost" {
		t.Fatal("another receiving account released the lease", isolated, err)
	}
	// A fresh authority history cannot replay an uncertain old reserve.
	fresh := New(t.TempDir())
	defer fresh.Close()
	fresh.Now = r.receiver.Now
	if err = fresh.Sync(r.node, r.inputs); err != nil {
		t.Fatal(err)
	}
	changed, err := fresh.ExchangeReservation(r.node, r.scope, r.inputs, "admit", intent.Envelope)
	if err != nil || changed.State != "history-changed" {
		t.Fatal(changed, err)
	}
	if err = r.source.s.ApplySharedReply(testNode, intent.Envelope, changed, r.source.now); err != nil {
		t.Fatal(err)
	}
	_, result, err = r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || result.Lease.Current || result.Shared.Transport != "history-changed" {
		t.Fatal(result, err)
	}
}
func TestSharedReservationSourceScopeAndFrozenDenials(t *testing.T) {
	r := sharedFixture(t)
	request := Request{ID: "unavailable-shared-route", Action: "reserve", Resource: "node:" + r.node + ":heavy"}
	denied, err := r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, nil)
	if err != nil || denied.Status != "peer-unavailable" {
		t.Fatal(denied, err)
	}
	repeated, err := r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, &r.route)
	if err != nil || !reflect.DeepEqual(repeated, denied) {
		t.Fatal(repeated, err)
	}
	request.ID = "inactive-shared-source"
	intent, _ := submitShared(t, r, request)
	r.source.inputs[0].Active = false
	if allowed, err := r.source.s.BeginSharedAttempt(testNode, r.source.tokens[0], r.source.inputs, request); allowed || !errors.Is(err, ErrUnauthorized) {
		t.Fatal(allowed, err)
	}
	r.source.inputs[0].Active = true
	r.source.inputs[0].Project = nil
	if allowed, err := r.source.s.BeginSharedAttempt(testNode, r.source.tokens[0], r.source.inputs, request); allowed || err != nil {
		t.Fatal(allowed, err)
	}
	_, result, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request)
	if err != nil || result.Status != "project-changed" {
		t.Fatal(result, err)
	}
	wrong := r.scope
	wrong.Node = strings.Repeat("c", 32)
	if _, err = r.receiver.ExchangeReservation(r.node, wrong, r.inputs, "admit", intent.Envelope); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}
func TestLocalReservationReceiptDoesNotReviveReleasedLease(t *testing.T) {
	h := fixture(t)
	request := Request{ID: "local-original-lease", Action: "reserve", Resource: "node:" + testNode + ":heavy"}
	grant := h.execute(t, 0, request)
	if grant.Lease == nil || !grant.Lease.Current {
		t.Fatal(grant)
	}
	h.execute(t, 0, Request{ID: "local-original-release", Action: "release-reservation", Resource: request.Resource, Record: grant.Record, Fence: grant.Fence})
	retry := h.execute(t, 0, request)
	if retry.Status != "reserved" || retry.Lease.Current || retry.Record != grant.Record || !retry.Expires.Equal(*grant.Expires) {
		t.Fatal(retry)
	}
}

func TestReservationSchemaUpgradePreservesHistoryAndFence(t *testing.T) {
	h := fixture(t)
	request := Request{ID: "legacy-local-reserve", Action: "reserve", Resource: "node:" + testNode + ":heavy"}
	grant := h.execute(t, 0, request)
	epoch := h.view(t, 0).HistoryEpoch
	db, _ := h.s.database()
	if err := db.Update(func(tx *bolt.Tx) error {
		held, _, err := get[Reservation](bucket(tx, "reservations"), request.Resource)
		if err != nil {
			return err
		}
		held.Custodian = ""
		if err = put(bucket(tx, "reservations"), request.Resource, held); err != nil {
			return err
		}
		return bucket(tx, "meta").Put([]byte("version"), []byte("2"))
	}); err != nil {
		t.Fatal(err)
	}
	repeated := h.execute(t, 0, request)
	if repeated.Record != grant.Record || repeated.Fence != grant.Fence || repeated.Lease == nil || !repeated.Lease.Current || h.view(t, 0).HistoryEpoch != epoch {
		t.Fatal(repeated)
	}
	if err := db.View(func(tx *bolt.Tx) error {
		if string(bucket(tx, "meta").Get([]byte("version"))) != "3" {
			t.Fatal("legacy ledger not atomically upgraded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
