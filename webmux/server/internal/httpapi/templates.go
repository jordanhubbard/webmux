package httpapi

import (
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/templates"
)

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, owner string) {
	values := templates.List()
	writeJSON(w, 200, map[string]any{"templates": values, "count": len(values)})
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	value, ok := templates.Get(id)
	if !ok {
		writeError(w, 404, "Template '"+id+"' not found")
		return
	}
	writeJSON(w, 200, value)
}
