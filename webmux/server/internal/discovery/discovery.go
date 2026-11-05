// Package discovery advertises and resolves WebMux listener ports.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	secureService = "_webmuxs._tcp"
	clearService  = "_webmux._tcp"
)

type Endpoint struct {
	Scheme string
	Port   int
}

// Advertise publishes the actual listeners on the local network. Discovery is
// best effort: a missing multicast network must not prevent the HTTP server
// from starting.
func Advertise(host string, httpPort, httpsPort int, hasTLS bool) (io.Closer, error) {
	host = strings.Split(strings.TrimSuffix(strings.TrimSuffix(host, "."), ".local"), ".")[0]
	if host == "" || strings.ContainsAny(host, ":/") {
		return nil, errors.New("invalid mDNS host name")
	}
	conn4, conn6, err := multicastSockets()
	if err != nil {
		return nil, err
	}
	opts := []mdns.ServerOption{mdns.WithLocalNames(host + ".local")}
	if hasTLS {
		opts = append(opts, mdns.WithService(mdns.ServiceInstance{Instance: host, Service: secureService, Host: host + ".local.", Port: uint16(httpsPort)}))
	} else {
		opts = append(opts, mdns.WithService(mdns.ServiceInstance{Instance: host, Service: clearService, Host: host + ".local.", Port: uint16(httpPort)}))
	}
	server, err := mdns.NewServer(packet4(conn4), packet6(conn6), opts...)
	if err != nil {
		closeSockets(conn4, conn6)
		return nil, err
	}
	return server, nil
}

func multicastSockets() (*net.UDPConn, *net.UDPConn, error) {
	listen := func(network, address string) (*net.UDPConn, error) {
		addr, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			return nil, err
		}
		return net.ListenUDP(network, addr)
	}
	v4, err4 := listen("udp4", mdns.DefaultAddressIPv4)
	v6, err6 := listen("udp6", mdns.DefaultAddressIPv6)
	if v4 == nil && v6 == nil {
		return nil, nil, errors.Join(err4, err6)
	}
	return v4, v6, nil
}

func packet4(conn *net.UDPConn) *ipv4.PacketConn {
	if conn != nil {
		return ipv4.NewPacketConn(conn)
	}
	return nil
}

func packet6(conn *net.UDPConn) *ipv6.PacketConn {
	if conn != nil {
		return ipv6.NewPacketConn(conn)
	}
	return nil
}

func closeSockets(v4, v6 *net.UDPConn) {
	if v4 != nil {
		_ = v4.Close()
	}
	if v6 != nil {
		_ = v6.Close()
	}
}

// Resolve finds the listener for a host-only client argument. Explicit URLs
// and host:port arguments are handled by the caller and never reach this path.
// HTTP is selected only when the caller explicitly permits cleartext.
func Resolve(ctx context.Context, host string, allowHTTP bool) (string, error) {
	host = strings.Trim(host, "[]")
	var endpoint Endpoint
	var err error
	if net.ParseIP(host) != nil {
		if allowHTTP {
			endpoint = Endpoint{Scheme: "http", Port: 8080}
		} else {
			endpoint = Endpoint{Scheme: "https", Port: 8443}
		}
	} else if strings.HasSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".local") {
		endpoint, err = resolveMulticast(ctx, host, allowHTTP)
	} else {
		endpoint, err = resolveDNS(ctx, host, allowHTTP)
	}
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: endpoint.Scheme, Host: net.JoinHostPort(host, strconv.Itoa(endpoint.Port))}).String(), nil
}

func choose(secure, clear *Endpoint, allowHTTP bool) (Endpoint, error) {
	if allowHTTP && clear != nil {
		return *clear, nil
	}
	if secure != nil {
		return *secure, nil
	}
	if clear != nil {
		return Endpoint{}, errors.New("server advertises HTTP only; pass --allow-insecure-http on a trusted network")
	}
	return Endpoint{}, errors.New("no WebMux service advertised for this host; supply an explicit URL or publish DNS SRV records")
}

func resolveDNS(ctx context.Context, host string, allowHTTP bool) (Endpoint, error) {
	return resolveDNSWithLookup(ctx, host, allowHTTP, net.DefaultResolver.LookupSRV)
}

type srvLookup func(context.Context, string, string, string) (string, []*net.SRV, error)

func resolveDNSWithLookup(ctx context.Context, host string, allowHTTP bool, lookupSRV srvLookup) (Endpoint, error) {
	lookup := func(service string) *Endpoint {
		_, records, err := lookupSRV(ctx, service, "tcp", host)
		if err != nil || len(records) == 0 || records[0].Port == 0 || !strings.EqualFold(strings.TrimSuffix(records[0].Target, "."), host) {
			return nil
		}
		scheme := "https"
		if service == "webmux" {
			scheme = "http"
		}
		return &Endpoint{Scheme: scheme, Port: int(records[0].Port)}
	}
	secure := lookup("webmuxs")
	clear := lookup("webmux")
	chosen, err := choose(secure, clear, allowHTTP)
	if err == nil {
		return chosen, nil
	}
	if clear != nil {
		return Endpoint{}, err
	}
	// Older servers do not advertise ports. Their documented listener defaults
	// are still more useful than silently choosing HTTPS port 443.
	if allowHTTP {
		return Endpoint{Scheme: "http", Port: 8080}, nil
	}
	return Endpoint{Scheme: "https", Port: 8443}, nil
}

func resolveMulticast(ctx context.Context, host string, allowHTTP bool) (Endpoint, error) {
	listener4, listener6, err := multicastSockets()
	if err != nil {
		return Endpoint{}, err
	}
	defer closeSockets(listener4, listener6)
	server, err := mdns.NewServer(packet4(listener4), packet6(listener6), mdns.WithCacheRefresh(false))
	if err != nil {
		return Endpoint{}, err
	}
	defer server.Close()
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	found := make(chan mdns.ServiceEvent, 8)
	server.OnServiceDiscovered(func(event mdns.ServiceEvent) {
		select {
		case found <- event:
		default:
		}
	})
	if err := server.Browse(query, secureService); err != nil {
		return Endpoint{}, err
	}
	if err := server.Browse(query, clearService); err != nil {
		return Endpoint{}, err
	}
	var secure, clear *Endpoint
	preferred := secureService
	if allowHTTP {
		preferred = clearService
	}
	for {
		select {
		case event := <-found:
			if !strings.EqualFold(strings.TrimSuffix(event.Instance.Host, "."), strings.TrimSuffix(host, ".")) {
				continue
			}
			if event.Instance.Port == 0 {
				continue
			}
			endpoint := Endpoint{Port: int(event.Instance.Port)}
			switch event.Instance.Service {
			case secureService:
				endpoint.Scheme = "https"
				secure = &endpoint
			case clearService:
				endpoint.Scheme = "http"
				clear = &endpoint
			default:
				continue
			}
			if event.Instance.Service == preferred {
				return endpoint, nil
			}
		case <-query.Done():
			endpoint, chooseErr := choose(secure, clear, allowHTTP)
			if chooseErr == nil {
				return endpoint, nil
			}
			if clear != nil {
				return Endpoint{}, fmt.Errorf("discover %s: %w", host, chooseErr)
			}
			if allowHTTP {
				return Endpoint{Scheme: "http", Port: 8080}, nil
			}
			return Endpoint{Scheme: "https", Port: 8443}, nil
		}
	}
}
