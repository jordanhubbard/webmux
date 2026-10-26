package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

type hostLookup func(context.Context, string) ([]net.IPAddr, error)

// dialResolvedHost keeps the original URL hostname available to the TLS layer
// while dialing an address supplied by a platform resolver. This is required
// for Linux .local names when the client is built without cgo: Go's native DNS
// resolver does not implement mDNS and otherwise sends the name to unicast DNS.
func dialResolvedHost(base *net.Dialer, lookup hostLookup) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !isLocalHostname(host) {
			return base.DialContext(ctx, network, address)
		}
		addresses, err := lookup(ctx, strings.TrimSuffix(host, "."))
		if err != nil {
			return nil, fmt.Errorf("resolve %s with multicast DNS: %w", host, err)
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("resolve %s with multicast DNS: no addresses", host)
		}
		var failures []error
		for _, resolved := range addresses {
			candidate := net.JoinHostPort(resolved.String(), port)
			connection, dialErr := base.DialContext(ctx, network, candidate)
			if dialErr == nil {
				return connection, nil
			}
			failures = append(failures, fmt.Errorf("%s: %w", candidate, dialErr))
		}
		return nil, errors.Join(failures...)
	}
}

func isLocalHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return strings.HasSuffix(host, ".local")
}
