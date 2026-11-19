package httpapi

import (
	"strings"
	"testing"
)

func TestLocalRigIdentitySurvivesAttachmentChange(t *testing.T) {
	seat := rigCanonicalSeat{RigID: "rig-one", LogicalID: "agents.builder", CanonicalSessionName: "first", TmuxTarget: "first", Openable: true}
	raw := rigStatusEnvelope{Seats: []rigCanonicalSeat{seat}}
	first := rigCoordinationStatus(raw).Seats[0]
	raw.Seats[0].CanonicalSessionName, raw.Seats[0].TmuxTarget = "second", "second"
	second := rigCoordinationStatus(raw).Seats[0]
	if first.ID != second.ID || first.ID == first.AttachmentTarget || second.AttachmentTarget != "second" || second.ManagerTarget != "second" {
		t.Fatal("mutable address became logical identity", first, second)
	}
	for _, target := range []string{first.ID, "second"} {
		resolved, err := resolveLocalRigSeat(raw, target)
		if err != nil || resolved.CanonicalSessionName != "second" {
			t.Fatal("current canonical binding did not resolve", target, resolved, err)
		}
	}
	if _, err := resolveLocalRigSeat(raw, "first"); err == nil {
		t.Fatal("old address silently resolved to a changed attachment")
	}
	if localRigSeatID("other-rig", seat.LogicalID) == first.ID || localRigSeatID(seat.RigID, "other") == first.ID {
		t.Fatal("canonical namespace collision")
	}
}

func TestLocalRigIdentityRefusesMissingOrAmbiguousCanonicalRows(t *testing.T) {
	for _, value := range []string{"", "bad\nidentity", strings.Repeat("x", 257)} {
		if localRigSeatID(value, "worker") != "" || localRigSeatID("rig", value) != "" {
			t.Fatal("unbounded or absent identity accepted", value)
		}
	}
	seat := rigCanonicalSeat{RigID: "rig", LogicalID: "worker", CanonicalSessionName: "first", Openable: true}
	raw := rigStatusEnvelope{Seats: []rigCanonicalSeat{seat, seat}}
	if status := rigCoordinationStatus(raw); len(status.Seats) != 0 || len(status.Errors) == 0 {
		t.Fatal("ambiguous rows became actionable", status)
	}
	for _, target := range []string{"first", localRigSeatID("rig", "worker")} {
		if _, err := resolveLocalRigSeat(raw, target); err == nil {
			t.Fatal("ambiguous row resolved", target)
		}
	}
}

func TestLocalRigIdentityRefusesAnAddressSharedByDistinctSeats(t *testing.T) {
	raw := rigStatusEnvelope{Seats: []rigCanonicalSeat{
		{RigID: "rig", LogicalID: "worker-one", CanonicalSessionName: "shared", TmuxTarget: "shared", Openable: true},
		{RigID: "rig", LogicalID: "worker-two", CanonicalSessionName: "shared", TmuxTarget: "shared", Openable: true},
	}}
	status := rigCoordinationStatus(raw)
	if len(status.Seats) != 2 || status.Seats[0].Openable || status.Seats[1].Openable {
		t.Fatal("ambiguous address advertised as openable", status)
	}
	for _, seat := range status.Seats {
		if _, err := resolveLocalRigSeat(raw, seat.ID); err == nil {
			t.Fatal("logical ID authorized ambiguous native address", seat)
		}
	}
}
