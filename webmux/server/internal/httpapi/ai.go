package httpapi

import (
	"encoding/json"
	"github.com/jordanhubbard/webmux/server/internal/credentialhub"
	"net/http"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/ai"
	"github.com/jordanhubbard/webmux/server/internal/config"
)

func (s *Server) aiStatus(w http.ResponseWriter, r *http.Request, owner string) {
	b, p, token, found, err := s.credentialBindingSnapshot(owner)
	if found || err != nil {
		status := ai.Status{}
		generation, ge := s.auth.Generation(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err == nil && (ge != nil || generation != b.Principal) {
			err = credentialhub.ErrDenied
		}
		if err == nil {
			_, err = s.exchangeCredential(r.Context(), owner, p, token, credentialExchangeInput{Operation: "availability", SecretID: b.SecretID, Consumer: b.Consumer})
			status.Available = err == nil
			status.Providers.OpenAI = b.Consumer == "ai.openai"
			status.Providers.NVIDIA = b.Consumer == "ai.nvidia"
		}
		writeJSON(w, 200, struct {
			ai.Status
			CredentialHub bool `json:"credential_hub"`
		}{status, true})
		return
	}
	writeJSON(w, 200, s.ai.Status())
}

func (s *Server) aiChat(w http.ResponseWriter, r *http.Request, owner string) {
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
	generation, _ := s.auth.Generation(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	privateKey, consumer, selected, err := s.credentialForAI(r.Context(), owner, generation)
	if err != nil {
		s.credentialError(w, owner, "consume", err)
		return
	}
	service := s.ai
	if selected {
		service, err = s.ai.WithCredential(consumer, privateKey)
		if err != nil {
			s.credentialError(w, owner, "consume", credentialhub.ErrUnavailable)
			return
		}
	}
	response, err := service.Chat(r.Context(), input)
	if privateKey != "" {
		response.Reply = strings.ReplaceAll(response.Reply, privateKey, "[redacted credential]")
		response.Model = strings.ReplaceAll(response.Model, privateKey, "[redacted credential]")
	}
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "AI assistant unavailable", "detail": err.Error(), "hint": ai.Hint})
		return
	}
	writeJSON(w, 200, response)
}
