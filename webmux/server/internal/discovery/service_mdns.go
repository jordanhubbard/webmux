package discovery

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// The system mDNS responder (Avahi, systemd-resolved, or Bonjour) is the
// hostname authority. Service advertisements are PTR, SRV, and TXT only.
// Address records for the hostname are never sent: an A or AAAA answer for
// the host makes the system responder treat the name as contested and rename
// it (sparky.local becomes sparky-2.local).

const (
	ptrTTL          uint32           = 4500
	serviceTTL      uint32           = 120
	classQU         dnsmessage.Class = 1 << 15
	classCacheFlush dnsmessage.Class = 1 << 15
	serviceEnumName                  = "_services._dns-sd._udp.local."
	mdnsPort                         = 5353
)

func listenerServices(host string, httpPort, httpsPort int, hasTLS bool) ([]mdns.ServiceInstance, error) {
	host = strings.Split(strings.TrimSuffix(strings.TrimSuffix(host, "."), ".local"), ".")[0]
	if host == "" || strings.ContainsAny(host, ":/") {
		return nil, errors.New("invalid mDNS host name")
	}
	instance := host
	if len(instance) > 57 {
		instance = instance[:57]
	}
	hostName := host + ".local."
	var services []mdns.ServiceInstance
	if hasTLS {
		services = append(services, mdns.ServiceInstance{Instance: instance + " HTTPS", Service: secureService, Host: hostName, Port: uint16(httpsPort)})
	}
	services = append(services, mdns.ServiceInstance{Instance: instance + " HTTP", Service: clearService, Host: hostName, Port: uint16(httpPort)})
	return services, nil
}

func advertisedNames(svc mdns.ServiceInstance) (instance, service, host string) {
	domain := strings.Trim(strings.TrimSuffix(svc.Domain, "."), ".")
	if domain == "" {
		domain = "local"
	}
	service = svc.Service + "." + domain + "."
	instance = escapeDNSLabel(svc.Instance) + "." + service
	host = strings.TrimSpace(svc.Host)
	if !strings.HasSuffix(host, ".") {
		host += "."
	}
	return instance, service, host
}

func escapeDNSLabel(label string) string {
	var b strings.Builder
	for i := 0; i < len(label); i++ {
		if label[i] == '.' || label[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(label[i])
	}
	return b.String()
}

func responseToQuery(services []mdns.ServiceInstance, query []byte, src *net.UDPAddr) ([]byte, *net.UDPAddr, bool) {
	var msg dnsmessage.Message
	if err := msg.Unpack(query); err != nil || msg.Header.Response || len(msg.Questions) == 0 {
		return nil, nil, false
	}
	unicast := src != nil && src.Port != mdnsPort
	for _, q := range msg.Questions {
		if q.Class&classQU != 0 {
			unicast = true
		}
	}
	var answers, extras []dnsmessage.Resource
	var questions []dnsmessage.Question
	for _, q := range msg.Questions {
		a, e := recordsForQuestion(services, q)
		answers = append(answers, a...)
		extras = append(extras, e...)
		if unicast {
			q.Class &^= classQU
			questions = append(questions, q)
		}
	}
	answers = withoutHostnameRecords(answers)
	extras = withoutHostnameRecords(extras)
	if len(answers) == 0 {
		return nil, nil, false
	}
	raw, err := (&dnsmessage.Message{
		Header:      dnsmessage.Header{ID: msg.Header.ID, Response: true, Authoritative: true},
		Questions:   questions,
		Answers:     answers,
		Additionals: extras,
	}).Pack()
	if err != nil {
		return nil, nil, false
	}
	return raw, replyDestination(src, unicast), true
}

func withoutHostnameRecords(in []dnsmessage.Resource) []dnsmessage.Resource {
	out := make([]dnsmessage.Resource, 0, len(in))
	for _, rr := range in {
		if rr.Header.Type == dnsmessage.TypeA || rr.Header.Type == dnsmessage.TypeAAAA {
			continue
		}
		out = append(out, rr)
	}
	return out
}

func recordsForQuestion(services []mdns.ServiceInstance, q dnsmessage.Question) (answers, extra []dnsmessage.Resource) {
	if q.Class&^classQU != dnsmessage.ClassINET {
		return nil, nil
	}
	if q.Type == dnsmessage.TypeA || q.Type == dnsmessage.TypeAAAA {
		return nil, nil
	}
	qname := strings.ToLower(q.Name.String())
	for _, svc := range services {
		instance, serviceName, host := advertisedNames(svc)
		ptr, srv, txt, err := serviceRecords(svc, instance, serviceName, host)
		if err != nil {
			continue
		}
		if qname == strings.ToLower(serviceName) && (q.Type == dnsmessage.TypePTR || q.Type == dnsmessage.TypeALL) {
			answers = append(answers, ptr)
			extra = append(extra, srv, txt)
		}
		if qname == serviceEnumName && (q.Type == dnsmessage.TypePTR || q.Type == dnsmessage.TypeALL) {
			enum, err := ptrRecord(serviceEnumName, serviceName, ptrTTL)
			if err == nil {
				answers = append(answers, enum)
			}
		}
		if qname == strings.ToLower(instance) && (q.Type == dnsmessage.TypeSRV || q.Type == dnsmessage.TypeALL) {
			answers = append(answers, srv)
		}
		if qname == strings.ToLower(instance) && (q.Type == dnsmessage.TypeTXT || q.Type == dnsmessage.TypeALL) {
			answers = append(answers, txt)
		}
	}
	return answers, extra
}

func serviceRecords(svc mdns.ServiceInstance, instance, serviceName, host string) (ptr, srv, txt dnsmessage.Resource, err error) {
	ptr, err = ptrRecord(serviceName, instance, ptrTTL)
	if err != nil {
		return dnsmessage.Resource{}, dnsmessage.Resource{}, dnsmessage.Resource{}, err
	}
	srv, err = srvRecord(instance, host, svc.Port, svc.Priority, svc.Weight)
	if err != nil {
		return dnsmessage.Resource{}, dnsmessage.Resource{}, dnsmessage.Resource{}, err
	}
	txt, err = txtRecord(instance, svc.Text)
	if err != nil {
		return dnsmessage.Resource{}, dnsmessage.Resource{}, dnsmessage.Resource{}, err
	}
	return ptr, srv, txt, nil
}

func ptrRecord(name, target string, ttl uint32) (dnsmessage.Resource, error) {
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return dnsmessage.Resource{}, err
	}
	tgt, err := dnsmessage.NewName(target)
	if err != nil {
		return dnsmessage.Resource{}, err
	}
	return dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: n, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET, TTL: ttl},
		Body:   &dnsmessage.PTRResource{PTR: tgt},
	}, nil
}

func srvRecord(name, target string, port, priority, weight uint16) (dnsmessage.Resource, error) {
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return dnsmessage.Resource{}, err
	}
	tgt, err := dnsmessage.NewName(target)
	if err != nil {
		return dnsmessage.Resource{}, err
	}
	return dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: n, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET | classCacheFlush, TTL: serviceTTL},
		Body:   &dnsmessage.SRVResource{Priority: priority, Weight: weight, Port: port, Target: tgt},
	}, nil
}

func txtRecord(name string, text []mdns.TXTEntry) (dnsmessage.Resource, error) {
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return dnsmessage.Resource{}, err
	}
	values := make([]string, 0, len(text))
	if len(text) == 0 {
		values = []string{""}
	}
	for _, entry := range text {
		values = append(values, entry.Key+"="+string(entry.Value))
	}
	return dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: n, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET | classCacheFlush, TTL: serviceTTL},
		Body:   &dnsmessage.TXTResource{TXT: values},
	}, nil
}

func replyDestination(src *net.UDPAddr, unicast bool) *net.UDPAddr {
	if unicast && src != nil {
		return src
	}
	if src != nil && src.IP != nil && src.IP.To4() == nil {
		return &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: mdnsPort}
	}
	return &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: mdnsPort}
}

type partialService struct {
	instance, service, domain, host string
	port, priority, weight          uint16
	text                            []mdns.TXTEntry
	hasSRV                          bool
	addr                            netip.Addr
	explicitAddr                    bool
}

func (p *partialService) event() (mdns.ServiceEvent, bool) {
	if !p.hasSRV || p.port == 0 || !p.addr.IsValid() || p.addr.IsUnspecified() || p.addr.IsMulticast() {
		return mdns.ServiceEvent{}, false
	}
	return mdns.ServiceEvent{
		Instance: mdns.ServiceInstance{
			Instance: p.instance, Service: p.service, Domain: p.domain, Host: p.host,
			Port: p.port, Priority: p.priority, Weight: p.weight,
			Text: append([]mdns.TXTEntry(nil), p.text...),
		},
		Addr: p.addr,
	}, true
}

type serviceAssembler struct {
	browse  []string
	mu      sync.Mutex
	partial map[string]*partialService
}

func (a *serviceAssembler) wanted(service string) bool {
	for _, name := range a.browse {
		if strings.EqualFold(name, service) {
			return true
		}
	}
	return false
}

func (a *serviceAssembler) slot(instance, service, domain string) *partialService {
	key := strings.ToLower(instance) + "\x00" + strings.ToLower(service)
	p := a.partial[key]
	if p == nil {
		p = &partialService{instance: instance, service: service, domain: domain}
		a.partial[key] = p
	}
	return p
}

// ingest turns observed service records into events without requiring an
// address record from the advertiser. The packet source is the address when
// the system responder has not supplied one.
func (a *serviceAssembler) ingest(records []dnsmessage.Resource, src netip.Addr) []mdns.ServiceEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.partial == nil {
		a.partial = map[string]*partialService{}
	}
	touched := map[*partialService]bool{}
	for _, rr := range records {
		name := rr.Header.Name.String()
		switch rr.Header.Type {
		case dnsmessage.TypePTR:
			body, ok := rr.Body.(*dnsmessage.PTRResource)
			if !ok {
				continue
			}
			instance, service, domain, ok := parseInstanceFQDN(body.PTR.String())
			if !ok || !strings.EqualFold(domain, "local") || !a.wanted(service) {
				continue
			}
			touched[a.slot(instance, service, domain)] = true
		case dnsmessage.TypeSRV:
			body, ok := rr.Body.(*dnsmessage.SRVResource)
			if !ok {
				continue
			}
			instance, service, domain, ok := parseInstanceFQDN(name)
			if !ok || !a.wanted(service) {
				continue
			}
			p := a.slot(instance, service, domain)
			p.host = body.Target.String()
			p.port = body.Port
			p.priority = body.Priority
			p.weight = body.Weight
			p.hasSRV = true
			touched[p] = true
		case dnsmessage.TypeTXT:
			body, ok := rr.Body.(*dnsmessage.TXTResource)
			if !ok {
				continue
			}
			instance, service, domain, ok := parseInstanceFQDN(name)
			if !ok || !a.wanted(service) {
				continue
			}
			p := a.slot(instance, service, domain)
			p.text = txtEntries(body.TXT)
			touched[p] = true
		case dnsmessage.TypeA, dnsmessage.TypeAAAA:
			addr, ok := addrFromResource(rr)
			if !ok {
				continue
			}
			for _, p := range a.partial {
				if p.hasSRV && strings.EqualFold(strings.TrimSuffix(p.host, "."), strings.TrimSuffix(name, ".")) {
					p.addr = addr
					p.explicitAddr = true
					touched[p] = true
				}
			}
		}
	}
	if src.IsValid() && !src.IsUnspecified() && !src.IsMulticast() && !src.IsLoopback() {
		for p := range touched {
			if !p.explicitAddr {
				p.addr = src
			}
		}
	}
	var events []mdns.ServiceEvent
	for p := range touched {
		if event, ok := p.event(); ok {
			events = append(events, event)
		}
	}
	return events
}

func txtEntries(values []string) []mdns.TXTEntry {
	var text []mdns.TXTEntry
	for _, raw := range values {
		if raw == "" {
			continue
		}
		key, value, _ := strings.Cut(raw, "=")
		text = append(text, mdns.TXTEntry{Key: key, Value: []byte(value)})
	}
	return text
}

func addrFromResource(rr dnsmessage.Resource) (netip.Addr, bool) {
	switch body := rr.Body.(type) {
	case *dnsmessage.AResource:
		addr, ok := netip.AddrFromSlice(body.A[:])
		if !ok {
			return netip.Addr{}, false
		}
		return addr.Unmap(), true
	case *dnsmessage.AAAAResource:
		return netip.AddrFromSlice(body.AAAA[:])
	default:
		return netip.Addr{}, false
	}
}

func parseInstanceFQDN(name string) (instance, service, domain string, ok bool) {
	labels := splitDNSLabels(strings.TrimSuffix(name, "."))
	if len(labels) < 4 {
		return "", "", "", false
	}
	idx := -1
	for i := 1; i < len(labels)-1; i++ {
		if strings.HasPrefix(labels[i], "_") && strings.HasPrefix(labels[i+1], "_") {
			idx = i
			break
		}
	}
	if idx < 1 || idx+2 > len(labels) {
		return "", "", "", false
	}
	instance = strings.Join(labels[:idx], ".")
	service = labels[idx] + "." + labels[idx+1]
	domain = strings.Join(labels[idx+2:], ".")
	return instance, service, domain, instance != "" && service != "" && domain != ""
}

func splitDNSLabels(name string) []string {
	var labels []string
	var cur strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] == '\\' && i+3 < len(name) && isDigit(name[i+1]) && isDigit(name[i+2]) && isDigit(name[i+3]) {
			cur.WriteByte((name[i+1]-'0')*100 + (name[i+2]-'0')*10 + (name[i+3] - '0'))
			i += 3
			continue
		}
		if name[i] == '\\' && i+1 < len(name) {
			cur.WriteByte(name[i+1])
			i++
			continue
		}
		if name[i] == '.' {
			labels = append(labels, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(name[i])
	}
	labels = append(labels, cur.String())
	return labels
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

type serviceMDNS struct {
	cancel context.CancelFunc
	done   chan struct{}
	close  func()
}

func (s *serviceMDNS) Close() error {
	s.cancel()
	s.close()
	<-s.done
	return nil
}

func listenMDNS(network, address string) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var sockErr error
		err := raw.Control(func(fd uintptr) { sockErr = setReuseAddr(fd) })
		return errors.Join(err, sockErr)
	}}
	pc, err := lc.ListenPacket(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, errors.New("mDNS listener is not a UDP socket")
	}
	return conn, nil
}

func multicastInterfaces(selected []net.Interface) ([]net.Interface, error) {
	if len(selected) > 0 {
		return selected, nil
	}
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.Interface
	for _, ifi := range all {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		out = append(out, ifi)
	}
	if len(out) == 0 {
		return nil, errors.New("no multicast interface")
	}
	return out, nil
}

func startServiceMDNS(parent context.Context, services []mdns.ServiceInstance, ifaces []net.Interface, browse []string, queryEvery time.Duration, onEvent func(mdns.ServiceEvent), failed func(error)) (io.Closer, error) {
	if len(services) == 0 && len(browse) == 0 {
		return nil, errors.New("mDNS session has nothing to publish or browse")
	}
	ifaces, err := multicastInterfaces(ifaces)
	if err != nil {
		return nil, err
	}
	conn4, err4 := listenMDNS("udp4", mdns.DefaultAddressIPv4)
	conn6, err6 := listenMDNS("udp6", mdns.DefaultAddressIPv6)
	if conn4 == nil && conn6 == nil {
		return nil, errors.Join(err4, err6)
	}
	ctx, cancel := context.WithCancel(parent)
	session := &serviceMDNS{cancel: cancel, done: make(chan struct{})}
	var closeOnce sync.Once
	session.close = func() {
		closeOnce.Do(func() {
			if conn4 != nil {
				_ = conn4.Close()
			}
			if conn6 != nil {
				_ = conn6.Close()
			}
		})
	}
	fail := func(err error) (io.Closer, error) {
		session.close()
		cancel()
		return nil, err
	}
	var p4 *ipv4.PacketConn
	var p6 *ipv6.PacketConn
	if conn4 != nil {
		p4 = ipv4.NewPacketConn(conn4)
		if err := prepareIPv4(p4, ifaces); err != nil {
			_ = conn4.Close()
			conn4 = nil
			p4 = nil
		}
	}
	if conn6 != nil {
		p6 = ipv6.NewPacketConn(conn6)
		if err := prepareIPv6(p6, ifaces); err != nil {
			_ = conn6.Close()
			conn6 = nil
			p6 = nil
		}
	}
	if p4 == nil && p6 == nil {
		return fail(errors.Join(err4, err6, errors.New("could not join an mDNS multicast group")))
	}
	allowed := map[int]bool{}
	for _, ifi := range ifaces {
		allowed[ifi.Index] = true
	}
	assembler := &serviceAssembler{browse: browse}
	if queryEvery <= 0 {
		queryEvery = 30 * time.Second
	}
	go session.run(ctx, p4, p6, ifaces, allowed, services, browse, queryEvery, assembler, onEvent, failed)
	return session, nil
}

func prepareIPv4(p *ipv4.PacketConn, ifaces []net.Interface) error {
	_ = p.SetControlMessage(ipv4.FlagInterface|ipv4.FlagTTL, true)
	_ = p.SetMulticastTTL(255)
	_ = p.SetMulticastLoopback(true)
	group := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: mdnsPort}
	joined := 0
	var joinErr error
	for _, ifi := range ifaces {
		if err := p.JoinGroup(&ifi, group); err != nil {
			joinErr = err
			continue
		}
		joined++
	}
	if joined == 0 {
		if joinErr == nil {
			joinErr = errors.New("could not join IPv4 mDNS group")
		}
		return joinErr
	}
	return nil
}

func prepareIPv6(p *ipv6.PacketConn, ifaces []net.Interface) error {
	_ = p.SetControlMessage(ipv6.FlagInterface|ipv6.FlagHopLimit, true)
	_ = p.SetMulticastHopLimit(255)
	_ = p.SetMulticastLoopback(true)
	group := &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: mdnsPort}
	joined := 0
	for _, ifi := range ifaces {
		if err := p.JoinGroup(&ifi, group); err != nil {
			continue
		}
		joined++
	}
	if joined == 0 {
		return errors.New("could not join IPv6 mDNS group")
	}
	return nil
}

func (s *serviceMDNS) run(ctx context.Context, p4 *ipv4.PacketConn, p6 *ipv6.PacketConn, ifaces []net.Interface, allowed map[int]bool, services []mdns.ServiceInstance, browse []string, queryEvery time.Duration, assembler *serviceAssembler, onEvent func(mdns.ServiceEvent), failed func(error)) {
	defer close(s.done)
	defer s.close()
	var wg sync.WaitGroup
	if p4 != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readIPv4(ctx, p4, allowed, services, assembler, onEvent, failed)
		}()
	}
	if p6 != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readIPv6(ctx, p6, allowed, services, assembler, onEvent, failed)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		queryLoop(ctx, p4, p6, ifaces, browse, queryEvery)
	}()
	wg.Wait()
}

func queryLoop(ctx context.Context, p4 *ipv4.PacketConn, p6 *ipv6.PacketConn, ifaces []net.Interface, browse []string, every time.Duration) {
	if len(browse) == 0 {
		<-ctx.Done()
		return
	}
	send := func() {
		for _, service := range browse {
			raw, err := ptrQuery(service + ".local.")
			if err != nil {
				continue
			}
			for _, ifi := range ifaces {
				if p4 != nil {
					_, _ = p4.WriteTo(raw, &ipv4.ControlMessage{IfIndex: ifi.Index}, &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: mdnsPort})
				}
				if p6 != nil {
					_, _ = p6.WriteTo(raw, &ipv6.ControlMessage{IfIndex: ifi.Index}, &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: mdnsPort})
				}
			}
		}
	}
	send()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func ptrQuery(name string) ([]byte, error) {
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return nil, err
	}
	return (&dnsmessage.Message{Questions: []dnsmessage.Question{{
		Name: n, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET,
	}}}).Pack()
}

func readIPv4(ctx context.Context, p *ipv4.PacketConn, allowed map[int]bool, services []mdns.ServiceInstance, assembler *serviceAssembler, onEvent func(mdns.ServiceEvent), failed func(error)) {
	buf := make([]byte, 9000)
	for ctx.Err() == nil {
		_ = p.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, cm, src, err := p.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if failed != nil {
				failed(err)
			}
			return
		}
		if cm != nil && cm.TTL != 0 && cm.TTL != 255 {
			continue
		}
		ifIndex := 0
		if cm != nil {
			ifIndex = cm.IfIndex
		}
		if len(allowed) > 0 && ifIndex != 0 && !allowed[ifIndex] {
			continue
		}
		udp, _ := src.(*net.UDPAddr)
		handlePacket(func(payload []byte, index int, dst *net.UDPAddr) {
			_, _ = p.WriteTo(payload, &ipv4.ControlMessage{IfIndex: index}, dst)
		}, ifIndex, udp, append([]byte(nil), buf[:n]...), services, assembler, onEvent)
	}
}

func readIPv6(ctx context.Context, p *ipv6.PacketConn, allowed map[int]bool, services []mdns.ServiceInstance, assembler *serviceAssembler, onEvent func(mdns.ServiceEvent), failed func(error)) {
	buf := make([]byte, 9000)
	for ctx.Err() == nil {
		_ = p.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, cm, src, err := p.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if failed != nil {
				failed(err)
			}
			return
		}
		if cm != nil && cm.HopLimit != 0 && cm.HopLimit != 255 {
			continue
		}
		ifIndex := 0
		if cm != nil {
			ifIndex = cm.IfIndex
		}
		if len(allowed) > 0 && ifIndex != 0 && !allowed[ifIndex] {
			continue
		}
		udp, _ := src.(*net.UDPAddr)
		handlePacket(func(payload []byte, index int, dst *net.UDPAddr) {
			_, _ = p.WriteTo(payload, &ipv6.ControlMessage{IfIndex: index}, dst)
		}, ifIndex, udp, append([]byte(nil), buf[:n]...), services, assembler, onEvent)
	}
}

func handlePacket(write func([]byte, int, *net.UDPAddr), ifIndex int, src *net.UDPAddr, pkt []byte, services []mdns.ServiceInstance, assembler *serviceAssembler, onEvent func(mdns.ServiceEvent)) {
	var msg dnsmessage.Message
	if err := msg.Unpack(pkt); err != nil {
		return
	}
	if !msg.Header.Response {
		if write == nil || len(services) == 0 {
			return
		}
		raw, dst, ok := responseToQuery(services, pkt, src)
		if !ok {
			return
		}
		write(raw, ifIndex, dst)
		return
	}
	if onEvent == nil || assembler == nil {
		return
	}
	addr := netip.Addr{}
	if src != nil {
		addr = sourceAddr(src.IP, zoneName(ifIndex))
	}
	records := append(append([]dnsmessage.Resource{}, msg.Answers...), msg.Additionals...)
	for _, event := range assembler.ingest(records, addr) {
		onEvent(event)
	}
}

func sourceAddr(ip net.IP, zone string) netip.Addr {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}
	}
	addr = addr.Unmap()
	if zone != "" && addr.Is6() && addr.IsLinkLocalUnicast() {
		addr = addr.WithZone(zone)
	}
	return addr
}

func zoneName(index int) string {
	if index <= 0 {
		return ""
	}
	ifi, err := net.InterfaceByIndex(index)
	if err != nil {
		return ""
	}
	return ifi.Name
}

func resolveMulticastWithoutHostnameClaim(ctx context.Context, host string, allowHTTP bool) (Endpoint, error) {
	listeners := &multicastListeners{}
	session, err := startServiceMDNS(ctx, nil, nil, []string{secureService, clearService}, time.Second, func(event mdns.ServiceEvent) {
		listeners.observeHost(host, event)
	}, nil)
	if err != nil {
		return Endpoint{}, err
	}
	defer session.Close()
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return collectMulticastEndpoints(ctx, query, host, allowHTTP, listeners)
}
