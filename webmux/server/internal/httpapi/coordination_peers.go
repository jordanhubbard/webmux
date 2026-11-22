package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

const coordinationSnapshotPath = "/api/federation/coordination/snapshot"
const coordinationPeerAge = 30 * time.Second

type coordinationPeerCache struct {
	barriers map[string]time.Time // A late response cannot resurrect data after refusal.
	sizes    map[string]int
	total    int
	mu       sync.RWMutex
	values   map[string]plane.PeerView
	cursor   int // Fair rotation; protected by mu.
}
type coordinationPeerJob struct {
	key, project string
	peer         serverPeer
	token        string
}

func coordinationPeerKey(peer serverPeer, token, project string) string {
	// Changing any authorization/endpoint input invalidates cached observations,
	// including a replaced token_env value. Keys never contain a credential.
	data, _ := json.Marshal(struct {
		Peer                serverPeer
		URL, Token, Project string
	}{peer, peer.URL, token, project})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *Server) coordinationPeerJobs(inputs []plane.MemberInput) ([]coordinationPeerJob, error) {
	directory, err := s.serverDirectory()
	if err != nil {
		return nil, err
	}
	jobs := []coordinationPeerJob{}
	seen := map[string]bool{}
	for _, peer := range directory.Servers {
		if peer.CoordinationOwner == "" || !s.federationOwnerAllowed(peer.CoordinationOwner) || !validNodePin(peer.Identity) {
			continue
		}
		for _, input := range inputs {
			if !input.Active || input.Ref.Owner != peer.CoordinationOwner || input.Project == nil {
				continue
			}
			token := serverCredential(peer.TokenEnv, peer.Credential)
			key := coordinationPeerKey(peer, token, input.Project.ID)
			if !seen[key] {
				jobs = append(jobs, coordinationPeerJob{key, input.Project.ID, peer, token})
				seen[key] = true
			}
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].key < jobs[j].key })
	return jobs, nil
}
func (s *Server) exportCoordinationSnapshot(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	grant, ok := r.Context().Value(federationContextKey{}).(serverGrant)
	if !ok || !grant.Coordination {
		writeError(w, 403, "This incoming grant does not permit coordination project summaries")
		return
	}
	if r.TLS == nil {
		writeError(w, 403, "Coordination project summaries require end-to-end HTTPS")
		return
	}
	var selector struct {
		Project string `json:"project_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if !decodeJSON(w, r, &selector) {
		return
	}
	if !plane.ValidProjectSelector(selector.Project) {
		writeError(w, 400, "A verified project selector is required")
		return
	}
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	own := []session.Session{}
	for _, snapshot := range snapshots {
		if snapshot.Owner == owner {
			own = append(own, snapshot)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	s.refreshCoordinationBindings(ctx, own)
	cancel()
	node, err := s.coordinationNode()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	var view plane.View
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		view, e = s.sessionCoord.service.OwnerView(node, owner, s.coordinationInputs(node, all))
		return e
	})
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	// No network/peer aggregation occurs here: cyclic node connections cannot
	// recurse, and grants never transitively share another node's data.
	writeJSON(w, 200, plane.Snapshot(view, node, owner, selector.Project))
}
func (s *Server) fetchCoordinationSnapshot(ctx context.Context, job coordinationPeerJob) (plane.ProjectSnapshot, string) {
	var snapshot plane.ProjectSnapshot
	if !validSavedCredential(job.token) {
		return snapshot, "unauthorized"
	}
	target, err := url.Parse(job.peer.URL)
	if err != nil || target.Scheme != "https" {
		return snapshot, "unavailable"
	}
	target.Path = coordinationSnapshotPath
	target.RawPath = ""
	target.RawQuery = ""
	data, _ := json.Marshal(map[string]string{"project_id": job.project})
	request, err := http.NewRequestWithContext(ctx, "POST", target.String(), bytes.NewReader(data))
	if err != nil {
		return snapshot, "unavailable"
	}
	request.Header = nodeScopeHeaders(job.peer, job.token)
	request.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = false
	transport.DialTLSContext = s.nodePeerDialer(job.peer, job.token)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("coordination redirect refused") }}
	response, err := client.Do(request)
	if err != nil {
		return snapshot, "unavailable"
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return snapshot, "unauthorized"
	}
	if response.StatusCode != 200 {
		return snapshot, "unavailable"
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, &snapshot) != nil || !snapshot.Valid(job.peer.Identity.NodeID, job.peer.Identity.Owner, job.project) {
		return plane.ProjectSnapshot{}, "invalid"
	}
	if !snapshot.Enabled {
		return snapshot, "disabled"
	}
	return snapshot, "current"
}
func (s *Server) refreshCoordinationPeers(parent context.Context, inputs []plane.MemberInput) error {
	jobs, err := s.coordinationPeerJobs(inputs)
	if err != nil {
		return err
	}
	cache := &s.sessionCoord.peers
	cache.mu.Lock()
	if cache.values == nil {
		cache.values = map[string]plane.PeerView{}
		cache.barriers = map[string]time.Time{}
		cache.sizes = map[string]int{}
	}
	keep := map[string]bool{}
	for _, job := range jobs {
		keep[job.key] = true
	}
	for key := range cache.values {
		if !keep[key] {
			cache.total -= cache.sizes[key]
			delete(cache.sizes, key)
			delete(cache.barriers, key)
			delete(cache.values, key)
		}
	}
	cache.mu.Unlock()
	if len(jobs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	// A bounded batch and rotating cursor keep unavailable nodes from starving
	// other projects. Neither work nor network runs under the broker/ledger lock.
	cache.mu.Lock()
	offset := cache.cursor % len(jobs)
	cache.mu.Unlock()
	queue := make(chan coordinationPeerJob, len(jobs))
	for i := 0; i < len(jobs); i++ {
		queue <- jobs[(offset+i)%len(jobs)]
	}
	close(queue)
	var workers sync.WaitGroup
	var attempted atomic.Int64
	for i := 0; i < min(4, len(jobs)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range queue {
				if ctx.Err() != nil {
					return
				}
				attempted.Add(1)
				started := time.Now()
				requestCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				snapshot, state := s.fetchCoordinationSnapshot(requestCtx, job)
				stop()
				now := time.Now().UTC()
				observation := plane.PeerView{Server: job.peer.ID, Name: job.peer.Name, Node: job.peer.Identity.NodeID, Owner: job.peer.Identity.Owner, Project: job.project, Mode: "read-only", State: state}
				cache.mu.Lock()
				if state == "unauthorized" || state == "disabled" || state == "invalid" {
					cache.barriers[job.key] = now
				}
				if (state == "current" || state == "unavailable") && started.Before(cache.barriers[job.key]) {
					cache.mu.Unlock()
					continue
				}
				if state == "current" {
					expires := now.Add(coordinationPeerAge)
					observation.Observed = &now
					observation.Expires = &expires
					observation.Snapshot = &snapshot
				} else if old, ok := cache.values[job.key]; ok && state == "unavailable" && old.Expires != nil && now.Before(*old.Expires) && old.Snapshot != nil {
					observation = old
					observation.State = "stale"
				}
				encoded, _ := json.Marshal(observation.Snapshot)
				cache.total -= cache.sizes[job.key]
				cache.sizes[job.key] = len(encoded)
				cache.total += len(encoded)
				cache.values[job.key] = observation
				// Bound untrusted remote context independently of the local ledger.
				for cache.total > 16<<20 {
					oldest := ""
					var when time.Time
					for key, value := range cache.values {
						if value.Snapshot != nil && value.Observed != nil && (oldest == "" || value.Observed.Before(when)) {
							oldest = key
							when = *value.Observed
						}
					}
					if oldest == "" {
						break
					}
					value := cache.values[oldest]
					value.Snapshot = nil
					value.State = "unavailable"
					value.Reason = "cache-capacity"
					cache.values[oldest] = value
					cache.total -= cache.sizes[oldest]
					cache.sizes[oldest] = 0
				}
				cache.mu.Unlock()
			}
		}()
	}
	workers.Wait()
	cache.mu.Lock()
	cache.cursor = (offset + max(1, int(attempted.Load()))) % len(jobs)
	cache.mu.Unlock()
	return nil
}
func (s *Server) attachCoordinationPeers(view *plane.View, owner string) {
	if !view.Enabled {
		return
	}
	inputs := []plane.MemberInput{}
	if view.Self != nil {
		inputs = append(inputs, view.Self.MemberInput)
	} else {
		for _, member := range view.Members {
			inputs = append(inputs, member.MemberInput)
		}
	}
	jobs, err := s.coordinationPeerJobs(inputs)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	cache := &s.sessionCoord.peers
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	budget := 3 << 20
	for _, job := range jobs {
		if job.peer.CoordinationOwner != owner {
			continue
		}
		observation, ok := cache.values[job.key]
		if !ok {
			observation = plane.PeerView{Server: job.peer.ID, Name: job.peer.Name, Node: job.peer.Identity.NodeID, Owner: job.peer.Identity.Owner, Project: job.project, Mode: "read-only", State: "pending"}
		}
		if observation.Expires != nil && !now.Before(*observation.Expires) {
			observation.State = "unavailable"
			observation.Snapshot = nil
		}
		if observation.Snapshot != nil {
			encoded, _ := json.Marshal(observation.Snapshot)
			if len(encoded) > budget {
				observation.Snapshot = nil
				observation.State = "unavailable"
				observation.Reason = "view-capacity"
			} else {
				budget -= len(encoded)
			}
		}
		view.Peers = append(view.Peers, observation)
	}
}
