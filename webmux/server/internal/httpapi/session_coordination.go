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

	"github.com/jordanhubbard/webmux/server/internal/auth"
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
	bindings       map[string]*plane.Binding
}

func coordinationRef(node string, s session.Session) plane.Ref {
	hash := sha256.Sum256([]byte(s.ID + "\x00" + s.CreatedAt))
	return plane.Ref{Node: node, Owner: s.Owner, Session: s.ID, Incarnation: hex.EncodeToString(hash[:16])}
}
func coordinationLocal(s session.Session) bool {
	return (s.Transport == "local" || s.Transport == "exec") && s.Rig == nil && !s.Agent() && s.Companion == "" && s.OpenRigView == ""
}
func coordinationDirectory(s session.Session) string {
	if !coordinationLocal(s) {
		return ""
	}
	if s.CoordinationActivity != nil && s.CoordinationActivity.Directory != "" {
		return s.CoordinationActivity.Directory
	}
	return s.ExecCwd
}
func coordinationBindingKey(s session.Session) string {
	return s.Transport + "\x00" + s.Hostname + "\x00" + coordinationDirectory(s)
}
func (s *Server) coordinationSnapshots() ([]session.Session, error) {
	var out []session.Session
	err := s.sessions.WithCoordinationSnapshot(func(all []session.Session) error { out = all; return nil })
	return out, err
}
func (s *Server) refreshCoordinationBindings(ctx context.Context, snapshots []session.Session) int {
	accounts, err := auth.LoadConfig(s.store)
	if err != nil {
		return len(snapshots)
	}
	seen := map[string]bool{}
	for i, snapshot := range snapshots {
		if ctx.Err() != nil {
			return i
		}
		if !federationAccountAllowed(accounts, snapshot.Owner) {
			continue
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
		binding, _ := plane.InspectRepository(ctx, directory)
		s.sessionCoord.bindingsMu.Lock()
		s.sessionCoord.bindings[key] = binding
		s.sessionCoord.bindingsMu.Unlock()
	}
	return len(snapshots)
}
func (s *Server) coordinationInputs(node string, snapshots []session.Session) []plane.MemberInput {
	inputs := make([]plane.MemberInput, 0, len(snapshots))
	accounts, accountErr := auth.LoadConfig(s.store)
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
		// A retained terminal may outlive its WebMux account. Keep its lifecycle
		// intact while refusing authority under the removed account's identity.
		active := snapshot.CoordinationLive && accountErr == nil && federationAccountAllowed(accounts, snapshot.Owner)
		input := plane.MemberInput{Ref: coordinationRef(node, snapshot), Active: active, Transport: transport, ExecutionHost: host}
		input.Interface = "execution-host-onboarding-pending"
		input.ProjectStatus = "execution-host-verification-pending"
		if coordinationLocal(snapshot) {
			input.Interface = "local-cli+mcp; native-alias-bootstrap-available"
			if snapshot.CoordinationActivity != nil && snapshot.CoordinationActivity.Interface != "" {
				input.Interface = "local-cli; " + snapshot.CoordinationActivity.Interface
			}
			if s.sessionCoord.endpoint == "" {
				input.Interface = "local-interface-unavailable"
			}
			input.Project = s.sessionCoord.bindings[coordinationBindingKey(snapshot)]
			input.ProjectStatus = "repository-unverified"
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
	if !coordinationLocal(current) || s.sessionCoord.endpoint == "" || !s.federationOwnerAllowed(current.Owner) {
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
		"This interface supports local actions and explicitly authorized remote project summaries. Remote summaries are read-only; remote messages, claims and reservations remain pending. Qualified native aliases attach MCP guidance; other launch paths require separate acceptance.",
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
	if current.RecoveryKey != "" {
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
				if inputs[i].Ref.Session == current.ID && s.federationOwnerAllowed(current.Owner) {
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
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	if len(snapshots) > 0 {
		offset := s.sessionCoord.bindingCursor % len(snapshots)
		rotated := append([]session.Session(nil), snapshots[offset:]...)
		rotated = append(rotated, snapshots[:offset]...)
		processed := s.refreshCoordinationBindings(ctx, rotated)
		s.sessionCoord.bindingCursor = (offset + max(1, processed)) % len(snapshots)
	}
	cancel()
	node, err := s.coordinationNode()
	if err != nil {
		return err
	}
	enabled := false
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
	return s.refreshCoordinationPeers(parent, s.coordinationInputs(node, latest))
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
	if !s.federationOwnerAllowed(ref.Owner) {
		s.coordinationError(w, plane.ErrUnauthorized)
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
	var response any
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		inputs := s.coordinationInputs(node, all)
		if r.Method == "GET" {
			view, e := s.sessionCoord.service.AgentView(node, token, inputs, after)
			response = view
			return e
		}
		result, e := s.sessionCoord.service.ExecuteRouted(node, token, inputs, operation, route)
		response = result
		return e
	})
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	if view, ok := response.(plane.View); ok {
		s.attachCoordinationPeers(&view, ref.Owner)
		response = view
	}
	if result, ok := response.(plane.Result); ok && result.Transport == "pending" {
		s.wakeCoordinationMessages()
	}
	writeJSON(w, 200, response)
}
