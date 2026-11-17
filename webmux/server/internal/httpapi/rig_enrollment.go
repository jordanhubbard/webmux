package httpapi

import (
	"regexp"
	"time"
)

type rigEnrollment struct {
	EnrollmentOrigin     string `json:"enrollmentOrigin,omitempty"`
	EnrollmentSource     string `json:"enrollmentSource,omitempty"`
	EnrollmentID         string `json:"enrollmentId,omitempty"`
	EnrollmentObservedAt string `json:"enrollmentObservedAt,omitempty"`
}

var enrollmentID = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func canonicalRigEnrollment(value rigEnrollment) rigEnrollment {
	unknown := rigEnrollment{EnrollmentOrigin: "unknown", EnrollmentSource: "unavailable"}
	if value.EnrollmentSource != "rig-session-registry" || (value.EnrollmentOrigin != "launched" && value.EnrollmentOrigin != "claimed") || !enrollmentID.MatchString(value.EnrollmentID) {
		return unknown
	}
	if _, err := time.Parse(time.RFC3339Nano, value.EnrollmentObservedAt); err != nil {
		return unknown
	}
	return value
}
