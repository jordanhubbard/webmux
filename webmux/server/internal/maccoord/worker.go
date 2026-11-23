package maccoord

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"net/url"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const WorkerIdentitySchema = "webmux.mac.worker-identity.v1"

// WorkerObservation joins only the owner of an authorized project task. A
// shared worker's other task, capabilities, resources and persona context are
// never projected. Names and hostnames remain labels alongside canonical IDs.
type WorkerObservation struct {
	Schema          string          `json:"schema"`
	Task            TaskObservation `json:"task"`
	WorkerID        string          `json:"worker_id"`
	WorkerName      string          `json:"worker_name"`
	MachineID       string          `json:"machine_id"`
	MachineHostname string          `json:"machine_hostname"`
	WorkerStatus    string          `json:"worker_status"`
	HealthStatus    string          `json:"health_status"`
	WorkerLastSeen  time.Time       `json:"worker_last_seen"`
	WorkerUpdatedAt time.Time       `json:"worker_updated_at"`
	WorkerDeletedAt *time.Time      `json:"worker_deleted_at"`
	MachineLastSeen time.Time       `json:"machine_last_seen"`
	MachineTrusted  bool            `json:"machine_trusted"`
	Availability    string          `json:"availability"`
	Freshness       string          `json:"freshness"`
	ObservedAt      time.Time       `json:"observed_at"`
}

// ObserveTaskWorker performs no arbitrary worker lookup. The selected task must
// first belong to the receiving project's binding and identify its owner. The
// owner/machine association and task owner/attempt/lease are rechecked around the
// join. This is bounded metadata sampling, not an atomic snapshot or lease check.
func (b *Bindings) ObserveTaskWorker(parent context.Context, owner string, repository *coordination.Binding, taskID, token string, roots *x509.CertPool) (WorkerObservation, error) {
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return WorkerObservation{}, err
	}
	if parent == nil || !validReadToken(token) {
		return WorkerObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	task, err := b.ObserveTask(ctx, owner, repository, taskID, token, roots)
	if err != nil {
		return WorkerObservation{}, err
	}
	if task.OwnerWorkerID == nil {
		return WorkerObservation{}, ErrUnmapped
	}
	workerID := *task.OwnerWorkerID
	if !validReadID(workerID) {
		return WorkerObservation{}, ErrInvalid
	}
	workerRoute := selected.Authority.Origin + "/agents/" + url.PathEscape(workerID)
	readWorker := func() (workerRecord, error) {
		body, err := authenticatedReadJSON(ctx, workerRoute, token, roots)
		if err != nil {
			return workerRecord{}, err
		}
		return decodeWorker(body, workerID)
	}
	worker, err := readWorker()
	if err != nil {
		return WorkerObservation{}, err
	}
	body, err := authenticatedReadJSON(ctx, selected.Authority.Origin+"/machines/"+url.PathEscape(worker.machine), token, roots)
	if err != nil {
		return WorkerObservation{}, err
	}
	machine, err := decodeMachine(body, worker.machine)
	if err != nil {
		return WorkerObservation{}, err
	}
	afterWorker, err := readWorker()
	if err != nil {
		return WorkerObservation{}, err
	}
	afterTask, err := b.ObserveTask(ctx, owner, repository, taskID, token, roots)
	if err != nil {
		return WorkerObservation{}, err
	}
	if afterWorker.machine != worker.machine || !sameTime(afterWorker.deleted, worker.deleted) ||
		!sameString(afterTask.OwnerWorkerID, task.OwnerWorkerID) || afterTask.AttemptCount != task.AttemptCount ||
		!sameString(afterTask.LeaseID, task.LeaseID) || !sameTime(afterTask.LeasedUntil, task.LeasedUntil) || afterTask.State != task.State {
		return WorkerObservation{}, ErrUnmapped
	}
	if ctx.Err() != nil {
		return WorkerObservation{}, &UnavailableError{canceled: ctx.Err() == context.Canceled, timedOut: ctx.Err() == context.DeadlineExceeded}
	}
	availability := "observed"
	if afterWorker.deleted != nil {
		availability = "deleted"
	}
	afterTask.MachineIdentityState = "observed_owner_machine"
	return WorkerObservation{
		Schema: WorkerIdentitySchema, Task: afterTask, WorkerID: workerID, WorkerName: afterWorker.name,
		MachineID: worker.machine, MachineHostname: machine.hostname, WorkerStatus: afterWorker.status,
		HealthStatus: afterWorker.health, WorkerLastSeen: afterWorker.seen, WorkerUpdatedAt: afterWorker.updated,
		WorkerDeletedAt: afterWorker.deleted, MachineLastSeen: machine.seen, MachineTrusted: machine.trusted,
		Availability: availability, Freshness: "unqualified", ObservedAt: time.Now().UTC(),
	}, nil
}

type workerRecord struct {
	machine, name, status, health string
	seen, updated                 time.Time
	deleted                       *time.Time
}

type machineRecord struct {
	hostname string
	seen     time.Time
	trusted  bool
}

// MAC's Agent/Machine records are unversioned. The adapter requires these exact
// audited fields, ignores unrelated fields and returns its own versioned schema.
func decodeWorker(body []byte, expected string) (workerRecord, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return workerRecord{}, ErrInvalid
	}
	id, ok := recordText(fields, "id", 256)
	if !ok || id != expected {
		return workerRecord{}, ErrUnmapped
	}
	machine, machineOK := recordText(fields, "machine_id", 256)
	name, nameOK := recordText(fields, "name", 256)
	status, statusOK := recordText(fields, "status", 128)
	health, healthOK := recordText(fields, "health_status", 128)
	seen, seenOK := recordTime(fields, "last_seen_at")
	updated, updatedOK := recordTime(fields, "updated_at")
	if !machineOK || !validReadID(machine) || !nameOK || !statusOK || !healthOK || !seenOK || !updatedOK {
		return workerRecord{}, ErrInvalid
	}
	var deleted *time.Time
	raw, exists := fields["deleted_at"]
	if !exists {
		return workerRecord{}, ErrInvalid
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value, ok := recordTime(fields, "deleted_at")
		if !ok {
			return workerRecord{}, ErrInvalid
		}
		deleted = &value
	}
	return workerRecord{machine, name, status, health, seen, updated, deleted}, nil
}

func decodeMachine(body []byte, expected string) (machineRecord, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return machineRecord{}, ErrInvalid
	}
	id, ok := recordText(fields, "id", 256)
	if !ok || id != expected {
		return machineRecord{}, ErrUnmapped
	}
	hostname, hostnameOK := recordText(fields, "hostname", 1024)
	seen, seenOK := recordTime(fields, "last_seen_at")
	var trusted *bool
	if !hostnameOK || !seenOK || json.Unmarshal(fields["trusted"], &trusted) != nil || trusted == nil {
		return machineRecord{}, ErrInvalid
	}
	return machineRecord{hostname, seen, *trusted}, nil
}

func recordText(fields map[string]json.RawMessage, key string, limit int) (string, bool) {
	var value string
	if json.Unmarshal(fields[key], &value) != nil || !validText(value, limit) {
		return "", false
	}
	return value, true
}

func recordTime(fields map[string]json.RawMessage, key string) (time.Time, bool) {
	text, ok := recordText(fields, key, 128)
	if !ok {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	return value, err == nil && !value.IsZero()
}

func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func sameTime(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}
