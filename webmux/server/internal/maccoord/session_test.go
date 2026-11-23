package maccoord

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func sessionMember() coordination.Member {
	return coordination.Member{
		MemberInput: coordination.MemberInput{
			Ref:     coordination.Ref{Node: strings.Repeat("1", 32), Owner: "alice", Session: "session-a", Incarnation: strings.Repeat("2", 32)},
			Project: repository(), ExecutionHost: "rocky", Transport: "ssh", Active: true,
		},
		Present: true, Observed: time.Date(2026, 11, 23, 6, 0, 0, 0, time.UTC),
		Task: taskCanary,
	}
}

func TestSessionIdentityRetainsIncarnationWithoutInventingMACLink(t *testing.T) {
	bindings := accessBindings(t, "https://authority.example")
	member := sessionMember()
	got, err := bindings.ObserveSession("alice", member)
	if err != nil || got.Schema != SessionIdentitySchema || got.Session != member.Ref || got.BindingRevision == "" ||
		got.AuthorityID != "fleet-a" || got.ProjectKey != "alpha" || got.ExecutionHost != "rocky" || got.Transport != "ssh" ||
		got.Availability != "present" || !got.BrokerActive || !got.ObservedAt.Equal(member.Observed) || got.ProjectedAt.IsZero() ||
		got.Freshness != "unqualified" || got.MACLinkState != "unlinked" || got.MACMachineID != nil || got.MACWorkerID != nil ||
		got.MACTaskID != nil || got.MACAttemptCount != nil || got.MACLeaseID != nil {
		t.Fatalf("session inventory became a MAC identity or lost provenance: %+v, %v", got, err)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), taskCanary) {
		t.Fatal("free-form declared work became a canonical MAC task")
	}
	member.Ref.Incarnation = strings.Repeat("3", 32)
	next, err := bindings.ObserveSession("alice", member)
	if err != nil || next.Session.Key() == got.Session.Key() || next.MACLinkState != "unlinked" {
		t.Fatal("a new session incarnation reused an old identity", err)
	}
}

func TestSessionIdentityRefusesForeignAndMalformedInventory(t *testing.T) {
	bindings := accessBindings(t, "https://authority.example")
	for _, test := range []struct {
		name string
		edit func(*coordination.Member)
		want error
	}{
		{"foreign receiving owner", func(m *coordination.Member) { m.Ref.Owner = "bob" }, ErrUnmapped},
		{"other repository", func(m *coordination.Member) { m.Project.ID = "foreign-root" }, ErrUnmapped},
		{"missing project", func(m *coordination.Member) { m.Project = nil }, ErrUnmapped},
		{"display name project", func(m *coordination.Member) { m.Project.Source = "display-name" }, ErrUnmapped},
		{"missing incarnation", func(m *coordination.Member) { m.Ref.Incarnation = "" }, ErrInvalid},
		{"invalid node", func(m *coordination.Member) { m.Ref.Node = "rocky" }, ErrInvalid},
		{"missing session", func(m *coordination.Member) { m.Ref.Session = "" }, ErrInvalid},
		{"malformed routing host", func(m *coordination.Member) { m.ExecutionHost = "private\ncanary" }, ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			member := sessionMember()
			test.edit(&member)
			got, err := bindings.ObserveSession("alice", member)
			if !errors.Is(err, test.want) || got.Schema != "" || got.Session.Owner != "" || got.ExecutionHost != "" {
				t.Fatalf("refused session exposed metadata: %+v, %v", got, err)
			}
		})
	}
}

func TestSessionIdentityPreservesUnavailableAndRevokedObservations(t *testing.T) {
	bindings := accessBindings(t, "https://authority.example")
	member := sessionMember()
	member.Present, member.Active = false, false
	got, err := bindings.ObserveSession("alice", member)
	if err != nil || got.Availability != "not_present" || got.BrokerActive {
		t.Fatal("absent broker session became live", err)
	}
	member.Observed = time.Time{}
	got, err = bindings.ObserveSession("alice", member)
	if err != nil || got.Availability != "unknown" || got.Freshness != "unqualified" {
		t.Fatal("missing observation became fresh absence", err)
	}
	when := time.Date(2026, 11, 23, 6, 1, 0, 0, time.UTC)
	member.Revoked = &when
	got, err = bindings.ObserveSession("alice", member)
	if err != nil || got.Availability != "revoked" || got.RevokedAt == nil || !got.RevokedAt.Equal(when) {
		t.Fatal("revoked incarnation was lost", err)
	}
	*got.RevokedAt = time.Time{}
	if member.Revoked.IsZero() {
		t.Fatal("caller mutated broker revocation metadata")
	}
}
