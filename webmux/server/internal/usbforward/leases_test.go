package usbforward

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func leaseFixture(t *testing.T) (*LeaseManager, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	return &LeaseManager{Store: &storage.Store{Home: home}, now: func() time.Time { return now }}, &now
}
func testLeaseSpec() LeaseSpec {
	return LeaseSpec{Host: "operator@receiver", SSHPort: 22, ExportPort: 3240, Seconds: 600}
}
func allowLeaseOwner(owner string) bool { return owner == "alice" }
func TestUSBLeasePairingIsolationReplayAndDurableStop(t *testing.T) {
	m, now := leaseFixture(t)
	spec := testLeaseSpec()
	ticket, err := m.Create("alice", spec)
	if err != nil {
		t.Fatal(err)
	}
	if leases, err := m.List("bob"); err != nil || len(leases) != 0 {
		t.Fatalf("foreign listing: %v %v", leases, err)
	}
	if _, err := m.Stop("bob", ticket.Lease.ID); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("foreign stop: %v", err)
	}
	wrong := spec
	wrong.ExportPort++
	if _, err := m.Claim(ticket.Lease.ID, ticket.PairCode, wrong, allowLeaseOwner); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("mismatched target: %v", err)
	}
	credential, err := m.Claim(ticket.Lease.ID, ticket.PairCode, spec, allowLeaseOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Claim(ticket.Lease.ID, ticket.PairCode, spec, allowLeaseOwner); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("pair replay: %v", err)
	}
	ready := LeaseReport{Phase: "ready", RemotePort: 23240}
	if _, err := m.Report(ticket.Lease.ID, credential.Credential, ready, allowLeaseOwner); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(9 * time.Second)
	values, err := m.List("alice")
	if err != nil || len(values) != 1 || !values[0].Stale || values[0].Phase != "ready" {
		t.Fatalf("freshness: %+v %v", values, err)
	}
	if _, err := m.Stop("alice", ticket.Lease.ID); err != nil {
		t.Fatal(err)
	}
	reloaded := &LeaseManager{Store: m.Store, now: m.now}
	control, err := reloaded.Report(ticket.Lease.ID, credential.Credential, ready, allowLeaseOwner)
	if err != nil || !control.Stop {
		t.Fatalf("durable stop: %+v %v", control, err)
	}
	values, err = reloaded.List("alice")
	if err != nil || values[0].Phase != "stopping" || values[0].ReleaseConfirmed {
		t.Fatalf("stop request claimed release: %+v %v", values, err)
	}
	if _, err := reloaded.Report(ticket.Lease.ID, credential.Credential, LeaseReport{Phase: "ended", RemotePort: 23240}, allowLeaseOwner); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("unconfirmed end: %v", err)
	}
	ended := LeaseReport{Phase: "ended", RemotePort: 23240, ReleaseConfirmed: true}
	if _, err := reloaded.Report(ticket.Lease.ID, credential.Credential, ended, allowLeaseOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.Report(ticket.Lease.ID, credential.Credential, ready, allowLeaseOwner); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("resurrected terminal lease: %v", err)
	}
	raw, err := os.ReadFile(m.Store.ConfigPath(leaseFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), ticket.PairCode) || strings.Contains(string(raw), credential.Credential) {
		t.Fatal("plaintext credential persisted")
	}
}
func TestUSBLeaseConcurrentPairingHasOneWinner(t *testing.T) {
	m, _ := leaseFixture(t)
	spec := testLeaseSpec()
	ticket, err := m.Create("alice", spec)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		group.Go(func() { _, err := m.Claim(ticket.Lease.ID, ticket.PairCode, spec, allowLeaseOwner); results <- err })
	}
	group.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrLeaseNotFound) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("pairing winners = %d", wins)
	}
}
func TestUSBLeaseExpiryRevocationAndQuotas(t *testing.T) {
	m, now := leaseFixture(t)
	spec := testLeaseSpec()
	var first LeaseTicket
	for i := range 8 {
		ticket, err := m.Create("alice", spec)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = ticket
		}
	}
	if _, err := m.Create("alice", spec); !errors.Is(err, ErrLeaseLimit) {
		t.Fatalf("owner quota: %v", err)
	}
	if _, err := m.Claim(first.Lease.ID, first.PairCode, spec, func(string) bool { return false }); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("revoked owner: %v", err)
	}
	*now = now.Add(5*time.Minute + time.Second)
	expired, err := m.List("alice")
	if err != nil || len(expired) != 8 || expired[0].Phase != "expired" || !expired[0].ReleaseConfirmed {
		t.Fatalf("unstarted expiration falsely claimed a remote lease: %+v %v", expired, err)
	}

	if _, err := m.Claim(first.Lease.ID, first.PairCode, spec, allowLeaseOwner); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("expired pairing: %v", err)
	}
	ticket, err := m.Create("alice", spec)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := m.Claim(ticket.Lease.ID, ticket.PairCode, spec, allowLeaseOwner)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(601 * time.Second)
	control, err := m.Report(ticket.Lease.ID, credential.Credential, LeaseReport{Phase: "starting"}, allowLeaseOwner)
	if err != nil || !control.Stop {
		t.Fatalf("expiry cleanup grace: %+v %v", control, err)
	}
	*now = now.Add(31 * time.Second)
	if _, err := m.Report(ticket.Lease.ID, credential.Credential, LeaseReport{Phase: "starting"}, allowLeaseOwner); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("expired reporting credential: %v", err)
	}
}
func TestUSBLeaseCancelPendingNeverClaimsATunnel(t *testing.T) {
	m, _ := leaseFixture(t)
	ticket, err := m.Create("alice", testLeaseSpec())
	if err != nil {
		t.Fatal(err)
	}
	value, err := m.Stop("alice", ticket.Lease.ID)
	if err != nil || value.Phase != "ended" || !value.ReleaseConfirmed || value.RemotePort != 0 {
		t.Fatalf("pending cancellation: %+v %v", value, err)
	}
	if _, err := m.Claim(ticket.Lease.ID, ticket.PairCode, testLeaseSpec(), allowLeaseOwner); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("canceled pairing started: %v", err)
	}
}

func TestUSBLeaseCorruptAuthorityIsNotPresentedAsReady(t *testing.T) {
	m, _ := leaseFixture(t)
	if _, err := m.Create("alice", testLeaseSpec()); err != nil {
		t.Fatal(err)
	}
	var d leaseDocument
	if err := m.Store.ReadConfig(leaseFile, &d); err != nil {
		t.Fatal(err)
	}
	d.Leases[0].Lease.Phase = "unrecognized"
	if err := m.Store.WriteConfig(leaseFile, d); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(m.Store.ConfigPath(leaseFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.List("alice"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("corrupt status accepted: %v", err)
	}
	if _, err := m.Create("alice", testLeaseSpec()); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("corrupt authority reset: %v", err)
	}
	after, err := os.ReadFile(m.Store.ConfigPath(leaseFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("corrupt authority overwritten")
	}
}

func TestUSBLeaseGlobalQuotaBoundsMultipleOwners(t *testing.T) {
	m, _ := leaseFixture(t)
	for owner := range 4 {
		for range 8 {
			if _, err := m.Create(fmt.Sprintf("owner-%d", owner), testLeaseSpec()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := m.Create("another-owner", testLeaseSpec()); !errors.Is(err, ErrLeaseLimit) {
		t.Fatalf("global quota: %v", err)
	}
}
