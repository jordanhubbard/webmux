package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRigEnrollmentProjectionKeepsLaunchRecordsSeparate(t *testing.T) {
	for _, tc := range []struct {
		origin  string
		managed bool
	}{{"claimed", true}, {"launched", false}} {
		var raw rigStatusEnvelope
		data := `{"seats":[{"rigId":"rig","rigName":"project","logicalId":"agents.one","canonicalSessionName":"native","enrollmentOrigin":"` + tc.origin + `","enrollmentSource":"rig-session-registry","enrollmentId":"01M7NQ0XV3KPXQDNNC18KY64M7","enrollmentObservedAt":"2026-11-17T12:00:00Z","resumeToken":"PRIVATE_TOKEN"}]}`
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			t.Fatal(err)
		}
		raw.Seats[0].Managed = tc.managed
		status := rigCoordinationStatus(raw)
		if len(status.Seats) != 1 {
			t.Fatalf("missing seat: %+v", status)
		}
		seat := status.Seats[0]
		if seat.Managed != tc.managed || seat.EnrollmentOrigin != tc.origin || seat.EnrollmentSource != "rig-session-registry" {
			t.Fatalf("conflated launch record and enrollment: %+v", seat)
		}
		encoded, err := json.Marshal(seat)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"enrollmentId":"01M7NQ0XV3KPXQDNNC18KY64M7"`) || strings.Contains(string(encoded), "PRIVATE_") {
			t.Fatalf("unsafe enrollment projection: %s", encoded)
		}
	}
}
func TestRigEnrollmentUnknownIsNotNativeOwnership(t *testing.T) {
	for _, value := range []rigEnrollment{
		{}, {EnrollmentOrigin: "launched", EnrollmentSource: "WebMux launch record"},
		{EnrollmentOrigin: "claimed", EnrollmentSource: "rig-session-registry", EnrollmentID: "PRIVATE_TOKEN", EnrollmentObservedAt: "2026-11-17T12:00:00Z"},
		{EnrollmentOrigin: "launched", EnrollmentSource: "rig-session-registry", EnrollmentID: "01M7NQ0XV3KPXQDNNC18KY64M7", EnrollmentObservedAt: "invalid"},
	} {
		got := canonicalRigEnrollment(value)
		if got.EnrollmentOrigin != "unknown" || got.EnrollmentSource != "unavailable" || got.EnrollmentID != "" || got.EnrollmentObservedAt != "" {
			t.Fatalf("unverified enrollment claimed: %+v", got)
		}
	}
}
