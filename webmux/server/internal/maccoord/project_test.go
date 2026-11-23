package maccoord

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func projectWire() map[string]any {
	nodes := []any{}
	for _, id := range []string{"task-a", "task-b", "foreign-task-private"} {
		nodes = append(nodes, map[string]any{"id": id, "title": taskCanary, "priority": 999,
			"no_dispatch": false, "join_policy": "all_success"})
	}
	edge := func(from, to string) any {
		return map[string]any{"from": from, "to": to, "join_policy": "all_success", "verdict": "waiting",
			"from_title": taskCanary, "from_project": taskCanary, "from_state": taskCanary}
	}
	return map[string]any{
		"schema": GraphSchema, "project": "alpha", "found": true, "server_time": "2026-11-23T05:00:00Z", "degraded": []any{},
		"graph": map[string]any{"nodes": nodes, "edges": []any{edge("task-b", "task-a"), edge("foreign-task-private", "task-a"), edge("outside-private", "task-a"), edge("task-b", "task-a")},
			"total": 3, "shown": 3, "omitted": 0, "omitted_live": 0, "omitted_edges": 1, "truncated": false},
	}
}

type projectFixture struct {
	editGraph        func(map[string]any)
	editTask         func(string, map[string]any)
	taskDown         bool
	revokeAfterTask  bool
	calls, taskCalls atomic.Int32
	revoked          atomic.Bool
}

func projectServer(t *testing.T, fixture *projectFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.calls.Add(1)
		if r.Method != "GET" || r.URL.RawQuery != "" {
			t.Error("project reader escaped its fixed GET routes")
		}
		if r.Header.Get("Authorization") != "Bearer "+accessToken || fixture.revoked.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/dashboard/observe/projects/alpha/graph" {
			payload := projectWire()
			if fixture.editGraph != nil {
				fixture.editGraph(payload)
			}
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/dashboard/observe/tasks/") {
			id := strings.TrimPrefix(r.URL.Path, "/dashboard/observe/tasks/")
			fixture.taskCalls.Add(1)
			if fixture.taskDown {
				w.WriteHeader(503)
				return
			}
			payload := taskWire()
			task := payload["task"].(map[string]any)
			payload["task_id"], task["id"], task["title"] = id, id, "Allowed "+id
			task["owner_agent_id"], task["lease_id"], task["leased_until"] = nil, nil, nil
			if id == "foreign-task-private" {
				task["project"], task["title"] = "beta", taskCanary
			}
			if fixture.editTask != nil {
				fixture.editTask(id, task)
			}
			_ = json.NewEncoder(w).Encode(payload)
			if fixture.revokeAfterTask {
				fixture.revoked.Store(true)
			}
			return
		}
		t.Error("project reader made an arbitrary related-object lookup")
		w.WriteHeader(404)
	}))
}

func TestProjectProjectionWithholdsForeignRowsAndRelatedDetails(t *testing.T) {
	fixture := &projectFixture{}
	server := projectServer(t, fixture)
	defer server.Close()
	got, err := accessBindings(t, server.URL).ObserveProject(t.Context(), "alice", repository(), accessToken, nil, 0, 8)
	if err != nil || got.Schema != ProjectSchema || got.ProjectKey != "alpha" || len(got.Tasks) != 2 ||
		got.UnavailableTasks != 1 || got.AdapterOmittedTasks != 1 || got.WithheldDependencies != 3 ||
		len(got.Dependencies) != 1 || got.Dependencies[0].From != "task-b" || got.Dependencies[0].To != "task-a" ||
		got.Coverage != "incomplete" || got.ConflictVerdict != "unknown" || got.OrderingVerdict != "unknown" ||
		got.RoadmapLinkState != "unlinked" || got.ProjectExistence != "unverified" || got.Freshness != "unqualified" ||
		got.ReadAuthentication.PrincipalIdentity != "unverified" || !got.ReadAuthentication.InvalidCredentialsRefused {
		t.Fatalf("incorrect project projection: %+v, %v", got, err)
	}
	for _, row := range got.Tasks {
		if row.Task.Title != "Allowed "+row.Task.TaskID || row.Task.Priority != 10 || row.Worker != nil || row.WorkerState != "unowned" {
			t.Fatal("graph metadata bypassed independently scoped task observation")
		}
	}
	encoded, _ := json.Marshal(got)
	for _, withheld := range []string{taskCanary, "foreign-task-private", "outside-private", server.URL, accessToken, "from_title", "from_project"} {
		if strings.Contains(string(encoded), withheld) {
			t.Fatal("foreign task/dependency content, endpoint or credential escaped")
		}
	}
}

func TestProjectProjectionBoundsFreshPagesAndRefusesForeignAccount(t *testing.T) {
	fixture := &projectFixture{}
	server := projectServer(t, fixture)
	defer server.Close()
	bindings := accessBindings(t, server.URL)
	if got, err := bindings.ObserveProject(t.Context(), "bob", repository(), accessToken, nil, 0, 1); !errors.Is(err, ErrUnmapped) || got.Schema != "" || fixture.calls.Load() != 0 {
		t.Fatal("foreign receiving account reached project observation")
	}
	for _, page := range [][2]int{{-1, 1}, {201, 1}, {0, 0}, {0, 9}} {
		if got, err := bindings.ObserveProject(t.Context(), "alice", repository(), accessToken, nil, page[0], page[1]); !errors.Is(err, ErrInvalid) || got.Schema != "" || fixture.calls.Load() != 0 {
			t.Fatal("invalid page crossed the pre-request boundary")
		}
	}
	first, err := bindings.ObserveProject(t.Context(), "alice", repository(), accessToken, nil, 0, 1)
	if err != nil || len(first.Tasks) != 1 || first.Tasks[0].Task.TaskID != "task-a" || first.NextOffset == nil || *first.NextOffset != 1 || first.AdapterOmittedTasks != 2 || first.Coverage != "incomplete" {
		t.Fatal("first page lost its explicit bounds", err)
	}
	second, err := bindings.ObserveProject(t.Context(), "alice", repository(), accessToken, nil, *first.NextOffset, 1)
	if err != nil || len(second.Tasks) != 1 || second.Tasks[0].Task.TaskID != "task-b" || fixture.taskCalls.Load() != 2 {
		t.Fatal("page selected arbitrary tasks or reused a cached response", err)
	}
}

func TestProjectProjectionDistinguishesEmptyUnavailableAndRevoked(t *testing.T) {
	for _, test := range []string{"empty", "unavailable tasks", "revoked after task", "missing graph", "degraded graph"} {
		t.Run(test, func(t *testing.T) {
			fixture := &projectFixture{taskDown: test == "unavailable tasks", revokeAfterTask: test == "revoked after task"}
			if test == "empty" || test == "missing graph" || test == "degraded graph" {
				fixture.editGraph = func(payload map[string]any) {
					switch test {
					case "empty":
						payload["graph"] = map[string]any{"nodes": []any{}, "edges": []any{}, "total": 0, "shown": 0, "omitted": 0, "omitted_live": 0, "omitted_edges": 0, "truncated": false}
					case "missing graph":
						delete(payload, "graph")
					case "degraded graph":
						payload["degraded"] = []any{taskCanary}
					}
				}
			}
			server := projectServer(t, fixture)
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveProject(t.Context(), "alice", repository(), accessToken, nil, 0, 1)
			switch test {
			case "empty":
				if err != nil || len(got.Tasks) != 0 || got.ProjectExistence != "unverified" || got.ConflictVerdict != "unknown" || got.Coverage != "bounded_observation" {
					t.Fatal("empty observation became an existence/exclusivity verdict", err)
				}
			case "unavailable tasks":
				if err != nil || len(got.Tasks) != 0 || got.UnavailableTasks != 1 || got.Coverage != "incomplete" {
					t.Fatal("unavailable task became successful empty coverage", err)
				}
			case "revoked after task":
				if !errors.Is(err, ErrReadAccess) || got.Schema != "" || got.Tasks != nil {
					t.Fatal("credential revocation released prior partial observations", err)
				}
			default:
				if !errors.Is(err, ErrUnavailable) || got.Schema != "" || fixture.taskCalls.Load() != 0 || strings.Contains(err.Error(), taskCanary) {
					t.Fatal("degraded graph became an empty task set", err)
				}
			}
		})
	}
	fixture := &projectFixture{}
	server := projectServer(t, fixture)
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := accessBindings(t, server.URL).ObserveProject(ctx, "alice", repository(), accessToken, nil, 0, 1); !errors.Is(err, context.Canceled) || got.Schema != "" {
		t.Fatal("project observation lost cancellation or returned partial rows", err)
	}
}

func TestProjectProjectionRejectsAmbiguousGraph(t *testing.T) {
	for _, change := range []string{"duplicate node", "unsafe id", "null nodes", "missing edges", "null count", "shown mismatch", "wrong project", "wrong schema"} {
		t.Run(change, func(t *testing.T) {
			fixture := &projectFixture{editGraph: func(payload map[string]any) {
				graph := payload["graph"].(map[string]any)
				nodes := graph["nodes"].([]any)
				switch change {
				case "duplicate node":
					nodes[1] = nodes[0]
				case "unsafe id":
					nodes[0].(map[string]any)["id"] = "../private"
				case "null nodes":
					graph["nodes"] = nil
				case "missing edges":
					delete(graph, "edges")
				case "null count":
					graph["omitted"] = nil
				case "shown mismatch":
					graph["shown"] = 0
				case "wrong project":
					payload["project"] = taskCanary
				case "wrong schema":
					payload["schema"] = "mac.project.v999"
				}
			}}
			server := projectServer(t, fixture)
			defer server.Close()
			got, err := accessBindings(t, server.URL).ObserveProject(t.Context(), "alice", repository(), accessToken, nil, 0, 1)
			if err == nil || got.Schema != "" || fixture.taskCalls.Load() != 0 || strings.Contains(err.Error(), taskCanary) {
				t.Fatal("ambiguous graph reached task reads or exposed private data", err)
			}
		})
	}
}
