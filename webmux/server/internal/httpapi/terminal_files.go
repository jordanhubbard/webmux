package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/terminalfiles"
	"github.com/jordanhubbard/webmux/server/internal/upload"
)

func (s *Server) terminalFile(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	terminal, err := s.sessions.Get(owner, id)
	if err != nil {
		s.sessionError(w, err, "Failed to get terminal")
		return
	}
	if terminal.State != "connected" {
		writeError(w, 409, "Reconnect the terminal before transferring files")
		return
	}
	if terminal.Transport == "exec" && !terminal.Agent() && !terminal.BrowserLocal {
		writeError(w, 409, "Custom exec commands do not identify a file-transfer host; use a local tmux or SSH terminal")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/octet-stream") {
		writeError(w, 400, "Content-Type must be application/octet-stream")
		return
	}
	filename, err := url.PathUnescape(r.Header.Get("X-Filename"))
	if err != nil {
		writeError(w, 400, "Invalid filename encoding")
		return
	}
	staged, err := s.uploads.Store(r.Context(), filename, r.Body)
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrTooLarge):
			writeError(w, 413, err.Error())
		case errors.Is(err, upload.ErrQuota):
			writeError(w, 507, err.Error())
		default:
			s.internalError(w, err)
		}
		return
	}
	// Check ownership again after a potentially slow browser upload.
	if _, err := s.sessions.Get(owner, id); err != nil {
		s.sessionError(w, err, "Terminal was deleted")
		return
	}
	result, err := terminalfiles.Deliver(r.Context(), terminal, s.store, s.sessions.SSHControlPath(id), staged)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 201, result)
}
