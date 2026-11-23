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

func workerWire() map[string]any {
	return map[string]any{
		"id": "worker-canonical-a", "machine_id": "machine-canonical-a", "name": "rocky",
		"status": "busy", "health_status": "healthy", "last_seen_at": "2026-11-23T04:59:00Z",
		"updated_at": "2026-11-23T04:59:00Z", "deleted_at": nil,
		"current_task_id": taskCanary, "resources": map[string]any{"private": taskCanary},
		"hermes_instance_id": taskCanary,
	}
}

func machineWire() map[string]any {
	return map[string]any{
		"id": "machine-canonical-a", "hostname": "puck", "trusted": true,
		"last_seen_at": "2026-11-23T04:59:00Z", "labels": map[string]any{"private": taskCanary},
	}
}

type workerRequests struct{ all, task, worker, machine atomic.Int32 }

func workerServer(t *testing.T, edit func(string, int, map[string]any)) (*httptest.Server, *workerRequests) {
	t.Helper()
	counts := &workerRequests{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts.all.Add(1)
		if r.Method != "GET" || r.URL.RawQuery != "" {
			t.Error("worker join used an unsupported operation/query")
			w.WriteHeader(500)
			return
		}
		var kind string
		var payload map[string]any
		var counter *atomic.Int32
		switch r.URL.Path {
		case "/dashboard/observe/tasks/task-a":
			kind, payload, counter = "task", taskWire(), &counts.task
		case "/agents/worker-canonical-a":
			kind, payload, counter = "worker", workerWire(), &counts.worker
		case "/machines/machine-canonical-a":
			kind, payload, counter = "machine", machineWire(), &counts.machine
		default:
			t.Error("worker join escaped its fixed authorized routes")
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		n := int(counter.Add(1))
		if edit != nil {
			edit(kind, n, payload)
		}
		_ = json.NewEncoder(w).Encode(payload)
	})), counts
}

func TestWorkerJoinPreservesCanonicalIDsAndWithholdsSharedContext(t *testing.T) {
	server, counts := workerServer(t, nil)
	defer server.Close()
	got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "alice", repository(), "task-a", accessToken, nil)
	if err != nil || got.Schema != WorkerIdentitySchema || got.WorkerID != "worker-canonical-a" || got.WorkerName != "rocky" ||
		got.MachineID != "machine-canonical-a" || got.MachineHostname != "puck" || got.Task.TaskID != "task-a" ||
		got.Task.MachineIdentityState != "observed_owner_machine" || got.Task.SessionLinkState != "unlinked" ||
		got.Freshness != "unqualified" || got.Availability != "observed" || counts.task.Load() != 2 ||
		counts.worker.Load() != 2 || counts.machine.Load() != 1 || counts.all.Load() != 15 {
		t.Fatalf("incorrect owner/machine join: %+v, %v", got, err)
	}
	encoded, _ := json.Marshal(got)
	for _, withheld := range []string{taskCanary, "current_task_id", "resources", "hermes_instance_id", accessToken, server.URL} {
		if strings.Contains(string(encoded), withheld) {
			t.Fatal("shared worker context, endpoint or credential escaped")
		}
	}
}

func TestWorkerJoinRefusesUnownedAndForeignBeforeWorkerRead(t *testing.T) {
	for _, test := range []string{"unowned", "foreign", "route injection"} {
		t.Run(test, func(t *testing.T) {
			server, counts := workerServer(t, func(kind string, n int, payload map[string]any) {
				if kind != "task" {
					return
				}
				task := payload["task"].(map[string]any)
				switch test {
				case "unowned":
					task["owner_agent_id"] = nil
				case "foreign":
					task["project"] = "beta"
				case "route injection":
					task["owner_agent_id"] = "../other"
				}
			})
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "alice", repository(), "task-a", accessToken, nil)
			if err == nil || got.Schema != "" || got.WorkerID != "" || counts.all.Load() != 3 || counts.worker.Load() != 0 || counts.machine.Load() != 0 {
				t.Fatalf("unqualified task reached a worker identity: %+v, %v", got, err)
			}
		})
	}
	server, counts := workerServer(t, nil)
	defer server.Close()
	if got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "bob", repository(), "task-a", accessToken, nil); !errors.Is(err, ErrUnmapped) || got.Schema != "" || counts.all.Load() != 0 {
		t.Fatal("foreign receiving account reached worker lookup")
	}
}

func TestWorkerJoinFencesChangedOwnershipAndMachine(t *testing.T) {
	for _, change := range []string{"owner", "attempt", "lease", "expiry", "task state", "machine", "deleted"} {
		t.Run(change, func(t *testing.T) {
			server, _ := workerServer(t, func(kind string, n int, payload map[string]any) {
				if n != 2 {
					return
				}
				if kind == "task" {
					task := payload["task"].(map[string]any)
					switch change {
					case "owner":
						task["owner_agent_id"] = "different-worker"
					case "attempt":
						task["attempt_count"] = 3
					case "lease":
						task["lease_id"] = "different-lease"
					case "expiry":
						task["leased_until"] = "2026-11-23T07:00:00Z"
					case "task state":
						task["state"] = "completed"
					}
				} else if kind == "worker" {
					switch change {
					case "machine":
						payload["machine_id"] = "different-machine"
					case "deleted":
						payload["deleted_at"] = "2026-11-23T05:00:00Z"
					}
				}
			})
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "alice", repository(), "task-a", accessToken, nil)
			if !errors.Is(err, ErrUnmapped) || got.Schema != "" || got.MachineID != "" || got.Task.Schema != "" {
				t.Fatalf("changed ownership returned a mixed observation: %+v, %v", got, err)
			}
		})
	}
}

func TestWorkerJoinRefusesMalformedAndRetainsTombstone(t *testing.T) {
	for _, change := range []string{"worker id", "machine id", "case alias", "missing tombstone", "timestamp", "null trust"} {
		t.Run(change, func(t *testing.T) {
			server, _ := workerServer(t, func(kind string, n int, payload map[string]any) {
				if kind == "worker" {
					switch change {
					case "worker id":
						payload["id"] = "different-worker"
					case "case alias":
						payload["Machine_id"] = payload["machine_id"]
						delete(payload, "machine_id")
					case "missing tombstone":
						delete(payload, "deleted_at")
					case "timestamp":
						payload["last_seen_at"] = taskCanary
					}
				} else if kind == "machine" {
					switch change {
					case "machine id":
						payload["id"] = "different-machine"
					case "null trust":
						payload["trusted"] = nil
					}
				}
			})
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "alice", repository(), "task-a", accessToken, nil)
			if err == nil || got.Schema != "" || strings.Contains(err.Error(), taskCanary) {
				t.Fatal("malformed related record became a worker identity", err)
			}
		})
	}
	server, _ := workerServer(t, func(kind string, n int, payload map[string]any) {
		if kind == "worker" {
			payload["deleted_at"] = "2026-11-23T05:00:00Z"
		}
	})
	defer server.Close()
	got, err := accessBindings(t, server.URL).ObserveTaskWorker(t.Context(), "alice", repository(), "task-a", accessToken, nil)
	if err != nil || got.WorkerDeletedAt == nil || got.Availability != "deleted" || got.Freshness != "unqualified" || got.Task.SessionLinkState != "unlinked" {
		t.Fatal("historical deleted owner became active or lost its identity", err)
	}
}
