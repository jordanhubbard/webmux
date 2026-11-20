package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The HTTP contract must preserve native turn evidence across both read paths.
// Identical display text/timestamps do not identify a turn, and rereading a
// native turn must neither generate a fresh identity nor consume/deduplicate it.
func TestAgentDeckCompletionEvidenceSurvivesCoordinationAPI(t *testing.T) {
	for _, inboxProfile := range []string{"work", "default"} {
		t.Run(inboxProfile, func(t *testing.T) {
			executable, calls := fakeAgentDeck(t)
			t.Setenv("AGENTDECK_PROFILE", "unrelated")
			t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
			records := []map[string]any{}
			for _, fingerprint := range []string{"child@opaque/Ω==", "child@opaque/Ω==", "child@next-turn", ""} {
				record := map[string]any{"child_session_id": "remote-1", "source_remote": "sparky", "text": "same displayed completion", "timestamp": "2026-11-19T20:00:00Z", "kind": "finished", "done_status": "ok", "tier": "info"}
				if fingerprint != "" {
					record["turn_fingerprint"] = fingerprint
					record["profile"] = "source-profile"
				}
				records = append(records, record)
			}
			eventFile := filepath.Join(t.TempDir(), "events.json")
			data, err := json.Marshal(records)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(eventFile, data, 0o600); err != nil {
				t.Fatal(err)
			}
			drainFile := filepath.Join(t.TempDir(), "drain.json")
			data, err = json.Marshal(map[string]any{"remote": "sparky", "records": records, "written": 4})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(drainFile, data, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("WEBMUX_AGENT_DECK_TEST_EVENTS", eventFile)
			t.Setenv("WEBMUX_AGENT_DECK_TEST_DRAIN", drainFile)
			script, err := os.ReadFile(executable)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(script), "\n")
			for i, line := range lines {
				switch {
				case strings.Contains(line, `"id":"local-1","title"`):
					lines[i] = strings.Replace(line, `"id":"local-1","title"`, `"id":"local-1","profile":"`+inboxProfile+`","title"`, 1)
				case strings.Contains(line, `printf '%s\n' '[{"child_session_id"`):
					lines[i] = `    cat "$WEBMUX_AGENT_DECK_TEST_EVENTS"`
				case strings.Contains(line, `printf '%s\n' '{"remote":"sparky","fetched"`):
					lines[i] = `    cat "$WEBMUX_AGENT_DECK_TEST_DRAIN"`
				}
			}
			text := strings.Join(lines, "\n")
			for _, command := range []string{"inbox peek --json local-1", "remote drain sparky --into local-1 --json"} {
				text = strings.ReplaceAll(text, `"`+command+`")`, `"-p `+inboxProfile+` `+command+`")`)
			}
			if err = os.WriteFile(executable, []byte(text), 0o700); err != nil {
				t.Fatal(err)
			}
			server, handler := fixture(t, "none")
			setCoordinationIntegrations(t, server, "agent-deck")
			check := func(body []byte) {
				t.Helper()
				var result struct {
					Messages []map[string]any `json:"messages"`
				}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Messages) != 4 {
					t.Fatalf("native records lost or collapsed: %s", body)
				}
				for i, message := range result.Messages {
					if message["provider"] != "agent-deck" || message["from"] != "sparky:remote-1" || message["to"] != "local:local-1" || message["sourceRemote"] != "sparky" || message["inboxProfile"] != inboxProfile {
						t.Fatalf("event provenance lost: %#v", message)
					}
					if i < 3 {
						if message["turnFingerprint"] != records[i]["turn_fingerprint"] || message["sourceProfile"] != "source-profile" {
							t.Fatalf("native turn/profile changed: %#v", message)
						}
					} else {
						if _, ok := message["turnFingerprint"]; ok {
							t.Fatalf("legacy event acquired invented turn identity: %#v", message)
						}
						if _, ok := message["sourceProfile"]; ok {
							t.Fatalf("recipient profile substituted for missing native profile: %#v", message)
						}
					}
				}
			}
			for range 2 {
				response := request(handler, "GET", "/api/coordination", "", "")
				requireStatus(t, response, 200)
				check(response.Body.Bytes())
			}
			response := request(handler, "POST", "/api/coordination/drain", `{"provider":"agent-deck","remote":"sparky","target":"local:local-1"}`, "")
			requireStatus(t, response, 200)
			check(response.Body.Bytes())
			log, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(log), "inbox drain") || strings.Contains(string(log), "session send") || !strings.Contains(string(log), "-p "+inboxProfile+" inbox peek --json local-1") || !strings.Contains(string(log), "-p "+inboxProfile+" remote drain sparky --into local-1 --json") {
				t.Fatalf("observation escaped profile or dispatched/consumed work: %s", log)
			}
		})
	}
}
