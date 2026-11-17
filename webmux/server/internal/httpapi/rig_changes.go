package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

// One wait per owner and at most eight in this server. Slow streams cannot queue
// behind lifecycle commands or multiply into an unbounded number of Node processes.
func (s *Server) getRigChanges(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	started := false
	reply := func(change openrigcoord.Change) {
		if !started {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
		}
		_ = json.NewEncoder(w).Encode(struct {
			Contract string `json:"contract"`
			openrigcoord.Change
		}{"webmux-rig-changes/v1", change})
	}
	result := openrigcoord.Change{}
	if !s.coordinationEnabled("rig") || os.Getenv("WEBMUX_COORDINATION_DISABLED") == "1" {
		reply(result)
		return
	}
	if executable, _ := openRigExecutable(); executable == "" {
		reply(result)
		return
	}
	s.rigChangesMu.Lock()
	if s.rigChangesOwners == nil {
		s.rigChangesOwners = make(map[string]bool)
	}
	if s.rigChangesOwners[owner] || len(s.rigChangesOwners) >= 8 {
		s.rigChangesMu.Unlock()
		reply(result)
		return
	}
	s.rigChangesOwners[owner] = true
	s.rigChangesMu.Unlock()
	defer func() { s.rigChangesMu.Lock(); delete(s.rigChangesOwners, owner); s.rigChangesMu.Unlock() }()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	reply(openrigcoord.WaitChange(ctx, s.store.Home, owner, func() {
		started = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-WebMux-Rig-Changes", "subscribed")
		w.WriteHeader(200)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
}
