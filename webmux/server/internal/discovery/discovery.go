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
	"sync"
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
	instance := host
	if len(instance) > 57 {
		instance = instance[:57]
	}
	conn4, conn6, err := multicastSockets()
	if err != nil {
		return nil, err
	}
	opts := []mdns.ServerOption{mdns.WithLocalNames(host + ".local")}
	if hasTLS {
		opts = append(opts, mdns.WithService(mdns.ServiceInstance{Instance: instance + " HTTPS", Service: secureService, Host: host + ".local.", Port: uint16(httpsPort)}))
	}
	opts = append(opts, mdns.WithService(mdns.ServiceInstance{Instance: instance + " HTTP", Service: clearService, Host: host + ".local.", Port: uint16(httpPort)}))
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
	listeners := &multicastListeners{}
	server.OnServiceDiscovered(func(event mdns.ServiceEvent) {
		listeners.observeHost(host, event)
	})
	if err := server.Browse(query, secureService); err != nil {
		return Endpoint{}, err
	}
	if err := server.Browse(query, clearService); err != nil {
		return Endpoint{}, err
	}
	return collectMulticastEndpoints(ctx, query, host, allowHTTP, listeners)
}

// ErrAmbiguousListener means host-only discovery cannot identify one listener.
// Callers must supply an explicit URL/port rather than select an arbitrary peer.
var ErrAmbiguousListener = errors.New("multiple WebMux listeners advertised for this host; supply an explicit URL with a port")

var errIncompleteListenerDiscovery = errors.New("WebMux listener discovery was incomplete; supply an explicit URL with a port")

const maxListenerInstances = 256

type listenerIdentity struct {
	service, instance string
}

type multicastListeners struct {
	mu         sync.Mutex
	instances  map[listenerIdentity]Endpoint
	incomplete bool
}

func matchingListenerEvent(host string, event mdns.ServiceEvent) bool {
	s := event.Instance
	return (s.Service == secureService || s.Service == clearService) && s.Port != 0 &&
		len(s.Instance) > 0 && len(s.Instance) <= 63 &&
		strings.EqualFold(strings.TrimSuffix(s.Host, "."), strings.TrimSuffix(host, "."))
}

// Accumulate listener state synchronously: address refreshes do not consume an
// event queue, and a flood cannot displace a different listener's observation.
func (listeners *multicastListeners) observeHost(host string, event mdns.ServiceEvent) {
	if !matchingListenerEvent(host, event) {
		return
	}
	listeners.mu.Lock()
	defer listeners.mu.Unlock()
	s := event.Instance
	key := listenerIdentity{service: s.Service, instance: s.Instance}
	if listeners.instances == nil {
		listeners.instances = make(map[listenerIdentity]Endpoint)
	}
	if _, exists := listeners.instances[key]; !exists && len(listeners.instances) >= maxListenerInstances {
		listeners.incomplete = true
		return
	}
	scheme := "https"
	if s.Service == clearService {
		scheme = "http"
	}
	// Updates replace the same instance; duplicate addresses and a changed port
	// on one instance must not be mistaken for multiple current listeners.
	listeners.instances[key] = Endpoint{Scheme: scheme, Port: int(s.Port)}
}

func (listeners *multicastListeners) choose(allowHTTP bool) (Endpoint, error) {
	listeners.mu.Lock()
	defer listeners.mu.Unlock()
	if listeners.incomplete {
		return Endpoint{}, errIncompleteListenerDiscovery
	}
	var secure, clear *Endpoint
	var secureAmbiguous, clearAmbiguous bool
	for _, endpoint := range listeners.instances {
		if endpoint.Scheme == "https" {
			secureAmbiguous = secureAmbiguous || (secure != nil && secure.Port != endpoint.Port)
			secure = &endpoint
		} else {
			clearAmbiguous = clearAmbiguous || (clear != nil && clear.Port != endpoint.Port)
			clear = &endpoint
		}
	}
	if (allowHTTP && clear != nil && clearAmbiguous) || ((!allowHTTP || clear == nil) && secureAmbiguous) {
		return Endpoint{}, ErrAmbiguousListener
	}
	endpoint, err := choose(secure, clear, allowHTTP)
	if err == nil || clear != nil {
		return endpoint, err
	}
	// Preserve documented defaults for older servers with no usable records.
	if allowHTTP {
		return Endpoint{Scheme: "http", Port: 8080}, nil
	}
	return Endpoint{Scheme: "https", Port: 8443}, nil
}

func collectMulticastEndpoints(parent, query context.Context, host string, allowHTTP bool, listeners *multicastListeners) (Endpoint, error) {
	// Every callback commits directly to the bounded state above. Wait for the
	// full observation window before taking one coherent selection snapshot.
	select {
	case <-parent.Done():
		return Endpoint{}, parent.Err()
	case <-query.Done():
	}
	if err := parent.Err(); err != nil {
		return Endpoint{}, err
	}
	endpoint, err := listeners.choose(allowHTTP)
	if err != nil {
		return Endpoint{}, fmt.Errorf("discover %s: %w", host, err)
	}
	return endpoint, nil
}
