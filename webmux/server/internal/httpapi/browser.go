package httpapi

import (
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
	if r.Method == http.MethodDelete {
		s.browsers.End(id)
		w.WriteHeader(204)
		return
	}
	var req browser.Request
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Action == "start" {
		if err := s.browsers.Start(terminal, s.store); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		// A terminal may have been deleted while the worker was being started.
		if _, err := s.sessions.Get(owner, id); err != nil {
			s.browsers.End(id)
			s.sessionError(w, err, "Terminal was deleted")
			return
		}
		req.Action = "frame"
	}
	if req.Action == "navigate" && !browser.ValidURL(req.URL) {
		writeError(w, 400, "Enter an HTTP or HTTPS URL without embedded credentials")
		return
	}
	frame, err := s.browsers.Call(r.Context(), id, req)
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	writeJSON(w, 200, frame)
}
