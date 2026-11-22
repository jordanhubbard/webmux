package discovery

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pion/mdns/v2"
)

// NodeService identifies candidate servers, separately from client port lookup.
// TXT identity is an unverified claim and grants no authority.
const NodeService = "_webmux-node._tcp"
const candidateLifetime = 90 * time.Second
const maxCandidates = 256

// NodeOptions is server-wide startup policy. Empty Interfaces selects all
// suitable multicast interfaces. Explicit names fail closed if unavailable.
type NodeOptions struct {
	ID         string
	Browse     bool
	Advertise  bool
	Interfaces []string
	Host       string
	HTTPPort   int // Optional legacy HTTP listener advertisement, under the same policy.
	Port       int
	TLS        bool
}

// Candidate is untrusted network input. These fields must never automatically
// change a configured peer's URL or be treated as authenticated identity.
type Candidate struct {
	Instance  string    `json:"instance"`
	ClaimedID string    `json:"claimed_id"`
	Host      string    `json:"host"`
	Address   string    `json:"address"`
	Port      uint16    `json:"port"`
	TLS       bool      `json:"tls"`
	LastSeen  time.Time `json:"last_seen"`
	Expires   time.Time `json:"expires"`
}

type NodeBrowser struct {
	mu         sync.Mutex
	candidates map[string]Candidate
	cancel     context.CancelFunc
	conn       *mdns.Conn
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

func ValidNodeID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func nodeService(o NodeOptions) (mdns.ServiceInstance, error) {
	if !ValidNodeID(o.ID) || o.Port < 1 || o.Port > 65535 {
		return mdns.ServiceInstance{}, errors.New("invalid node identity or listener port")
	}
	host := strings.ToLower(strings.TrimSuffix(o.Host, "."))
	if !strings.HasSuffix(host, ".local") {
		host += ".local"
	}
	if len(host) > 253 {
		return mdns.ServiceInstance{}, errors.New("invalid node hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return mdns.ServiceInstance{}, errors.New("invalid node hostname")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return mdns.ServiceInstance{}, errors.New("invalid node hostname")
			}
		}
	}
	tls := "0"
	if o.TLS {
		tls = "1"
	}
	return mdns.ServiceInstance{Instance: "WebMux-" + o.ID, Service: NodeService, Host: host + ".", Port: uint16(o.Port), Text: []mdns.TXTEntry{{Key: "v", Value: []byte("1")}, {Key: "id", Value: []byte(o.ID)}, {Key: "tls", Value: []byte(tls)}}}, nil
}

// StartNodes makes no HTTP requests and sends no credentials. Short browse
// rounds compensate for library deduplication and absent departure events.
func StartNodes(parent context.Context, o NodeOptions) (*NodeBrowser, error) {
	b := &NodeBrowser{candidates: make(map[string]Candidate), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(parent)
	b.cancel = cancel
	if !o.Browse && !o.Advertise {
		close(b.done)
		return b, nil
	}
	svc, err := nodeService(o)
	if err != nil {
		cancel()
		return nil, err
	}
	options := []mdns.ServerOption{mdns.WithCacheRefresh(false)}
	if len(o.Interfaces) != 0 {
		interfaces := make([]net.Interface, 0, len(o.Interfaces))
		for _, name := range o.Interfaces {
			iface, err := net.InterfaceByName(name)
			if err != nil || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
				cancel()
				return nil, fmt.Errorf("unavailable multicast interface %q", name)
			}
			interfaces = append(interfaces, *iface)
		}
		options = append(options, mdns.WithInterfaces(interfaces...))
	}
	if o.Advertise {
		options = append(options, mdns.WithLocalNames(svc.Host), mdns.WithService(svc), mdns.WithResponseTTL(60))
		if o.HTTPPort > 0 {
			if o.HTTPPort > 65535 {
				cancel()
				return nil, errors.New("invalid HTTP listener port")
			}
			options = append(options, mdns.WithService(mdns.ServiceInstance{Instance: svc.Instance + " HTTP", Service: clearService, Host: svc.Host, Port: uint16(o.HTTPPort)}))
			if o.TLS {
				options = append(options, mdns.WithService(mdns.ServiceInstance{Instance: svc.Instance + " HTTPS", Service: secureService, Host: svc.Host, Port: svc.Port}))
			}
		}
	}
	v4, v6, err := multicastSockets()
	if err != nil {
		cancel()
		return nil, err
	}
	b.conn, err = mdns.NewServer(packet4(v4), packet6(v6), options...)
	if err != nil {
		cancel()
		closeSockets(v4, v6)
		return nil, err
	}
	b.conn.OnServiceDiscovered(func(e mdns.ServiceEvent) { b.observe(e, o.ID, time.Now()) })
	go func() {
		defer close(b.done)
		if !o.Browse {
			<-ctx.Done()
			return
		}
		for ctx.Err() == nil {
			round, finish := context.WithTimeout(ctx, 30*time.Second)
			if err := b.conn.Browse(round, NodeService); err != nil {
				finish()
				return
			}
			<-round.Done()
			finish()
		}
	}()
	return b, nil
}

func (b *NodeBrowser) Close() error {
	b.closeOnce.Do(func() {
		b.cancel()
		if b.conn != nil {
			b.closeErr = b.conn.Close()
		}
		<-b.done
	})
	return b.closeErr
}

func candidateKey(c Candidate) string { return c.Instance + "\x00" + c.Address }
func (b *NodeBrowser) observe(e mdns.ServiceEvent, self string, now time.Time) {
	s := e.Instance
	if s.Service != NodeService || !strings.EqualFold(strings.TrimSuffix(s.Domain, "."), "local") || s.Port == 0 || !e.Addr.IsValid() || e.Addr.IsUnspecified() || e.Addr.IsMulticast() || e.Addr.IsLoopback() {
		return
	}
	if e.Addr.IsLinkLocalUnicast() && e.Addr.Is6() && e.Addr.Zone() == "" {
		return
	}
	if !utf8.ValidString(s.Instance) || len(s.Instance) == 0 || len(s.Instance) > 63 || strings.ContainsFunc(s.Instance, unicode.IsControl) {
		return
	}
	if len(s.Host) > 254 || !strings.HasSuffix(strings.ToLower(s.Host), ".local.") {
		return
	}
	if _, err := nodeService(NodeOptions{ID: strings.Repeat("a", 32), Host: s.Host, Port: int(s.Port)}); err != nil {
		return
	}
	text := map[string]string{}
	if len(s.Text) != 3 {
		return
	}
	for _, entry := range s.Text {
		key := strings.ToLower(entry.Key)
		if _, ok := text[key]; ok {
			return
		}
		text[key] = string(entry.Value)
	}
	if text["v"] != "1" || !ValidNodeID(text["id"]) || text["id"] == self || (text["tls"] != "0" && text["tls"] != "1") {
		return
	}
	c := Candidate{Instance: s.Instance, ClaimedID: text["id"], Host: strings.TrimSuffix(strings.ToLower(s.Host), "."), Address: e.Addr.String(), Port: s.Port, TLS: text["tls"] == "1", LastSeen: now, Expires: now.Add(candidateLifetime)}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)
	key := candidateKey(c)
	if _, exists := b.candidates[key]; !exists && len(b.candidates) >= maxCandidates {
		return
	}
	b.candidates[key] = c
}
func (b *NodeBrowser) prune(now time.Time) {
	for k, c := range b.candidates {
		if !now.Before(c.Expires) {
			delete(b.candidates, k)
		}
	}
}

// Snapshot keeps addresses separate until authenticated identity reconciliation.
func (b *NodeBrowser) Snapshot(now time.Time) []Candidate {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)
	out := make([]Candidate, 0, len(b.candidates))
	for _, c := range b.candidates {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return candidateKey(out[i]) < candidateKey(out[j]) })
	return out
}
