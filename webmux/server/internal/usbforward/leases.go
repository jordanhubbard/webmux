package usbforward

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

var (
	ErrLeaseInvalid  = errors.New("invalid USB forwarding lease")
	ErrLeaseNotFound = errors.New("USB forwarding lease unavailable")
	ErrLeaseConflict = errors.New("USB forwarding lease state changed")
	ErrLeaseLimit    = errors.New("USB forwarding lease limit reached")
)

const leaseFile = "usb-forwardings.yaml"

type LeaseSpec struct {
	Host       string `json:"host" yaml:"host"`
	SSHPort    int    `json:"ssh_port" yaml:"ssh_port"`
	ExportPort int    `json:"export_port" yaml:"export_port"`
	Seconds    int    `json:"seconds" yaml:"seconds"`
}

func (s LeaseSpec) validate() error {
	o := options{host: s.Host, sshPort: s.SSHPort, exportPort: s.ExportPort, duration: time.Duration(s.Seconds) * time.Second}
	if s.Seconds < 1 || s.Seconds > 3600 || o.validate() != nil {
		return ErrLeaseInvalid
	}
	return nil
}

type LeaseView struct {
	ID               string    `json:"id" yaml:"id"`
	Spec             LeaseSpec `json:"spec" yaml:"spec"`
	Phase            string    `json:"phase" yaml:"phase"`
	CreatedAt        time.Time `json:"created_at" yaml:"created_at"`
	ExpiresAt        time.Time `json:"expires_at" yaml:"expires_at"`
	LastSeen         time.Time `json:"last_seen" yaml:"last_seen"`
	RemotePort       int       `json:"remote_port" yaml:"remote_port"`
	StopRequested    bool      `json:"stop_requested" yaml:"stop_requested"`
	ReleaseConfirmed bool      `json:"release_confirmed" yaml:"release_confirmed"`
	Stale            bool      `json:"stale" yaml:"-"`
}

type leaseRecord struct {
	Lease        LeaseView `yaml:"lease"`
	Owner        string    `yaml:"owner"`
	PairHash     string    `yaml:"pair_hash,omitempty"`
	ReporterHash string    `yaml:"reporter_hash,omitempty"`
	PairExpires  time.Time `yaml:"pair_expires"`
}
type leaseDocument struct {
	Version int           `yaml:"version"`
	Leases  []leaseRecord `yaml:"leases"`
}

type LeaseTicket struct {
	Lease    LeaseView `json:"lease"`
	PairCode string    `json:"pair_code"`
}
type LeaseCredential struct {
	Credential string    `json:"credential"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type LeaseReport struct {
	Phase            string `json:"phase"`
	RemotePort       int    `json:"remote_port"`
	ReleaseConfirmed bool   `json:"release_confirmed"`
}
type LeaseControl struct {
	Stop bool `json:"stop"`
}

// Manager stores only credential digests. The store serializes all mutations;
// a replacement controller reads the same stop request and reporter identity.
type LeaseManager struct {
	Store *storage.Store
	now   func() time.Time
}

func (m *LeaseManager) clock() time.Time {
	if m.now != nil {
		return m.now().UTC()
	}
	return time.Now().UTC()
}
func leaseSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func leaseHash(value string) string {
	b := sha256.Sum256([]byte(value))
	return hex.EncodeToString(b[:])
}
func matchesLeaseSecret(value, digest string, n int) bool {
	if len(value) != n || len(digest) != 64 {
		return false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(leaseHash(value)), []byte(digest)) == 1
}
func terminalLease(phase string) bool {
	return phase == "ended" || phase == "failed" || phase == "uncertain"
}
func retainLease(r leaseRecord, now time.Time) bool {
	end := r.Lease.ExpiresAt
	if terminalLease(r.Lease.Phase) && !r.Lease.LastSeen.IsZero() {
		end = r.Lease.LastSeen
	}
	return now.Before(end.Add(5 * time.Minute))
}
func leaseView(v LeaseView, now time.Time) LeaseView {
	if terminalLease(v.Phase) {
		return v
	}
	if now.After(v.ExpiresAt) {
		neverStarted := v.Phase == "pending"
		v.Phase = "expired"
		v.ReleaseConfirmed = neverStarted
		v.Stale = !neverStarted
		return v
	}
	if v.StopRequested {
		v.Phase = "stopping"
	}
	v.Stale = !v.LastSeen.IsZero() && now.Sub(v.LastSeen) > 8*time.Second
	return v
}
func validLeaseHex(value string, n int) bool {
	if len(value) != n {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func validLeaseDocument(d leaseDocument) bool {
	if d.Version != 1 || len(d.Leases) > 128 {
		return false
	}
	ids := map[string]bool{}
	for _, r := range d.Leases {
		v := r.Lease
		if !validLeaseHex(v.ID, 32) || ids[v.ID] || r.Owner == "" || v.Spec.validate() != nil || v.CreatedAt.IsZero() || v.ExpiresAt.IsZero() {
			return false
		}
		ids[v.ID] = true
		if v.Phase != "pending" && v.Phase != "starting" && v.Phase != "ready" && !terminalLease(v.Phase) {
			return false
		}
		if v.RemotePort != 0 && (v.RemotePort < 1024 || v.RemotePort > 65535) || v.Phase == "ready" && v.RemotePort == 0 {
			return false
		}
		if v.ReleaseConfirmed && !terminalLease(v.Phase) || v.Phase == "ended" && !v.ReleaseConfirmed || v.Phase == "uncertain" && v.ReleaseConfirmed {
			return false
		}
		if v.Phase == "pending" {
			if !validLeaseHex(r.PairHash, 64) || r.ReporterHash != "" || r.PairExpires.IsZero() {
				return false
			}
		} else if r.PairHash != "" || !validLeaseHex(r.ReporterHash, 64) && !(v.Phase == "ended" && v.StopRequested && v.RemotePort == 0 && v.ReleaseConfirmed) {
			return false
		}
	}
	return true
}
func (m *LeaseManager) update(change func(*leaseDocument, time.Time) error) error {
	return storage.UpdateConfigOrCreate(m.Store, leaseFile, leaseDocument{Version: 1}, func(d *leaseDocument) error {
		if !validLeaseDocument(*d) {
			return ErrLeaseInvalid
		}
		return change(d, m.clock())
	})
}
func (m *LeaseManager) Create(owner string, spec LeaseSpec) (LeaseTicket, error) {
	if owner == "" || spec.validate() != nil {
		return LeaseTicket{}, ErrLeaseInvalid
	}
	id, err := leaseSecret(16)
	if err != nil {
		return LeaseTicket{}, err
	}
	code, err := leaseSecret(24)
	if err != nil {
		return LeaseTicket{}, err
	}
	var ticket LeaseTicket
	err = m.update(func(d *leaseDocument, now time.Time) error {
		retained := d.Leases[:0]
		active, ownerActive := 0, 0
		for _, r := range d.Leases {
			if !retainLease(r, now) {
				continue
			}
			retained = append(retained, r)
			if !terminalLease(r.Lease.Phase) && now.Before(r.Lease.ExpiresAt) {
				active++
				if r.Owner == owner {
					ownerActive++
				}
			}
		}
		d.Leases = retained
		if active >= 32 || ownerActive >= 8 || len(d.Leases) >= 128 {
			return ErrLeaseLimit
		}
		v := LeaseView{ID: id, Spec: spec, Phase: "pending", CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
		d.Leases = append(d.Leases, leaseRecord{Lease: v, Owner: owner, PairHash: leaseHash(code), PairExpires: v.ExpiresAt})
		ticket = LeaseTicket{Lease: v, PairCode: code}
		return nil
	})
	return ticket, err
}
func (m *LeaseManager) List(owner string) ([]LeaseView, error) {
	var d leaseDocument
	if err := m.Store.ReadConfig(leaseFile, &d); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LeaseView{}, nil
		}
		return nil, err
	}
	if !validLeaseDocument(d) {
		return nil, ErrLeaseInvalid
	}
	values := []LeaseView{}
	now := m.clock()
	for i := len(d.Leases) - 1; i >= 0; i-- {
		r := d.Leases[i]
		if r.Owner == owner && retainLease(r, now) {
			values = append(values, leaseView(r.Lease, now))
		}
	}
	return values, nil
}
func (m *LeaseManager) Stop(owner, id string) (LeaseView, error) {
	var result LeaseView
	err := m.update(func(d *leaseDocument, now time.Time) error {
		for i := range d.Leases {
			r := &d.Leases[i]
			if r.Owner != owner || r.Lease.ID != id {
				continue
			}
			r.Lease.StopRequested = true
			// A canceled pairing has never opened a remote listener.
			if r.Lease.Phase == "pending" {
				r.PairHash = ""
				r.Lease.Phase = "ended"
				r.Lease.LastSeen = now
				r.Lease.ReleaseConfirmed = true
			}
			result = leaseView(r.Lease, now)
			return nil
		}
		return ErrLeaseNotFound
	})
	return result, err
}
func (m *LeaseManager) Claim(id, code string, spec LeaseSpec, allowed func(string) bool) (LeaseCredential, error) {
	if spec.validate() != nil {
		return LeaseCredential{}, ErrLeaseInvalid
	}
	secret, err := leaseSecret(32)
	if err != nil {
		return LeaseCredential{}, err
	}
	var credential LeaseCredential
	err = m.update(func(d *leaseDocument, now time.Time) error {
		for i := range d.Leases {
			r := &d.Leases[i]
			if r.Lease.ID != id {
				continue
			}
			if !matchesLeaseSecret(code, r.PairHash, 48) || !now.Before(r.PairExpires) || allowed == nil || !allowed(r.Owner) {
				return ErrLeaseNotFound
			}
			if r.Lease.Phase != "pending" || r.Lease.StopRequested || r.Lease.Spec != spec {
				return ErrLeaseConflict
			}
			r.PairHash = ""
			r.ReporterHash = leaseHash(secret)
			r.Lease.Phase = "starting"
			r.Lease.LastSeen = now
			r.Lease.ExpiresAt = now.Add(time.Duration(spec.Seconds) * time.Second)
			credential = LeaseCredential{Credential: secret, ExpiresAt: r.Lease.ExpiresAt}
			return nil
		}
		return ErrLeaseNotFound
	})
	return credential, err
}
func (m *LeaseManager) Report(id, secret string, report LeaseReport, allowed func(string) bool) (LeaseControl, error) {
	if report.Phase != "starting" && report.Phase != "ready" && !terminalLease(report.Phase) {
		return LeaseControl{}, ErrLeaseInvalid
	}
	if report.RemotePort != 0 && (report.RemotePort < 1024 || report.RemotePort > 65535) {
		return LeaseControl{}, ErrLeaseInvalid
	}
	if report.Phase == "ready" && report.RemotePort == 0 || report.ReleaseConfirmed && !terminalLease(report.Phase) || report.Phase == "ended" && !report.ReleaseConfirmed || report.Phase == "uncertain" && report.ReleaseConfirmed {
		return LeaseControl{}, ErrLeaseInvalid
	}
	var control LeaseControl
	err := m.update(func(d *leaseDocument, now time.Time) error {
		for i := range d.Leases {
			r := &d.Leases[i]
			if r.Lease.ID != id {
				continue
			}
			if !matchesLeaseSecret(secret, r.ReporterHash, 64) || now.After(r.Lease.ExpiresAt.Add(30*time.Second)) || allowed == nil || !allowed(r.Owner) {
				return ErrLeaseNotFound
			}
			if terminalLease(r.Lease.Phase) {
				if report.Phase != r.Lease.Phase || report.RemotePort != r.Lease.RemotePort || report.ReleaseConfirmed != r.Lease.ReleaseConfirmed {
					return ErrLeaseConflict
				}
			} else {
				if r.Lease.Phase == "ready" && report.Phase == "starting" || r.Lease.RemotePort != 0 && report.RemotePort != r.Lease.RemotePort {
					return ErrLeaseConflict
				}
				r.Lease.Phase = report.Phase
				r.Lease.RemotePort = report.RemotePort
				r.Lease.ReleaseConfirmed = report.ReleaseConfirmed
			}
			r.Lease.LastSeen = now
			control.Stop = r.Lease.StopRequested || !now.Before(r.Lease.ExpiresAt)
			return nil
		}
		return ErrLeaseNotFound
	})
	return control, err
}
