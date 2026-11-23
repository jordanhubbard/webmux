package coordination

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

func validateRequest(r Request) bool {
	if !validText(r.ID, 128) || len(r.ID) < 8 {
		return false
	}
	switch r.Action {
	case "work":
		return len(r.Text) <= 1024 && utf8.ValidString(r.Text) && !strings.ContainsFunc(r.Text, unicode.IsControl)
	case "claim":
		_, ok := canonicalResource(r.Kind, r.Resource)
		_, lease := leaseDuration(r.LeaseSeconds)
		return ok && lease
	case "release-claim":
		return validHex(r.Record, 32)
	case "message":
		return r.Target != nil && validRef(*r.Target) && (r.Kind == "message" || r.Kind == "handoff") && strings.TrimSpace(r.Text) != "" && len(r.Text) <= 8192 && utf8.ValidString(r.Text) && !strings.ContainsFunc(r.Text, func(c rune) bool { return unicode.IsControl(c) && c != '\n' && c != '\t' })
	case "delivered", "acknowledge", "complete":
		return validHex(r.Record, 32)
	case "reserve":
		_, ok := leaseDuration(r.LeaseSeconds)
		return validText(r.Resource, 256) && ok
	case "renew-reservation":
		_, ok := leaseDuration(r.LeaseSeconds)
		return validText(r.Resource, 256) && validHex(r.Record, 32) && r.Fence > 0 && ok
	case "release-reservation":
		return validText(r.Resource, 256) && validHex(r.Record, 32) && r.Fence > 0
	}
	return false
}

// Execute freezes the result of each admitted request ID, including conflict
// denials. A lost response can be retried without duplicating a message or
// acquiring a worker later under an old request. No external work is executed.
func (s *Service) Execute(node, token string, inputs []MemberInput, request Request) (Result, error) {
	return s.ExecuteRouted(node, token, inputs, request, nil)
}
func (s *Service) ExecuteRouted(node, token string, inputs []MemberInput, request Request, route *RemoteRoute) (Result, error) {
	return s.ExecuteWithPeers(node, token, inputs, request, route, nil)
}
func (s *Service) ExecuteWithPeers(node, token string, inputs []MemberInput, request Request, route *RemoteRoute, peers []ClaimPeer) (Result, error) {
	if !validateRequest(request) {
		return Result{}, ErrInvalid
	}
	db, err := s.database()
	if err != nil {
		return Result{}, err
	}
	var result Result
	var admitted error
	err = db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err := authorityTime(tx, now)
		if err != nil {
			return err
		}
		if err = syncMembers(tx, node, inputs, now); err != nil {
			return err
		}
		if !enabled(tx) {
			admitted = ErrDisabled
			return nil
		}
		member, err := authenticate(tx, token)
		if err != nil {
			admitted = err
			return nil
		}
		if err := s.admit(member.Ref.Owner, member.Project, request.Action); err != nil {
			admitted = err
			return nil
		}
		key := digest([]byte(member.Ref.Key() + "\n" + request.ID))
		data, _ := json.Marshal(request)
		hash := digest(data)
		saved, exists, err := get[receipt](bucket(tx, "receipts"), key)
		if err != nil {
			return err
		}
		if exists {
			if saved.Hash != hash {
				admitted = ErrInvalid
				return nil
			}
			if saved.Reservation != nil {
				result = sharedResult(tx, saved, now)
				return nil
			}
			result = saved.Result
			return decorateLease(tx, node, member.Ref, request, &result, now)
		}
		member.Awareness = "interface-confirmed"
		if err = put(bucket(tx, "members"), member.Ref.Key(), member); err != nil {
			return err
		}
		if target, shared := ReservationTarget(request); shared && target != node {
			var intent *ReservationIntent
			result, intent, err = queueReservation(tx, member, request, route, now)
			if err != nil {
				return err
			}
			saved := receipt{Hash: hash, Result: result, Reservation: intent}
			if err = put(bucket(tx, "receipts"), key, saved); err != nil {
				return err
			}
			result = sharedResult(tx, saved, now)
			return nil
		}
		if request.Action == "message" && request.Target != nil && request.Target.Node != node {
			result, err = queueRemote(tx, member, request, route, now)
		} else {
			result, err = apply(tx, member, request, now)
		}
		if err != nil {
			return err
		}
		if request.Action == "claim" && result.Status == "claimed" {
			claim, _, e := get[Claim](bucket(tx, "claims"), result.Record)
			if e != nil {
				return e
			}
			result.PeerClaims, result.Conflicts, e = observeClaim(tx, member, claim, peers, result.Conflicts, now)
			if e != nil {
				return e
			}
		}
		if err = put(bucket(tx, "receipts"), key, receipt{Hash: hash, Result: result}); err != nil {
			return err
		}
		return decorateLease(tx, node, member.Ref, request, &result, now)
	})
	if err != nil {
		return result, err
	}
	return result, admitted
}
func apply(tx *bolt.Tx, actor Member, r Request, now time.Time) (Result, error) {
	if r.Action == "work" {
		actor.Task = r.Text
		return Result{Status: "updated"}, put(bucket(tx, "members"), actor.Ref.Key(), actor)
	}
	if r.Action != "release-claim" && r.Action != "release-reservation" && projectID(actor) == "" {
		return Result{Status: "project-unavailable"}, nil
	}
	switch r.Action {
	case "claim":
		return claimWork(tx, actor, r, now)
	case "release-claim":
		claim, exists, err := get[Claim](bucket(tx, "claims"), r.Record)
		if err != nil {
			return Result{}, err
		}
		if !exists || claim.Actor != actor.Ref {
			return Result{Status: "not-owned"}, nil
		}
		return Result{Status: "released", Record: claim.ID}, bucket(tx, "claims").Delete([]byte(claim.ID))
	case "message":
		target, exists, err := get[Member](bucket(tx, "members"), r.Target.Key())
		if err != nil {
			return Result{}, err
		}
		if !exists || target.Ref != *r.Target || !target.Present || target.Revoked != nil || !sameProject(actor, target) {
			return Result{Status: "recipient-unavailable"}, nil
		}
		message := Message{ID: digest([]byte("message\n" + actor.Ref.Key() + "\n" + r.ID)), From: actor.Ref, To: target.Ref, Project: projectID(actor), Kind: r.Kind, Text: r.Text, State: "admitted", Created: now}
		_, err = addMessage(tx, message)
		return Result{Status: "admitted", Record: message.ID, State: "admitted"}, err
	case "delivered", "acknowledge", "complete":
		return receiptMessage(tx, actor, r, now)
	case "reserve", "renew-reservation", "release-reservation":
		return reserveWorker(tx, actor, r, now)
	}
	return Result{}, ErrInvalid
}

func addMessage(tx *bolt.Tx, message Message) (Message, error) {
	if old, exists, err := get[Message](bucket(tx, "messages"), message.ID); err != nil {
		return Message{}, err
	} else if exists {
		return old, nil
	}
	inbox, err := bucket(tx, "inboxes").CreateBucketIfNotExists([]byte(message.To.Key()))
	if err != nil {
		return Message{}, err
	}
	index, err := inbox.NextSequence()
	if err != nil {
		return Message{}, err
	}
	message.Index = index
	if err = put(bucket(tx, "messages"), message.ID, message); err != nil {
		return Message{}, err
	}
	if err = put(inbox, string(sequence(index)), message.ID); err != nil {
		return Message{}, err
	}
	outbox, err := bucket(tx, "outboxes").CreateBucketIfNotExists([]byte(message.From.Key()))
	if err != nil {
		return Message{}, err
	}
	sent, err := outbox.NextSequence()
	if err != nil {
		return Message{}, err
	}
	return message, put(outbox, string(sequence(sent)), message.ID)
}
func receiptMessage(tx *bolt.Tx, actor Member, r Request, now time.Time) (Result, error) {
	message, exists, err := get[Message](bucket(tx, "messages"), r.Record)
	if err != nil {
		return Result{}, err
	}
	if !exists || message.To != actor.Ref || message.Project != projectID(actor) {
		return Result{Status: "not-owned"}, nil
	}
	ranks := map[string]int{"admitted": 0, "delivered": 1, "acknowledged": 2, "completed": 3}
	desired := map[string]string{"delivered": "delivered", "acknowledge": "acknowledged", "complete": "completed"}[r.Action]
	current, target := ranks[message.State], ranks[desired]
	if target > current+1 {
		return Result{Status: "receipt-order", Record: message.ID, State: message.State}, nil
	}
	if target > current {
		message.State = desired
		switch desired {
		case "delivered":
			message.Delivered = &now
		case "acknowledged":
			message.Acknowledged = &now
		case "completed":
			message.Completed = &now
		}
		if err = put(bucket(tx, "messages"), message.ID, message); err != nil {
			return Result{}, err
		}
	}
	return Result{Status: "recorded", Record: message.ID, State: message.State}, nil
}
func claimWork(tx *bolt.Tx, actor Member, r Request, now time.Time) (Result, error) {
	resource, _ := canonicalResource(r.Kind, r.Resource)
	duration, _ := leaseDuration(r.LeaseSeconds)
	claim := Claim{ID: digest([]byte("claim\n" + actor.Ref.Key() + "\n" + r.ID)), Actor: actor.Ref, Project: projectID(actor), Kind: r.Kind, Resource: resource, CaseFold: actor.Project.CaseFold, Expires: now.Add(duration)}
	all, err := readAll[Claim](bucket(tx, "claims"))
	if err != nil {
		return Result{}, err
	}
	conflicts := []Claim{}
	fresh := true
	for _, other := range all {
		if !now.Before(other.Expires) {
			if err = bucket(tx, "claims").Delete([]byte(other.ID)); err != nil {
				return Result{}, err
			}
			continue
		}
		if other.Actor == actor.Ref && other.Project == claim.Project && other.Kind == claim.Kind && other.Resource == claim.Resource {
			claim.ID = other.ID
			fresh = false
		}
		if other.Actor != actor.Ref && other.Actor.Owner == actor.Ref.Owner && overlaps(claim, other) {
			conflicts = append(conflicts, other)
		}
	}
	if err = put(bucket(tx, "claims"), claim.ID, claim); err != nil {
		return Result{}, err
	}
	if fresh {
		for _, other := range conflicts {
			for _, to := range []Ref{other.Actor, actor.Ref} {
				id := digest([]byte("overlap\n" + claim.ID + "\n" + other.ID + "\n" + to.Key()))
				from := actor.Ref
				if to == actor.Ref {
					from = other.Actor
				}
				_, err = addMessage(tx, Message{ID: id, From: from, To: to, Project: claim.Project, Kind: "overlap", Text: "Overlapping " + claim.Kind + " claim: " + claim.Resource, State: "admitted", Created: now})
				if err != nil {
					return Result{}, err
				}
			}
		}
	}
	return Result{Status: "claimed", Record: claim.ID, Expires: &claim.Expires, Conflicts: conflicts}, nil
}
func reserveWorker(tx *bolt.Tx, actor Member, r Request, now time.Time) (Result, error) {
	return reserveWorkerAt(tx, actor.Ref.Node, actor.Ref.Owner, actor, r, now)
}
func reserveWorkerAt(tx *bolt.Tx, node, owner string, actor Member, r Request, now time.Time) (Result, error) {
	resource := "node:" + node + ":heavy"
	if r.Resource != resource {
		return Result{Status: "unknown-resource"}, nil
	}
	held, exists, err := get[Reservation](bucket(tx, "reservations"), resource)
	if err != nil {
		return Result{}, err
	}
	active := exists && now.Before(held.Expires)
	if r.Action == "reserve" {
		if active {
			return Result{Status: "busy", Fence: held.Fence, Expires: &held.Expires}, nil
		}
		var fence uint64
		if raw := bucket(tx, "fences").Get([]byte(resource)); len(raw) == 8 {
			fence = binary.BigEndian.Uint64(raw)
		} else if raw != nil {
			return Result{}, ErrInvalid
		}
		if fence == ^uint64(0) {
			return Result{Status: "fence-exhausted"}, nil
		}
		fence++
		duration, _ := leaseDuration(r.LeaseSeconds)
		held = Reservation{Custodian: owner, ID: digest([]byte("reserve\n" + actor.Ref.Key() + "\n" + r.ID)), Resource: resource, Actor: actor.Ref, Project: projectID(actor), Fence: fence, Expires: now.Add(duration)}
		if err = bucket(tx, "fences").Put([]byte(resource), sequence(fence)); err != nil {
			return Result{}, err
		}
		if err = put(bucket(tx, "reservations"), resource, held); err != nil {
			return Result{}, err
		}
		return Result{Status: "reserved", Record: held.ID, Fence: held.Fence, Expires: &held.Expires}, nil
	}
	if !active || held.ID != r.Record || held.Actor != actor.Ref || reservationOwner(held) != owner || held.Fence != r.Fence {
		return Result{Status: "lease-lost"}, nil
	}
	if r.Action == "release-reservation" {
		return Result{Status: "released", Record: held.ID, Fence: held.Fence}, bucket(tx, "reservations").Delete([]byte(resource))
	}
	if held.Project != projectID(actor) {
		return Result{Status: "project-changed"}, nil
	}
	duration, _ := leaseDuration(r.LeaseSeconds)
	held.Expires = now.Add(duration)
	return Result{Status: "renewed", Record: held.ID, Fence: held.Fence, Expires: &held.Expires}, put(bucket(tx, "reservations"), resource, held)
}
