package discovery

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/pion/mdns/v2"
)

func nodeEvent() mdns.ServiceEvent {
	s, _ := nodeService(NodeOptions{ID: strings.Repeat("a", 32), Host: "builder", Port: 8443, TLS: true})
	s.Domain = "local"
	return mdns.ServiceEvent{Instance: s, Addr: netip.MustParseAddr("192.0.2.10")}
}
func TestNodeCandidateValidation(t *testing.T) {
	for name, change := range map[string]func(*mdns.ServiceEvent){
		"other service":    func(e *mdns.ServiceEvent) { e.Instance.Service = clearService },
		"other domain":     func(e *mdns.ServiceEvent) { e.Instance.Domain = "example.org" },
		"self":             func(e *mdns.ServiceEvent) { e.Instance.Text[1].Value = []byte(strings.Repeat("b", 32)) },
		"bad version":      func(e *mdns.ServiceEvent) { e.Instance.Text[0].Value = []byte("2") },
		"missing identity": func(e *mdns.ServiceEvent) { e.Instance.Text = e.Instance.Text[:1] },
		"invalid identity": func(e *mdns.ServiceEvent) { e.Instance.Text[1].Value = []byte("../identity") },
		"duplicate keys":   func(e *mdns.ServiceEvent) { e.Instance.Text[2] = e.Instance.Text[0] },
		"extra secret": func(e *mdns.ServiceEvent) {
			e.Instance.Text = append(e.Instance.Text, mdns.TXTEntry{Key: "token", Value: []byte("private")})
		},
		"invalid tls":         func(e *mdns.ServiceEvent) { e.Instance.Text[2].Value = []byte("yes") },
		"zero port":           func(e *mdns.ServiceEvent) { e.Instance.Port = 0 },
		"userinfo":            func(e *mdns.ServiceEvent) { e.Instance.Host = "user@host.local." },
		"path":                func(e *mdns.ServiceEvent) { e.Instance.Host = "host/path.local." },
		"external hostname":   func(e *mdns.ServiceEvent) { e.Instance.Host = "external.example." },
		"control display":     func(e *mdns.ServiceEvent) { e.Instance.Instance = "name\nforged" },
		"unspecified":         func(e *mdns.ServiceEvent) { e.Addr = netip.IPv4Unspecified() },
		"multicast":           func(e *mdns.ServiceEvent) { e.Addr = netip.MustParseAddr("224.0.0.251") },
		"loopback":            func(e *mdns.ServiceEvent) { e.Addr = netip.MustParseAddr("::1") },
		"unscoped link local": func(e *mdns.ServiceEvent) { e.Addr = netip.MustParseAddr("fe80::1") },
	} {
		t.Run(name, func(t *testing.T) {
			b := &NodeBrowser{candidates: map[string]Candidate{}}
			e := nodeEvent()
			change(&e)
			b.observe(e, strings.Repeat("b", 32), time.Now())
			if len(b.Snapshot(time.Now())) != 0 {
				t.Fatal("admitted invalid announcement")
			}
		})
	}
}
func TestNodeCandidateRefreshExpiryAndSpoofIsolation(t *testing.T) {
	now := time.Now()
	b := &NodeBrowser{candidates: map[string]Candidate{}}
	e := nodeEvent()
	b.observe(e, "", now)
	// Equal claimed IDs do not merge addresses or imply trust; duplicate names
	// remain distinct, and scoped IPv6 must preserve its receiving interface.
	e.Addr = netip.MustParseAddr("fe80::10%en0")
	b.observe(e, "", now)
	if got := b.Snapshot(now); len(got) != 2 || got[1].ClaimedID != strings.Repeat("a", 32) {
		t.Fatal(got)
	}
	b.observe(e, "", now.Add(60*time.Second))
	got := b.Snapshot(now.Add(candidateLifetime))
	if len(got) != 1 || got[0].Address != "fe80::10%en0" {
		t.Fatal(got)
	}
	got[0].Host = "mutated"
	if b.Snapshot(now.Add(candidateLifetime))[0].Host == "mutated" {
		t.Fatal("snapshot aliases registry")
	}
	if got := b.Snapshot(now.Add(150 * time.Second)); len(got) != 0 {
		t.Fatal("departed node retained", got)
	}
}
func TestNodeCandidateBoundAndConcurrency(t *testing.T) {
	now := time.Now()
	b := &NodeBrowser{candidates: map[string]Candidate{}}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				e := nodeEvent()
				e.Instance.Instance = fmt.Sprintf("node-%d-%d", worker, n)
				b.observe(e, "", now)
				b.Snapshot(now)
			}
		}(worker)
	}
	wg.Wait()
	if len(b.Snapshot(now)) != maxCandidates {
		t.Fatal("registry not bounded")
	}
	e := nodeEvent()
	b.observe(e, "", now.Add(candidateLifetime))
	if len(b.Snapshot(now.Add(candidateLifetime))) != 1 {
		t.Fatal("expired capacity not reclaimed")
	}
}
func TestNodePolicyIdentityAndRefusal(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	store := &storage.Store{Home: home}
	p, err := LoadNodePolicy(store)
	if err != nil || !ValidNodeID(p.ID) || !p.Browse || !p.Advertise {
		t.Fatal(p, err)
	}
	again, err := LoadNodePolicy(store)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("identity changed", again, err)
	}
	p.Browse = false
	p.Advertise = false
	p.Interfaces = []string{"private0"}
	if err := store.WriteConfig("node-discovery.yaml", p); err != nil {
		t.Fatal(err)
	}
	again, err = LoadNodePolicy(store)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("policy broadened", again, err)
	}
	p.ID = "invalid"
	if err := store.WriteConfig("node-discovery.yaml", p); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNodePolicy(store); err == nil {
		t.Fatal("invalid identity replaced")
	}
	var saved NodePolicy
	if err := store.ReadConfig("node-discovery.yaml", &saved); err != nil || saved.ID != "invalid" {
		t.Fatal("invalid configuration mutated", err)
	}
}
func TestNodeServicePolicyAndCancellation(t *testing.T) {
	b, err := StartNodes(context.Background(), NodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	for _, o := range []NodeOptions{{ID: "invalid", Port: 8443}, {ID: strings.Repeat("a", 32), Port: 65536}, {ID: strings.Repeat("a", 32), Port: 8443, Host: "bad/name"}} {
		if _, err := nodeService(o); err == nil {
			t.Fatal("invalid advertisement admitted", o)
		}
	}
	o := NodeOptions{ID: strings.Repeat("a", 32), Host: "builder", Port: 8443, Browse: true, Interfaces: []string{"webmux-nonexistent-interface"}}
	if _, err := StartNodes(context.Background(), o); err == nil {
		t.Fatal("explicit interface scope silently broadened")
	}
}
func TestNodeMulticastRoundTrip(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_MDNS") == "" {
		t.Skip("requires multicast; set WEBMUX_TEST_MDNS=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a, err := StartNodes(ctx, NodeOptions{ID: strings.Repeat("c", 32), Host: "webmux-node-fixture", HTTPPort: 19080, Port: 19443, TLS: true, Advertise: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := StartNodes(ctx, NodeOptions{ID: strings.Repeat("d", 32), Host: "webmux-browser-fixture", Port: 19080, Browse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("node advertisement not discovered")
		case <-tick.C:
			for _, c := range b.Snapshot(time.Now()) {
				if c.ClaimedID == strings.Repeat("c", 32) {
					if c.Port != 19443 || !c.TLS {
						t.Fatal(c)
					}
					// The server still supplies the existing client lookup services,
					// now governed by the same interface/advertisement policy.
					for _, allowHTTP := range []bool{false, true} {
						got, err := Resolve(ctx, "webmux-node-fixture.local", allowHTTP)
						want := "https://webmux-node-fixture.local:19443"
						if allowHTTP {
							want = "http://webmux-node-fixture.local:19080"
						}
						if err != nil || got != want {
							t.Fatalf("legacy lookup: got %q, %v; want %q", got, err, want)
						}
					}
					return
				}
			}
		}
	}
}
