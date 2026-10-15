package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/upload"
)

func TestTerminalFileOwnershipAndHostRouting(t *testing.T) {
	s, handler := fixture(t, "none")
	_ = s.sessions.Close()
	if err := s.store.WriteSessions(map[string]any{"sessions": []map[string]any{
		{"id": "local", "owner": "anonymous", "transport": "local", "state": "connected"},
		{"id": "foreign", "owner": "someone-else", "transport": "local", "state": "connected"},
		{"id": "offline", "owner": "anonymous", "transport": "local", "state": "disconnected"},
		{"id": "exec", "owner": "anonymous", "transport": "exec", "state": "connected"},
	}}); err != nil {
		t.Fatal(err)
	}
	var err error
	s.sessions, err = session.New(s.store, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	body := "\x89PNG\r\n\x1a\nexact image bytes"
	send := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/sessions/"+id+"/files", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/octet-stream")
		r.Header.Set("X-Filename", "screen%20shot.png")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	requireStatus(t, send("foreign"), 404)
	requireStatus(t, send("missing"), 404)
	requireStatus(t, send("offline"), 409)
	requireStatus(t, send("exec"), 409)
	w := send("local")
	requireStatus(t, w, 201)
	var result upload.Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(result.Path)
	if err != nil || string(bytes) != body {
		t.Fatalf("bytes changed: %v", err)
	}
	if !strings.HasPrefix(result.Path, s.store.Home) || !strings.HasSuffix(result.Path, ".png") {
		t.Fatalf("wrong local path: %s", result.Path)
	}
}
