package httpapi

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAgentRoutePolicyAndValidation(t *testing.T) {
	s, handler := fixture(t, "none")
	set := func(enabled bool) {
		t.Helper()
		if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"agents": map[string]any{"enabled": enabled, "disable_in_multi_user_mode": false, "definitions": []any{map[string]any{"id": "alpha", "label": "Alpha", "tmux_socket": "never-contact"}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	set(false)
	requireStatus(t, request(handler, "GET", "/api/agents/config", "", ""), 200)
	requireStatus(t, request(handler, "GET", "/api/agents/sessions", "", ""), 404)
	set(true)
	t.Setenv("PATH", t.TempDir())
	requireStatus(t, request(handler, "GET", "/api/agents/missing/sessions", "", ""), 404)
	requireStatus(t, request(handler, "GET", "/api/agents/alpha/sessions", "", ""), 503)
	all := request(handler, "GET", "/api/agents/sessions", "", "")
	requireStatus(t, all, 200)
	var items []any
	if err := json.Unmarshal(all.Body.Bytes(), &items); err != nil || items == nil || len(items) != 0 {
		t.Fatal(all.Body.String(), err)
	}
	for _, body := range []string{`{}`, `{"name":false}`, `{"name":""}`} {
		requireStatus(t, request(handler, "POST", "/api/agents/alpha/attach", body, ""), 400)
	}
	requireStatus(t, request(handler, "POST", "/api/agents/alpha/scratch", `{"selectedName":null}`, ""), 400)
	if err := os.WriteFile(s.store.ConfigPath("app.yaml"), []byte("[bad"), 0600); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "GET", "/api/agents/config", "", ""), 500)
}
func TestAgentTermSizeDefaultsFloorAndClamp(t *testing.T) {
	for _, tc := range []struct {
		input      map[string]any
		cols, rows int
	}{{nil, 120, 40}, {map[string]any{"cols": "100", "rows": true}, 120, 40}, {map[string]any{"cols": 100.9, "rows": 30.9}, 100, 30}, {map[string]any{"cols": -1.0, "rows": 999.0}, 40, 80}} {
		cols, rows := agentTermSize(tc.input)
		if cols != tc.cols || rows != tc.rows {
			t.Fatal(cols, rows, tc)
		}
	}
}
