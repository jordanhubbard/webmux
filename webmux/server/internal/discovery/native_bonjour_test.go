//go:build darwin

package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
)

// These controls contact the real local Bonjour daemon only when explicitly
// requested. Every registration uses one fresh random instance and is owned by
// this test. They do not start HTTP, modify OS policy or restart a service.
func TestNativeBonjourRegistrationControls(t *testing.T) {
	selected := os.Getenv("WEBMUX_TEST_BONJOUR_INTERFACE")
	if selected == "" {
		t.Skip("explicit native Bonjour registration controls only")
	}
	iface, err := net.InterfaceByName(selected)
	if err != nil || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
		t.Fatal("explicit multicast interface unavailable", err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	service, err := nodeService(NodeOptions{ID: hex.EncodeToString(nonce[:]), Host: "native-fixture", Port: 19311, TLS: true})
	if err != nil {
		t.Fatal(err)
	}
	api, err := loadNativeDNS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	firstFailed, secondFailed := make(chan error, 1), make(chan error, 1)
	first, err := startNativeAnnouncementsWithAPI(ctx, api, []mdns.ServiceInstance{service}, []net.Interface{*iface}, func(err error) { firstFailed <- err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if len(first.registrations) != 1 || !first.registrations[0].confirmed {
		t.Fatal("native registration not acknowledged")
	}
	t.Log("real daemon acknowledged the fresh registration on the explicit interface")
	conflict := service
	conflict.Port++
	second, startErr := startNativeAnnouncementsWithAPI(ctx, api, []mdns.ServiceInstance{conflict}, []net.Interface{*iface}, func(err error) { secondFailed <- err })
	winner := first
	var loser *nativeAnnouncements
	var conflictErr error
	if startErr != nil {
		conflictErr = startErr
	} else {
		t.Cleanup(func() { _ = second.Close() })
		// The daemon can acknowledge the newer request before delivering a
		// conflict to the earlier registration. NoAutoRename does not promise
		// which owner wins. Require a surfaced error and owned cleanup, rather
		// than incorrectly treating an initial acknowledgment as final success.
		select {
		case conflictErr = <-firstFailed:
			loser, winner = first, second
		case conflictErr = <-secondFailed:
			loser = second
		case <-time.After(3 * time.Second):
			t.Fatal("conflicting native registrations remained silently healthy")
		}
	}
	if conflictErr == nil || !strings.Contains(conflictErr.Error(), "-65548") {
		t.Fatal("expected actual native NameConflict code -65548", conflictErr)
	}
	if loser != nil {
		select {
		case <-loser.done:
		case <-time.After(time.Second):
			t.Fatal("conflict leaked the losing registration")
		}
		if err := loser.Close(); err == nil || !strings.Contains(err.Error(), "-65548") {
			t.Fatal("losing registration lost its terminal conflict status", err)
		}
		if _, ok := nativeRegistrations.Load(loser.registrations[0].ref); ok {
			t.Fatal("losing registration retained callback routing")
		}
	}
	t.Log("real daemon surfaced NameConflict; the losing registration released its owned state")
	if _, ok := nativeRegistrations.Load(winner.registrations[0].ref); !ok {
		t.Fatal("conflict cleanup removed the remaining registration")
	}
	if err := winner.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := nativeRegistrations.Load(winner.registrations[0].ref); ok {
		t.Fatal("closed owner retained callback routing")
	}
	thirdFailed := make(chan error, 1)
	third, err := startNativeAnnouncementsWithAPI(ctx, api, []mdns.ServiceInstance{conflict}, []net.Interface{*iface}, func(err error) { thirdFailed <- err })
	if err != nil {
		t.Fatal("owned cancellation did not release the name", err)
	}
	t.Cleanup(func() { _ = third.Close() })
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-thirdFailed:
		t.Fatal("live registration failed", err)
	default:
	}
	t.Log("owned cancellation released the instance; a fresh registration succeeded and closed")
}
