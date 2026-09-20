package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/ai"
	"github.com/jordanhubbard/webmux/server/internal/config"
)

func (s *Server) aiStatus(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, 200, s.ai.Status())
}

func (s *Server) aiChat(w http.ResponseWriter, r *http.Request, _ string) {
	var raw struct {
		Message json.RawMessage `json:"message"`
		Context json.RawMessage `json:"context"`
		History json.RawMessage `json:"history"`
	}
	if !decodeJSON(w, r, &raw) {
		return
	}
	var input ai.Request
	if json.Unmarshal(raw.Message, &input.Message) != nil || config.TrimSpace(input.Message) == "" {
		writeError(w, 400, "message required")
		return
	}
	if len(raw.Context) != 0 && json.Unmarshal(raw.Context, &input.Context) != nil {
		writeError(w, 400, "context must be a string")
		return
	}
	if len(raw.History) != 0 && (string(raw.History) == "null" || json.Unmarshal(raw.History, &input.History) != nil) {
		writeError(w, 400, "history must be an array of messages")
		return
	}
	for _, message := range input.History {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" {
			writeError(w, 400, "history contains an invalid role")
			return
		}
	}
	response, err := s.ai.Chat(r.Context(), input)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "AI assistant unavailable", "detail": err.Error(), "hint": ai.Hint})
		return
	}
	writeJSON(w, 200, response)
}
