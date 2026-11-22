package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

const coordinationReservationsPath = "/api/federation/coordination/reservations"

var errReservationScope = errors.New("reservation scope refused")

type reservationExchange struct {
	Operation string                    `json:"operation"`
	Envelope  plane.ReservationEnvelope `json:"envelope"`
}

func (s *Server) exchangeCoordinationReservation(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	grant, ok := r.Context().Value(federationContextKey{}).(serverGrant)
	if !ok || !grant.Coordination || !grant.CoordinationReservations || r.TLS == nil || !plane.ValidSenderScope(grant.SourceNode, grant.SourceOwner) {
		writeError(w, 403, "This HTTPS grant does not permit shared reservations from the selected source")
		return
	}
	var input reservationExchange
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Operation != "status" && input.Operation != "admit" {
		writeError(w, 400, "Expected status or admit")
		return
	}
	if input.Operation == "admit" && input.Envelope.Request.Action != "release-reservation" {
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
	}
	node, err := s.coordinationNode()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	var reply plane.ReservationReply
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		reply, e = s.sessionCoord.service.ExchangeReservation(node, plane.SenderScope{Node: grant.SourceNode, Owner: grant.SourceOwner, ReceiverOwner: owner}, s.coordinationInputs(node, all), input.Operation, input.Envelope)
		return e
	})
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	writeJSON(w, 200, reply)
}
func (s *Server) coordinationReservationRoute(from plane.Ref, request plane.Request, inputs []plane.MemberInput) *plane.RemoteRoute {
	node, _ := plane.ReservationTarget(request)
	own := []plane.MemberInput{}
	for _, input := range inputs {
		if input.Ref == from && input.Active {
			own = append(own, input)
		}
	}
	jobs, err := s.coordinationPeerJobs(own)
	if err != nil {
		return nil
	}
	cache := &s.sessionCoord.peers
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	now := time.Now().UTC()
	var selected *plane.RemoteRoute
	for _, job := range jobs {
		if !job.peer.CoordinationReservations || job.peer.Identity.NodeID != node {
			continue
		}
		view, ok := cache.values[job.key]
		if !ok || view.State != "current" || view.Expires == nil || !now.Before(*view.Expires) || view.Snapshot == nil {
			continue
		}
		snapshot := view.Snapshot
		if !snapshot.Reservations || snapshot.SourceNode != from.Node || snapshot.SourceOwner != from.Owner {
			continue
		}
		if request.Target != nil {
			memberFound := false
			for _, member := range snapshot.Members {
				if member.Ref == *request.Target && member.Active && member.Present && member.Revoked == nil {
					memberFound = true
					break
				}
			}
			if !memberFound {
				continue
			}
		}
		route := &plane.RemoteRoute{Peer: job.peer.ID, Node: node, Owner: job.peer.Identity.Owner, Project: job.project, TLSKey: job.peer.Identity.TLSKey, Epoch: snapshot.HistoryEpoch}
		if route.Valid() {
			if selected != nil {
				return nil
			}
			selected = route
		}
	}
	return selected
}
func (s *Server) reservationPeer(intent plane.ReservationIntent) (serverPeer, string, bool) {
	directory, err := s.serverDirectory()
	if err != nil {
		return serverPeer{}, "", false
	}
	for _, peer := range directory.Servers {
		if peer.ID == intent.Route.Peer && peer.CoordinationReservations && peer.CoordinationOwner == intent.Envelope.Actor.Owner && s.federationOwnerAllowed(peer.CoordinationOwner) && validNodePin(peer.Identity) && peer.Identity.NodeID == intent.Route.Node && peer.Identity.Owner == intent.Route.Owner && peer.Identity.TLSKey == intent.Route.TLSKey {
			token := serverCredential(peer.TokenEnv, peer.Credential)
			if validSavedCredential(token) {
				return peer, token, true
			}
		}
	}
	return serverPeer{}, "", false
}
func (s *Server) fetchReservationReply(ctx context.Context, peer serverPeer, token, operation string, e plane.ReservationEnvelope) (plane.ReservationReply, time.Time, error) {
	var reply plane.ReservationReply
	target, err := url.Parse(peer.URL)
	if err != nil || target.Scheme != "https" {
		return reply, time.Time{}, errReservationScope
	}
	target.Path = coordinationReservationsPath
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	data, err := json.Marshal(reservationExchange{operation, e})
	if err != nil {
		return reply, time.Time{}, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", target.String(), bytes.NewReader(data))
	if err != nil {
		return reply, time.Time{}, err
	}
	request.Header = nodeScopeHeaders(peer, token)
	request.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = false
	transport.DialTLSContext = s.nodePeerDialer(peer, token)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("reservation redirect refused") }}
	started := time.Now().UTC()
	response, err := client.Do(request)
	if err != nil {
		return reply, started, err
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return reply, started, errReservationScope
	}
	if response.StatusCode != 200 {
		return reply, started, errors.New("reservation authority unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (32<<10)+1))
	if err != nil {
		return reply, started, err
	}
	if len(body) > 32<<10 || json.Unmarshal(body, &reply) != nil || !reply.Valid(e) {
		return plane.ReservationReply{}, started, errors.New("invalid reservation receipt")
	}
	return reply, started, nil
}
func (s *Server) reservationIntent(node, token string, r plane.Request) (plane.ReservationIntent, plane.Result, error) {
	var intent plane.ReservationIntent
	var result plane.Result
	err := s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		intent, result, e = s.sessionCoord.service.SharedIntent(node, token, s.coordinationInputs(node, all), r)
		return e
	})
	return intent, result, err
}
func (s *Server) resolveCoordinationReservation(parent context.Context, node, token string, r plane.Request) (plane.Result, error) {
	intent, result, err := s.reservationIntent(node, token, r)
	if err != nil || intent.Terminal {
		return result, err
	}
	peer, credential, ok := s.reservationPeer(intent)
	if !ok {
		err = s.sessionCoord.service.MarkSharedUnavailable(node, intent.Envelope, "authorization-unavailable")
		if err != nil {
			return result, err
		}
		_, result, err = s.reservationIntent(node, token, r)
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	unavailable := func(err error) (plane.Result, error) {
		state := "uncertain"
		if errors.Is(err, errReservationScope) {
			state = "authorization-unavailable"
		}
		if e := s.sessionCoord.service.MarkSharedUnavailable(node, intent.Envelope, state); e != nil {
			return result, e
		}
		_, current, e := s.reservationIntent(node, token, r)
		return current, e
	}
	if intent.Attempted || intent.Resolved {
		reply, started, e := s.fetchReservationReply(ctx, peer, credential, "status", intent.Envelope)
		if e != nil {
			return unavailable(e)
		}
		if e = s.sessionCoord.service.ApplySharedReply(node, intent.Envelope, reply, started); e != nil {
			return result, e
		}
		if reply.State != "absent" || intent.Resolved {
			_, result, err = s.reservationIntent(node, token, r)
			return result, err
		}
	}
	// Inspect Git outside locks, then authorize the immutable source immediately
	// before mutation. A request can never silently switch node/account/history.
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		return result, err
	}
	own := []session.Session{}
	for _, snapshot := range snapshots {
		if coordinationRef(node, snapshot) == intent.Envelope.Actor {
			own = append(own, snapshot)
		}
	}
	inspect, stop := context.WithTimeout(ctx, 2*time.Second)
	s.refreshCoordinationBindings(inspect, own)
	stop()
	allowed := false
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		allowed, e = s.sessionCoord.service.BeginSharedAttempt(node, token, s.coordinationInputs(node, all), r)
		return e
	})
	if err != nil {
		return result, err
	}
	if !allowed {
		_, result, err = s.reservationIntent(node, token, r)
		return result, err
	}
	reply, started, err := s.fetchReservationReply(ctx, peer, credential, "admit", intent.Envelope)
	if err != nil {
		return unavailable(err)
	}
	if err = s.sessionCoord.service.ApplySharedReply(node, intent.Envelope, reply, started); err != nil {
		return result, err
	}
	_, result, err = s.reservationIntent(node, token, r)
	return result, err
}
