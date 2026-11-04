// Package buildinfo exposes bounded, self-reported producer observations.
// These fields are compatibility metadata, never artifact attestation.
package buildinfo

import "regexp"

// The canonical standalone builder sets these strings before compiling its
// staged source. Plain go builds intentionally retain unknown values.
var version, revision, modified string

var versionSyntax = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
var revisionSyntax = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type Info struct {
	Version    *string `json:"version"`
	Revision   *string `json:"revision"`
	Modified   *bool   `json:"modified"`
	Provenance string  `json:"provenance"`
	Attested   bool    `json:"attested"`
}

func ValidVersion(value string) bool  { return len(value) <= 128 && versionSyntax.MatchString(value) }
func ValidRevision(value string) bool { return revisionSyntax.MatchString(value) }

func Current() Info { return observations(version, revision, modified) }

func observations(version, revision, modified string) Info {
	result := Info{Provenance: "unavailable"}
	if ValidVersion(version) {
		result.Version = &version
	}
	if ValidRevision(revision) {
		result.Revision = &revision
		if modified == "true" || modified == "false" {
			value := modified == "true"
			result.Modified = &value
		}
	}
	if result.Version != nil || result.Revision != nil {
		result.Provenance = "build-producer-self-report"
	}
	return result
}
