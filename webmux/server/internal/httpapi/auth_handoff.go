package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/authhandoff"
	"github.com/jordanhubbard/webmux/server/internal/browser"
)

func (s *Server) authHandoff(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	terminal, err := s.sessions.Get(owner, id)
	if err != nil {
		s.sessionError(w, err, "Failed to get terminal")
		return
	}
	if r.Method == http.MethodDelete {
		s.handoffs.End(owner, id)
		w.WriteHeader(204)
		return
	}
	var req struct {
		Authorization string `json:"authorization"`
		ID            string `json:"id"`
		Callback      string `json:"callback"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if terminal.State != "connected" {
		writeError(w, 409, "Reconnect the terminal and restart sign-in")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cmd, err := browser.WorkerCommand(ctx, terminal, s.store, "--auth-callback")
	if err != nil {
		writeError(w, 409, "Callback relay requires a Local or SSH/Mosh terminal with a known host")
		return
	}
	if req.ID == "" {
		ticket, err := s.handoffs.Begin(owner, id, req.Authorization)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"id": ticket, "expires_in": int(authhandoff.Lifetime.Seconds())})
		return
	}
	payload, err := s.handoffs.Take(owner, id, req.ID, req.Callback)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if terminal.Transport == "local" || terminal.BrowserLocal {
		err = authhandoff.Deliver(ctx, payload)
	} else {
		// Secrets travel only on stdin, not process arguments or diagnostics.
		data, _ := json.Marshal(payload)
		cmd.Stdin = bytes.NewReader(data)
		var output []byte
		output, err = cmd.Output()
		if err == nil {
			var result struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(output, &result) != nil || result.Error != "" {
				writeError(w, 502, "Remote callback failed; check the CLI and restart sign-in")
				return
			}
		}
	}
	if err != nil {
		writeError(w, 502, "Callback delivery failed; check the CLI listener, SSH access and matching WebMux installation, then restart sign-in")
		return
	}
	writeJSON(w, 200, map[string]bool{"delivered": true})
}
