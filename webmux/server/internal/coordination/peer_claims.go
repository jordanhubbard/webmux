package coordination

import (
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ClaimPeer is an authorized observation supplied by the controller, never by an
// agent request. Started precedes network I/O, so remote clock offsets and transit
// time cannot extend the remaining lease advertised at the remote authority.
type ClaimPeer struct {
	Node, Owner, Project, State string
	Started, Expires            time.Time
	Snapshot                    *ProjectSnapshot
	Route                       *RemoteRoute
}
type PeerClaimCheck struct {
	Node               string `json:"node_id"`
	Owner              string `json:"owner"`
	State              string `json:"state"`
	NotificationPolicy string `json:"notification_policy"`
}

// NoticeClaim omits the renewable deadline. A notice identifies the original
// claim cycle; lease renewal cannot change its immutable envelope or request ID.
type NoticeClaim struct {
	ID       string `json:"id"`
	Actor    Ref    `json:"actor"`
	Project  string `json:"project"`
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
	CaseFold bool   `json:"case_fold"`
}

func noticeClaim(c Claim) NoticeClaim {
	return NoticeClaim{c.ID, c.Actor, c.Project, c.Kind, c.Resource, c.CaseFold}
}
func (c NoticeClaim) matches(other Claim) bool {
	return c.ID == other.ID && c.Actor == other.Actor && c.Project == other.Project && c.Kind == other.Kind && c.Resource == other.Resource
}
func (c NoticeClaim) claim() Claim {
	return Claim{ID: c.ID, Actor: c.Actor, Project: c.Project, Kind: c.Kind, Resource: c.Resource, CaseFold: c.CaseFold}
}
func (c NoticeClaim) valid() bool {
	resource, ok := canonicalResource(c.Kind, c.Resource)
	return ok && resource == c.Resource && validHex(c.ID, 32) && validRef(c.Actor) && ValidProjectSelector(c.Project)
}

type OverlapNotice struct {
	Sender    NoticeClaim `json:"sender"`
	Recipient NoticeClaim `json:"recipient"`
}

func (n OverlapNotice) requestID() string {
	return digest([]byte("peer-overlap\n" + n.Sender.ID + "\n" + n.Recipient.ID))
}
func (n OverlapNotice) text() string {
	return "Overlapping " + n.Sender.Kind + " claim: " + n.Sender.Resource + "\nYour claim: " + n.Recipient.Kind + " " + n.Recipient.Resource
}
func (n OverlapNotice) valid(w WireMessage) bool {
	return n.Sender.valid() && n.Recipient.valid() && n.Sender.Actor.Node != n.Recipient.Actor.Node && overlaps(n.Sender.claim(), n.Recipient.claim()) && w.Kind == "overlap" && w.Request == n.requestID() && w.Text == n.text() && w.From == n.Sender.Actor && w.To == n.Recipient.Actor && w.Project == n.Sender.Project && w.Project == n.Recipient.Project
}
func noticeEnvelope(sender, recipient Claim, epoch string) WireMessage {
	notice := &OverlapNotice{noticeClaim(sender), noticeClaim(recipient)}
	request := notice.requestID()
	return WireMessage{Contract: MessageContract, ID: digest([]byte("message\n" + sender.Actor.Key() + "\n" + request)), Request: request, Epoch: epoch, From: sender.Actor, To: recipient.Actor, Project: sender.Project, Kind: "overlap", Text: notice.text(), Overlap: notice}
}
func currentClaimPeer(peer ClaimPeer, now time.Time) bool {
	return peer.State == "current" && !peer.Started.IsZero() && !peer.Expires.IsZero() && !peer.Started.After(now) && now.Before(peer.Expires) && peer.Snapshot != nil && peer.Snapshot.Enabled && !peer.Snapshot.AuthorityTime.IsZero() && peer.Snapshot.Valid(peer.Node, peer.Owner, peer.Project)
}
func livePeerClaim(peer ClaimPeer, claim Claim, now time.Time) bool {
	// A full network round trip is conservatively deducted from remaining lease.
	if !now.Before(peer.Started.Add(claim.Expires.Sub(peer.Snapshot.AuthorityTime))) {
		return false
	}
	return true // Advisory ownership persists until lease expiry, including inactive owners.

}
func observeClaim(tx *bolt.Tx, actor Member, claim Claim, peers []ClaimPeer, conflicts []Claim, now time.Time) ([]PeerClaimCheck, []Claim, error) {
	checks := []PeerClaimCheck{}
	for _, peer := range peers {
		if peer.Project != claim.Project {
			continue
		}
		check := PeerClaimCheck{Node: peer.Node, Owner: peer.Owner, State: peer.State, NotificationPolicy: "unavailable"}
		if peer.Node == actor.Ref.Node || !currentClaimPeer(peer, now) {
			if check.State == "current" {
				check.State = "unavailable"
			}
			checks = append(checks, check)
			continue
		}
		route := peer.Route
		canNotify := route != nil && route.Valid() && route.Node == peer.Node && route.Owner == peer.Owner && route.Project == claim.Project && route.Epoch == peer.Snapshot.HistoryEpoch && peer.Snapshot.Messaging && peer.Snapshot.OverlapNotices && peer.Snapshot.SourceNode == actor.Ref.Node && peer.Snapshot.SourceOwner == actor.Ref.Owner
		if canNotify {
			check.NotificationPolicy = "allowed"
		}
		checks = append(checks, check)
		for _, other := range peer.Snapshot.Claims {
			if !livePeerClaim(peer, other, now) || !overlaps(claim, other) {
				continue
			}
			conflicts = append(conflicts, other)
			// Both authorities derive the same recipient-specific event identity. Local
			// observation and later authenticated delivery coalesce into one inbox entry.
			incoming := noticeEnvelope(other, claim, string(bucket(tx, "meta").Get([]byte("history_epoch"))))
			if !incoming.Valid() {
				return nil, nil, ErrInvalid
			}
			if _, err := addMessage(tx, Message{ID: incoming.ID, From: incoming.From, To: incoming.To, Project: incoming.Project, Kind: incoming.Kind, Text: incoming.Text, Overlap: incoming.Overlap, State: "admitted", Transport: "peer-observation", Created: now}); err != nil {
				return nil, nil, err
			}
			targetActive := false
			for _, member := range peer.Snapshot.Members {
				if member.Ref == other.Actor && member.Active && member.Present && member.Revoked == nil && projectID(member) == claim.Project {
					targetActive = true
					break
				}
			}
			if !canNotify || !targetActive {
				continue
			}
			outgoing := noticeEnvelope(claim, other, route.Epoch)
			if _, exists, err := get[RemoteRecord](bucket(tx, "remote_outbox"), outgoing.ID); err != nil {
				return nil, nil, err
			} else if exists {
				continue
			}
			if _, err := queueRemoteEnvelope(tx, actor, outgoing, route, now); err != nil {
				return nil, nil, err
			}
		}
	}
	return checks, conflicts, nil
}

// ReconcileClaims catches simultaneous claims made before either node's next
// snapshot. It is browser-independent and sends no terminal/provider input.
func (s *Service) ReconcileClaims(node string, inputs []MemberInput, peersByOwner map[string][]ClaimPeer) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err = authorityTime(tx, now)
		if err != nil {
			return err
		}
		if err = syncMembers(tx, node, inputs, now); err != nil {
			return err
		}
		if !enabled(tx) {
			return nil
		}
		claims, err := readAll[Claim](bucket(tx, "claims"))
		if err != nil {
			return err
		}
		for _, claim := range claims {
			if !now.Before(claim.Expires) {
				continue
			}
			actor, exists, err := get[Member](bucket(tx, "members"), claim.Actor.Key())
			if err != nil {
				return err
			}
			if !exists {
				return errors.New("claim actor history missing")
			}
			if !actor.Active || !actor.Present || actor.Revoked != nil || projectID(actor) != claim.Project {
				continue
			}
			if _, _, err = observeClaim(tx, actor, claim, peersByOwner[actor.Ref.Owner], nil, now); err != nil {
				return err
			}
		}
		return nil
	})
}
