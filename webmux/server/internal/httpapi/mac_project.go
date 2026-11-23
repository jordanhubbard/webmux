package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/maccoord"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

// Policy selection here is immutable and performs no I/O: this callback also
// runs inside coordination transactions. Backend changes require a coordinated
// server reopen; editing a file cannot silently transfer running ownership.
func (s *Server) standaloneAdmission(owner string, project *plane.Binding, action string) error {
	switch action {
	case "release-claim", "release-reservation", "delivered", "acknowledge", "complete":
		return nil // Preserve the ability to drain existing standalone history.
	}
	mode, err := s.macObserver.AcceptedBackend(owner, project)
	if err != nil || mode != "standalone" {
		return plane.ErrExternalAuthority
	}
	return nil
}

func (s *Server) getMACProject(w http.ResponseWriter, r *http.Request, owner string) {
	s.serveMACProject(w, r, owner, r.PathValue("id"), nil)
}

func (s *Server) serveMACProject(w http.ResponseWriter, r *http.Request, owner, id string, expected *plane.Ref) {
	w.Header().Set("Cache-Control", "no-store")
	offset := 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		s.coordinationError(w, plane.ErrInvalid)
		return
	}
	for key, values := range query {
		if key != "offset" || len(values) != 1 {
			s.coordinationError(w, plane.ErrInvalid)
			return
		}
		parsed, err := strconv.Atoi(values[0])
		if err != nil || parsed < 0 || parsed > 200 {
			s.coordinationError(w, plane.ErrInvalid)
			return
		}
		offset = parsed
	}
	view, err := s.macProjectForSession(r.Context(), owner, id, expected, offset)
	if err != nil {
		switch {
		case errors.Is(err, maccoord.ErrUnmapped):
			writeError(w, 404, "No MAC project observation is available for this session")
		case errors.Is(err, session.ErrNotFound):
			writeError(w, 404, "Session not found")
		case errors.Is(err, maccoord.ErrPolicy), errors.Is(err, maccoord.ErrReadAccess), errors.Is(err, maccoord.ErrInvalid):
			writeError(w, 503, "MAC project observation is unavailable; refresh after its connection or configuration is resolved")
		default:
			s.coordinationError(w, err)
		}
		return
	}
	writeJSON(w, 200, view)
}

// Both browser and session capability routes use this exact selection/read
// path. No request can choose the receiving account, MAC origin or project key.
func (s *Server) macProjectForSession(ctx context.Context, owner, id string, expected *plane.Ref, offset int) (maccoord.SessionProjectObservation, error) {
	var empty maccoord.SessionProjectObservation
	if !s.federationOwnerAllowed(owner) {
		return empty, plane.ErrUnauthorized
	}
	current, err := s.sessions.Get(owner, id)
	if err != nil {
		return empty, err
	}
	node, err := s.coordinationNode()
	if err != nil {
		return empty, err
	}
	ref := coordinationRef(node, current)
	if expected != nil && *expected != ref {
		return empty, plane.ErrUnauthorized
	}
	if enabled, err := s.sessionCoord.service.Enabled(node); err != nil || !enabled {
		if err != nil {
			return empty, err
		}
		return empty, plane.ErrDisabled
	}
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		return empty, err
	}
	own := []session.Session{current}
	for _, candidate := range snapshots {
		if candidate.Owner == owner && candidate.ID != current.ID {
			own = append(own, candidate)
		}
	}
	inspect, cancel := context.WithTimeout(ctx, 4*time.Second)
	s.refreshCoordinationBindings(inspect, own)
	cancel()
	inputs := s.coordinationInputs(node, own)
	if len(inputs) == 0 || inputs[0].Ref != ref {
		return empty, plane.ErrUnauthorized
	}
	repository := inputs[0].Project
	observed, err := s.macObserver.ObserveProject(ctx, owner, repository, offset, maccoord.ProjectPageLimit)
	if err != nil {
		return empty, err
	}
	retained, err := s.sessionCoord.service.ProjectInventory(owner, repository)
	if err != nil {
		return empty, err
	}
	members := map[string]plane.Member{}
	for _, member := range retained {
		members[member.Ref.Key()] = member
	}
	for _, input := range inputs {
		old, exists := members[input.Ref.Key()]
		if exists && old.Revoked != nil {
			continue
		}
		// Broker presence is not a heartbeat or proof of task execution.
		members[input.Ref.Key()] = plane.Member{MemberInput: input, Present: true, Observed: time.Now().UTC()}
	}
	sessions := []maccoord.SessionObservation{}
	for _, member := range members {
		if member.Project == nil || member.Project.ID != repository.ID || member.Project.Repository != repository.Repository {
			continue
		}
		projection, err := s.macObserver.ObserveSession(owner, member)
		if err != nil {
			return empty, err
		}
		sessions = append(sessions, projection)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Session.Key() < sessions[j].Session.Key() })
	// Drop the whole result if account, broker incarnation, working directory,
	// remote verification or accepted policy changed while MAC was queried.
	latest, err := s.sessions.Get(owner, id)
	if err != nil || coordinationRef(node, latest) != ref || coordinationBindingKey(latest) != coordinationBindingKey(current) || !s.federationOwnerAllowed(owner) {
		return empty, plane.ErrUnauthorized
	}
	final := s.coordinationInputs(node, []session.Session{latest})
	if len(final) != 1 || final[0].Project == nil || *final[0].Project != *repository {
		return empty, maccoord.ErrUnmapped
	}
	if mode, err := s.macObserver.Backend(owner, final[0].Project); err != nil || mode != "mac" {
		return empty, maccoord.ErrPolicy
	}
	if enabled, err := s.sessionCoord.service.Enabled(node); err != nil || !enabled {
		return empty, plane.ErrDisabled
	}
	return maccoord.SessionProjectObservation{Schema: "webmux.mac.session-project-view.v1", Subject: ref, Project: observed, Sessions: sessions, HistoryState: "retained_unreconciled"}, nil
}
