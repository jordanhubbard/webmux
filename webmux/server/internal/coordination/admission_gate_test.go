package coordination

import (
	"errors"
	"testing"
)

func externalGate(_ string, _ *Binding, action string) error {
	if action == "release-claim" || action == "release-reservation" || action == "delivered" || action == "acknowledge" || action == "complete" {
		return nil
	}
	return ErrExternalAuthority
}

func TestAdmissionGateRefusesNewAndCachedLocalGrantsButAllowsDrain(t *testing.T) {
	h := fixture(t)
	request := Request{ID: "gate-original-reserve", Action: "reserve", Resource: "node:" + testNode + ":heavy", LeaseSeconds: 120}
	original := h.execute(t, 0, request)
	claim := h.execute(t, 0, Request{ID: "gate-original-claim", Action: "claim", Kind: "task", Resource: "one", LeaseSeconds: 120})
	h.s.AdmissionGate = externalGate
	for _, operation := range []Request{request, {ID: "gate-new-reserve", Action: "reserve", Resource: request.Resource, LeaseSeconds: 120}, {ID: "gate-new-claim", Action: "claim", Kind: "task", Resource: "two", LeaseSeconds: 120}, {ID: "gate-renew-existing", Action: "renew-reservation", Resource: request.Resource, Record: original.Record, Fence: original.Fence, LeaseSeconds: 120}} {
		got, err := h.s.Execute(testNode, h.tokens[0], h.inputs, operation)
		if !errors.Is(err, ErrExternalAuthority) || got.Status != "" || got.Lease != nil {
			t.Fatal("standalone authority survived its admission gate", got, err)
		}
	}
	if got := h.execute(t, 0, Request{ID: "gate-drain-resource", Action: "release-reservation", Resource: request.Resource, Record: original.Record, Fence: original.Fence}); got.Status != "released" {
		t.Fatal(got)
	}
	h.execute(t, 0, Request{ID: "gate-drain-claim", Action: "release-claim", Record: claim.Record})
}

func TestAdmissionGateRefusesPeerReservationAdmitAndCurrentStatus(t *testing.T) {
	r := sharedFixture(t)
	request := Request{ID: "gate-peer-original", Action: "reserve", Resource: "node:" + r.node + ":heavy", LeaseSeconds: 120}
	intent, _ := submitShared(t, r, request)
	original, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, "admit", intent.Envelope)
	if err != nil || original.Result == nil || original.Result.Status != "reserved" {
		t.Fatal(original, err)
	}
	r.receiver.AdmissionGate = externalGate
	for _, operation := range []string{"status", "admit"} {
		got, err := r.receiver.ExchangeReservation(r.node, r.scope, r.inputs, operation, intent.Envelope)
		if !errors.Is(err, ErrExternalAuthority) || got.Current || got.Result != nil {
			t.Fatal("peer replay returned a current competing grant", got, err)
		}
	}
}

func TestAdmissionGateRefusesQueuedSharedAttemptAndCachedSourceLease(t *testing.T) {
	r := sharedFixture(t)
	request := Request{ID: "gate-queued-shared", Action: "reserve", Resource: "node:" + r.node + ":heavy", LeaseSeconds: 120}
	intent, _ := submitShared(t, r, request)
	applySharedFixture(t, r, intent, "admit")
	r.source.s.AdmissionGate = externalGate
	if allowed, err := r.source.s.BeginSharedAttempt(testNode, r.source.tokens[0], r.source.inputs, request); allowed || !errors.Is(err, ErrExternalAuthority) {
		t.Fatal("queued reservation crossed authority boundary", allowed, err)
	}
	if _, result, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, request); !errors.Is(err, ErrExternalAuthority) || result.Lease != nil {
		t.Fatal("cached source-side lease survived authority switch", result, err)
	}
}

func TestAdmissionGateRefusesQueuedAndInboundPeerMessages(t *testing.T) {
	r := remoteFixture(t)
	_, _, message := r.queue(t, "gate-queued-message")
	r.source.s.AdmissionGate = externalGate
	if allowed, err := r.source.s.BeginRemoteAttempt(testNode, message.ID, r.source.inputs); allowed || !errors.Is(err, ErrExternalAuthority) {
		t.Fatal("queued message crossed authority boundary", allowed, err)
	}
	r.receiver.AdmissionGate = externalGate
	if result, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", message); !errors.Is(err, ErrExternalAuthority) || result.State != "" && result.State != "absent" {
		t.Fatal("new inbound message crossed authority boundary", result, err)
	}
}
