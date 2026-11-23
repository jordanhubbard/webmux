package maccoord

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const ProjectSchema = "webmux.mac.project-observation.v1"
const ProjectPageLimit = 8

// SessionProjectObservation is the same read-only DTO for browser and agent
// consumers. Retained sessions and observed MAC tasks are not implicitly joined.
type SessionProjectObservation struct {
	Schema         string               `json:"schema"`
	Subject        coordination.Ref     `json:"subject"`
	Project        ProjectObservation   `json:"project"`
	Sessions       []SessionObservation `json:"sessions"`
	HistoryState   string               `json:"standalone_history_state"`
	Source         *SourceObservation   `json:"source"`
	SourceState    string               `json:"source_state"`
	SourceTaskLink string               `json:"source_task_link_state"`
}

type ProjectTask struct {
	Task          TaskObservation    `json:"task"`
	Worker        *WorkerObservation `json:"worker"`
	DeclaredHold  *bool              `json:"declared_hold"`
	JoinPolicy    *string            `json:"reported_join_policy"`
	MetadataState string             `json:"metadata_state"`
	WorkerState   string             `json:"worker_state"`
}

type ProjectDependency struct {
	From            string `json:"from_task_id"`
	To              string `json:"to_task_id"`
	JoinPolicy      string `json:"reported_join_policy"`
	ReportedVerdict string `json:"reported_verdict"`
}

// ProjectObservation is shared read data for later human and agent consumers.
// Every task is reauthorized; raw graph nodes/edges are never returned. Numeric
// offsets select a fresh bounded observation, not a stable pagination snapshot.
type ProjectObservation struct {
	Schema               string              `json:"schema"`
	BindingRevision      string              `json:"binding_revision"`
	AuthorityID          string              `json:"authority_id"`
	ProjectKey           string              `json:"project_key"`
	ProjectExistence     string              `json:"project_existence"`
	Coverage             string              `json:"coverage"`
	Tasks                []ProjectTask       `json:"tasks"`
	Dependencies         []ProjectDependency `json:"dependencies"`
	Offset               int                 `json:"offset"`
	NextOffset           *int                `json:"next_offset"`
	ReportedTotal        int                 `json:"reported_total"`
	UpstreamOmittedTasks int                 `json:"upstream_omitted_tasks"`
	UpstreamOmittedLive  int                 `json:"upstream_omitted_live"`
	UpstreamOmittedEdges int                 `json:"upstream_omitted_edges"`
	AdapterOmittedTasks  int                 `json:"adapter_omitted_tasks"`
	UnavailableTasks     int                 `json:"unavailable_tasks"`
	UnavailableWorkers   int                 `json:"unavailable_workers"`
	WithheldDependencies int                 `json:"withheld_dependencies"`
	RoadmapLinkState     string              `json:"roadmap_link_state"`
	ConflictVerdict      string              `json:"conflict_verdict"`
	OrderingVerdict      string              `json:"ordering_verdict"`
	Freshness            string              `json:"freshness"`
	ReadAuthentication   ReadEvidence        `json:"read_authentication"`
	AuthorityTime        time.Time           `json:"authority_time"`
	ObservedAt           time.Time           `json:"observed_at"`
}

// ObserveProject allows only fixed read routes and eight task candidates per
// page. Missing/foreign tasks expose no identifiers or related-object details.
// Any observed credential refusal discards the whole result. No success is
// cached, and a final read qualification precedes release of the observation.
func (b *Bindings) ObserveProject(parent context.Context, owner string, repository *coordination.Binding, token string, roots *x509.CertPool, offset, limit int) (ProjectObservation, error) {
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return ProjectObservation{}, err
	}
	if parent == nil || !validReadToken(token) || offset < 0 || offset > 200 || limit < 1 || limit > ProjectPageLimit {
		return ProjectObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	endpoint := selected.Authority.Origin + "/dashboard/observe/projects/" + url.PathEscape(selected.Project.ProjectKey) + "/graph"
	body, err := authenticatedReadJSON(ctx, endpoint, token, roots)
	if err != nil {
		return ProjectObservation{}, err
	}
	graph, err := decodeProjectGraph(body, selected.Project.ProjectKey)
	if err != nil {
		return ProjectObservation{}, err
	}
	view := ProjectObservation{
		Schema: ProjectSchema, BindingRevision: selected.Revision, AuthorityID: selected.Authority.ID,
		ProjectKey: selected.Project.ProjectKey, ProjectExistence: "unverified", Coverage: "bounded_observation",
		Tasks: []ProjectTask{}, Dependencies: []ProjectDependency{}, Offset: offset, ReportedTotal: graph.total,
		UpstreamOmittedTasks: graph.omitted, UpstreamOmittedLive: graph.omittedLive, UpstreamOmittedEdges: graph.omittedEdges,
		RoadmapLinkState: "unlinked", ConflictVerdict: "unknown", OrderingVerdict: "unknown", Freshness: "unqualified",
		AuthorityTime: graph.clock,
	}
	start := min(offset, len(graph.nodes))
	end := min(start+limit, len(graph.nodes))
	if end < len(graph.nodes) {
		next := end
		view.NextOffset = &next
	}
	type result struct {
		row ProjectTask
		err error
	}
	results := make([]result, end-start)
	jobs := make(chan int, len(results))
	for i := range results {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for n := 0; n < min(4, len(results)); n++ {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				node := graph.nodes[start+i]
				task, err := b.ObserveTask(ctx, owner, repository, node.id, token, roots)
				if err != nil {
					results[i].err = err
					continue
				}
				row := ProjectTask{Task: task, DeclaredHold: node.hold, JoinPolicy: node.joinPolicy, MetadataState: "observed", WorkerState: "unowned"}
				if node.hold == nil || node.joinPolicy == nil {
					row.MetadataState = "unavailable"
				}
				if task.OwnerWorkerID != nil {
					joined, err := b.ObserveTaskWorker(ctx, owner, repository, node.id, token, roots)
					if errors.Is(err, ErrReadAccess) {
						results[i].err = err
						continue
					}
					row.WorkerState = "unavailable"
					if err == nil {
						row.Worker, row.Task, row.WorkerState = &joined, joined.Task, "observed"
					}
				}
				results[i].row = row
			}
		})
	}
	workers.Wait()
	for _, result := range results {
		if errors.Is(result.err, ErrReadAccess) {
			return ProjectObservation{}, ErrReadAccess
		}
	}
	if ctx.Err() != nil {
		return ProjectObservation{}, &UnavailableError{canceled: ctx.Err() == context.Canceled, timedOut: ctx.Err() == context.DeadlineExceeded}
	}
	allowed := map[string]bool{}
	for _, result := range results {
		if result.err != nil {
			view.UnavailableTasks++
			continue
		}
		view.Tasks = append(view.Tasks, result.row)
		allowed[result.row.Task.TaskID] = true
		if result.row.WorkerState == "unavailable" {
			view.UnavailableWorkers++
		}
		if result.row.MetadataState == "unavailable" {
			view.Coverage = "incomplete"
		}
	}
	view.AdapterOmittedTasks = len(graph.nodes) - len(view.Tasks)
	seen := map[[2]string]bool{}
	for _, raw := range graph.edges {
		var edge map[string]json.RawMessage
		if json.Unmarshal(raw, &edge) != nil {
			view.WithheldDependencies++
			continue
		}
		from, fromOK := recordText(edge, "from", 256)
		to, toOK := recordText(edge, "to", 256)
		join, joinOK := recordText(edge, "join_policy", 64)
		verdict, verdictOK := recordText(edge, "verdict", 64)
		key := [2]string{from, to}
		if !fromOK || !toOK || !allowed[from] || !allowed[to] || !joinOK || !verdictOK || seen[key] {
			view.WithheldDependencies++
			continue
		}
		seen[key] = true
		view.Dependencies = append(view.Dependencies, ProjectDependency{from, to, join, verdict})
	}
	if graph.truncated || view.AdapterOmittedTasks > 0 || view.UpstreamOmittedTasks > 0 || view.UpstreamOmittedLive > 0 ||
		view.UpstreamOmittedEdges > 0 || view.UnavailableTasks > 0 || view.UnavailableWorkers > 0 || view.WithheldDependencies > 0 {
		view.Coverage = "incomplete"
	}
	// Recheck actual authority authentication, not an earlier cached success.
	evidence, err := b.CheckReadAccess(ctx, owner, repository, token, roots)
	if err != nil {
		return ProjectObservation{}, err
	}
	view.ReadAuthentication = evidence
	view.ObservedAt = time.Now().UTC()
	return view, nil
}

type projectNode struct {
	id         string
	hold       *bool
	joinPolicy *string
}

type projectGraph struct {
	nodes                                     []projectNode
	edges                                     []json.RawMessage
	total, omitted, omittedLive, omittedEdges int
	truncated                                 bool
	clock                                     time.Time
}

func decodeProjectGraph(body []byte, expected string) (projectGraph, error) {
	var envelope, fields map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return projectGraph{}, ErrInvalid
	}
	schema, ok := recordText(envelope, "schema", 128)
	if !ok || schema != GraphSchema {
		return projectGraph{}, ErrUnsupported
	}
	project, ok := recordText(envelope, "project", 256)
	if !ok || project != expected {
		return projectGraph{}, ErrUnmapped
	}
	clock, ok := recordTime(envelope, "server_time")
	if !ok {
		return projectGraph{}, ErrInvalid
	}
	var degraded *[]string
	if json.Unmarshal(envelope["degraded"], &degraded) != nil || degraded == nil || len(*degraded) != 0 ||
		json.Unmarshal(envelope["graph"], &fields) != nil || fields == nil {
		return projectGraph{}, ErrUnavailable
	}
	var nodes []map[string]json.RawMessage
	var edges []json.RawMessage
	if json.Unmarshal(fields["nodes"], &nodes) != nil || nodes == nil || len(nodes) > 200 ||
		json.Unmarshal(fields["edges"], &edges) != nil || edges == nil || len(edges) > 4096 {
		return projectGraph{}, ErrInvalid
	}
	count := func(key string) (int, bool) {
		var n int
		value := fields[key]
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &n) != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	total, totalOK := count("total")
	omitted, omittedOK := count("omitted")
	omittedLive, liveOK := count("omitted_live")
	omittedEdges, edgesOK := count("omitted_edges")
	shown, shownOK := count("shown")
	var truncated *bool
	if !totalOK || !omittedOK || !liveOK || !edgesOK || !shownOK || shown != len(nodes) ||
		total < shown || omitted != total-shown || omittedLive > total || omittedEdges > len(edges) ||
		json.Unmarshal(fields["truncated"], &truncated) != nil || truncated == nil {
		return projectGraph{}, ErrInvalid
	}
	graph := projectGraph{edges: edges, total: total, omitted: omitted, omittedLive: omittedLive, omittedEdges: omittedEdges, truncated: *truncated, clock: clock}
	seen := map[string]bool{}
	for _, node := range nodes {
		id, ok := recordText(node, "id", 256)
		if !ok || !validReadID(id) || seen[id] {
			return projectGraph{}, ErrInvalid
		}
		seen[id] = true
		var hold *bool
		if json.Unmarshal(node["no_dispatch"], &hold) != nil {
			hold = nil
		}
		var join *string
		if value, ok := recordText(node, "join_policy", 64); ok {
			join = &value
		}
		graph.nodes = append(graph.nodes, projectNode{id, hold, join})
	}
	return graph, nil
}
