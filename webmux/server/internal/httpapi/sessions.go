package httpapi

import (
	"errors"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/agent"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (s *Server) sessionError(w http.ResponseWriter, err error, message string) {
	var validation *config.ValidationError
	var access *agent.AccessError
	switch {
	case errors.As(err, &access):
		writeError(w, access.Status, access.Message)
	case errors.Is(err, session.ErrNotFound):
		writeError(w, 404, "Session not found")
	case errors.Is(err, session.ErrRecoveryConflict):
		writeError(w, 409, err.Error())
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

func (s *Server) getSessionRecovery(w http.ResponseWriter, r *http.Request, owner string) {
	value, err := s.sessions.Recovery(owner, r.PathValue("id"))
	if err != nil {
		s.sessionError(w, err, "Failed to read session recovery")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, value)
}

func (s *Server) recoverSession(w http.ResponseWriter, r *http.Request, owner string) {
	if _, err := s.sessions.Get(owner, r.PathValue("id")); err != nil {
		s.sessionError(w, err, "Failed to recover session")
		return
	}
	var input session.RecoveryRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.sessions.Recover(owner, r.PathValue("id"), input)
	if err != nil {
		s.sessionError(w, err, "Failed to recover session")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, value)
}

func (s *Server) patchRecovery(w http.ResponseWriter, r *http.Request, owner string) {
	var input struct {
		AutoResume *bool `json:"auto_resume"`
	}
	if _, err := s.sessions.Get(owner, r.PathValue("id")); err != nil {
		s.sessionError(w, err, "Failed to update recovery")
		return
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.AutoResume == nil {
		writeError(w, 400, "auto_resume is required")
		return
	}
	if err := s.sessions.SetAutoRecovery(owner, r.PathValue("id"), *input.AutoResume); err != nil {
		s.sessionError(w, err, "Failed to update recovery")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"auto_resume": *input.AutoResume})
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
	if input.HostID != nil {
		current, err := s.sessions.Get(owner, r.PathValue("id"))
		if err != nil {
			s.sessionError(w, err, "Failed to associate host")
			return
		}
		var hosts hostsConfig
		if err := s.store.ReadConfig("hosts.yaml", &hosts); err != nil {
			s.internalError(w, err)
			return
		}
		match := false
		for _, host := range hosts.Hosts {
			if host["id"] == *input.HostID && host["hostname"] == current.Hostname && host["username"] == current.Username && host["port"] == current.Port {
				match = true
				break
			}
		}
		if !match {
			writeError(w, 400, "Saved host must match this SSH session's hostname, account and port")
			return
		}
	}
	value, err := s.sessions.Patch(owner, r.PathValue("id"), input)
	if err != nil {
		s.sessionError(w, err, "Failed to update session")
		return
	}
	writeJSON(w, 200, value)
}
func (s *Server) reconnectSession(w http.ResponseWriter, r *http.Request, owner string) {
	value, err := s.sessions.Get(owner, r.PathValue("id"))
	if err != nil {
		s.sessionError(w, err, "Failed to reconnect session")
		return
	}
	if (value.Companion == "openrig" || value.OpenRigView != "") && !s.openRigAccess(w, owner) {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	s.handoffs.End(owner, r.PathValue("id"))
	value, err = s.sessions.Reconnect(owner, r.PathValue("id"), input.Password)
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
	s.handoffs.End(owner, r.PathValue("id"))
	s.browsers.End(r.PathValue("id"))
	w.WriteHeader(204)
}
