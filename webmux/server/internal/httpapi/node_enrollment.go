package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/discovery"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

// Update only the exact configuration whose credential and identity were
// verified. An edit, removal, grant/environment change or concurrent enrollment
// during a network request must not resurrect or overwrite another peer.
func (s *Server) commitNodePeer(previous, next serverPeer, secret string) error {
	return storage.UpdateConfigOrCreate(s.store, "servers.yaml", serverDirectory{Version: 1}, func(d *serverDirectory) error {
		for i, current := range d.Servers {
			if current.ID != previous.ID {
				continue
			}
			if current != previous || serverCredential(current.TokenEnv, current.Credential) != secret {
				return errors.New("Server changed during verification; refresh and retry")
			}
			for _, other := range d.Servers {
				if other.ID != current.ID && other.Identity.NodeID == next.Identity.NodeID && other.Identity.Owner == next.Identity.Owner {
					return errors.New("This node and account are already enrolled as " + other.ID)
				}
			}
			d.Servers[i] = next
			return validateServerDirectory(*d)
		}
		return errors.New("Server was removed during verification")
	})
}

func (s *Server) enrollNodePeer(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	peer, secret, err := s.peer(r.PathValue("server"))
	if err != nil {
		writeError(w, 404, "Server is unavailable or no longer configured")
		return
	}
	target, _ := url.Parse(peer.URL)
	if target.Scheme != "https" {
		writeError(w, 400, "Use HTTPS to verify a node identity. Manual HTTP peers remain available without automatic discovery reconciliation.")
		return
	}
	connection, identity, err := dialNodePeer(r.Context(), peer, secret, s.peerRoots)
	if err != nil {
		writeError(w, 502, "Node identity verification failed; check HTTPS trust, the incoming grant and the original node identity")
		return
	}
	_ = connection.Close()
	local, err := discovery.LoadNodePolicy(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if identity.NodeID == local.ID {
		writeError(w, 409, "The peer has this server's node identity; use separate WebMux homes and identities")
		return
	}
	next := peer
	next.Identity = identity
	if err := s.commitNodePeer(peer, next, secret); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func candidatePeer(previous serverPeer, c discovery.Candidate, now time.Time) (serverPeer, bool) {
	if !validNodePin(previous.Identity) || !c.TLS || c.ClaimedID != previous.Identity.NodeID || !now.Before(c.Expires) || c.Port == 0 {
		return serverPeer{}, false
	}
	address, err := netip.ParseAddr(c.Address)
	if err != nil || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() || (address.Is6() && address.IsLinkLocalUnicast() && address.Zone() == "") {
		return serverPeer{}, false
	}
	// The discovery parser already validates DNS names. Repeat the trust-boundary
	// check here so callers cannot construct an arbitrary proxy destination.
	host := strings.ToLower(strings.TrimSuffix(c.Host, "."))
	if len(host) > 253 || !strings.HasSuffix(host, ".local") {
		return serverPeer{}, false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return serverPeer{}, false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return serverPeer{}, false
			}
		}
	}
	next := previous
	next.URL = (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, strconv.Itoa(int(c.Port)))}).String()
	next.DiscoveredAddress = address.String()
	return next, true
}

// Reconciliation only changes an enrolled peer, after proving its original node,
// TLS key AND receiving account at the discovered endpoint. No bearer is sent
// while evaluating advertisements, and independent account grants stay distinct.
func (s *Server) reconcileNodePeer(ctx context.Context, peer serverPeer, candidates []discovery.Candidate) bool {
	if !validNodePin(peer.Identity) {
		return false
	}
	secret := serverCredential(peer.TokenEnv, peer.Credential)
	if !validSavedCredential(secret) {
		return false
	}
	// Prefer the existing reachable endpoint to avoid flapping between interfaces.
	if connection, _, err := dialNodePeer(ctx, peer, secret, s.peerRoots); err == nil {
		_ = connection.Close()
		return false
	}
	attempted := 0
	for _, candidate := range candidates {
		if ctx.Err() != nil || attempted >= 4 {
			break
		}
		next, ok := candidatePeer(peer, candidate, time.Now())
		if !ok || next == peer {
			continue
		}
		attempted++
		connection, _, err := dialNodePeer(ctx, next, secret, s.peerRoots)
		if err != nil {
			continue
		}
		_ = connection.Close()
		if err = s.commitNodePeer(peer, next, secret); err == nil {
			return true
		}
		return false // A concurrent configuration change requires a fresh snapshot.
	}
	return false
}

func (s *Server) maintainNodePeers(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor, candidateCursor := 0, 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		directory, err := s.serverDirectory()
		if err != nil || len(directory.Servers) == 0 {
			continue
		}
		candidates := s.nodes.Snapshot(time.Now())
		if len(candidates) == 0 {
			continue
		}
		// Rotate a bounded candidate probe window so a fixed set of spoofed
		// records cannot permanently occupy the first four attempts.
		offset := candidateCursor % len(candidates)
		rotated := append([]discovery.Candidate(nil), candidates[offset:]...)
		candidates = append(rotated, candidates[:offset]...)
		candidateCursor = (candidateCursor + 4) % len(candidates)
		round, cancel := context.WithTimeout(ctx, 8*time.Second)
		// Four peers per round bounds work and rotates through a full directory.
		count := min(4, len(directory.Servers))
		processed := 0
		for i := 0; i < count && round.Err() == nil; i++ {
			peer := directory.Servers[(cursor+i)%len(directory.Servers)]
			s.reconcileNodePeer(round, peer, candidates)
			processed++
		}
		cursor = (cursor + max(1, processed)) % len(directory.Servers)
		cancel()
	}
}
