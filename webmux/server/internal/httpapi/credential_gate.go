package httpapi

import (
	"net"
	"sync"
	"time"
)

// The public exchange has one fixed-size, per-second admission window. The
// global cap also bounds the source map, even with distributed invalid bearers.
// Rejected requests never touch configuration, vault or durable event storage.
type credentialExchangeGate struct {
	mu       sync.Mutex
	start    time.Time
	admitted int
	sources  map[string]int
}

func (g *credentialExchangeGate) allow(remote string, now time.Time) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil || net.ParseIP(host) == nil {
		return false
	}
	host = net.ParseIP(host).String()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.start.IsZero() || now.Sub(g.start) >= time.Second {
		g.start, g.admitted, g.sources = now, 0, make(map[string]int)
	}
	if g.admitted >= 128 || g.sources[host] >= 32 {
		return false
	}
	g.admitted++
	g.sources[host]++
	return true
}
