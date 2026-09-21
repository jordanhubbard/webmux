package netguard

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestTargetPolicy(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "127.255.1.2", "0.1.2.3", "169.254.1.2", "224.0.0.1", "255.255.255.255", "::", "::1", "fe80::1", "febf::1", "ff02::1", "::ffff:7f00:1"} {
		if !Blocked(netip.MustParseAddr(ip)) {
			t.Errorf("blocked range accepted: %s", ip)
		}
	}
	for _, ip := range []string{"10.1.2.3", "172.16.0.1", "192.168.1.1", "192.0.2.1", "2001:db8::1", "fd00::1"} {
		if Blocked(netip.MustParseAddr(ip)) {
			t.Errorf("supported private/routable address blocked: %s", ip)
		}
	}
}
func TestResolvedAddressIsPinnedAndErrorsFailClosed(t *testing.T) {
	g := New(false)
	calls := 0
	g.lookup = func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		calls++
		if host != "desktop.invalid" || network != "ip" {
			t.Fatal(network, host)
		}
		return []netip.Addr{netip.MustParseAddr("192.0.2.42")}, nil
	}
	target, err := g.Resolve(context.Background(), "desktop.invalid")
	if err != nil || target != "192.0.2.42" || calls != 1 {
		t.Fatal(target, err, calls)
	}
	g.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	if _, err := g.Resolve(context.Background(), "desktop.invalid"); !errors.Is(err, ErrBlocked) {
		t.Fatal(err)
	}
	for _, host := range []string{"", "bad host", "-option", "name/path", "[fe80::1%eth0]"} {
		if _, err := g.Resolve(context.Background(), host); !errors.Is(err, ErrHostname) {
			t.Fatal(host, err)
		}
	}
	g.lookup = func(ctx context.Context, _, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Resolve(ctx, "desktop.invalid"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	g.AllowLocal = true
	if target, err := g.Resolve(context.Background(), "[::1]"); err != nil || target != "::1" {
		t.Fatal(target, err)
	}
}
