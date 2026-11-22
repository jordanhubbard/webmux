package coordination

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

func projectID(member Member) string {
	if member.Project == nil {
		return ""
	}
	return member.Project.ID
}
func sameProject(a, b Member) bool {
	return a.Ref.Owner == b.Ref.Owner && projectID(a) != "" && projectID(a) == projectID(b)
}
func syncMembers(tx *bolt.Tx, node string, inputs []MemberInput, now time.Time) error {
	seen := map[string]bool{}
	for _, input := range inputs {
		if !validRef(input.Ref) || input.Ref.Node != node || !validBinding(input.Project) || !validText(input.ExecutionHost, 1024) || !validText(input.Transport, 64) {
			return ErrInvalid
		}
		key := input.Ref.Key()
		if seen[key] {
			return ErrInvalid
		}
		seen[key] = true
	}
	previous, err := readAll[Member](bucket(tx, "members"))
	if err != nil {
		return err
	}
	for _, input := range inputs {
		key := input.Ref.Key()
		member, exists, err := get[Member](bucket(tx, "members"), key)
		if err != nil {
			return err
		}
		if exists && member.Revoked != nil {
			continue
		} // Tombstones never revive a stale incarnation.
		joined := !exists || (projectID(member) == "" && input.Project != nil)
		if !exists {
			member = Member{Awareness: "interface-unprovisioned", Joined: now}
		}
		changed := !reflect.DeepEqual(member.MemberInput, input) || !member.Present || now.Sub(member.Observed) >= 30*time.Second
		member.MemberInput = input
		member.Present = true
		if changed {
			member.Observed = now
			if err = put(bucket(tx, "members"), key, member); err != nil {
				return err
			}
		}
		if joined && input.Project != nil && enabled(tx) {
			peers, err := readAll[Member](bucket(tx, "members"))
			if err != nil {
				return err
			}
			for _, peer := range peers {
				if peer.Ref != member.Ref && peer.Present && peer.Revoked == nil && sameProject(member, peer) {
					id := digest([]byte("peer-joined\n" + key + "\n" + peer.Ref.Key() + "\n" + input.Project.ID))
					if _, err = addMessage(tx, Message{ID: id, From: member.Ref, To: peer.Ref, Project: input.Project.ID, Kind: "peer-joined", Text: "A session joined this verified project.", State: "admitted", Created: now}); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, old := range previous {
		if !seen[old.Ref.Key()] && old.Revoked == nil {
			old.Present = false
			old.Active = false
			old.Revoked = &now
			if err = put(bucket(tx, "members"), old.Ref.Key(), old); err != nil {
				return err
			}
		}
	}
	return nil
}

// Sync registers the authoritative broker inventory without issuing capabilities
// to a browser. Existing members retain identity, work and receipt state.
func (s *Service) Sync(node string, inputs []MemberInput) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err := authorityTime(tx, now)
		if err != nil {
			return err
		}
		return syncMembers(tx, node, inputs, now)
	})
}

// Provision is internal launch plumbing, not an administrator/federation API.
// The scoped capability never authorizes terminal input, process control or
// account/configuration APIs. A stopped/removed member cannot use it.
func (s *Service) Provision(node string, input MemberInput) (string, error) {
	db, err := s.database()
	if err != nil {
		return "", err
	}
	token := ""
	err = db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		if !enabled(tx) {
			return ErrDisabled
		}
		if !validRef(input.Ref) || input.Ref.Node != node {
			return ErrInvalid
		}
		member, exists, err := get[Member](bucket(tx, "members"), input.Ref.Key())
		if err != nil {
			return err
		}
		if !exists || member.Revoked != nil || !member.Present || !member.Active {
			return ErrUnauthorized
		}
		token = capability(tx, member.Ref)
		return nil
	})
	return token, err
}
func (s *Service) SetEnabled(node string, value bool) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		if err := initialize(tx, node, s.Now()); err != nil {
			return err
		}
		setting := []byte("0")
		if value {
			setting = []byte("1")
		}
		return bucket(tx, "meta").Put([]byte("enabled"), setting)
	})
}
func (s *Service) OwnerView(node, owner string, inputs []MemberInput) (View, error) {
	db, err := s.database()
	if err != nil {
		return View{}, err
	}
	var view View
	err = db.Update(func(tx *bolt.Tx) error {
		now := s.Now()
		if err := initialize(tx, node, now); err != nil {
			return err
		}
		now, err := authorityTime(tx, now)
		if err != nil {
			return err
		}
		if err := syncMembers(tx, node, inputs, now); err != nil {
			return err
		}
		view, err = buildView(tx, owner, nil, 0, now)
		return err
	})
	return view, err
}
func (s *Service) AgentView(node, token string, inputs []MemberInput, after uint64) (View, error) {
	db, err := s.database()
	if err != nil {
		return View{}, err
	}
	var view View
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
		if err := syncMembers(tx, node, inputs, now); err != nil {
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
		view, err = buildView(tx, member.Ref.Owner, &member, after, now)
		return err
	})
	if err != nil {
		return view, err
	}
	return view, admitted
}
func buildView(tx *bolt.Tx, owner string, self *Member, after uint64, now time.Time) (View, error) {
	view := View{AuthorityTime: now, HistoryEpoch: string(bucket(tx, "meta").Get([]byte("history_epoch"))), Scope: "node-local", Contract: Contract, Enabled: enabled(tx), Self: self, Members: []Member{}, Claims: []Claim{}, Inbox: []Message{}, Outbox: []Message{}, Resources: []ResourceView{}, NextCursor: after}
	members, err := readAll[Member](bucket(tx, "members"))
	if err != nil {
		return view, err
	}
	for _, member := range members {
		if member.Ref.Owner == owner && member.Present && member.Revoked == nil && (self == nil || member.Ref == self.Ref || sameProject(member, *self)) {
			view.Members = append(view.Members, member)
		}
	}
	sort.Slice(view.Members, func(i, j int) bool { return view.Members[i].Ref.Key() < view.Members[j].Ref.Key() })
	claims, err := readAll[Claim](bucket(tx, "claims"))
	if err != nil {
		return view, err
	}
	for _, claim := range claims {
		if claim.Actor.Owner == owner && now.Before(claim.Expires) && (self == nil || claim.Project == projectID(*self) && claim.Project != "") {
			view.Claims = append(view.Claims, claim)
		}
	}
	resource := "node:" + string(bucket(tx, "meta").Get([]byte("node"))) + ":heavy"
	r := ResourceView{ID: resource, Authority: string(bucket(tx, "meta").Get([]byte("node")))}
	if held, exists, err := get[Reservation](bucket(tx, "reservations"), resource); err != nil {
		return view, err
	} else if exists && now.Before(held.Expires) {
		r.Occupied = true
		r.Fence = held.Fence
		r.Expires = &held.Expires
		if held.Actor.Owner == owner && (self == nil || held.Project == projectID(*self)) {
			r.Holder = &held.Actor
		}
	}
	view.Resources = append(view.Resources, r)
	if self != nil {
		inbox := bucket(tx, "inboxes").Bucket([]byte(self.Ref.Key()))
		if inbox != nil && after != ^uint64(0) {
			cursor := inbox.Cursor()
			key, value := cursor.Seek(sequence(after + 1))
			for count := 0; key != nil && count < 100; key, value = cursor.Next() {
				var messageID string
				if err := json.Unmarshal(value, &messageID); err != nil {
					return view, err
				}
				message, exists, err := get[Message](bucket(tx, "messages"), messageID)
				if err != nil || !exists {
					return view, fmt.Errorf("invalid inbox reference")
				}
				view.NextCursor = message.Index
				if message.Project == projectID(*self) && message.Project != "" {
					view.Inbox = append(view.Inbox, message)
					count++
				}
			}
		}
		outbox := bucket(tx, "outboxes").Bucket([]byte(self.Ref.Key()))
		if outbox != nil {
			cursor := outbox.Cursor()
			for key, value := cursor.Last(); key != nil && len(view.Outbox) < 100; key, value = cursor.Prev() {
				var messageID string
				if err := json.Unmarshal(value, &messageID); err != nil {
					return view, err
				}
				message, exists, err := get[Message](bucket(tx, "messages"), messageID)
				if err != nil || !exists {
					return view, fmt.Errorf("invalid outbox reference")
				}
				if message.Project == projectID(*self) && message.Project != "" {
					view.Outbox = append(view.Outbox, message)
				}
			}
		}
	}
	return view, nil
}
