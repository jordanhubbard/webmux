package httpapi

import (
	"errors"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (s *Server) sessionError(w http.ResponseWriter, err error, message string) {
	var validation *config.ValidationError
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, 404, "Session not found")
	case errors.As(err, &validation):
		writeError(w, 400, validation.Error())
	case errors.Is(err, session.ErrClosed):
		writeError(w, 503, "Server is shutting down")
	default:
		s.logger.Error(message, "error", err)
		writeError(w, 500, message)
	}
}
func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, owner string) {
	writeJSON(w, 200, s.sessions.List(owner))
}
func (s *Server) getSession(w http.ResponseWriter, r *http.Request, owner string) {
	value, err := s.sessions.Get(owner, r.PathValue("id"))
	if err != nil {
		s.sessionError(w, err, "Failed to get session")
		return
	}
	writeJSON(w, 200, value)
}
func (s *Server) createSession(w http.ResponseWriter, r *http.Request, owner string) {
	var input session.CreateRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.sessions.Create(owner, input)
	if err != nil {
		s.sessionError(w, err, "Failed to create session")
		return
	}
	writeJSON(w, 201, value)
}
func (s *Server) patchSession(w http.ResponseWriter, r *http.Request, owner string) {
	if _, err := s.sessions.Get(owner, r.PathValue("id")); err != nil {
		s.sessionError(w, err, "Failed to update session")
		return
	}
	var input session.Patch
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.sessions.Patch(owner, r.PathValue("id"), input)
	if err != nil {
		s.sessionError(w, err, "Failed to update session")
		return
	}
	writeJSON(w, 200, value)
}
func (s *Server) reconnectSession(w http.ResponseWriter, r *http.Request, owner string) {
	if _, err := s.sessions.Get(owner, r.PathValue("id")); err != nil {
		s.sessionError(w, err, "Failed to reconnect session")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.sessions.Reconnect(owner, r.PathValue("id"), input.Password)
	if err != nil {
		s.sessionError(w, err, "Failed to reconnect session")
		return
	}
	writeJSON(w, 200, value)
}
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, owner string) {
	if err := s.sessions.Delete(owner, r.PathValue("id")); err != nil {
		s.sessionError(w, err, "Failed to delete session")
		return
	}
	w.WriteHeader(204)
}
