package coordination

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func claimPeerFixture(t *testing.T, r *remoteHarness) ClaimPeer {
	t.Helper()
	view, err := r.receiver.OwnerView(r.node, "receiver", r.inputs)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot(view, r.node, "receiver", r.inputs[0].Project.ID)
	snapshot.Messaging = true
	snapshot.OverlapNotices = true
	snapshot.SourceNode = testNode
	snapshot.SourceOwner = "owner"
	return ClaimPeer{Node: r.node, Owner: "receiver", Project: snapshot.Project, State: "current", Started: r.source.now, Expires: r.source.now.Add(30 * time.Second), Snapshot: &snapshot, Route: &r.route}
}
func remoteClaim(t *testing.T, r *remoteHarness, request Request) Result {
	t.Helper()
	result, err := r.receiver.Execute(r.node, r.token, r.inputs, request)
	if err != nil || result.Status != "claimed" {
		t.Fatal(result, err)
	}
	return result
}
func prepareClaims(t *testing.T) *remoteHarness {
	t.Helper()
	return remoteFixture(t)
}
func TestPeerClaimConflictAndPairNoticeCoalescence(t *testing.T) {
	r := prepareClaims(t)
	theirs := remoteClaim(t, r, Request{ID: "remote-claim-directory", Action: "claim", Kind: "directory", Resource: "src"})
	// Both claims predate peer refresh: neither initial request saw a conflict.
	ours := r.source.execute(t, 0, Request{ID: "local-concurrent-file", Action: "claim", Kind: "file", Resource: "src/main.go"})
	peer := claimPeerFixture(t, r)
	for i := 0; i < 3; i++ {
		if err := r.source.s.ReconcileClaims(testNode, r.source.inputs, map[string][]ClaimPeer{"owner": {peer}}); err != nil {
			t.Fatal(err)
		}
	}
	notice := messagesOfKind(r.source.view(t, 0).Inbox, "overlap")
	if len(notice) != 1 || notice[0].From != r.inputs[0].Ref || notice[0].Transport != "peer-observation" {
		t.Fatal(notice)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 1 || records[0].Envelope.Overlap == nil {
		t.Fatal(records, err)
	}
	w := records[0].Envelope
	// A reciprocal observation derives the exact recipient inbox identity even
	// when it happens before the authenticated message reaches this node.
	localView := r.source.view(t, 0)
	snapshot := Snapshot(localView, testNode, "owner", r.inputs[0].Project.ID)
	snapshot.Messaging = true
	snapshot.OverlapNotices = true
	snapshot.SourceNode = r.node
	snapshot.SourceOwner = "receiver"
	route := RemoteRoute{Peer: "source", Node: testNode, Owner: "owner", Project: peer.Project, TLSKey: strings.Repeat("c", 64), Epoch: localView.HistoryEpoch}
	reverse := ClaimPeer{Node: testNode, Owner: "owner", Project: peer.Project, State: "current", Started: r.source.now, Expires: r.source.now.Add(30 * time.Second), Snapshot: &snapshot, Route: &route}
	if err = r.receiver.ReconcileClaims(r.node, r.inputs, map[string][]ClaimPeer{"receiver": {reverse}}); err != nil {
		t.Fatal(err)
	}
	view, err := r.receiver.AgentView(r.node, r.token, r.inputs, 0)
	if err != nil {
		t.Fatal(err)
	}
	inbox := messagesOfKind(view.Inbox, "overlap")
	if len(inbox) != 1 || inbox[0].ID != w.ID {
		t.Fatal("reciprocal detection changed event identity", inbox, w.ID)
	}
	// The recipient may review and complete the observed event before its peer's
	// delivery. Actual peer admission preserves those explicit event receipts.
	for _, action := range []string{"delivered", "acknowledge", "complete"} {
		result, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "observed-event-" + action, Action: action, Record: w.ID})
		if err != nil || result.Status != "recorded" {
			t.Fatal(result, err)
		}
	}
	allowed, err := r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	receipt, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "completed" {
		t.Fatal(receipt, err)
	}
	if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
		t.Fatal(err)
	}
	view, err = r.receiver.AgentView(r.node, r.token, r.inputs, 0)
	if err != nil {
		t.Fatal(err)
	}
	inbox = messagesOfKind(view.Inbox, "overlap")
	if len(inbox) != 1 || inbox[0].Transport != "remote-inbox" || inbox[0].State != "completed" {
		t.Fatal(inbox)
	}
	// A fresh claim-check intent renews the original claim and reports its remote
	// owner; an exact retry freezes that observation even after context disappears.
	request := Request{ID: "check-peer-conflicts", Action: "claim", Kind: "file", Resource: "src/main.go"}
	result, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || result.Record != ours.Record || len(result.Conflicts) != 1 || result.Conflicts[0].ID != theirs.Record || len(result.PeerClaims) != 1 || result.PeerClaims[0].State != "current" {
		t.Fatal(result, err)
	}
	repeated, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, nil)
	if err != nil || !reflect.DeepEqual(repeated, result) {
		t.Fatal(repeated, err)
	}
	records, err = r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal("renewal requeued a completed overlap event", records, err)
	}
	if err = r.source.s.Close(); err != nil {
		t.Fatal(err)
	}
	r.source.s = New(r.source.home)
	r.source.s.Now = func() time.Time { return r.source.now }
	if err = r.source.s.ReconcileClaims(testNode, r.source.inputs, map[string][]ClaimPeer{"owner": {peer}}); err != nil {
		t.Fatal(err)
	}
	records, err = r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 || len(messagesOfKind(r.source.view(t, 0).Inbox, "overlap")) != 1 {
		t.Fatal("reconstruction duplicated the event", records, err)
	}
}
func TestPeerClaimClockOffsetsExpiryAndIncompleteContext(t *testing.T) {
	r := prepareClaims(t)
	// A receiving clock one day ahead does not extend or hide the lease. Remaining
	// duration is anchored to the source request start, not compared across hosts.
	r.receiver.Now = func() time.Time { return r.source.now.Add(24 * time.Hour) }
	remoteClaim(t, r, Request{ID: "skewed-remote-claim", Action: "claim", Kind: "file", Resource: "src/main.go", LeaseSeconds: 15})
	peer := claimPeerFixture(t, r)
	request := Request{ID: "skewed-local-claim", Action: "claim", Kind: "directory", Resource: "src"}
	result, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || len(result.Conflicts) != 1 {
		t.Fatal("clock offset hid remote conflict", result, err)
	}
	r.source.now = r.source.now.Add(16 * time.Second)
	request.ID = "expired-remote-claim"
	result, err = r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || len(result.Conflicts) != 0 || result.PeerClaims[0].State != "current" {
		t.Fatal("relative lease expiry ignored", result, err)
	}
	r.source.now = r.source.now.Add(15 * time.Second)
	request.ID = "expired-peer-context"
	result, err = r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || len(result.PeerClaims) != 1 || result.PeerClaims[0].State != "unavailable" {
		t.Fatal("expired observation called current", result, err)
	}
}
func TestPeerClaimSummaryPermissionDoesNotSendAndForeignProjectsDoNotConflict(t *testing.T) {
	r := prepareClaims(t)
	remoteClaim(t, r, Request{ID: "summary-remote-claim", Action: "claim", Kind: "branch", Resource: "main"})
	peer := claimPeerFixture(t, r)
	peer.Route = nil
	peer.Snapshot.Messaging = false
	request := Request{ID: "summary-local-claim", Action: "claim", Kind: "branch", Resource: "main"}
	result, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || len(result.Conflicts) != 1 || result.PeerClaims[0].NotificationPolicy != "unavailable" {
		t.Fatal(result, err)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal("summary scope sent a message", records, err)
	}
	peer.Project = "unrelated"
	request.ID = "unrelated-peer-claim"
	result, err = r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, request, nil, []ClaimPeer{peer})
	if err != nil || len(result.Conflicts) != 0 || len(result.PeerClaims) != 0 {
		t.Fatal(result, err)
	}
	// Ordinary agents cannot forge internal overlap envelopes through /act.
	target := r.inputs[0].Ref
	if _, err = r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, Request{ID: "forged-overlap-message", Action: "message", Target: &target, Kind: "overlap", Text: "fake"}, &r.route); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestPeerOverlapReleasedClaimsRefuseDelivery(t *testing.T) {
	r := prepareClaims(t)
	remote := remoteClaim(t, r, Request{ID: "refused-target-claim", Action: "claim", Kind: "file", Resource: "main.go"})
	peer := claimPeerFixture(t, r)
	_, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, Request{ID: "refused-source-claim", Action: "claim", Kind: "file", Resource: "main.go"}, nil, []ClaimPeer{peer})
	if err != nil {
		t.Fatal(err)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 1 {
		t.Fatal(records, err)
	}
	w := records[0].Envelope
	bad := w
	bad.Text = "different"
	if bad.Valid() {
		t.Fatal("mutable internal notice text accepted")
	}
	if _, err = r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "release-target-claim", Action: "release-claim", Record: remote.Record}); err != nil {
		t.Fatal(err)
	}
	receipt, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "refused" {
		t.Fatal(receipt, err)
	}
	// Releasing the source claim also fences any new delivery attempt.
	r.source.execute(t, 0, Request{ID: "release-source-claim", Action: "release-claim", Record: w.Overlap.Sender.ID})
	allowed, err := r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || allowed {
		t.Fatal(allowed, err)
	}
	records, err = r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
}

func TestPeerClaimsHoldInactiveOwnersLeaseWithoutSending(t *testing.T) {
	r := prepareClaims(t)
	remoteClaim(t, r, Request{ID: "inactive-peer-claim", Action: "claim", Kind: "file", Resource: "main.go"})
	r.inputs[0].Active = false
	peer := claimPeerFixture(t, r)
	result, err := r.source.s.ExecuteWithPeers(testNode, r.source.tokens[0], r.source.inputs, Request{ID: "inactive-peer-check", Action: "claim", Kind: "file", Resource: "main.go"}, nil, []ClaimPeer{peer})
	if err != nil || len(result.Conflicts) != 1 {
		t.Fatal("inactive owner lost its unexpired advisory lease", result, err)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal("inactive peer was sent a new notice", records, err)
	}
}
