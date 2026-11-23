package maccoord

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const TaskSchema = "mac.dashboard.observe.task.v1"
const TaskIdentitySchema = "webmux.mac.task-identity.v1"

// TaskObservation retains separate task, owner, attempt counter and lease
// identities. Neither the counter nor the owner is a WebMux session identity.
// A lease timestamp observation is not an admission or publication grant.
type TaskObservation struct {
	Schema               string     `json:"schema"`
	BindingRevision      string     `json:"binding_revision"`
	AuthorityID          string     `json:"authority_id"`
	ProjectKey           string     `json:"project_key"`
	TaskID               string     `json:"task_id"`
	Title                string     `json:"title"`
	Priority             int        `json:"priority"`
	State                string     `json:"state"`
	OwnerWorkerID        *string    `json:"owner_worker_id"`
	AttemptCount         int        `json:"attempt_count"`
	LeaseID              *string    `json:"lease_id"`
	LeasedUntil          *time.Time `json:"leased_until"`
	LeaseObservation     string     `json:"lease_observation"`
	SessionLinkState     string     `json:"session_link_state"`
	MachineIdentityState string     `json:"machine_identity_state"`
	SourceLinkState      string     `json:"source_link_state"`
	AuthorityTime        time.Time  `json:"authority_time"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ObservedAt           time.Time  `json:"observed_at"`
}

// ObserveTask reads only MAC's versioned read-only observability route. Ordinary
// task-detail GETs can rebuild task-flow records in the inspected MAC version.
// This is trusted server plumbing, not a browser-selectable API proxy. Missing
// and foreign-project tasks both return ErrUnmapped, with no partial metadata.
func (b *Bindings) ObserveTask(parent context.Context, owner string, repository *coordination.Binding, taskID, token string, roots *x509.CertPool) (TaskObservation, error) {
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return TaskObservation{}, err
	}
	if parent == nil || !validReadToken(token) || !validReadID(taskID) {
		return TaskObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	endpoint := selected.Authority.Origin + "/dashboard/observe/tasks/" + url.PathEscape(taskID)
	body, err := authenticatedReadJSON(ctx, endpoint, token, roots)
	if err != nil {
		return TaskObservation{}, err
	}
	observation, err := projectTask(body, selected, taskID)
	if err != nil {
		return TaskObservation{}, err
	}
	if ctx.Err() != nil {
		return TaskObservation{}, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
	return observation, nil
}

func projectTask(body []byte, selected Selection, taskID string) (TaskObservation, error) {
	var envelope, task map[string]json.RawMessage
	var schema, returnedID string
	var found bool
	if json.Unmarshal(body, &envelope) != nil ||
		json.Unmarshal(envelope["schema"], &schema) != nil ||
		json.Unmarshal(envelope["task_id"], &returnedID) != nil || returnedID != taskID ||
		bytes.Equal(bytes.TrimSpace(envelope["found"]), []byte("null")) || json.Unmarshal(envelope["found"], &found) != nil {
		return TaskObservation{}, ErrInvalid
	}
	if schema != TaskSchema {
		return TaskObservation{}, ErrUnsupported
	}
	if !found {
		return TaskObservation{}, ErrUnmapped
	}
	if json.Unmarshal(envelope["task"], &task) != nil || task == nil {
		return TaskObservation{}, ErrInvalid
	}
	var id, project, state, title, updatedAt, authorityTime string
	var priority int
	var attempt int
	if json.Unmarshal(task["id"], &id) != nil || json.Unmarshal(task["project"], &project) != nil ||
		id != taskID || project != selected.Project.ProjectKey {
		return TaskObservation{}, ErrUnmapped
	}
	if json.Unmarshal(task["title"], &title) != nil || !validText(title, 4096) ||
		bytes.Equal(bytes.TrimSpace(task["priority"]), []byte("null")) || json.Unmarshal(task["priority"], &priority) != nil ||
		json.Unmarshal(task["state"], &state) != nil || !validText(state, 128) ||
		bytes.Equal(bytes.TrimSpace(task["attempt_count"]), []byte("null")) || json.Unmarshal(task["attempt_count"], &attempt) != nil || attempt < 0 ||
		json.Unmarshal(task["updated_at"], &updatedAt) != nil ||
		json.Unmarshal(envelope["server_time"], &authorityTime) != nil {
		return TaskObservation{}, ErrInvalid
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil || updated.IsZero() {
		return TaskObservation{}, ErrInvalid
	}
	clock, err := time.Parse(time.RFC3339Nano, authorityTime)
	if err != nil || clock.IsZero() {
		return TaskObservation{}, ErrInvalid
	}
	// Explicit null is different from a missing/unsupported wire field. Do not
	// let an omitted owner/lease field become a successful "unleased" assertion.
	nullableID := func(key string) (*string, bool) {
		raw, exists := task[key]
		if !exists {
			return nil, false
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, true
		}
		var value string
		if json.Unmarshal(raw, &value) != nil || !validText(value, 256) {
			return nil, false
		}
		return &value, true
	}
	worker, workerOK := nullableID("owner_agent_id")
	lease, leaseOK := nullableID("lease_id")
	until, untilOK := nullableID("leased_until")
	if !workerOK || !leaseOK || !untilOK {
		return TaskObservation{}, ErrInvalid
	}
	var expires *time.Time
	if until != nil {
		value, err := time.Parse(time.RFC3339Nano, *until)
		if err != nil {
			return TaskObservation{}, ErrInvalid
		}
		expires = &value
	}
	leaseState := "incomplete"
	if lease == nil && expires == nil {
		leaseState = "unleased"
	} else if lease != nil && expires != nil && worker != nil {
		leaseState = "observed_expired"
		if expires.After(clock) {
			leaseState = "observed_unexpired"
		}
	}
	return TaskObservation{
		Schema: TaskIdentitySchema, BindingRevision: selected.Revision, AuthorityID: selected.Authority.ID,
		ProjectKey: project, TaskID: id, Title: title, Priority: priority, State: state, OwnerWorkerID: worker, AttemptCount: attempt,
		LeaseID: lease, LeasedUntil: expires, LeaseObservation: leaseState,
		SessionLinkState: "unlinked", MachineIdentityState: "unavailable", SourceLinkState: "unlinked",
		AuthorityTime: clock, UpdatedAt: updated, ObservedAt: time.Now().UTC(),
	}, nil
}

func validReadID(value string) bool {
	return validText(value, 256) && !strings.ContainsAny(value, "/\\?#%") && value != "." && value != ".."
}
