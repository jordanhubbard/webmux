package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

const coordinationMessagesPath = "/api/federation/coordination/messages"

type coordinationMessageExchange struct {
	Operation string            `json:"operation"`
	Envelope  plane.WireMessage `json:"envelope"`
}

func (s *Server) exchangeCoordinationMessage(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	grant, ok := r.Context().Value(federationContextKey{}).(serverGrant)
	if !ok || !grant.Coordination || !grant.CoordinationMessages || r.TLS == nil || !plane.ValidSenderScope(grant.SourceNode, grant.SourceOwner) {
		writeError(w, 403, "This HTTPS grant does not permit messages from the selected node/account")
		return
	}
	var input coordinationMessageExchange
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Operation != "status" && input.Operation != "admit" {
		writeError(w, 400, "Expected status or admit")
		return
	}
	node, err := s.coordinationNode()
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	// Resolve current Git identity outside the broker/ledger lock before admission.
	if input.Operation == "admit" {
		snapshots, err := s.coordinationSnapshots()
		if err != nil {
			s.coordinationError(w, err)
			return
		}
		own := []session.Session{}
		for _, snapshot := range snapshots {
			if snapshot.Owner == owner && coordinationRef(node, snapshot) == input.Envelope.To {
				own = append(own, snapshot)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		s.refreshCoordinationBindings(ctx, own)
		cancel()
	}
	var receipt plane.RemoteReceipt
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		receipt, e = s.sessionCoord.service.Exchange(node, plane.SenderScope{Node: grant.SourceNode, Owner: grant.SourceOwner, ReceiverOwner: owner}, s.coordinationInputs(node, all), input.Operation, input.Envelope)
		return e
	})
	if err != nil {
		s.coordinationError(w, err)
		return
	}
	writeJSON(w, 200, receipt)
}
func (s *Server) coordinationMessageRoute(from, to plane.Ref, inputs []plane.MemberInput) *plane.RemoteRoute {
	own := []plane.MemberInput{}
	for _, input := range inputs {
		if input.Ref == from && input.Active {
			own = append(own, input)
			break
		}
	}
	jobs, err := s.coordinationPeerJobs(own)
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	cache := &s.sessionCoord.peers
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	for _, job := range jobs {
		if !job.peer.CoordinationMessages || job.peer.Identity.NodeID != to.Node || job.peer.Identity.Owner != to.Owner {
			continue
		}
		view, ok := cache.values[job.key]
		if !ok || view.State != "current" || view.Expires == nil || !now.Before(*view.Expires) || view.Snapshot == nil {
			continue
		}
		snapshot := view.Snapshot
		if !snapshot.Messaging || snapshot.SourceNode != from.Node || snapshot.SourceOwner != from.Owner {
			continue
		}
		for _, member := range snapshot.Members {
			if member.Ref == to && member.Active && member.Present && member.Revoked == nil {
				route := &plane.RemoteRoute{Project: job.project, Peer: job.peer.ID, Node: to.Node, Owner: to.Owner, TLSKey: job.peer.Identity.TLSKey, Epoch: snapshot.HistoryEpoch}
				if route.Valid() {
					return route
				}
			}
		}
	}
	return nil
}
func (s *Server) coordinationMessagePeer(record plane.RemoteRecord) (serverPeer, string, bool) {
	directory, err := s.serverDirectory()
	if err != nil {
		return serverPeer{}, "", false
	}
	for _, peer := range directory.Servers {
		if peer.ID == record.Route.Peer && peer.CoordinationMessages && peer.CoordinationOwner == record.Envelope.From.Owner && s.federationOwnerAllowed(peer.CoordinationOwner) && validNodePin(peer.Identity) && peer.Identity.NodeID == record.Route.Node && peer.Identity.Owner == record.Route.Owner && peer.Identity.TLSKey == record.Route.TLSKey {
			token := serverCredential(peer.TokenEnv, peer.Credential)
			if validSavedCredential(token) {
				return peer, token, true
			}
		}
	}
	return serverPeer{}, "", false
}
func (s *Server) fetchCoordinationReceipt(ctx context.Context, peer serverPeer, token, operation string, envelope plane.WireMessage) (plane.RemoteReceipt, error) {
	var receipt plane.RemoteReceipt
	target, err := url.Parse(peer.URL)
	if err != nil || target.Scheme != "https" {
		return receipt, errors.New("message peer requires HTTPS")
	}
	target.Path = coordinationMessagesPath
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	data, err := json.Marshal(coordinationMessageExchange{operation, envelope})
	if err != nil {
		return receipt, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", target.String(), bytes.NewReader(data))
	if err != nil {
		return receipt, err
	}
	request.Header = nodeScopeHeaders(peer, token)
	request.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = false
	transport.DialTLSContext = s.nodePeerDialer(peer, token)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("message redirect refused") }}
	response, err := client.Do(request)
	if err != nil {
		return receipt, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return receipt, errors.New("message peer refused exchange")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (32<<10)+1))
	if err != nil {
		return receipt, err
	}
	if len(body) > 32<<10 || json.Unmarshal(body, &receipt) != nil || !receipt.Valid(envelope) {
		return plane.RemoteReceipt{}, errors.New("invalid message receipt")
	}
	return receipt, nil
}
func (s *Server) deliverCoordinationMessage(ctx context.Context, node string, record plane.RemoteRecord) error {
	peer, token, ok := s.coordinationMessagePeer(record)
	if !ok {
		return s.sessionCoord.service.RemoteTransport(node, record.Envelope.ID, "authorization-unavailable", false)
	}
	if record.Attempted || record.Admitted {
		receipt, err := s.fetchCoordinationReceipt(ctx, peer, token, "status", record.Envelope)
		if err != nil {
			return s.sessionCoord.service.RemoteTransport(node, record.Envelope.ID, "uncertain", false)
		}
		if err = s.sessionCoord.service.ApplyRemoteReceipt(node, record.Envelope, receipt); err != nil {
			return err
		}
		if receipt.State != "absent" || record.Admitted {
			return nil
		}
	}
	snapshots, err := s.coordinationSnapshots()
	if err != nil {
		return err
	}
	own := []session.Session{}
	for _, snapshot := range snapshots {
		if coordinationRef(node, snapshot) == record.Envelope.From {
			own = append(own, snapshot)
		}
	}
	inspect, cancel := context.WithTimeout(ctx, 2*time.Second)
	s.refreshCoordinationBindings(inspect, own)
	cancel()
	// Only authenticated absence in the same history epoch permits exact replay.
	// No network call occurs under the broker/ledger lock.
	allowed := false
	err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
		var e error
		allowed, e = s.sessionCoord.service.BeginRemoteAttempt(node, record.Envelope.ID, s.coordinationInputs(node, all))
		return e
	})
	if err != nil || !allowed {
		return err
	}
	receipt, err := s.fetchCoordinationReceipt(ctx, peer, token, "admit", record.Envelope)
	if err != nil {
		return s.sessionCoord.service.RemoteTransport(node, record.Envelope.ID, "uncertain", false)
	}
	return s.sessionCoord.service.ApplyRemoteReceipt(node, record.Envelope, receipt)
}
func (s *Server) wakeCoordinationMessages() {
	select {
	case s.sessionCoord.messagesWake <- struct{}{}:
	default:
	}
}
func (s *Server) runCoordinationMessages(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.maintainCoordinationMessages(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("coordination message maintenance unavailable", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.sessionCoord.messagesWake:
		}
	}
}
func (s *Server) maintainCoordinationMessages(parent context.Context) error {
	node, err := s.coordinationNode()
	if err != nil {
		return err
	}
	records, err := s.sessionCoord.service.RemoteRecords(node)
	if err != nil || len(records) == 0 {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	offset := s.sessionCoord.messagesCursor % len(records)
	queue := make(chan plane.RemoteRecord, len(records))
	for i := range records {
		queue <- records[(offset+i)%len(records)]
	}
	close(queue)
	var workers sync.WaitGroup
	var attempted atomic.Int64
	var first error
	var mu sync.Mutex
	for i := 0; i < min(4, len(records)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for record := range queue {
				if ctx.Err() != nil {
					return
				}
				attempted.Add(1)
				requestCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := s.deliverCoordinationMessage(requestCtx, node, record)
				stop()
				if err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	workers.Wait()
	s.sessionCoord.messagesCursor = (offset + max(1, int(attempted.Load()))) % len(records)
	return first
}
