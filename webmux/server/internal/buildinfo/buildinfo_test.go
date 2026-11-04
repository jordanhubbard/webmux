package buildinfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnknownAndPartialProducerObservations(t *testing.T) {
	for _, input := range []struct{ version, revision, modified string }{
		{}, {"not-a-version", "not-a-revision", "false"},
	} {
		value := observations(input.version, input.revision, input.modified)
		data, err := json.Marshal(value)
		if err != nil || string(data) != `{"version":null,"revision":null,"modified":null,"provenance":"unavailable","attested":false}` {
			t.Fatalf("unknown facts became known: %s (%v)", data, err)
		}
	}
	versionOnly := observations("1.4.2", "", "false")
	if versionOnly.Version == nil || *versionOnly.Version != "1.4.2" || versionOnly.Revision != nil || versionOnly.Modified != nil || versionOnly.Attested {
		t.Fatal("source archive invented Git facts", versionOnly)
	}
	partial := observations("", strings.Repeat("a", 40), "unavailable")
	if partial.Version != nil || partial.Revision == nil || partial.Modified != nil || partial.Attested {
		t.Fatal("partial observation invented clean state", partial)
	}
	for _, modified := range []string{"false", "true"} {
		value := observations("1.4.2", strings.Repeat("a", 40), modified)
		if value.Modified == nil || *value.Modified != (modified == "true") || value.Attested || value.Provenance != "build-producer-self-report" {
			t.Fatal("invalid self-report", value)
		}
	}
}
