package coordination

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestWorkerQueueConcurrentFIFOAndNoLegacyBarging(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	holder := h.execute(t, 3, Request{ID: "initial-live-holder", Action: "reserve", Resource: resource})
	type queued struct {
		who    int
		result Result
		err    error
	}
	results := make(chan queued, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(who int) {
			defer wg.Done()
			r, err := h.s.Execute(testNode, h.tokens[who], h.inputs, Request{ID: fmt.Sprintf("queue-client-%d", who), Action: "enqueue-reservation", Resource: resource})
			results <- queued{who, r, err}
		}(i)
	}
	wg.Wait()
	close(results)
	entries := []queued{}
	for r := range results {
		if r.err != nil || r.result.Status != "queued" || r.result.Lease != nil {
			t.Fatal(r)
		}
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].result.Queue.Sequence < entries[j].result.Queue.Sequence })
	for _, e := range entries {
		repeat := h.execute(t, e.who, Request{ID: fmt.Sprintf("queue-client-%d", e.who), Action: "enqueue-reservation", Resource: resource})
		if !reflect.DeepEqual(e.result, repeat) {
			t.Fatal("enqueue retry changed order", e, repeat)
		}
	}
	head := entries[0]
	denied := h.execute(t, head.who, Request{ID: "head-waits-current-holder", Action: "acquire-reservation", Resource: resource, Record: head.result.Record})
	if denied.Status != "queued" || denied.Queue.Position != 1 {
		t.Fatal(denied)
	}
	renewed := h.execute(t, 3, Request{ID: "holder-authorized-renewal", Action: "renew-reservation", Resource: resource, Record: holder.Record, Fence: holder.Fence})
	if renewed.Status != "renewed" || !renewed.Lease.Current {
		t.Fatal(renewed)
	}
	released := h.execute(t, 3, Request{ID: "holder-done-release", Action: "release-reservation", Resource: resource, Record: holder.Record, Fence: holder.Fence})
	if released.Status != "released" {
		t.Fatal(released)
	}
	for i, e := range entries {
		barge := h.execute(t, 3, Request{ID: fmt.Sprintf("legacy-barging-%d", i), Action: "reserve", Resource: resource})
		if barge.Status != "busy" || barge.Lease != nil {
			t.Fatal("legacy bypassed FIFO", barge)
		}
		if i+1 < len(entries) {
			next := entries[i+1]
			out := h.execute(t, next.who, Request{ID: fmt.Sprintf("out-of-order-%d", i), Action: "acquire-reservation", Resource: resource, Record: next.result.Record})
			if out.Status != "queued" {
				t.Fatal("queue barged", out)
			}
		}
		grant := h.execute(t, e.who, Request{ID: fmt.Sprintf("head-acquisition-%d", i), Action: "acquire-reservation", Resource: resource, Record: e.result.Record})
		if grant.Status != "reserved" || grant.Record != e.result.Record || grant.Fence != holder.Fence+uint64(i)+1 || !grant.Lease.Current {
			t.Fatal(grant)
		}
		old := h.execute(t, 3, Request{ID: fmt.Sprintf("stale-holder-%d", i), Action: "release-reservation", Resource: resource, Record: holder.Record, Fence: holder.Fence})
		if old.Status != "lease-lost" {
			t.Fatal(old)
		}
		h.execute(t, e.who, Request{ID: fmt.Sprintf("finish-queued-%d", i), Action: "release-reservation", Resource: resource, Record: grant.Record, Fence: grant.Fence})
		completed := h.execute(t, e.who, Request{ID: fmt.Sprintf("completed-ticket-%d", i), Action: "acquire-reservation", Resource: resource, Record: grant.Record})
		if completed.Status != "queue-completed" {
			t.Fatal(completed)
		}
	}
}

func TestWorkerQueueCancellationExpiryAndRestart(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	first := h.execute(t, 0, Request{ID: "first-queue-expiring", Action: "enqueue-reservation", Resource: resource, WaitSeconds: 15})
	second := h.execute(t, 1, Request{ID: "second-queue-persistent", Action: "enqueue-reservation", Resource: resource, WaitSeconds: 120})
	third := h.execute(t, 2, Request{ID: "third-queue-cancelled", Action: "enqueue-reservation", Resource: resource})
	bad := h.execute(t, 0, Request{ID: "foreign-cancel-attempt", Action: "cancel-reservation", Resource: resource, Record: third.Record})
	if bad.Status != "not-owned" {
		t.Fatal(bad)
	}
	cancel := h.execute(t, 2, Request{ID: "cancel-own-queue-ticket", Action: "cancel-reservation", Resource: resource, Record: third.Record})
	if cancel.Status != "queue-cancelled" {
		t.Fatal(cancel)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	h.now = h.now.Add(16 * time.Second)
	expired := h.execute(t, 0, Request{ID: "read-expired-head-ticket", Action: "acquire-reservation", Resource: resource, Record: first.Record})
	if expired.Status != "queue-expired" {
		t.Fatal(expired)
	}
	grant := h.execute(t, 1, Request{ID: "restarted-head-acquire", Action: "acquire-reservation", Resource: resource, Record: second.Record})
	if grant.Status != "reserved" || !grant.Lease.Current || grant.Fence != 1 {
		t.Fatal(grant)
	}
	cancelGrant := h.execute(t, 1, Request{ID: "cancel-cannot-preempt-grant", Action: "cancel-reservation", Resource: resource, Record: second.Record})
	if cancelGrant.Status != "queue-granted" {
		t.Fatal(cancelGrant)
	}
	renew := h.execute(t, 1, Request{ID: "grant-remains-live-after-cancel", Action: "renew-reservation", Resource: resource, Record: grant.Record, Fence: grant.Fence})
	if renew.Status != "renewed" || !renew.Lease.Current {
		t.Fatal(renew)
	}
	// The original admission receipt cannot reacquire after its lease expires.
	h.now = h.now.Add(121 * time.Second)
	old := h.execute(t, 1, Request{ID: "restarted-head-acquire", Action: "acquire-reservation", Resource: resource, Record: second.Record})
	if old.Status != "reserved" || old.Lease.Current {
		t.Fatal(old)
	}
	newGrant := h.execute(t, 0, Request{ID: "legacy-after-queue-drained", Action: "reserve", Resource: resource})
	if newGrant.Status != "reserved" || newGrant.Fence != 2 {
		t.Fatal(newGrant)
	}
}

func TestWorkerQueueSameActorNewEnqueueIDPreservesOriginalTicketAndLease(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	holder := h.execute(t, 3, Request{ID: "holder-before-deduplicated-enqueue", Action: "reserve", Resource: resource, LeaseSeconds: 120})
	first := h.execute(t, 0, Request{ID: "original-waiting-enqueue", Action: "enqueue-reservation", Resource: resource, LeaseSeconds: 15, WaitSeconds: 120})
	if first.Status != "queued" || first.Queue == nil || first.Lease != nil {
		t.Fatal(first)
	}
	h.now = h.now.Add(20 * time.Second)
	// A new request ID with longer requested deadlines must reuse the original
	// waiting ticket, not extend its wait, change its lease or consume an order.
	duplicate := h.execute(t, 0, Request{ID: "different-enqueue-id-same-actor", Action: "enqueue-reservation", Resource: resource, LeaseSeconds: 900, WaitSeconds: 900})
	if !reflect.DeepEqual(duplicate, first) {
		t.Fatal("new enqueue ID replaced or extended the waiting ticket", first, duplicate)
	}
	view := h.view(t, 3)
	if len(view.Resources) != 1 || view.Resources[0].Fence != holder.Fence || view.Resources[0].Expires == nil || !view.Resources[0].Expires.Equal(*holder.Expires) {
		t.Fatal("deduplicated enqueue changed the current holder's lease", holder, view.Resources)
	}
	next := h.execute(t, 1, Request{ID: "next-actor-after-deduplication", Action: "enqueue-reservation", Resource: resource})
	if next.Queue == nil || next.Queue.Sequence != first.Queue.Sequence+1 || next.Queue.Position != 2 {
		t.Fatal("deduplication consumed an order or created another waiter", first, next)
	}
	h.execute(t, 3, Request{ID: "holder-releases-before-original-grant", Action: "release-reservation", Resource: resource, Record: holder.Record, Fence: holder.Fence})
	grant := h.execute(t, 0, Request{ID: "acquire-original-deduplicated-ticket", Action: "acquire-reservation", Resource: resource, Record: first.Record})
	if grant.Status != "reserved" || grant.Record != first.Record || grant.Fence != holder.Fence+1 || grant.Lease == nil || !grant.Lease.Current || grant.Expires == nil || !grant.Expires.Equal(h.now.Add(15*time.Second)) {
		t.Fatal("new enqueue ID changed the original ticket's lease duration", grant)
	}
}

func TestWorkerQueueRemoteAndLocalShareFIFOAuthority(t *testing.T) {
	r := sharedFixture(t)
	resource := "node:" + r.node + ":heavy"
	remoteEnqueue := Request{ID: "remote-queue-first", Action: "enqueue-reservation", Resource: resource}
	intent, _ := submitShared(t, r, remoteEnqueue)
	reply := applySharedFixture(t, r, intent, "admit")
	if reply.Result.Status != "queued" || reply.Current {
		t.Fatal(reply)
	}
	ticket := reply.Result.Record
	local, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "local-queue-second", Action: "enqueue-reservation", Resource: resource})
	if err != nil || local.Queue.Position != 2 {
		t.Fatal(local, err)
	}
	barge, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "legacy-local-barge", Action: "reserve", Resource: resource})
	if err != nil || barge.Status != "busy" {
		t.Fatal(barge, err)
	}
	inspect, _ := submitShared(t, r, Request{ID: "remote-head-inspection", Action: "inspect-reservation", Resource: resource, Record: ticket})
	ready := applySharedFixture(t, r, inspect, "admit")
	if ready.Result.Queue == nil || !ready.Result.Queue.Ready || ready.Current {
		t.Fatal(ready)
	}
	acquire := Request{ID: "remote-head-acquisition", Action: "acquire-reservation", Resource: resource, Record: ticket}
	grantIntent, _ := submitShared(t, r, acquire)
	grant := applySharedFixture(t, r, grantIntent, "admit")
	if grant.Result.Status != "reserved" || !grant.Current || grant.Result.Fence != 1 {
		t.Fatal(grant)
	}
	_, sourceGrant, err := r.source.s.SharedIntent(testNode, r.source.tokens[0], r.source.inputs, acquire)
	if err != nil || sourceGrant.Lease == nil || !sourceGrant.Lease.Current {
		t.Fatal(sourceGrant, err)
	}
	release, _ := submitShared(t, r, Request{ID: "remote-queue-release", Action: "release-reservation", Resource: resource, Record: ticket, Fence: grant.Result.Fence})
	applySharedFixture(t, r, release, "admit")
	localGrant, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "local-next-acquisition", Action: "acquire-reservation", Resource: resource, Record: local.Record})
	if err != nil || localGrant.Status != "reserved" || localGrant.Fence != 2 || !localGrant.Lease.Current {
		t.Fatal(localGrant, err)
	}
	// Replaying the remote acquisition after release cannot revive its lease.
	late := applySharedFixture(t, r, grantIntent, "status")
	if late.Current {
		t.Fatal("released remote lease revived", late)
	}
}

func TestWorkerQueueRemoteCancellationAfterProjectWithdrawal(t *testing.T) {
	r := sharedFixture(t)
	resource := "node:" + r.node + ":heavy"
	intent, _ := submitShared(t, r, Request{ID: "remote-before-project-change", Action: "enqueue-reservation", Resource: resource})
	queued := applySharedFixture(t, r, intent, "admit")
	r.source.inputs[0].Project = nil
	cancel, _ := submitShared(t, r, Request{ID: "remote-cancel-without-project", Action: "cancel-reservation", Resource: resource, Record: queued.Result.Record})
	cancelled := applySharedFixture(t, r, cancel, "admit")
	if cancelled.Result.Status != "queue-cancelled" {
		t.Fatal(cancelled)
	}
}

func TestWorkerQueueRevokedHeadAndIndependentAuthorities(t *testing.T) {
	h := fixture(t)
	other := fixture(t)
	resource := "node:" + testNode + ":heavy"
	head := h.execute(t, 0, Request{ID: "revocable-queue-head", Action: "enqueue-reservation", Resource: resource})
	next := h.execute(t, 1, Request{ID: "remaining-queue-member", Action: "enqueue-reservation", Resource: resource})
	// The queue in one retained authority does not occupy another authority.
	independent := other.execute(t, 0, Request{ID: "independent-authority-grant", Action: "reserve", Resource: resource})
	if independent.Status != "reserved" || independent.Fence != 1 {
		t.Fatal(independent)
	}
	h.inputs[0].Active = false
	grant := h.execute(t, 1, Request{ID: "after-inactive-head", Action: "acquire-reservation", Resource: resource, Record: next.Record})
	if grant.Status != "reserved" || !grant.Lease.Current {
		t.Fatal(grant)
	}
	h.inputs[0].Active = true
	expired := h.execute(t, 0, Request{ID: "inactive-ticket-stays-expired", Action: "inspect-reservation", Resource: resource, Record: head.Record})
	if expired.Status != "queue-expired" {
		t.Fatal(expired)
	}
}

func TestWorkerQueueSequenceExhaustionNeverReusesOrder(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	h.execute(t, 0, Request{ID: "initialize-queue-sequence", Action: "enqueue-reservation", Resource: resource})
	db, _ := h.s.database()
	if err := db.Update(func(tx *bolt.Tx) error { return bucket(tx, "worker_queue").SetSequence(^uint64(0)) }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := h.execute(t, 1, Request{ID: fmt.Sprintf("exhausted-order-attempt-%d", i), Action: "enqueue-reservation", Resource: resource})
		if r.Status != "queue-exhausted" {
			t.Fatal("queue order wrapped", r)
		}
	}
}
