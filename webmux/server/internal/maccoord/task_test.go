package maccoord

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const taskCanary = "private-transcript-command-or-foreign-project-canary"

func taskWire() map[string]any {
	return map[string]any{
		"schema": TaskSchema, "task_id": "task-a", "found": true, "server_time": "2026-11-23T05:00:00Z",
		"task": map[string]any{
			"id": "task-a", "project": "alpha", "state": "running", "owner_agent_id": "worker-canonical-a",
			"attempt_count": 2, "lease_id": "lease-canonical-a", "leased_until": "2026-11-23T06:00:00Z",
			"updated_at": "2026-11-23T04:59:00Z", "description": taskCanary,
		},
		"commands": []any{map[string]any{"argv": taskCanary, "cwd": taskCanary}},
		"history":  []any{map[string]any{"detail": taskCanary}},
	}
}

func taskHandler(t *testing.T, payload map[string]any, allowed *atomic.Bool, requests *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path != "/dashboard/observe/tasks/task-a" || r.URL.RawQuery != "" {
			t.Error("task observation used an unqualified operation or route")
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+accessToken || !allowed.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func TestTaskObservationKeepsIdentitiesSeparateAndRechecksRevocation(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	var requests atomic.Int32
	server := httptest.NewServer(taskHandler(t, taskWire(), &allowed, &requests))
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	got, err := bindings.ObserveTask(t.Context(), "alice", repository(), "task-a", accessToken, nil)
	if err != nil || got.Schema != TaskIdentitySchema || got.AuthorityID != "fleet-a" || got.ProjectKey != "alpha" ||
		got.TaskID != "task-a" || got.AttemptCount != 2 || got.OwnerWorkerID == nil || *got.OwnerWorkerID != "worker-canonical-a" ||
		got.LeaseID == nil || *got.LeaseID != "lease-canonical-a" || got.LeasedUntil == nil ||
		got.LeaseObservation != "observed_unexpired" || got.SessionLinkState != "unlinked" ||
		got.MachineIdentityState != "unavailable" || got.SourceLinkState != "unlinked" ||
		got.BindingRevision == "" || got.ObservedAt.IsZero() || requests.Load() != 3 {
		t.Fatalf("incorrect task identity projection: %+v, %v", got, err)
	}
	encoded, _ := json.Marshal(got)
	for _, secret := range []string{taskCanary, accessToken, server.URL, "argv", "description"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("unselected task content, endpoint or credential escaped")
		}
	}
	allowed.Store(false)
	if got, err := bindings.ObserveTask(t.Context(), "alice", repository(), "task-a", accessToken, nil); !errors.Is(err, ErrReadAccess) || got.Schema != "" {
		t.Fatal("revocation reused a successful task observation", err)
	}
}

func TestTaskObservationScopeAndNegativeControls(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(taskWire()) // Deliberately unauthenticated development mode.
	}))
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	if got, err := bindings.ObserveTask(t.Context(), "bob", repository(), "task-a", accessToken, nil); !errors.Is(err, ErrUnmapped) || got.Schema != "" || requests.Load() != 0 {
		t.Fatal("foreign account reached task observation")
	}
	for _, id := range []string{"", ".", "..", "../agents", "task-a?expand=all", "task%2fa", "task\\a"} {
		if got, err := bindings.ObserveTask(t.Context(), "alice", repository(), id, accessToken, nil); !errors.Is(err, ErrInvalid) || got.Schema != "" || requests.Load() != 0 {
			t.Fatal("invalid task identifier reached the authority", err)
		}
	}
	if got, err := bindings.ObserveTask(nil, "alice", repository(), "task-a", accessToken, nil); !errors.Is(err, ErrInvalid) || got.Schema != "" || requests.Load() != 0 {
		t.Fatal("missing context reached task observation")
	}
	if got, err := bindings.ObserveTask(t.Context(), "alice", repository(), "task-a", accessToken, nil); !errors.Is(err, ErrReadAccess) || got.Schema != "" || requests.Load() != 1 {
		t.Fatal("anonymous task endpoint was considered authenticated")
	}
}

func TestTaskObservationMissingForeignAndMalformedWire(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(map[string]any, map[string]any)
		want error
	}{
		{"missing", func(e, task map[string]any) { e["found"] = false; delete(e, "task") }, ErrUnmapped},
		{"foreign", func(e, task map[string]any) { task["project"] = taskCanary }, ErrUnmapped},
		{"wrong task", func(e, task map[string]any) { task["id"] = taskCanary }, ErrUnmapped},
		{"unsupported schema", func(e, task map[string]any) { e["schema"] = "mac.dashboard.observe.task.v999" }, ErrUnsupported},
		{"case alias", func(e, task map[string]any) {
			task["Owner_agent_id"] = task["owner_agent_id"]
			delete(task, "owner_agent_id")
		}, ErrInvalid},
		{"missing owner", func(e, task map[string]any) { delete(task, "owner_agent_id") }, ErrInvalid},
		{"missing lease", func(e, task map[string]any) { delete(task, "lease_id") }, ErrInvalid},
		{"missing expiry", func(e, task map[string]any) { delete(task, "leased_until") }, ErrInvalid},
		{"null counter", func(e, task map[string]any) { task["attempt_count"] = nil }, ErrInvalid},
		{"fractional counter", func(e, task map[string]any) { task["attempt_count"] = 1.5 }, ErrInvalid},
		{"negative counter", func(e, task map[string]any) { task["attempt_count"] = -1 }, ErrInvalid},
		{"missing clock", func(e, task map[string]any) { delete(e, "server_time") }, ErrInvalid},
		{"invalid timestamp", func(e, task map[string]any) { task["leased_until"] = taskCanary }, ErrInvalid},
		{"null found", func(e, task map[string]any) { e["found"] = nil }, ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := taskWire()
			test.edit(payload, payload["task"].(map[string]any))
			var allowed atomic.Bool
			allowed.Store(true)
			var requests atomic.Int32
			server := httptest.NewServer(taskHandler(t, payload, &allowed, &requests))
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveTask(t.Context(), "alice", repository(), "task-a", accessToken, nil)
			if !errors.Is(err, test.want) || got.Schema != "" || got.TaskID != "" || strings.Contains(err.Error(), taskCanary) {
				t.Fatalf("malformed/foreign task released a partial identity: %+v, %v", got, err)
			}
		})
	}
}

func TestTaskObservationLeaseStates(t *testing.T) {
	for _, state := range []string{"unleased", "incomplete", "observed_expired"} {
		t.Run(state, func(t *testing.T) {
			payload := taskWire()
			task := payload["task"].(map[string]any)
			switch state {
			case "unleased":
				task["lease_id"], task["leased_until"] = nil, nil
			case "incomplete":
				task["owner_agent_id"] = nil
			case "observed_expired":
				task["leased_until"] = payload["server_time"]
			}
			var allowed atomic.Bool
			allowed.Store(true)
			var requests atomic.Int32
			server := httptest.NewServer(taskHandler(t, payload, &allowed, &requests))
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveTask(t.Context(), "alice", repository(), "task-a", accessToken, nil)
			if err != nil || got.LeaseObservation != state || got.SessionLinkState != "unlinked" {
				t.Fatalf("lease observation changed missing state into authority: %+v, %v", got, err)
			}
		})
	}
}
