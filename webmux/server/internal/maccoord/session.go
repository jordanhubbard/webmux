package maccoord

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const SessionIdentitySchema = "webmux.mac.session-identity.v1"

// SessionObservation is a projection of trusted WebMux broker inventory. The
// execution host is routing/display metadata, not a canonical MAC machine ID.
// Presence and broker activity do not prove task execution or MAC participation.
type SessionObservation struct {
	Schema          string           `json:"schema"`
	BindingRevision string           `json:"binding_revision"`
	AuthorityID     string           `json:"authority_id"`
	ProjectKey      string           `json:"project_key"`
	Session         coordination.Ref `json:"session"`
	ExecutionHost   string           `json:"execution_host"`
	Transport       string           `json:"transport"`
	BrokerActive    bool             `json:"broker_active"`
	Availability    string           `json:"availability"`
	ObservedAt      time.Time        `json:"observed_at"`
	ProjectedAt     time.Time        `json:"projected_at"`
	RevokedAt       *time.Time       `json:"revoked_at"`
	Freshness       string           `json:"freshness"`
	MACLinkState    string           `json:"mac_link_state"`
	MACMachineID    *string          `json:"mac_machine_id"`
	MACWorkerID     *string          `json:"mac_worker_id"`
	MACTaskID       *string          `json:"mac_task_id"`
	MACAttemptCount *int             `json:"mac_attempt_count"`
	MACLeaseID      *string          `json:"mac_lease_id"`
}

// ObserveSession accepts only server-owned broker inventory, never a request's
// self-description. It performs no I/O or registration and grants no capability.
// The caller must apply its view's freshness policy to the retained timestamps.
func (b *Bindings) ObserveSession(owner string, member coordination.Member) (SessionObservation, error) {
	if member.Ref.Owner != owner {
		return SessionObservation{}, ErrUnmapped
	}
	selected, err := b.Resolve(owner, member.Project)
	if err != nil {
		return SessionObservation{}, err
	}
	if !sessionHex(member.Ref.Node) || !sessionHex(member.Ref.Incarnation) ||
		!validText(member.Ref.Session, 128) || !validText(member.ExecutionHost, 1024) ||
		!validText(member.Transport, 64) || (member.Revoked != nil && member.Revoked.IsZero()) {
		return SessionObservation{}, ErrInvalid
	}
	availability := "unknown"
	if member.Revoked != nil {
		availability = "revoked"
	} else if !member.Observed.IsZero() {
		availability = "not_present"
		if member.Present {
			availability = "present"
		}
	}
	var revoked *time.Time
	if member.Revoked != nil {
		value := *member.Revoked
		revoked = &value
	}
	return SessionObservation{
		Schema: SessionIdentitySchema, BindingRevision: selected.Revision,
		AuthorityID: selected.Authority.ID, ProjectKey: selected.Project.ProjectKey,
		Session: member.Ref, ExecutionHost: member.ExecutionHost, Transport: member.Transport,
		BrokerActive: member.Active, Availability: availability, ObservedAt: member.Observed,
		ProjectedAt: time.Now().UTC(), RevokedAt: revoked, Freshness: "unqualified", MACLinkState: "unlinked",
	}, nil
}

func sessionHex(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
