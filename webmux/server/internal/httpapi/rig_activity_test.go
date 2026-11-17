package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRigActivityCanonicalProjection(t *testing.T) {
	var raw rigStatusEnvelope
	err := json.Unmarshal([]byte(`{"seats":[{"rigId":"rig","rigName":"project","logicalId":"agents.one","canonicalSessionName":"first","tmuxTarget":"first","startupStatus":"ready","lifecycleState":"running","agentActivity":{"state":"needs_input","evidenceSource":"runtime_hook","sampledAt":"2026-11-17T12:00:00Z","generationProvenance":"resolved","evidence":"PRIVATE_PROMPT","rawEvent":"PRIVATE_EVENT","resumeToken":"PRIVATE_TOKEN"}}]}`), &raw)
	if err != nil {
		t.Fatal(err)
	}
	status := rigCoordinationStatus(raw)
	if len(status.Seats) != 1 {
		t.Fatalf("unexpected seats: %+v", status)
	}
	seat := status.Seats[0]
	if seat.StartupStatus != "ready" || seat.AgentActivity == nil || seat.AgentActivity.State != "needs_input" || seat.AgentActivity.EvidenceSource != "runtime_hook" {
		t.Fatalf("lost canonical facts: %+v", seat)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("private evidence escaped canonical projection")
	}
}

func TestRigActivityUnverifiedIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name     string
		activity rigActivity
	}{
		{"stale", rigActivity{State: "needs_input", EvidenceSource: "runtime_hook", SampledAt: "2026-11-17T12:00:00Z", Stale: true}},
		{"unresolved", rigActivity{State: "running", EvidenceSource: "runtime_hook", SampledAt: "2026-11-17T12:00:00Z", GenerationProvenance: "unresolved"}},
		{"bad_source", rigActivity{State: "running", EvidenceSource: "invented", SampledAt: "2026-11-17T12:00:00Z"}},
		{"bad_time", rigActivity{State: "running", EvidenceSource: "runtime_hook", SampledAt: "invalid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalRigActivity(&tc.activity); got.State != "unknown" {
				t.Fatalf("unverified activity became positive: %+v", got)
			}
		})
	}
	if canonicalRigActivity(nil) != nil || canonicalRigStartup("invented") != "unknown" {
		t.Fatal("invented missing facts")
	}
}

func TestRigObservationsDoNotDiscoverOtherEnabledManagers(t *testing.T) {
	executable, log := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	reply := request(handler, "GET", "/api/coordination?provider=rig&observations=rig", "", "")
	requireStatus(t, reply, 200)
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("Rig tile polling invoked Agent Deck: %v", err)
	}
}
