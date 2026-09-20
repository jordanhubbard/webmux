package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/templates"
)

func TestTemplateRoutingAndAuthentication(t *testing.T) {
	_, local := fixture(t, "local")
	for _, path := range []string{"/api/sessions/templates", "/api/sessions/templates/htop"} {
		requireStatus(t, request(local, "GET", path, "", ""), 401)
	}
	_, trusted := fixture(t, "none")
	listed := request(trusted, "GET", "/api/sessions/templates/", "", "")
	requireStatus(t, listed, 200)
	var result struct {
		Templates []templates.Template `json:"templates"`
		Count     int                  `json:"count"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Count != 5 || len(result.Templates) != result.Count {
		t.Fatal("incomplete template list")
	}
	for _, value := range result.Templates {
		response := request(trusted, "GET", "/api/sessions/templates/"+value.ID, "", "")
		requireStatus(t, response, 200)
		var got templates.Template
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != value.ID || got.InitialCommand != templates.Command(value.ID) {
			t.Fatal("template launch mismatch")
		}
	}
	requireStatus(t, request(trusted, "GET", "/api/sessions/templates/missing", "", ""), 404)
	// The literal template routes must not interfere with normal session IDs.
	requireStatus(t, request(trusted, "GET", "/api/sessions/missing", "", ""), 404)
}
