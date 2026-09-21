// Package netguard validates desktop destinations and pins DNS results.
package netguard

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

var ErrHostname = errors.New("Invalid hostname")
var ErrBlocked = errors.New("Target blocked")
var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9.])?$|^\[[0-9a-fA-F:]+\]$`)

type Guard struct {
	AllowLocal bool
	lookup     func(context.Context, string, string) ([]netip.Addr, error)
}

func New(allowLocal bool) *Guard {
	return &Guard{AllowLocal: allowLocal, lookup: net.DefaultResolver.LookupNetIP}
}
func Blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.Is4() {
		octets := ip.As4()
		return octets[0] == 0 || octets[0] >= 224
	}
	return false
}
func (g *Guard) Resolve(ctx context.Context, hostname string) (string, error) {
	if len(hostname) > 255 || !hostnamePattern.MatchString(hostname) {
		return "", ErrHostname
	}
	host := strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ip, err := netip.ParseAddr(host)
	if err != nil {
		addresses, err := g.lookup(ctx, "ip", host)
		if err != nil {
			return "", err
		}
		if len(addresses) == 0 {
			return "", ErrBlocked
		}
		ip = addresses[0]
	}
	if !ip.IsValid() || ip.Zone() != "" || !g.AllowLocal && Blocked(ip) {
		return "", ErrBlocked
	}
	// Return the validated address, never the original name, so dialing cannot
	// trigger a second lookup with a different policy decision.
	return ip.Unmap().String(), nil
}
