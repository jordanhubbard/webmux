package coordination

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

func ValidSenderScope(node, owner string) bool { return validHex(node, 16) && validText(owner, 256) }

const MessageContract = "webmux-coordination-message/v1"

type RemoteRoute struct {
	Project string `json:"project_id"`
	Peer    string `json:"peer_id"`
	Node    string `json:"node_id"`
	Owner   string `json:"owner"`
	TLSKey  string `json:"tls_key_sha256"`
	Epoch   string `json:"history_epoch"`
}

func (r RemoteRoute) Valid() bool {
	return ValidProjectSelector(r.Project) && validText(r.Peer, 64) && validHex(r.Node, 16) && validText(r.Owner, 256) && validHex(r.TLSKey, 32) && validHex(r.Epoch, 16)
}

type WireMessage struct {
	Contract string         `json:"contract"`
	ID       string         `json:"message_id"`
	Request  string         `json:"request_id"`
	Epoch    string         `json:"history_epoch"`
	From     Ref            `json:"from"`
	To       Ref            `json:"to"`
	Project  string         `json:"project_id"`
	Kind     string         `json:"kind"`
	Text     string         `json:"text"`
	Overlap  *OverlapNotice `json:"overlap,omitempty"`
}

func (w WireMessage) Hash() string { data, _ := json.Marshal(w); return digest(data) }
func (w WireMessage) Valid() bool {
	kind := w.Kind
	if w.Overlap != nil {
		if !w.Overlap.valid(w) {
			return false
		}
		kind = "message"
	} else if kind == "overlap" {
		return false
	}
	return w.Contract == MessageContract && validHex(w.ID, 32) && validHex(w.Epoch, 16) && validRef(w.From) && validRef(w.To) && w.From.Node != w.To.Node && ValidProjectSelector(w.Project) && w.ID == digest([]byte("message\n"+w.From.Key()+"\n"+w.Request)) && validateRequest(Request{ID: w.Request, Action: "message", Target: &w.To, Kind: kind, Text: w.Text})
}

type SenderScope struct{ Node, Owner, ReceiverOwner string }
type RemoteReceipt struct {
	Contract     string     `json:"contract"`
	ID           string     `json:"message_id"`
	Hash         string     `json:"envelope_sha256"`
	Epoch        string     `json:"history_epoch"`
	State        string     `json:"state"`
	Delivered    *time.Time `json:"delivered,omitempty"`
	Acknowledged *time.Time `json:"acknowledged,omitempty"`
	Completed    *time.Time `json:"completed,omitempty"`
}

func (r RemoteReceipt) Valid(w WireMessage) bool {
	if r.Contract != MessageContract || r.ID != w.ID || r.Hash != w.Hash() || !validHex(r.Epoch, 16) {
		return false
	}
	if r.State == "history-changed" {
		return r.Epoch != w.Epoch
	}
	if r.Epoch != w.Epoch {
		return false
	}
	switch r.State {
	case "absent", "refused", "admitted", "delivered", "acknowledged", "completed":
	default:
		return false
	}
	rank := messageRank(r.State)
	return (rank < 1 || r.Delivered != nil) && (rank < 2 || r.Acknowledged != nil) && (rank < 3 || r.Completed != nil)
}
func messageRank(state string) int {
	switch state {
	case "admitted":
		return 0
	case "delivered":
		return 1
	case "acknowledged":
		return 2
	case "completed":
		return 3
	}
	return -1
}

type RemoteRecord struct {
	Envelope  WireMessage `json:"envelope"`
	Route     RemoteRoute `json:"route"`
	Admitted  bool        `json:"remote_admitted"`
	Attempted bool        `json:"attempted"`
	Terminal  bool        `json:"terminal"`
	Transport string      `json:"transport"`
}
type incomingMessage struct {
	Hash     string      `json:"hash"`
	Envelope WireMessage `json:"envelope"`
	Accepted bool        `json:"accepted"`
}

func queueRemote(tx *bolt.Tx, actor Member, r Request, route *RemoteRoute, now time.Time) (Result, error) {
	if projectID(actor) == "" {
		return Result{Status: "project-unavailable"}, nil
	}
	if route == nil || !route.Valid() || route.Node != r.Target.Node || route.Owner != r.Target.Owner || route.Project != projectID(actor) {
		return Result{Status: "peer-unavailable"}, nil
	}
	envelope := WireMessage{Contract: MessageContract, ID: digest([]byte("message\n" + actor.Ref.Key() + "\n" + r.ID)), Request: r.ID, Epoch: route.Epoch, From: actor.Ref, To: *r.Target, Project: projectID(actor), Kind: r.Kind, Text: r.Text}
	if !envelope.Valid() {
		return Result{}, ErrInvalid
	}
	return queueRemoteEnvelope(tx, actor, envelope, route, now)
}
func queueRemoteEnvelope(tx *bolt.Tx, actor Member, envelope WireMessage, route *RemoteRoute, now time.Time) (Result, error) {
	if !envelope.Valid() || route == nil || !route.Valid() || envelope.From != actor.Ref || envelope.Project != projectID(actor) || route.Project != envelope.Project || route.Node != envelope.To.Node || route.Owner != envelope.To.Owner || route.Epoch != envelope.Epoch {
		return Result{}, ErrInvalid
	}
	message := Message{Overlap: envelope.Overlap, ID: envelope.ID, From: envelope.From, To: envelope.To, Project: envelope.Project, Kind: envelope.Kind, Text: envelope.Text, State: "admitted", Transport: "pending", Created: now}
	if _, err := addMessage(tx, message); err != nil {
		return Result{}, err
	}
	record := RemoteRecord{Envelope: envelope, Route: *route, Transport: "pending"}
	if err := put(bucket(tx, "remote_active"), message.ID, true); err != nil {
		return Result{}, err
	}
	if err := put(bucket(tx, "remote_outbox"), message.ID, record); err != nil {
		return Result{}, err
	}
	return Result{Status: "admitted", Record: message.ID, State: "admitted", Transport: "pending"}, nil
}
func (s *Service) Exchange(node string, scope SenderScope, inputs []MemberInput, operation string, w WireMessage) (RemoteReceipt, error) {
	if !w.Valid() || (operation != "status" && operation != "admit") || w.To.Node != node || w.To.Owner != scope.ReceiverOwner || w.From.Node != scope.Node || w.From.Owner != scope.Owner {
		return RemoteReceipt{}, ErrUnauthorized
	}
	db, err := s.database()
	if err != nil {
		return RemoteReceipt{}, err
	}
	var out RemoteReceipt
	err = db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err = authorityTime(tx, now)
		if err != nil {
			return err
		}
		if err := syncMembers(tx, node, inputs, now); err != nil {
			return err
		}
		if !enabled(tx) {
			return ErrDisabled
		}
		epoch := string(bucket(tx, "meta").Get([]byte("history_epoch")))
		out = RemoteReceipt{Contract: MessageContract, ID: w.ID, Hash: w.Hash(), Epoch: epoch, State: "absent"}
		if w.Epoch != epoch {
			out.State = "history-changed"
			return nil
		}
		saved, exists, err := get[incomingMessage](bucket(tx, "remote_inbox"), w.ID)
		if err != nil {
			return err
		}
		if exists {
			if saved.Hash != w.Hash() {
				return ErrInvalid
			}
			if !saved.Accepted {
				out.State = "refused"
				return nil
			}
			message, found, err := get[Message](bucket(tx, "messages"), w.ID)
			if err != nil {
				return err
			}
			if !found {
				return errors.New("accepted remote inbox history is missing")
			}
			out.State, out.Delivered, out.Acknowledged, out.Completed = message.State, message.Delivered, message.Acknowledged, message.Completed
			return nil
		}
		if operation == "status" {
			return nil
		}
		target, found, err := get[Member](bucket(tx, "members"), w.To.Key())
		if err != nil {
			return err
		}
		accepted := found && target.Ref == w.To && target.Present && target.Active && target.Revoked == nil && projectID(target) == w.Project
		if accepted && w.Overlap != nil {
			claim, exists, e := get[Claim](bucket(tx, "claims"), w.Overlap.Recipient.ID)
			if e != nil {
				return e
			}
			accepted = exists && now.Before(claim.Expires) && w.Overlap.Recipient.matches(claim)
		}
		if accepted {
			existing, present, e := get[Message](bucket(tx, "messages"), w.ID)
			if e != nil {
				return e
			}
			if present {
				if w.Overlap == nil || existing.Transport != "peer-observation" || existing.From != w.From || existing.To != w.To || existing.Project != w.Project || existing.Kind != w.Kind || existing.Text != w.Text {
					return errors.New("unexpected remote inbox message collision")
				}
				existing.Transport = "remote-inbox"
				existing.Overlap = w.Overlap
				if e = put(bucket(tx, "messages"), w.ID, existing); e != nil {
					return e
				}
				out.State, out.Delivered, out.Acknowledged, out.Completed = existing.State, existing.Delivered, existing.Acknowledged, existing.Completed
			}
			_, err = addMessage(tx, Message{Overlap: w.Overlap, ID: w.ID, From: w.From, To: w.To, Project: w.Project, Kind: w.Kind, Text: w.Text, State: "admitted", Transport: "remote-inbox", Created: now})
			if err != nil {
				return err
			}
			if !present {
				out.State = "admitted"
			}
		} else {
			out.State = "refused"
		}
		return put(bucket(tx, "remote_inbox"), w.ID, incomingMessage{Hash: w.Hash(), Envelope: w, Accepted: accepted})
	})
	return out, err
}
func (s *Service) RemoteRecords(node string) ([]RemoteRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	var records []RemoteRecord
	err = db.View(func(tx *bolt.Tx) error {
		meta := bucket(tx, "meta")
		if meta == nil {
			return nil
		}
		if string(meta.Get([]byte("node"))) != node {
			return ErrIdentity
		}
		if string(meta.Get([]byte("version"))) == "1" {
			return nil // Normal maintenance performs the atomic legacy migration.
		}
		if string(meta.Get([]byte("version"))) != "2" || len(meta.Get([]byte("key"))) != 32 || !validHex(string(meta.Get([]byte("history_epoch"))), 16) {
			return errors.New("invalid remote outbox authority metadata")
		}
		for _, name := range buckets {
			if bucket(tx, name) == nil {
				return errors.New("missing coordination database bucket")
			}
		}
		clock, exists, err := get[time.Time](meta, "clock")
		if err != nil || !exists || clock.IsZero() {
			return errors.New("invalid coordination authority clock")
		}
		if s.Now().Before(clock) {
			return ErrClock
		}
		if string(meta.Get([]byte("enabled"))) != "1" && string(meta.Get([]byte("enabled"))) != "0" {
			return errors.New("invalid coordination policy")
		}
		if !enabled(tx) {
			return nil
		}
		active := bucket(tx, "remote_active")
		if active == nil || bucket(tx, "remote_outbox") == nil {
			return errors.New("remote outbox authority is missing")
		}
		return active.ForEach(func(key, value []byte) error {
			record, exists, err := get[RemoteRecord](bucket(tx, "remote_outbox"), string(key))
			if err != nil {
				return err
			}
			if !exists || !record.Envelope.Valid() || !record.Route.Valid() || record.Envelope.From.Node != node || record.Envelope.ID != string(key) || record.Envelope.To.Node != record.Route.Node || record.Envelope.To.Owner != record.Route.Owner || record.Envelope.Project != record.Route.Project || record.Envelope.Epoch != record.Route.Epoch {
				return errors.New("invalid active remote outbox record")
			}
			if !record.Terminal {
				records = append(records, record)
			}
			return nil
		})
	})
	return records, err
}

// BeginRemoteAttempt commits uncertainty before any delivery bytes can be sent.
// It rechecks the original logical actor against the observed live broker state.
func (s *Service) BeginRemoteAttempt(node, id string, inputs []MemberInput) (bool, error) {
	db, err := s.database()
	if err != nil {
		return false, err
	}
	allowed := false
	err = db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err = authorityTime(tx, now)
		if err != nil {
			return err
		}
		if err := syncMembers(tx, node, inputs, now); err != nil {
			return err
		}
		if !enabled(tx) {
			return ErrDisabled
		}
		record, exists, err := get[RemoteRecord](bucket(tx, "remote_outbox"), id)
		if err != nil {
			return err
		}
		if !exists || record.Terminal || record.Admitted {
			return nil
		}
		actor, exists, err := get[Member](bucket(tx, "members"), record.Envelope.From.Key())
		if err != nil {
			return err
		}
		if !exists || actor.Revoked != nil || !actor.Present || projectID(actor) != record.Envelope.Project {
			record.Terminal = true
			record.Transport = "sender-revoked"
		} else if record.Envelope.Overlap != nil {
			claim, exists, e := get[Claim](bucket(tx, "claims"), record.Envelope.Overlap.Sender.ID)
			if e != nil {
				return e
			}
			if !exists || !now.Before(claim.Expires) || !record.Envelope.Overlap.Sender.matches(claim) {
				record.Terminal = true
				record.Transport = "claim-expired"
			} else if !actor.Active {
				record.Transport = "sender-inactive"
			} else {
				allowed = true
				record.Attempted = true
				record.Transport = "uncertain"
			}
		} else if !actor.Active {
			record.Transport = "sender-inactive"
		} else {
			allowed = true
			record.Attempted = true
			record.Transport = "uncertain"
		}
		if record.Terminal {
			if err := bucket(tx, "remote_active").Delete([]byte(id)); err != nil {
				return err
			}
		}
		if err := put(bucket(tx, "remote_outbox"), id, record); err != nil {
			return err
		}
		return setMessageTransport(tx, id, record.Transport)
	})
	return allowed, err
}
func setMessageTransport(tx *bolt.Tx, id, state string) error {
	message, exists, err := get[Message](bucket(tx, "messages"), id)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("remote outbox message history is missing")
	}
	message.Transport = state
	return put(bucket(tx, "messages"), id, message)
}
func (s *Service) RemoteTransport(node, id, state string, terminal bool) error {
	switch state {
	case "uncertain", "authorization-unavailable", "peer-unavailable", "history-lost", "history-changed":
	default:
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
		record, exists, err := get[RemoteRecord](bucket(tx, "remote_outbox"), id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrInvalid
		}
		if record.Terminal {
			return nil
		}
		record.Transport, record.Terminal = state, terminal
		if record.Terminal {
			if err := bucket(tx, "remote_active").Delete([]byte(id)); err != nil {
				return err
			}
		}
		if err := put(bucket(tx, "remote_outbox"), id, record); err != nil {
			return err
		}
		return setMessageTransport(tx, id, state)
	})
}
func (s *Service) ApplyRemoteReceipt(node string, w WireMessage, receipt RemoteReceipt) error {
	if !receipt.Valid(w) {
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
		record, exists, err := get[RemoteRecord](bucket(tx, "remote_outbox"), w.ID)
		if err != nil {
			return err
		}
		if !exists || record.Envelope.Hash() != w.Hash() {
			return ErrInvalid
		}
		if record.Terminal {
			return nil
		}
		message, exists, err := get[Message](bucket(tx, "messages"), w.ID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("remote outbox message history is missing")
		}
		switch receipt.State {
		case "history-changed":
			record.Terminal = true
			record.Transport = "history-changed"
		case "absent":
			if record.Admitted {
				record.Terminal = true
				record.Transport = "history-lost"
			} else {
				return nil
			}
		case "refused":
			record.Terminal = true
			record.Transport = "refused"
		default:
			record.Admitted = true
			record.Transport = "remote-admitted"
			if messageRank(receipt.State) > messageRank(message.State) {
				message.State = receipt.State
				message.Delivered = receipt.Delivered
				message.Acknowledged = receipt.Acknowledged
				message.Completed = receipt.Completed
			}
			if receipt.State == "completed" {
				record.Terminal = true
			}
		}
		if record.Terminal {
			if err := bucket(tx, "remote_active").Delete([]byte(w.ID)); err != nil {
				return err
			}
		}
		message.Transport = record.Transport
		if err := put(bucket(tx, "messages"), message.ID, message); err != nil {
			return err
		}
		return put(bucket(tx, "remote_outbox"), message.ID, record)
	})
}
