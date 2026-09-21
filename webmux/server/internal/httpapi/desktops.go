package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/desktop"
)

func (s *Server) desktopError(w http.ResponseWriter, err error, message string) {
	var invalid *config.ValidationError
	switch {
	case errors.Is(err, desktop.ErrNotFound):
		writeError(w, 404, "Session not found")
	case errors.Is(err, desktop.ErrClosed):
		writeError(w, 503, "Server is shutting down")
	case errors.As(err, &invalid):
		writeError(w, 400, invalid.Error())
	default:
		s.logger.Error(message, "error", err)
		writeError(w, 500, message)
	}
}

func (s *Server) registerDesktops(mux *http.ServeMux) {
	for kind, broker := range map[string]*desktop.Broker{"vnc": s.vnc, "rdp": s.rdp} {
		base := "/api/" + kind + "/sessions"
		label := strings.ToUpper(kind)
		mux.Handle("GET "+base, s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) { writeJSON(w, 200, broker.List(owner)) }))
		mux.Handle("GET "+base+"/{id}", s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
			value, err := broker.Get(owner, r.PathValue("id"))
			if err != nil {
				s.desktopError(w, err, "Failed to get "+label+" session")
				return
			}
			writeJSON(w, 200, value)
		}))
		mux.Handle("POST "+base, s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
			var input desktop.CreateRequest
			if !decodeJSON(w, r, &input) {
				return
			}
			value, err := broker.Create(owner, input)
			if err != nil {
				s.desktopError(w, err, "Failed to create "+label+" session")
				return
			}
			writeJSON(w, 201, value)
		}))
		mux.Handle("PATCH "+base+"/{id}", s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
			id := r.PathValue("id")
			if _, err := broker.Get(owner, id); err != nil {
				s.desktopError(w, err, "Failed to update "+label+" session")
				return
			}
			var input map[string]any
			if !decodeJSON(w, r, &input) {
				return
			}
			_, hasRow := input["row"]
			_, hasCol := input["col"]
			if !hasRow || !hasCol {
				writeError(w, 400, "row and col are required")
				return
			}
			row, rowOK := input["row"].(float64)
			col, colOK := input["col"].(float64)
			if !rowOK || !colOK {
				writeError(w, 400, "row and col must be non-negative numbers")
				return
			}
			value, err := broker.Move(owner, id, row, col)
			if err != nil {
				s.desktopError(w, err, "Failed to update "+label+" session")
				return
			}
			writeJSON(w, 200, value)
		}))
		mux.Handle("POST "+base+"/{id}/reconnect", s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
			value, err := broker.SetState(owner, r.PathValue("id"), "connecting")
			if err != nil {
				s.desktopError(w, err, "Failed to reconnect "+label+" session")
				return
			}
			writeJSON(w, 200, value)
		}))
		mux.Handle("DELETE "+base+"/{id}", s.protected(false, func(w http.ResponseWriter, r *http.Request, owner string) {
			if err := broker.Delete(owner, r.PathValue("id")); err != nil {
				s.desktopError(w, err, "Failed to delete "+label+" session")
				return
			}
			w.WriteHeader(204)
		}))
	}
}
