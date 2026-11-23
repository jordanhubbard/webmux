//go:build darwin

package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/pion/mdns/v2"
	"golang.org/x/sys/unix"
)

const nativeNodeAnnouncements = true

// Apple's public dns_sd.h defines these functions. libSystem reexports the
// system DNS-SD implementation; no helper executable or C compiler is needed.
// Only scalars and pointers cross this boundary. In particular, the TXT pointer
// must be *byte, not a Go slice header, in Darwin ARM64's packed stack arguments.
// DNSServiceRegister copies its input strings/TXT and never retains Go memory.
type nativeDNS struct {
	register func(*uintptr, uint32, uint32, string, string, string, uintptr, uint16, uint16, *byte, uintptr, uintptr) int32
	socket   func(uintptr) int32
	process  func(uintptr) int32
	release  func(uintptr)
	callback uintptr
}

type nativeRegistration struct {
	ref       uintptr
	fd        int32
	confirmed bool
	err       error
}

// A single process-lifetime callback avoids PureGo's finite callback allocation
// limit. The keys are opaque native references, never pointers to Go memory.
// Only each registration's event-loop owner processes callbacks or releases it.
var nativeRegistrations sync.Map

func nativeRegistrationReply(ref uintptr, _ uint32, code int32, _, _, _, _ uintptr) {
	value, ok := nativeRegistrations.Load(ref)
	if !ok {
		return
	}
	registration := value.(*nativeRegistration)
	if code != 0 {
		registration.err = fmt.Errorf("Bonjour registration failed (code %d)", code)
		return
	}
	registration.confirmed = true
}

var loadNativeDNS = sync.OnceValues(func() (*nativeDNS, error) {
	library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load system Bonjour API: %w", err)
	}
	api := &nativeDNS{}
	for _, symbol := range []struct {
		name   string
		target any
	}{
		{"DNSServiceRegister", &api.register},
		{"DNSServiceRefSockFD", &api.socket},
		{"DNSServiceProcessResult", &api.process},
		{"DNSServiceRefDeallocate", &api.release},
	} {
		address, err := purego.Dlsym(library, symbol.name)
		if err != nil {
			_ = purego.Dlclose(library)
			return nil, fmt.Errorf("load system Bonjour symbol %s: %w", symbol.name, err)
		}
		purego.RegisterFunc(symbol.target, address)
	}
	api.callback = purego.NewCallback(nativeRegistrationReply)
	// Keep this system library loaded while the process-lifetime callback exists.
	return api, nil
})

type nativeAnnouncements struct {
	api           *nativeDNS
	registrations []*nativeRegistration
	cancel        context.CancelFunc
	done          chan struct{}
	mu            sync.Mutex
	err           error
}

func startNativeNodeAnnouncements(parent context.Context, services []mdns.ServiceInstance, interfaces []net.Interface, failed func(error)) (io.Closer, error) {
	api, err := loadNativeDNS()
	if err != nil {
		return nil, err
	}
	return startNativeAnnouncementsWithAPI(parent, api, services, interfaces, failed)
}

func startNativeAnnouncementsWithAPI(parent context.Context, api *nativeDNS, services []mdns.ServiceInstance, interfaces []net.Interface, failed func(error)) (*nativeAnnouncements, error) {
	ctx, cancel := context.WithCancel(parent)
	announcements := &nativeAnnouncements{api: api, cancel: cancel, done: make(chan struct{})}
	// Zero is Apple's "all eligible interfaces" policy. Explicit names have already
	// been checked for availability and multicast support by StartNodes.
	indexes := []uint32{0}
	if len(interfaces) > 0 {
		indexes = nil
		seen := map[int]bool{}
		for _, iface := range interfaces {
			if !seen[iface.Index] {
				indexes = append(indexes, uint32(iface.Index))
				seen[iface.Index] = true
			}
		}
	}
	cleanup := func() {
		for _, registration := range announcements.registrations {
			api.release(registration.ref)
			nativeRegistrations.Delete(registration.ref)
		}
	}
	for _, service := range services {
		var txt []byte
		for _, entry := range service.Text {
			text := append([]byte(entry.Key+"="), entry.Value...)
			if len(text) > 255 || len(txt)+len(text)+1 > 65535 {
				cancel()
				cleanup()
				return nil, errors.New("Bonjour TXT record exceeds DNS-SD bounds")
			}
			txt = append(txt, byte(len(text)))
			txt = append(txt, text...)
		}
		var txtPointer *byte
		if len(txt) > 0 {
			txtPointer = &txt[0]
		}
		var port [2]byte
		binary.BigEndian.PutUint16(port[:], service.Port)
		for _, index := range indexes {
			var ref uintptr
			// NoAutoRename preserves stable instance names on collision. NULL host
			// delegates hostname/address ownership, probing and changes to mDNSResponder.
			code := api.register(&ref, 8, index, service.Instance, service.Service, "local.", 0,
				binary.NativeEndian.Uint16(port[:]), uint16(len(txt)), txtPointer, api.callback, 0)
			if code != 0 {
				cancel()
				cleanup()
				return nil, fmt.Errorf("start Bonjour registration (code %d)", code)
			}
			registration := &nativeRegistration{ref: ref, fd: api.socket(ref)}
			announcements.registrations = append(announcements.registrations, registration)
			if registration.fd < 0 {
				cancel()
				cleanup()
				return nil, errors.New("Bonjour returned an invalid event socket")
			}
			nativeRegistrations.Store(ref, registration)
		}
	}
	// Require the daemon's registration acknowledgment and conflict check. Merely
	// accepting an asynchronous request does not establish startup success.
	startup, finish := context.WithTimeout(ctx, 3*time.Second)
	defer finish()
	for {
		if err := startup.Err(); err != nil {
			cancel()
			cleanup()
			return nil, fmt.Errorf("Bonjour registration acknowledgment: %w", err)
		}
		if err := announcements.poll(); err != nil {
			cancel()
			cleanup()
			return nil, err
		}
		ready := true
		for _, registration := range announcements.registrations {
			ready = ready && registration.confirmed
		}
		if ready {
			break
		}
	}
	go func() {
		defer close(announcements.done)
		defer cleanup()
		for ctx.Err() == nil {
			if err := announcements.poll(); err != nil {
				if ctx.Err() == nil {
					announcements.mu.Lock()
					announcements.err = err
					announcements.mu.Unlock()
					failed(err)
				}
				return
			}
		}
	}()
	return announcements, nil
}

func (a *nativeAnnouncements) poll() error {
	sockets := make([]unix.PollFd, len(a.registrations))
	for i, registration := range a.registrations {
		sockets[i] = unix.PollFd{Fd: registration.fd, Events: unix.POLLIN}
	}
	if _, err := unix.Poll(sockets, 100); err != nil && err != unix.EINTR {
		return fmt.Errorf("poll Bonjour event sockets: %w", err)
	}
	for i, socket := range sockets {
		registration := a.registrations[i]
		if socket.Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return errors.New("Bonjour event socket became unavailable")
		}
		if socket.Revents&unix.POLLIN != 0 {
			if code := a.api.process(registration.ref); code != 0 {
				return fmt.Errorf("process Bonjour event (code %d)", code)
			}
		}
		if registration.err != nil {
			return registration.err
		}
	}
	return nil
}

func (a *nativeAnnouncements) Close() error {
	a.cancel()
	<-a.done
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}
