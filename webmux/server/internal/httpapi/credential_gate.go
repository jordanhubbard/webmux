package httpapi

import (
	"net"
	"net/netip"
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
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	// Canonicalize mapped IPv4 and ignore the kernel's local IPv6 zone name;
	// neither source-port nor address spelling should create another allowance.
	host = ip.WithZone("").Unmap().String()
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
