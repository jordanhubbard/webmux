package httpapi

import (
	"errors"
	"math"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/agent"
)

func (s *Server) getAgentConfig(w http.ResponseWriter, r *http.Request, owner string) {
	value, err := s.sessions.Agents().Config()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, value)
}

func (s *Server) agentAccess(w http.ResponseWriter, r *http.Request) (agent.Definition, bool) {
	service := s.sessions.Agents()
	c, err := service.Access("")
	// Enforce per-definition policy as well as the global enabled/user-count gate.
	// Config errors are deliberately non-destructive, matching the existing server.
	if cleanupErr := s.sessions.EnforceAgentAccess(); cleanupErr != nil {
		s.logger.Error("Agent access cleanup failed", "error", cleanupErr)
		writeError(w, 500, "Failed to enforce agent access policy")
		return agent.Definition{}, false
	}
	if err != nil {
		var access *agent.AccessError
		if errors.As(err, &access) {
			writeError(w, access.Status, access.Message)
		} else {
			writeError(w, 500, err.Error())
		}
		return agent.Definition{}, false
	}
	id := r.PathValue("agentId")
	if id == "" {
		return agent.Definition{}, true
	}
	d, ok := c.Find(id)
	if !ok {
		writeError(w, 404, "Agent definition not found")
	}
	return d, ok
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request, owner string) {
	d, ok := s.agentAccess(w, r)
	if !ok {
		return
	}
	var items []agent.Session
	var err error
	message := "Failed to list agent sessions"
	if d.ID == "" {
		items, err = s.sessions.Agents().ListAll(r.Context())
	} else {
		message = "Failed to list " + d.Label + " sessions"
		items, err = s.sessions.Agents().List(r.Context(), d.ID)
	}
	if err != nil {
		s.logger.Warn(message, "error", err)
		writeError(w, 503, message)
		return
	}
	writeJSON(w, 200, items)
}

func agentTermSize(input map[string]any) (int, int) {
	dimension := func(key string, fallback, minimum, maximum int) int {
		value, ok := input[key].(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			return fallback
		}
		return int(math.Max(float64(minimum), math.Min(float64(maximum), math.Floor(value))))
	}
	return dimension("cols", 120, 40, 240), dimension("rows", 40, 10, 80)
}

func (s *Server) hasAgentSession(w http.ResponseWriter, r *http.Request, d agent.Definition, name, message string) bool {
	exists, err := s.sessions.Agents().HasSession(r.Context(), d.ID, name)
	if err != nil {
		s.logger.Warn(message, "error", err)
		writeError(w, 500, message)
		return false
	}
	if !exists {
		writeError(w, 404, d.Label+" session not found")
		return false
	}
	return true
}

func (s *Server) attachAgent(w http.ResponseWriter, r *http.Request, owner string) {
	d, ok := s.agentAccess(w, r)
	if !ok {
		return
	}
	var input map[string]any
	if !decodeJSON(w, r, &input) {
		return
	}
	name, ok := input["name"].(string)
	if !ok || name == "" {
		writeError(w, 400, "name is required")
		return
	}
	message := "Failed to attach " + d.Label + " session"
	if !s.hasAgentSession(w, r, d, name, message) {
		return
	}
	cols, rows := agentTermSize(input)
	value, created, err := s.sessions.EnsureAgentAttach(owner, d.ID, name, cols, rows)
	if err != nil {
		s.logger.Warn(message, "error", err)
		writeError(w, 500, message)
		return
	}
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, value)
}
func (s *Server) scratchAgent(w http.ResponseWriter, r *http.Request, owner string) {
	d, ok := s.agentAccess(w, r)
	if !ok {
		return
	}
	var input map[string]any
	if !decodeJSON(w, r, &input) {
		return
	}
	name := ""
	if value, present := input["selectedName"]; present {
		var ok bool
		name, ok = value.(string)
		if !ok {
			writeError(w, 400, "selectedName must be a string")
			return
		}
	}
	message := "Failed to create " + d.Label + " scratch shell"
	cwd := ""
	if name != "" {
		if !s.hasAgentSession(w, r, d, name, message) {
			return
		}
		cwd = s.sessions.Agents().PaneCurrentPath(r.Context(), d.ID, name)
	}
	cols, rows := agentTermSize(input)
	value, created, err := s.sessions.EnsureAgentScratch(owner, d.ID, cols, rows, cwd)
	if err != nil {
		s.logger.Warn(message, "error", err)
		writeError(w, 500, message)
		return
	}
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, value)
}
