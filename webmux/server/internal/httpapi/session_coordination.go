package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
	"github.com/jordanhubbard/webmux/server/internal/discovery"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type sessionCoordination struct {
	messagesCursor int // Owned only by the message delivery worker.
	messagesWake   chan struct{}
	peers          coordinationPeerCache
	bindingCursor  int // Owned only by the maintenance goroutine.
	service        *plane.Service
	once           sync.Once
	cancel         context.CancelFunc
	done           chan struct{}
	endpoint       string
	bindingsMu     sync.RWMutex
	bindings       map[string]coordinationProjectObservation
}

type coordinationProjectObservation struct {
	project *plane.Binding
	expires time.Time // Remote verification cannot survive an unavailable host indefinitely.
}

func coordinationRef(node string, s session.Session) plane.Ref {
	hash := sha256.Sum256([]byte(s.ID + "\x00" + s.CreatedAt))
	return plane.Ref{Node: node, Owner: s.Owner, Session: s.ID, Incarnation: hex.EncodeToString(hash[:16])}
}
func coordinationLocal(s session.Session) bool {
	return (s.Transport == "local" || s.Transport == "exec") && s.Rig == nil && !s.Agent() && s.Companion == "" && s.OpenRigView == ""
}
func coordinationDirectory(s session.Session) string {
	if !coordinationLocal(s) && !s.CoordinationRemote() {
		return ""
	}
	if s.CoordinationActivity != nil && s.CoordinationActivity.Directory != "" {
		return s.CoordinationActivity.Directory
	}
	if s.CoordinationRemote() {
		return ""
	}
	return s.ExecCwd
}
func coordinationBindingKey(s session.Session) string {
	key := s.Transport + "\x00" + s.Hostname + "\x00" + coordinationDirectory(s)
	if s.CoordinationRemote() {
		// The same hostname/path may refer to different accounts, ports, SSH
		// key selections and runs. Never share a remote verification by path.
		key += "\x00" + s.Owner + "\x00" + s.ID + "\x00" + s.CreatedAt + "\x00" + s.Username + "\x00" + strconv.Itoa(s.Port) + "\x00" + s.KeyID
	}
	return key
}
func (s *Server) coordinationSnapshots() ([]session.Session, error) {
	var out []session.Session
	err := s.sessions.WithCoordinationSnapshot(func(all []session.Session) error { out = all; return nil })
	return out, err
}

// Pruning requires the complete broker inventory. Owner/agent refreshes below
// intentionally inspect subsets and must not evict another owner's context.
func (s *Server) pruneCoordinationBindings(snapshots []session.Session) {
	current := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		if coordinationDirectory(snapshot) != "" {
			current[coordinationBindingKey(snapshot)] = true
		}
	}
	s.sessionCoord.bindingsMu.Lock()
	for key := range s.sessionCoord.bindings {
		if !current[key] {
			delete(s.sessionCoord.bindings, key)
		}
	}
	s.sessionCoord.bindingsMu.Unlock()
}
func (s *Server) refreshCoordinationBindings(ctx context.Context, snapshots []session.Session) int {
	node, err := s.coordinationNode()
	if err != nil {
		return 0
	}
	enabled, err := s.sessionCoord.service.Enabled(node)
	if err != nil || !enabled {
		s.sessionCoord.bindingsMu.Lock()
		clear(s.sessionCoord.bindings)
		s.sessionCoord.bindingsMu.Unlock()
		return len(snapshots)
	}
	seen := map[string]bool{}
	for i, snapshot := range snapshots {
		if ctx.Err() != nil {
			return i
		}
		directory := coordinationDirectory(snapshot)
		if directory == "" {
			continue
		}
		key := coordinationBindingKey(snapshot)
		if seen[key] {
			continue
		}
		seen[key] = true
		var binding *plane.Binding
		if snapshot.CoordinationRemote() {
			probe, cancel := context.WithTimeout(ctx, 2*time.Second)
			binding, _ = s.sessions.InspectCoordinationRepository(probe, snapshot)
			cancel()
		} else {
			binding, _ = plane.InspectRepository(ctx, directory)
		}
		s.sessionCoord.bindingsMu.Lock()
		expires := time.Time{}
		if snapshot.CoordinationRemote() {
			expires = time.Now().Add(25 * time.Second)
		}
		s.sessionCoord.bindings[key] = coordinationProjectObservation{project: binding, expires: expires}
		s.sessionCoord.bindingsMu.Unlock()
	}
	return len(snapshots)
}
func (s *Server) coordinationInputs(node string, snapshots []session.Session) []plane.MemberInput {
	inputs := make([]plane.MemberInput, 0, len(snapshots))
	s.sessionCoord.bindingsMu.RLock()
	defer s.sessionCoord.bindingsMu.RUnlock()
	for _, snapshot := range snapshots {
		host := snapshot.Hostname
		if snapshot.Rig != nil && snapshot.Rig.Host != "" {
			host = snapshot.Rig.Host
		}
		if host == "" {
			host = "execution-host-unknown"
		}
		transport := snapshot.Transport
		if transport == "" {
			transport = "unavailable"
		}
		input := plane.MemberInput{Ref: coordinationRef(node, snapshot), Active: snapshot.CoordinationLive, Transport: transport, ExecutionHost: host}
		input.Interface = "execution-host-onboarding-pending"
		input.ProjectStatus = "execution-host-verification-pending"
		if coordinationLocal(snapshot) || snapshot.CoordinationRemote() {
			input.Interface = "local-cli+mcp; native-alias-bootstrap-available"
			if snapshot.CoordinationActivity != nil && snapshot.CoordinationActivity.Interface != "" {
				input.Interface = "local-cli; " + snapshot.CoordinationActivity.Interface
			}
			if s.sessionCoord.endpoint == "" {
				input.Interface = "local-interface-unavailable"
			}
			if snapshot.CoordinationRemote() {
				input.Interface = "execution-host-onboarding-pending"
				if snapshot.CoordinationChannel != "" {
					input.Interface = "execution-host-cli; " + snapshot.CoordinationChannel
				}
				if snapshot.CoordinationActivity != nil && snapshot.CoordinationActivity.Interface != "" {
					input.Interface += "; " + snapshot.CoordinationActivity.Interface
				}
			}
			observation := s.sessionCoord.bindings[coordinationBindingKey(snapshot)]
			input.Project = observation.project
			if snapshot.CoordinationRemote() && !time.Now().Before(observation.expires) {
				input.Project = nil
			}
			input.ProjectStatus = "repository-unverified"
			if snapshot.CoordinationRemote() {
				input.ProjectStatus = "execution-host-verification-unavailable"
			}
			if coordinationDirectory(snapshot) == "" {
				input.ProjectStatus = "working-directory-unobserved"
			} else if input.Project != nil {
				input.ProjectStatus = "verified-git"
			}
		}
		inputs = append(inputs, input)
	}
	return inputs
}
func (s *Server) coordinationNode() (string, error) {
	policy, err := discovery.LoadNodePolicy(s.store)
	return policy.ID, err
}
func (s *Server) provisionCoordination(node string, snapshots []session.Session, current session.Session) (*terminal.CoordinationAccess, error) {
	if (!coordinationLocal(current) && !current.CoordinationRemote()) || s.sessionCoord.endpoint == "" {
		return nil, nil
	}
	inputs := s.coordinationInputs(node, snapshots)
	ref := coordinationRef(node, current)
	var subject plane.MemberInput
	for _, input := range inputs {
		if input.Ref == ref {
			subject = input
			break
		}
	}
	token, err := s.sessionCoord.service.Provision(node, subject)
	if errors.Is(err, plane.ErrDisabled) || errors.Is(err, plane.ErrUnauthorized) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	access := coordinationclient.Access{Contract: plane.Contract, Endpoint: s.sessionCoord.endpoint, Capability: token, Subject: ref, Instructions: []string{
		"This interface supports local actions and explicitly authorized remote project context, messages, overlap notices and heavy-worker reservations. Respect each peer's permissions and report incomplete or unavailable context. Qualified native aliases attach MCP guidance; other launch paths require separate acceptance.",
		"Use the WebMux coordinate view command to discover verified project peers and read the coordination inbox.",
		"Use coordinate act with a JSON request on stdin. Preserve request_id across uncertain retries; use a fresh ID for new work.",
		"Claim file, directory, branch or task ownership before overlapping work. Claims are advisory and do not prevent edits.",
		"Inbox states are admitted, delivered, acknowledged and completed. Explicitly record receipts; viewing an inbox does not acknowledge a handoff.",
		"Reserve the advertised heavy-worker resource before heavy work. Renew its original lease/fence while working; stop using a lost/expired lease.",
		"This capability authorizes coordination only. Provider lifecycle and terminal input remain with their existing owners.",
	}}
	name := filepath.Join(s.store.Home, "data", "coordination", "access", current.ID+".json")
	// Session IDs originate with the broker, and cannot choose arbitrary paths.
	if strings.ContainsAny(current.ID, "/\\.:\x00\r\n") || current.ID == "" {
		return nil, plane.ErrInvalid
	}
	if err = coordinationclient.WriteAccess(name, access); err != nil {
		return nil, err
	}
	if coordinationLocal(current) && current.RecoveryKey != "" {
		locator, err := coordinationclient.Locator(current.ID, current.RecoveryKey)
		if err != nil {
			return nil, err
		}
		if err = coordinationclient.WriteAccess(locator, access); err != nil {
			return nil, err
		}
	}
	if err = s.sessionCoord.service.MarkProvisioned(node, ref); err != nil {
		return nil, err
	}
	return &terminal.CoordinationAccess{File: name, CLI: executable}, nil
}

// EnableSessionCoordination is called once, before restoration/serving. The
// agent-only listener is private loopback; no administrator APIs live there.
func (s *Server) EnableSessionCoordination(endpoint string) {
	s.sessionCoord.once.Do(func() {
		s.sessionCoord.endpoint = endpoint
		s.sessions.SetCoordinationLaunch(func(all []session.Session, current session.Session) (*terminal.CoordinationAccess, error) {
			node, err := s.coordinationNode()
			if err != nil {
				return nil, err
			}
			inputs := s.coordinationInputs(node, all)
			for i := range inputs {
				if inputs[i].Ref.Session == current.ID {
					inputs[i].Active = true
				}
			}
			if err := s.sessionCoord.service.Sync(node, inputs); err != nil {
				return nil, err
			}
			return s.provisionCoordination(node, all, current)
		})
	})
}

func (s *Server) StartSessionCoordination() {
	if s.sessionCoord.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.sessionCoord.cancel = cancel
	s.sessionCoord.done = make(chan struct{})
	s.sessionCoord.messagesWake = make(chan struct{}, 1)
	messageDone := make(chan struct{})
	go func() { defer close(messageDone); s.runCoordinationMessages(ctx) }()
	go func() {
		defer close(s.sessionCoord.done)
		defer func() { <-messageDone }()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		lastError := ""
		for {
			err := s.maintainSessionCoordination(ctx)
			if err != nil && ctx.Err() == nil {
				if err.Error() != lastError {
					s.logger.Warn("session coordination maintenance unavailable", "error", err)
					lastError = err.Error()
				}
			} else {
				lastError = ""
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Server) maintainSessionCoordination(parent context.Context) error {
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		return err
	}
	node, err := s.coordinationNode()
	if err != nil {
		return err
	}
	enabled, err := s.sessionCoord.service.Enabled(node)
	if err != nil {
		return err
	}
	if !enabled {
		// Global disable also stops execution-host Git queries. Keep live
		// membership/revocation current without retaining verified context.
		s.sessionCoord.bindingsMu.Lock()
		clear(s.sessionCoord.bindings)
		s.sessionCoord.bindingsMu.Unlock()
		return s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
			return s.sessionCoord.service.Sync(node, s.coordinationInputs(node, all))
		})
	}
	s.pruneCoordinationBindings(snapshots)
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	if len(snapshots) > 0 {
		offset := s.sessionCoord.bindingCursor % len(snapshots)
		rotated := append([]session.Session(nil), snapshots[offset:]...)
		rotated = append(rotated, snapshots[:offset]...)
		processed := s.refreshCoordinationBindings(ctx, rotated)
		s.sessionCoord.bindingCursor = (offset + max(1, processed)) % len(snapshots)
	}
	cancel()
	enabled = false
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		if err := s.sessionCoord.service.Sync(node, s.coordinationInputs(node, all)); err != nil {
			return err
		}
		for _, current := range all {
			if current.CoordinationLive {
				if _, err := s.provisionCoordination(node, all, current); err != nil {
					return err
				}
			}
		}
		view, e := s.sessionCoord.service.OwnerView(node, "", s.coordinationInputs(node, all))
		enabled = view.Enabled
		return e
	})
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	latest, err := s.coordinationSnapshots()
	if err != nil {
		return err
	}
	if err = s.refreshCoordinationPeers(parent, s.coordinationInputs(node, latest)); err != nil {
		return err
	}
	latest, err = s.coordinationSnapshots()
	if err != nil {
		return err
	}
	inputs := s.coordinationInputs(node, latest)
	peers := map[string][]plane.ClaimPeer{}
	for _, input := range inputs {
		if _, exists := peers[input.Ref.Owner]; !exists {
			peers[input.Ref.Owner] = s.coordinationClaimPeers(input.Ref.Owner, inputs)
		}
	}
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		return s.sessionCoord.service.ReconcileClaims(node, s.coordinationInputs(node, all), peers)
	})
	if err == nil {
		s.wakeCoordinationMessages()
	}
	return err
}
func (s *Server) coordinationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, plane.ErrUnauthorized):
		writeError(w, 401, "Coordination session capability is invalid or inactive")
	case errors.Is(err, plane.ErrInvalid):
		writeError(w, 400, "Invalid coordination request or reused request ID")
	case errors.Is(err, plane.ErrDisabled):
		writeError(w, 503, "Session coordination is disabled by server policy")
	default:
		s.logger.Warn("session coordination unavailable", "error", err)
		writeError(w, 503, "Session coordination is unavailable; existing terminal/provider work is unaffected")
	}
}
func (s *Server) getSessionCoordination(w http.ResponseWriter, r *http.Request, owner string) {
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
	inspect, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	s.refreshCoordinationBindings(inspect, own)
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
	s.attachCoordinationPeers(&view, owner)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, view)
}
func (s *Server) setSessionCoordinationPolicy(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		writeError(w, 400, "enabled is required")
		return
	}
	node, err := s.coordinationNode()
	if err == nil {
		err = s.sessionCoord.service.SetEnabled(node, *input.Enabled)
	}
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) CoordinationAgentHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /view", s.coordinationAgent)
	mux.HandleFunc("POST /act", s.coordinationAgent)
	return newLimiter(1200, globalWindow).wrap(mux)
}
func (s *Server) coordinationAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") || !strings.HasPrefix(token, "wmc1.") || len(token) > 2048 {
		writeError(w, 401, "A session coordination capability is required")
		return
	}
	node, err := s.coordinationNode()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	ref, err := s.sessionCoord.service.Subject(node, token)
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	// Resolve the caller first, then its same-account peers. Two ordinary
	// agents can discover each other before the first background maintenance tick.
	own := []session.Session{}
	for _, snapshot := range snapshots {
		if coordinationRef(node, snapshot) == ref {
			own = append(own, snapshot)
			break
		}
	}
	for _, snapshot := range snapshots {
		if snapshot.Owner == ref.Owner && coordinationRef(node, snapshot) != ref {
			own = append(own, snapshot)
		}
	}
	inspect, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	s.refreshCoordinationBindings(inspect, own)
	cancel()
	var operation plane.Request
	after := uint64(0)
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if !decodeJSON(w, r, &operation) {
			return
		}
		if (operation.Action == "reserve" || operation.Action == "renew-reservation" || operation.Action == "release-reservation") && r.Header.Get(coordinationclient.LeaseProtocolHeader) != "1" {
			writeError(w, 428, "Update the coordination client: lease protocol 1 and current-lease checks are required")
			return
		}
	} else if raw := r.URL.Query().Get("after"); raw != "" {
		after, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			s.coordinationError(w, plane.ErrInvalid)
			return
		}
	}
	var route *plane.RemoteRoute
	if operation.Action == "message" && operation.Target != nil && operation.Target.Node != node {
		route = s.coordinationMessageRoute(ref, *operation.Target, s.coordinationInputs(node, snapshots))
	}
	if target, shared := plane.ReservationTarget(operation); shared && target != node {
		route = s.coordinationReservationRoute(ref, operation, s.coordinationInputs(node, snapshots))
	}
	var peers []plane.ClaimPeer
	if operation.Action == "claim" {
		peers = s.coordinationClaimPeers(ref.Owner, s.coordinationInputs(node, snapshots))
	}
	var response any
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		inputs := s.coordinationInputs(node, all)
		if r.Method == "GET" {
			view, e := s.sessionCoord.service.AgentView(node, token, inputs, after)
			response = view
			return e
		}
		result, e := s.sessionCoord.service.ExecuteWithPeers(node, token, inputs, operation, route, peers)
		response = result
		return e
	})
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	if result, ok := response.(plane.Result); ok && result.Shared != nil {
		result, err = s.resolveCoordinationReservation(r.Context(), node, token, operation)
		if err != nil {
			s.coordinationError(w, err)
			return
		}
		response = result
	}
	if view, ok := response.(plane.View); ok {
		s.attachCoordinationPeers(&view, ref.Owner)
		response = view
	}
	if result, ok := response.(plane.Result); ok && (result.Transport == "pending" || operation.Action == "claim" && result.Status == "claimed") {
		s.wakeCoordinationMessages()
	}
	writeJSON(w, 200, response)
}
