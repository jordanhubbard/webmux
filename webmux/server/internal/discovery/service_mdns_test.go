package discovery

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/dns/dnsmessage"
)

func mustQuery(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	raw, err := (&dnsmessage.Message{Questions: []dnsmessage.Question{{
		Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET,
	}}}).Pack()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func unpacked(t *testing.T, raw []byte) dnsmessage.Message {
	t.Helper()
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		t.Fatal(err)
	}
	return msg
}

func assertNoHostnameRecord(t *testing.T, msg dnsmessage.Message) {
	t.Helper()
	for _, rr := range append(append([]dnsmessage.Resource{}, msg.Answers...), msg.Additionals...) {
		if rr.Header.Type == dnsmessage.TypeA || rr.Header.Type == dnsmessage.TypeAAAA {
			t.Fatalf("published hostname address record %v for %s", rr.Header.Type, rr.Header.Name)
		}
	}
}

func TestServiceAdvertisementDoesNotClaimHostname(t *testing.T) {
	node, err := nodeService(NodeOptions{ID: strings.Repeat("ab", 16), Host: "sparky", Port: 8443, TLS: true})
	if err != nil {
		t.Fatal(err)
	}
	listeners, err := listenerServices("sparky", 8080, 8443, true)
	if err != nil {
		t.Fatal(err)
	}
	services := append([]mdns.ServiceInstance{node}, listeners...)
	src := &net.UDPAddr{IP: net.ParseIP("192.0.2.40"), Port: 5353}
	for _, query := range []struct {
		name string
		typ  dnsmessage.Type
	}{
		{"sparky.local.", dnsmessage.TypeA},
		{"sparky.local.", dnsmessage.TypeAAAA},
		{"sparky.local.", dnsmessage.TypeALL},
	} {
		raw, dst, ok := responseToQuery(services, mustQuery(t, query.name, query.typ), src)
		if ok || raw != nil || dst != nil {
			t.Fatalf("answered %s %v for the system hostname", query.name, query.typ)
		}
	}

	combined, err := (&dnsmessage.Message{Questions: []dnsmessage.Question{
		{Name: dnsmessage.MustNewName("_webmux._tcp.local."), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET},
		{Name: dnsmessage.MustNewName("sparky.local."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
	}}).Pack()
	if err != nil {
		t.Fatal(err)
	}
	raw, _, ok := responseToQuery(services, combined, src)
	if !ok {
		t.Fatal("service query produced no advertisement")
	}
	msg := unpacked(t, raw)
	assertNoHostnameRecord(t, msg)

	for _, service := range []string{"_webmux-node._tcp.local.", "_webmux._tcp.local.", "_webmuxs._tcp.local."} {
		raw, _, ok = responseToQuery(services, mustQuery(t, service, dnsmessage.TypePTR), src)
		if !ok {
			t.Fatalf("no PTR response for %s", service)
		}
		msg = unpacked(t, raw)
		assertNoHostnameRecord(t, msg)
		var srv *dnsmessage.SRVResource
		for _, rr := range append(msg.Answers, msg.Additionals...) {
			if rr.Header.Type == dnsmessage.TypeSRV {
				body, _ := rr.Body.(*dnsmessage.SRVResource)
				srv = body
			}
		}
		if srv == nil || srv.Target.String() != "sparky.local." {
			t.Fatalf("%s SRV target = %+v, want sparky.local.", service, srv)
		}
	}

	instance := node.Instance + "." + NodeService + ".local."
	raw, _, ok = responseToQuery(services, mustQuery(t, instance, dnsmessage.TypeALL), src)
	if !ok {
		t.Fatal("instance query produced no service records")
	}
	msg = unpacked(t, raw)
	assertNoHostnameRecord(t, msg)
	sawSRV, sawTXT := false, false
	for _, rr := range msg.Answers {
		switch rr.Header.Type {
		case dnsmessage.TypeSRV:
			sawSRV = true
		case dnsmessage.TypeTXT:
			sawTXT = true
		}
	}
	if !sawSRV || !sawTXT {
		t.Fatalf("instance response missing SRV or TXT: %+v", msg.Answers)
	}

	asm := &serviceAssembler{browse: []string{NodeService, clearService, secureService}}
	events := asm.ingest(append(append([]dnsmessage.Resource{}, msg.Answers...), msg.Additionals...), netip.MustParseAddr("192.0.2.40"))
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Addr.String() != "192.0.2.40" || events[0].Instance.Port != 8443 {
		t.Fatalf("event = %+v", events[0])
	}
	if !strings.EqualFold(strings.TrimSuffix(events[0].Instance.Host, "."), "sparky.local") {
		t.Fatalf("host = %s", events[0].Instance.Host)
	}
	if len(events[0].Instance.Text) != 3 {
		t.Fatalf("txt = %+v", events[0].Instance.Text)
	}
}
