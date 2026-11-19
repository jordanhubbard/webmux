package httpapi

import "time"

// rigActivity carries facts only: no prompts, raw evidence or hook credentials.
type rigActivity struct {
	State                string `json:"state"`
	EvidenceSource       string `json:"evidenceSource"`
	SampledAt            string `json:"sampledAt"`
	Stale                bool   `json:"stale,omitempty"`
	GenerationProvenance string `json:"generationProvenance,omitempty"`
}

func canonicalRigActivity(activity *rigActivity) *rigActivity {
	if activity == nil {
		return nil
	}
	value := *activity
	switch value.State {
	case "running", "needs_input", "idle", "unknown":
	default:
		value.State = "unknown"
	}
	switch value.EvidenceSource {
	case "runtime_hook", "pane_heuristic", "terminal_activity", "tmux_session", "external_cli", "session_registry":
	default:
		value.EvidenceSource = "unknown"
		value.State = "unknown"
	}
	switch value.GenerationProvenance {
	case "resolved", "unresolved", "":
	default:
		value.GenerationProvenance = "unresolved"
	}
	if _, err := time.Parse(time.RFC3339Nano, value.SampledAt); err != nil {
		value.SampledAt = ""
		value.Stale = true
	}
	if value.Stale || (value.GenerationProvenance == "unresolved" || value.EvidenceSource == "runtime_hook" && value.GenerationProvenance != "resolved") {
		value.State = "unknown"
	}
	return &value
}

func canonicalRigStartup(value string) string {
	switch value {
	case "pending", "ready", "attention_required", "failed":
		return value
	}
	return "unknown"
}
