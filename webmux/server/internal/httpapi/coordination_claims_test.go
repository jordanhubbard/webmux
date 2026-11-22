package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
)

func peerClaimAct(t *testing.T, p *coordinationPair, remote bool, who int, id, kind, resource string) plane.Result {
	t.Helper()
	handler, access := p.agent, p.access[who]
	if remote {
		handler = p.b.CoordinationAgentHandler()
		access = p.remote[who]
	}
	response := request(handler, "POST", "/act", directoryJSON(plane.Request{ID: id, Action: "claim", Kind: kind, Resource: resource}), access.Capability)
	requireStatus(t, response, 200)
	var result plane.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Status != "claimed" {
		t.Fatal(result, err)
	}
	return result
}
func waitPeerOverlap(t *testing.T, p *coordinationPair) (plane.Message, plane.Message) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		view := p.view(t, 0)
		var incoming, outgoing plane.Message
		for _, m := range view.Inbox {
			if m.Kind == "overlap" && m.From.Node == p.remote[0].Subject.Node {
				incoming = m
			}
		}
		for _, m := range view.Outbox {
			if m.Kind == "overlap" && m.To.Node == p.remote[0].Subject.Node {
				outgoing = m
			}
		}
		if incoming.ID != "" && outgoing.ID != "" {
			return incoming, outgoing
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("background reconciliation did not detect the remote overlap")
	return plane.Message{}, plane.Message{}
}
func TestCoordinationPeerClaimImmediateConflictAndDurableNotices(t *testing.T) {
	p := peerPair(t)
	p.messaging(t, true)
	theirs := peerClaimAct(t, p, true, 0, "remote-directory-claim", "directory", "src")
	p.refresh(t)
	result := peerClaimAct(t, p, false, 0, "local-file-claim", "file", "src/main.go")
	if len(result.Conflicts) != 1 || result.Conflicts[0].ID != theirs.Record || result.Conflicts[0].Actor != p.remote[0].Subject || len(result.PeerClaims) != 1 || result.PeerClaims[0].State != "current" {
		t.Fatal(result)
	}
	incoming, outgoing := waitPeerOverlap(t, p)
	if incoming.Transport != "peer-observation" || incoming.Overlap == nil {
		t.Fatal(incoming)
	}
	coordinationMessageWait(t, p, outgoing.ID, "admitted", "remote-admitted")
	response := request(p.b.CoordinationAgentHandler(), "GET", "/view", "", p.remote[0].Capability)
	requireStatus(t, response, 200)
	var view plane.View
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range view.Inbox {
		if m.ID == outgoing.ID {
			count++
			if m.Kind != "overlap" || m.Overlap == nil || m.Overlap.Sender.ID != result.Record {
				t.Fatal(m)
			}
		}
	}
	if count != 1 {
		t.Fatal("recipient overlap entry not unique", count)
	}
	foreign := peerClaimAct(t, p, false, 2, "foreign-file-claim", "file", "src/main.go")
	if len(foreign.Conflicts) != 0 || len(foreign.PeerClaims) != 0 {
		t.Fatal("foreign account borrowed mapped claim context", foreign)
	}
	text, err := p.b.sessions.Scrollback("owner", p.remote[0].Subject.Session)
	if err != nil || strings.Contains(text, outgoing.Text) {
		t.Fatal("overlap notice entered terminal", err)
	}
	// Renewing the claim with a new intent refreshes the lease and context but does
	// not create a second event for the same two claim cycles.
	again := peerClaimAct(t, p, false, 0, "local-file-recheck", "file", "src/main.go")
	if again.Record != result.Record || len(again.Conflicts) != 1 {
		t.Fatal(again)
	}
	after := p.view(t, 0)
	count = 0
	for _, m := range after.Outbox {
		if m.Kind == "overlap" && m.To == p.remote[0].Subject {
			count++
		}
	}
	if count != 1 {
		t.Fatal("renewal duplicated event", count)
	}
}
func TestCoordinationPeerClaimsConcurrentBeforeRefreshReconcileAutomatically(t *testing.T) {
	p := peerPair(t)
	p.messaging(t, true)
	ours := peerClaimAct(t, p, false, 0, "concurrent-local-claim", "directory", "src")
	if len(ours.Conflicts) != 0 || len(ours.PeerClaims) != 1 || ours.PeerClaims[0].State != "current" {
		t.Fatal(ours)
	}
	peerClaimAct(t, p, true, 0, "concurrent-remote-claim", "file", "src/main.go")
	// No direct refresh or maintenance call after the claims: the running server
	// worker discovers both, generates notices and dispatches the remote inbox.
	_, outgoing := waitPeerOverlap(t, p)
	coordinationMessageWait(t, p, outgoing.ID, "admitted", "remote-admitted")
}
func TestCoordinationPeerClaimsReportIncompleteAndLegacyContext(t *testing.T) {
	p := peerPair(t)
	p.mapOwner(t, "owner")
	result := peerClaimAct(t, p, false, 0, "pending-peer-claim", "file", "main.go")
	if len(result.PeerClaims) != 1 || result.PeerClaims[0].State == "current" {
		t.Fatal("missing permission appeared conflict-free", result)
	}
	p.share(t, true)
	p.refresh(t)
	result = peerClaimAct(t, p, false, 0, "fresh-peer-claim", "file", "main.go")
	if len(result.PeerClaims) != 1 || result.PeerClaims[0].State != "current" {
		t.Fatal(result)
	}
	// A legacy summary has no source authority clock. Preserve ordinary discovery,
	// but refuse to claim a complete cross-node ownership check.
	cache := &p.a.sessionCoord.peers
	cache.mu.Lock()
	for key, view := range cache.values {
		if view.Snapshot != nil {
			copy := *view.Snapshot
			copy.AuthorityTime = time.Time{}
			view.Snapshot = &copy
			cache.values[key] = view
		}
	}
	cache.mu.Unlock()
	result = peerClaimAct(t, p, false, 0, "legacy-peer-claim", "file", "main.go")
	if len(result.PeerClaims) != 1 || result.PeerClaims[0].State != "unavailable" {
		t.Fatal(result)
	}
}
