package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/discovery"
)

// StartNodeDiscovery must be called after binding listeners and before serving
// requests. It uses the actual bound port, never an advertised default.
func (s *Server) StartNodeDiscovery(host string, httpPort, httpsPort int) {
	port := httpPort
	if httpsPort > 0 {
		port = httpsPort
	}
	p, err := discovery.LoadNodePolicy(s.store)
	s.nodePolicy = p
	if err == nil {
		s.nodes, err = discovery.StartNodes(context.Background(), discovery.NodeOptions{ID: p.ID, Browse: p.Browse, Advertise: p.Advertise, Interfaces: p.Interfaces, Host: host, HTTPPort: httpPort, Port: port, TLS: httpsPort > 0})
	}
	if err == nil && p.Browse && s.nodes != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.nodePeersCancel = cancel
		s.nodePeersDone = make(chan struct{})
		go func() { defer close(s.nodePeersDone); s.maintainNodePeers(ctx) }()
	}
	if err != nil {
		s.nodeDiscoveryError = "Network discovery unavailable; manually configured peers remain available"
		s.logger.Warn("WebMux node discovery unavailable", "error", err)
	}
}

func (s *Server) getDiscoveredNodes(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	candidates := []discovery.Candidate{}
	started, discoveryError := s.nodes != nil, s.nodeDiscoveryError
	if started && s.nodes.Err() != nil {
		started = false
		discoveryError = "Network discovery unavailable; manually configured peers remain available"
	}
	if started {
		candidates = s.nodes.Snapshot(time.Now())
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"contract": "webmux-node-discovery/v1", "policy": s.nodePolicy, "started": started, "error": discoveryError, "candidates": candidates})
}
