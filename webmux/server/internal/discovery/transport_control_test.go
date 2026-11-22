package discovery

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// TestMulticastTransportControl diagnoses the native socket path independently
// of DNS-SD parsing. It intentionally fails when a selected interface cannot
// receive its own query. It is not a discovery or two-host acceptance test.
// Explicit interface names bound the probe and prevent a silent fallback.
func TestMulticastTransportControl(t *testing.T) {
	names := os.Getenv("WEBMUX_TEST_MDNS_CONTROL")
	if names == "" {
		t.Skip("set WEBMUX_TEST_MDNS_CONTROL to at most four comma-separated native interfaces")
	}
	interfaces := strings.Split(names, ",")
	if len(interfaces) > 4 {
		t.Fatal("at most four interfaces per bounded diagnostic run")
	}
	for _, name := range interfaces {
		t.Run(name, func(t *testing.T) {
			iface, err := net.InterfaceByName(name)
			if err != nil {
				t.Fatal(err)
			}
			if iface.Flags&net.FlagUp == 0 {
				t.Fatal("selected interface is down")
			}
			for _, family := range []string{"udp4", "udp6"} {
				t.Run(family, func(t *testing.T) { multicastTransportControl(t, iface, family) })
			}
		})
	}
}

func multicastTransportControl(t *testing.T, iface *net.Interface, family string) {
	t.Helper()
	v4, v6, err := multicastSockets()
	if err != nil {
		t.Fatal(err)
	}
	defer closeSockets(v4, v6)
	listener := v4
	group := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	if family == "udp6" {
		listener = v6
		group = &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353, Zone: iface.Name}
	}
	if listener == nil {
		t.Fatal("production multicast socket unavailable for selected family")
	}
	sender, err := net.ListenUDP(family, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	var send func([]byte) error
	var read func([]byte) (int, error)
	if family == "udp4" {
		r, s := ipv4.NewPacketConn(listener), ipv4.NewPacketConn(sender)
		if err := r.JoinGroup(iface, group); err != nil {
			t.Fatalf("join: %v", err)
		}
		if err := s.SetMulticastInterface(iface); err != nil {
			t.Fatalf("send interface: %v", err)
		}
		loop, err := s.MulticastLoopback()
		if err != nil || !loop {
			t.Fatalf("native sender multicast loopback: %t, %v", loop, err)
		}
		send = func(b []byte) error { _, err := s.WriteTo(b, nil, group); return err }
		read = func(b []byte) (int, error) { n, _, _, err := r.ReadFrom(b); return n, err }
	} else {
		r, s := ipv6.NewPacketConn(listener), ipv6.NewPacketConn(sender)
		if err := r.JoinGroup(iface, group); err != nil {
			t.Fatalf("join: %v", err)
		}
		if err := s.SetMulticastInterface(iface); err != nil {
			t.Fatalf("send interface: %v", err)
		}
		loop, err := s.MulticastLoopback()
		if err != nil || !loop {
			t.Fatalf("native sender multicast loopback: %t, %v", loop, err)
		}
		send = func(b []byte) error { _, err := s.WriteTo(b, nil, group); return err }
		read = func(b []byte) (int, error) { n, _, _, err := r.ReadFrom(b); return n, err }
	}
	// A unique, valid question avoids mistaking ambient traffic for the exact
	// query. No credentials or discovered host addresses enter the diagnostic.
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	query := dnsmessage.Message{Questions: []dnsmessage.Question{{
		Name: dnsmessage.MustNewName(fmt.Sprintf("_webmux-control-%x._udp.local.", nonce)),
		Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET,
	}}}
	wire, err := query.Pack()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	if err := listener.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := sender.SetWriteDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := send(wire); err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Log("production socket joined; native loopback enabled; unique DNS question sent")
	buffer := make([]byte, 65535)
	for time.Now().Before(deadline) {
		n, err := read(buffer)
		if err != nil {
			t.Fatalf("exact question not received: %v", err)
		}
		if bytes.Equal(buffer[:n], wire) {
			t.Log("exact question received; socket transport control only")
			return
		}
	}
	t.Fatal("exact question not received before original one-second deadline")
}
