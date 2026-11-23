package coordination

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const ReservationContract = "webmux-coordination-reservation/v1"

type LeaseCheck struct {
	Authority string `json:"authority_node_id"`
	Epoch     string `json:"history_epoch"`
	Current   bool   `json:"current"`
}
type SharedReservation struct {
	Intent    string `json:"intent_id"`
	Node      string `json:"node_id"`
	Owner     string `json:"owner"`
	Epoch     string `json:"history_epoch"`
	Transport string `json:"transport"`
}
type ReservationEnvelope struct {
	Contract string  `json:"contract"`
	ID       string  `json:"intent_id"`
	Epoch    string  `json:"history_epoch"`
	Actor    Ref     `json:"actor"`
	Node     string  `json:"node_id"`
	Owner    string  `json:"owner"`
	Project  string  `json:"project_id"`
	Request  Request `json:"request"`
}

func (e ReservationEnvelope) Hash() string { data, _ := json.Marshal(e); return digest(data) }
func (e ReservationEnvelope) Valid() bool {
	node, ok := ReservationTarget(e.Request)
	if (e.Request.Target != nil && (!validRef(*e.Request.Target) || e.Request.Target.Node != e.Node || e.Request.Target.Owner != e.Owner)) || e.Request.Text != "" || e.Request.Kind != "" {
		return false
	}
	return ok && e.Contract == ReservationContract && e.Node == node && e.Node != e.Actor.Node && validRef(e.Actor) && validText(e.Owner, 256) && validHex(e.Epoch, 16) && ValidProjectSelector(e.Project) && validateRequest(e.Request) && e.ID == requestKey(e.Actor, e.Request.ID)
}

type ReservationIntent struct {
	OutcomeHash string              `json:"outcome_sha256,omitempty"`
	Envelope    ReservationEnvelope `json:"envelope"`
	Route       RemoteRoute         `json:"route"`
	Attempted   bool                `json:"attempted"`
	Resolved    bool                `json:"resolved"`
	Terminal    bool                `json:"terminal"`
	Current     bool                `json:"current"`
	Transport   string              `json:"transport"`
}
type ReservationReply struct {
	Contract string    `json:"contract"`
	ID       string    `json:"intent_id"`
	Hash     string    `json:"envelope_sha256"`
	Epoch    string    `json:"history_epoch"`
	State    string    `json:"state"`
	Now      time.Time `json:"authority_time"`
	Result   *Result   `json:"result,omitempty"`
	Current  bool      `json:"current"`
}

func (r ReservationReply) Valid(e ReservationEnvelope) bool {
	if r.Contract != ReservationContract || r.ID != e.ID || r.Hash != e.Hash() || !validHex(r.Epoch, 16) || r.Now.IsZero() {
		return false
	}
	if r.State == "history-changed" {
		return r.Epoch != e.Epoch && r.Result == nil && !r.Current
	}
	if r.Epoch != e.Epoch {
		return false
	}
	if r.State == "absent" {
		return r.Result == nil && !r.Current
	}
	if r.State != "result" || r.Result == nil || r.Result.Shared != nil || r.Result.Lease != nil {
		return false
	}
	result := r.Result
	switch result.Status {
	case "reserved", "renewed":
		if !validHex(result.Record, 32) || result.Fence == 0 || result.Expires == nil {
			return false
		}
		if e.Request.Action == "reserve" && (result.Status != "reserved" || result.Record != digest([]byte("reserve\n"+e.Actor.Key()+"\n"+e.Request.ID))) {
			return false
		}
		if e.Request.Action != "reserve" && (e.Request.Action != "renew-reservation" || result.Status != "renewed" || result.Record != e.Request.Record || result.Fence != e.Request.Fence) {
			return false
		}
		return !r.Current || (result.Expires.After(r.Now) && !result.Expires.After(r.Now.Add(900*time.Second)))
	case "released":
		return !r.Current && e.Request.Action == "release-reservation" && result.Record == e.Request.Record && result.Fence == e.Request.Fence
	case "busy", "lease-lost", "project-changed", "project-unavailable", "fence-exhausted", "unknown-resource":
		return !r.Current
	}
	return false
}
func ReservationTarget(r Request) (string, bool) {
	if r.Action != "reserve" && r.Action != "renew-reservation" && r.Action != "release-reservation" {
		return "", false
	}
	node := strings.TrimSuffix(strings.TrimPrefix(r.Resource, "node:"), ":heavy")
	return node, validHex(node, 16) && r.Resource == "node:"+node+":heavy"
}
func reservationOwner(r Reservation) string {
	if r.Custodian != "" {
		return r.Custodian
	}
	return r.Actor.Owner
}
func requestKey(actor Ref, id string) string { return digest([]byte(actor.Key() + "\n" + id)) }
func leaseRouteKey(actor Ref, r Request) string {
	return "reservation-route/" + digest([]byte(actor.Key()+"\n"+r.Resource+"\n"+r.Record+"\n"+string(sequence(r.Fence))))
}
func decorateLease(tx *bolt.Tx, node string, actor Ref, r Request, result *Result, now time.Time) error {
	if result.Status != "reserved" && result.Status != "renewed" {
		return nil
	}
	held, exists, err := get[Reservation](bucket(tx, "reservations"), r.Resource)
	if err != nil {
		return err
	}
	member, present, memberErr := get[Member](bucket(tx, "members"), actor.Key())
	if memberErr != nil {
		return memberErr
	}
	result.Lease = &LeaseCheck{Authority: node, Epoch: string(bucket(tx, "meta").Get([]byte("history_epoch"))), Current: present && member.Active && member.Present && member.Revoked == nil && projectID(member) == held.Project && reservationOwner(held) == actor.Owner && exists && held.Actor == actor && held.ID == result.Record && held.Fence == result.Fence && now.Before(held.Expires) && result.Expires != nil && now.Before(*result.Expires)}
	return nil
}
func queueReservation(tx *bolt.Tx, actor Member, r Request, route *RemoteRoute, now time.Time) (Result, *ReservationIntent, error) {
	project := projectID(actor)
	if r.Action == "release-reservation" {
		saved, exists, err := get[RemoteRoute](bucket(tx, "meta"), leaseRouteKey(actor.Ref, r))
		if err != nil {
			return Result{}, nil, err
		}
		if !exists || !saved.Valid() {
			return Result{Status: "lease-lost"}, nil, nil
		}
		route = &saved
		project = saved.Project
	}
	if project == "" {
		return Result{Status: "project-unavailable"}, nil, nil
	}
	node, _ := ReservationTarget(r)
	if route == nil || !route.Valid() || route.Node != node || route.Project != project {
		return Result{Status: "peer-unavailable"}, nil, nil
	}
	e := ReservationEnvelope{Contract: ReservationContract, ID: requestKey(actor.Ref, r.ID), Epoch: route.Epoch, Actor: actor.Ref, Node: node, Owner: route.Owner, Project: project, Request: r}
	if !e.Valid() {
		return Result{}, nil, ErrInvalid
	}
	intent := &ReservationIntent{Envelope: e, Route: *route, Transport: "pending"}
	return Result{Status: "reservation-pending"}, intent, nil
}
func sharedResult(tx *bolt.Tx, saved receipt, now time.Time) Result {
	result := saved.Result
	if saved.Reservation == nil {
		return result
	}
	intent := saved.Reservation
	e := intent.Envelope
	result.Shared = &SharedReservation{Intent: e.ID, Node: e.Node, Owner: e.Owner, Epoch: e.Epoch, Transport: intent.Transport}
	if result.Status == "reserved" || result.Status == "renewed" {
		request := e.Request
		request.Record = result.Record
		request.Fence = result.Fence
		released, _, err := get[bool](bucket(tx, "meta"), "released/"+leaseRouteKey(e.Actor, request))
		result.Lease = &LeaseCheck{Authority: e.Node, Epoch: e.Epoch, Current: err == nil && !released && intent.Current && !intent.Terminal && result.Expires != nil && now.Before(*result.Expires)}
	}
	return result
}
func (s *Service) SharedIntent(node, token string, inputs []MemberInput, r Request) (ReservationIntent, Result, error) {
	var intent ReservationIntent
	var result Result
	db, err := s.database()
	if err != nil {
		return intent, result, err
	}
	var admitted error
	err = db.Update(func(tx *bolt.Tx) error {
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
			admitted = ErrDisabled
			return nil
		}
		actor, e := authenticate(tx, token)
		if e != nil {
			admitted = e
			return nil
		}
		if err := s.admit(actor.Ref.Owner, actor.Project, r.Action); err != nil {
			admitted = err
			return nil
		}
		saved, exists, e := get[receipt](bucket(tx, "receipts"), requestKey(actor.Ref, r.ID))
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(r)
		if !exists || saved.Hash != digest(raw) || saved.Reservation == nil {
			admitted = ErrInvalid
			return nil
		}
		intent = *saved.Reservation
		result = sharedResult(tx, saved, now)
		if r.Action != "release-reservation" && projectID(actor) != intent.Envelope.Project {
			intent.Terminal = true
			if result.Lease != nil {
				result.Lease.Current = false
			}
			result.Shared.Transport = "project-changed"
		}
		return nil
	})
	if err == nil {
		err = admitted
	}
	return intent, result, err
}

// BeginSharedAttempt commits uncertainty after the final live source/project
// check, before any mutation bytes. Status reads need not extend a lease.
func (s *Service) BeginSharedAttempt(node, token string, inputs []MemberInput, r Request) (bool, error) {
	db, err := s.database()
	if err != nil {
		return false, err
	}
	allowed := false
	var admitted error
	err = db.Update(func(tx *bolt.Tx) error {
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
			admitted = ErrDisabled
			return nil
		}
		actor, e := authenticate(tx, token)
		if e != nil {
			admitted = e
			return nil
		}
		if err := s.admit(actor.Ref.Owner, actor.Project, r.Action); err != nil {
			admitted = err
			return nil
		}
		key := requestKey(actor.Ref, r.ID)
		saved, exists, e := get[receipt](bucket(tx, "receipts"), key)
		if e != nil {
			return e
		}
		if !exists || saved.Reservation == nil {
			return ErrInvalid
		}
		intent := saved.Reservation
		raw, _ := json.Marshal(r)
		if digest(raw) != saved.Hash {
			return ErrInvalid
		}
		if intent.Terminal || intent.Resolved {
			return nil
		}
		if r.Action != "release-reservation" && projectID(actor) != intent.Envelope.Project {
			intent.Transport = "project-changed"
			intent.Terminal = true
			saved.Result = Result{Status: "project-changed"}
		} else {
			allowed = true
			intent.Attempted = true
			intent.Current = false
			intent.Transport = "uncertain"
		}
		return put(bucket(tx, "receipts"), key, saved)
	})
	if err == nil {
		err = admitted
	}
	return allowed, err
}
func (s *Service) MarkSharedUnavailable(node string, e ReservationEnvelope, state string) error {
	if state != "uncertain" && state != "authorization-unavailable" {
		return ErrInvalid
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		if err := initialize(tx, node, s.Now()); err != nil {
			return err
		}
		if _, err := authorityTime(tx, s.Now()); err != nil {
			return err
		}
		saved, exists, err := get[receipt](bucket(tx, "receipts"), e.ID)
		if err != nil {
			return err
		}
		if !exists || saved.Reservation == nil || saved.Reservation.Envelope.Hash() != e.Hash() {
			return ErrInvalid
		}
		if !saved.Reservation.Terminal {
			saved.Reservation.Transport = state
			saved.Reservation.Current = false
		}
		return put(bucket(tx, "receipts"), e.ID, saved)
	})
}
func (s *Service) ApplySharedReply(node string, e ReservationEnvelope, reply ReservationReply, started time.Time) error {
	if !reply.Valid(e) || started.IsZero() {
		return ErrInvalid
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		if _, err = authorityTime(tx, now); err != nil {
			return err
		}
		if started.After(now) {
			return ErrInvalid
		}
		saved, exists, err := get[receipt](bucket(tx, "receipts"), e.ID)
		if err != nil {
			return err
		}
		if !exists || saved.Reservation == nil || saved.Reservation.Envelope.Hash() != e.Hash() {
			return ErrInvalid
		}
		intent := saved.Reservation
		if intent.Terminal {
			return nil
		}
		switch reply.State {
		case "history-changed":
			intent.Transport = "history-changed"
			intent.Current = false
			intent.Terminal = true
			if !intent.Resolved {
				saved.Result = Result{Status: "history-changed"}
			}
		case "absent":
			if intent.Resolved {
				intent.Transport = "history-lost"
				intent.Current = false
				intent.Terminal = true
			}
			if !intent.Terminal {
				return nil
			}
		case "result":
			outcome, _ := json.Marshal(reply.Result)
			if !intent.Resolved {
				intent.OutcomeHash = digest(outcome)
				saved.Result = *reply.Result
				if saved.Result.Expires != nil {
					deadline := started.Add(saved.Result.Expires.Sub(reply.Now))
					saved.Result.Expires = &deadline
				}
				intent.Resolved = true
			} else if intent.OutcomeHash != digest(outcome) {
				return errors.New("reservation authority changed its frozen outcome")
			}

			if saved.Result.Status == "released" {
				if err = put(bucket(tx, "meta"), "released/"+leaseRouteKey(e.Actor, e.Request), true); err != nil {
					return err
				}
			}
			intent.Transport = "resolved"
			intent.Current = reply.Current && saved.Result.Expires != nil && now.Before(*saved.Result.Expires)
			if saved.Result.Status == "reserved" || saved.Result.Status == "renewed" {
				request := e.Request
				request.Record = saved.Result.Record
				request.Fence = saved.Result.Fence
				if err = put(bucket(tx, "meta"), leaseRouteKey(e.Actor, request), intent.Route); err != nil {
					return err
				}
			}
		}
		return put(bucket(tx, "receipts"), e.ID, saved)
	})
}
func (s *Service) ExchangeReservation(node string, scope SenderScope, inputs []MemberInput, operation string, e ReservationEnvelope) (ReservationReply, error) {
	if !e.Valid() || e.Node != node || e.Owner != scope.ReceiverOwner || e.Actor.Node != scope.Node || e.Actor.Owner != scope.Owner || (operation != "status" && operation != "admit") {
		return ReservationReply{}, ErrUnauthorized
	}
	db, err := s.database()
	if err != nil {
		return ReservationReply{}, err
	}
	var reply ReservationReply
	err = db.Update(func(tx *bolt.Tx) error {
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
			return ErrDisabled
		}
		var project *Binding
		for _, input := range inputs {
			if input.Ref.Owner == scope.ReceiverOwner && input.Project != nil && input.Project.ID == e.Project {
				project = input.Project
				break
			}
		}
		if err := s.admit(scope.ReceiverOwner, project, e.Request.Action); err != nil {
			return err
		}
		reply = ReservationReply{Contract: ReservationContract, ID: e.ID, Hash: e.Hash(), Epoch: string(bucket(tx, "meta").Get([]byte("history_epoch"))), State: "absent", Now: now}
		if reply.Epoch != e.Epoch {
			reply.State = "history-changed"
			return nil
		}
		key := digest([]byte("shared-reservation\n" + scope.ReceiverOwner + "\n" + e.ID))
		saved, exists, err := get[receipt](bucket(tx, "receipts"), key)
		if err != nil {
			return err
		}
		if exists {
			if saved.Hash != e.Hash() {
				return ErrInvalid
			}
		} else {
			if operation == "status" {
				return nil
			}
			projectAvailable := false
			for _, input := range inputs {
				if input.Ref.Owner == scope.ReceiverOwner && input.Active && input.Project != nil && input.Project.ID == e.Project {
					member, found, er := get[Member](bucket(tx, "members"), input.Ref.Key())
					if er != nil {
						return er
					}
					if found && member.Present && member.Revoked == nil {
						projectAvailable = true
						break
					}
				}
			}
			result := Result{Status: "project-unavailable"}
			if projectAvailable || e.Request.Action == "release-reservation" {
				actor := Member{MemberInput: MemberInput{Ref: e.Actor, Active: true, Project: &Binding{ID: e.Project}}}
				result, err = reserveWorkerAt(tx, node, scope.ReceiverOwner, actor, e.Request, now)
				if err != nil {
					return err
				}
			}
			saved = receipt{Hash: e.Hash(), Result: result}
			if err = put(bucket(tx, "receipts"), key, saved); err != nil {
				return err
			}
		}
		reply.State = "result"
		reply.Result = &saved.Result
		if saved.Result.Status == "reserved" || saved.Result.Status == "renewed" {
			held, found, err := get[Reservation](bucket(tx, "reservations"), e.Request.Resource)
			if err != nil {
				return err
			}
			reply.Current = found && held.Actor == e.Actor && reservationOwner(held) == scope.ReceiverOwner && held.Project == e.Project && held.ID == saved.Result.Record && held.Fence == saved.Result.Fence && now.Before(held.Expires) && saved.Result.Expires != nil && now.Before(*saved.Result.Expires)
		}
		return nil
	})
	return reply, err
}
