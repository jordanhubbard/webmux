//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func clientDialContext() func(context.Context, string, string) (net.Conn, error) {
	base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialResolvedHost(base, lookupMulticastHost)
}

func lookupMulticastHost(ctx context.Context, host string) ([]net.IPAddr, error) {
	addr4, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
	if err != nil {
		return nil, err
	}
	listener4, err := net.ListenUDP("udp4", addr4)
	if err != nil {
		return nil, fmt.Errorf("listen for IPv4 mDNS: %w", err)
	}

	var listener6 *net.UDPConn
	if addr6, resolveErr := net.ResolveUDPAddr("udp6", mdns.DefaultAddressIPv6); resolveErr == nil {
		listener6, _ = net.ListenUDP("udp6", addr6)
	}
	var packet6 *ipv6.PacketConn
	if listener6 != nil {
		packet6 = ipv6.NewPacketConn(listener6)
	}
	resolver, err := mdns.NewServer(
		ipv4.NewPacketConn(listener4),
		packet6,
		mdns.WithCacheRefresh(false),
		mdns.WithRecordTypes(dnsmessage.TypeA, dnsmessage.TypeAAAA),
	)
	if err != nil {
		_ = listener4.Close()
		if listener6 != nil {
			_ = listener6.Close()
		}
		return nil, err
	}
	defer resolver.Close()

	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, address, err := resolver.QueryAddr(queryCtx, host)
	if err != nil {
		return nil, err
	}
	return []net.IPAddr{netIPAddr(address)}, nil
}

func netIPAddr(address netip.Addr) net.IPAddr {
	return net.IPAddr{IP: net.IP(address.AsSlice()), Zone: address.Zone()}
}
