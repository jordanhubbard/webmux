package httpapi

import (
	"errors"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/browser"
)

func (s *Server) browserAction(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	terminal, err := s.sessions.Get(owner, id)
	if err != nil {
		s.sessionError(w, err, "Failed to get terminal")
		return
	}
	var req browser.Request
	if !decodeJSONLimit(w, r, &req, 16<<20) {
		return
	}
	// Internal host keys are never accepted from a browser request.
	req.ProfileKey = ""
	if r.Method == http.MethodDelete {
		if err := s.browsers.EndControlled(id, req.Viewer, req.ControlToken); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		w.WriteHeader(204)
		return
	}
	if req.Action == "start" {
		if !browser.ValidViewer(req.Viewer) {
			writeError(w, 400, "Reload WebMux to identify this browser viewer")
			return
		}
		s.browserProfileMu.Lock()
		var profile *browser.Profile
		if req.ProfileID != nil {
			scope, scopeErr := browserProfileScope(terminal)
			if scopeErr != nil {
				s.browserProfileMu.Unlock()
				writeError(w, 409, scopeErr.Error())
				return
			}
			profile, err = s.selectedBrowserProfile(owner, scope, *req.ProfileID)
			if err != nil {
				s.browserProfileMu.Unlock()
				s.catalogError(w, err, "Profile unavailable")
				return
			}
		}
		err = s.browsers.StartProfile(terminal, s.store, profile)
		s.browserProfileMu.Unlock()
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		if _, err := s.sessions.Get(owner, id); err != nil {
			s.browsers.End(id)
			s.sessionError(w, err, "Terminal was deleted")
			return
		}
		control, err := s.browsers.Control(id, req.Viewer, req.ControlToken, true, false, false)
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		req.ControlToken = control.Token
		req.Action = "frame"
	} else if req.Action == "claim" || req.Action == "release" {
		control, err := s.browsers.Control(id, req.Viewer, req.ControlToken, req.Action == "claim", req.Force, req.Action == "release")
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		req.ControlToken = control.Token
		req.Action = "frame"
	}
	if req.Action == "navigate" && !browser.ValidURL(req.URL) {
		writeError(w, 400, "Enter an HTTP or HTTPS URL without embedded credentials")
		return
	}
	frame, err := s.browsers.CallControlled(r.Context(), id, req)
	if err != nil {
		status := 503
		if errors.Is(err, browser.ErrControl) {
			status = 409
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, frame)
}
