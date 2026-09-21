package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/ai"
)

func TestAIAuthenticationValidationAndUnavailable(t *testing.T) {
	_, local := fixture(t, "local")
	requireStatus(t, request(local, "GET", "/api/ai/status", "", ""), 401)
	requireStatus(t, request(local, "POST", "/api/ai/chat", `{"message":"help"}`, ""), 401)
	s, trusted := fixture(t, "none")
	s.ai = ai.New(func(string) string { return "" }, nil)
	status := request(trusted, "GET", "/api/ai/status", "", "")
	requireStatus(t, status, 200)
	var got ai.Status
	if err := json.Unmarshal(status.Body.Bytes(), &got); err != nil || got.Available || got.Model != "gpt-4o-mini" {
		t.Fatal(got, err)
	}
	for _, body := range []string{`{}`, `{"message":2}`, `{"message":"  "}`, `{"message":"help","context":42}`, `{"message":"help","history":null}`, `{"message":"help","history":[{"role":"invalid","content":"bad"}]}`} {
		requireStatus(t, request(trusted, "POST", "/api/ai/chat", body, ""), 400)
	}
	result := request(trusted, "POST", "/api/ai/chat", `{"message":"help"}`, "")
	requireStatus(t, result, 503)
	var unavailable map[string]string
	if err := json.Unmarshal(result.Body.Bytes(), &unavailable); err != nil || unavailable["detail"] != "No LLM API key configured" || unavailable["hint"] != ai.Hint {
		t.Fatal(unavailable, err)
	}
}
