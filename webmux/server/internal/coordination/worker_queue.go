package coordination

import (
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Tickets and actual leases share the authority's serializable transaction and
// fence counter. A ticket is never permission to execute work. Acquisition is
// explicit so an absent client is not silently given a running lease.
type workerTicket struct {
	ID           string    `json:"id"`
	Resource     string    `json:"resource"`
	Project      string    `json:"project"`
	Custodian    string    `json:"custodian"`
	Actor        Ref       `json:"actor"`
	Sequence     uint64    `json:"sequence"`
	Deadline     time.Time `json:"deadline"`
	LeaseSeconds int       `json:"lease_seconds"`
	State        string    `json:"state"`
}

func waitingWorkers(tx *bolt.Tx, node, resource string, now time.Time) ([]workerTicket, error) {
	if bucket(tx, "worker_queue") == nil {
		return []workerTicket{}, nil
	}
	all, err := readAll[workerTicket](bucket(tx, "worker_queue"))
	if err != nil {
		return nil, err
	}
	waiting := []workerTicket{}
	for _, ticket := range all {
		if ticket.Resource != resource || ticket.State != "waiting" {
			continue
		}
		valid := now.Before(ticket.Deadline)
		if ticket.Actor.Node == node {
			m, found, err := get[Member](bucket(tx, "members"), ticket.Actor.Key())
			if err != nil {
				return nil, err
			}
			valid = valid && found && m.Present && m.Active && m.Revoked == nil && projectID(m) == ticket.Project
		}
		if !valid {
			ticket.State = "expired"
			if err := put(bucket(tx, "worker_queue"), ticket.ID, ticket); err != nil {
				return nil, err
			}
			continue
		}
		waiting = append(waiting, ticket)
	}
	sort.Slice(waiting, func(i, j int) bool { return waiting[i].Sequence < waiting[j].Sequence })
	return waiting, nil
}

func finishQueuedWorker(tx *bolt.Tx, id, state string) error {
	if bucket(tx, "worker_queue") == nil {
		return nil
	}
	ticket, found, err := get[workerTicket](bucket(tx, "worker_queue"), id)
	if err != nil || !found {
		return err
	}
	ticket.State = state
	return put(bucket(tx, "worker_queue"), id, ticket)
}

func ticketResult(ticket workerTicket, waiting []workerTicket) Result {
	position := 0
	for i, t := range waiting {
		if t.ID == ticket.ID {
			position = i + 1
			break
		}
	}
	status := "queue-" + ticket.State
	if ticket.State == "waiting" {
		status = "queued"
	}
	return Result{Status: status, Record: ticket.ID, State: ticket.State, Queue: &QueuePosition{Sequence: ticket.Sequence, Position: position, Deadline: ticket.Deadline, State: ticket.State}}
}

func queueWorker(tx *bolt.Tx, node, owner string, actor Member, r Request, now time.Time) (Result, error) {
	if bucket(tx, "worker_queue") == nil {
		if r.Action != "enqueue-reservation" {
			return Result{Status: "not-owned"}, nil
		}
		if _, err := tx.CreateBucket([]byte("worker_queue")); err != nil {
			return Result{}, err
		}
		if err := bucket(tx, "meta").Put([]byte("version"), []byte("4")); err != nil {
			return Result{}, err
		}
	}
	waiting, err := waitingWorkers(tx, node, r.Resource, now)
	if err != nil {
		return Result{}, err
	}
	var ticket workerTicket
	if r.Action == "enqueue-reservation" {
		// One outstanding ticket per actor and resource prevents repeated enqueue
		// IDs from filling the queue or changing the actor's original position.
		for _, t := range waiting {
			if t.Actor == actor.Ref && t.Custodian == owner {
				return ticketResult(t, waiting), nil
			}
		}
		if len(waiting) >= 1024 {
			return Result{Status: "queue-full"}, nil
		}
		if bucket(tx, "worker_queue").Sequence() == ^uint64(0) {
			return Result{Status: "queue-exhausted"}, nil
		}
		order, err := bucket(tx, "worker_queue").NextSequence()
		if err != nil {
			return Result{}, err
		}
		if order == 0 {
			return Result{Status: "queue-exhausted"}, nil
		}
		seconds := r.WaitSeconds
		if seconds == 0 {
			seconds = 900
		}
		ticket = workerTicket{ID: digest([]byte("queue\n" + owner + "\n" + r.Resource + "\n" + actor.Ref.Key() + "\n" + r.ID)), Resource: r.Resource, Actor: actor.Ref, Project: projectID(actor), Custodian: owner, Sequence: order, Deadline: now.Add(time.Duration(seconds) * time.Second), LeaseSeconds: r.LeaseSeconds, State: "waiting"}
		if err := put(bucket(tx, "worker_queue"), ticket.ID, ticket); err != nil {
			return Result{}, err
		}
		return ticketResult(ticket, append(waiting, ticket)), nil
	}
	var found bool
	ticket, found, err = get[workerTicket](bucket(tx, "worker_queue"), r.Record)
	if err != nil {
		return Result{}, err
	}
	if !found || ticket.Actor != actor.Ref || ticket.Custodian != owner || ticket.Resource != r.Resource {
		return Result{Status: "not-owned"}, nil
	}
	if r.Action == "cancel-reservation" && ticket.State == "waiting" {
		ticket.State = "cancelled"
		if err := put(bucket(tx, "worker_queue"), ticket.ID, ticket); err != nil {
			return Result{}, err
		}
		return ticketResult(ticket, nil), nil
	}
	if ticket.Project != projectID(actor) && ticket.State == "waiting" {
		return Result{Status: "project-changed"}, nil
	}
	if ticket.State == "granted" {
		held, exists, err := get[Reservation](bucket(tx, "reservations"), r.Resource)
		if err != nil {
			return Result{}, err
		}
		if !exists || held.ID != ticket.ID || !now.Before(held.Expires) {
			ticket.State = "expired"
			if err := put(bucket(tx, "worker_queue"), ticket.ID, ticket); err != nil {
				return Result{}, err
			}
		}
	}
	if ticket.State != "waiting" {
		return ticketResult(ticket, nil), nil
	}
	if r.Action == "inspect-reservation" {
		result := ticketResult(ticket, waiting)
		held, exists, err := get[Reservation](bucket(tx, "reservations"), r.Resource)
		if err != nil {
			return Result{}, err
		}
		result.Queue.Ready = result.Queue.Position == 1 && (!exists || !now.Before(held.Expires))
		return result, nil
	}
	if len(waiting) == 0 || waiting[0].ID != ticket.ID {
		return ticketResult(ticket, waiting), nil
	}
	grant := r
	grant.Action = "grant-queued"
	grant.LeaseSeconds = ticket.LeaseSeconds
	result, err := reserveWorkerAt(tx, node, owner, actor, grant, now)
	if err != nil {
		return Result{}, err
	}
	if result.Status == "busy" {
		return ticketResult(ticket, waiting), nil
	}
	if result.Status == "reserved" {
		ticket.State = "granted"
		if err := put(bucket(tx, "worker_queue"), ticket.ID, ticket); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}
