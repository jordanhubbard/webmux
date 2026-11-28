package httpapi

import (
	"net/http"
	"os"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

type coordinationSeat struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	SessionID string `json:"sessionId,omitempty"`
	Minimized bool   `json:"minimized"`
	Status    string `json:"status"`
	External  bool   `json:"external,omitempty"`
}

type coordinationProject struct {
	Provider string             `json:"provider"`
	Name     string             `json:"name"`
	Seats    []coordinationSeat `json:"seats"`
}

type coordinationInventory struct {
	EnabledProviders      []string              `json:"enabledProviders"`
	DefaultLaunchProvider string                `json:"defaultLaunchProvider"`
	DefaultView           string                `json:"defaultView"`
	Projects              []coordinationProject `json:"projects"`
	RemovedProviders      []string              `json:"removedProviders"`
	Message               string                `json:"message"`
}

func (s *Server) coordinationAdmin(w http.ResponseWriter, owner string) bool {
	loaded, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return false
	}
	if loaded.Auth.Mode == "none" {
		return true
	}
	if err := checkAdmin(&loaded, owner); err != nil {
		s.changeError(w, err)
		return false
	}
	return true
}

func (s *Server) getCoordination(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.coordinationAdmin(w, owner) {
		return
	}
	launch, view := "webmux", "all"
	var document config.Document
	if err := s.store.ReadConfig("app.yaml", &document); err != nil && !os.IsNotExist(err) {
		s.internalError(w, err)
		return
	}
	if document.App != nil {
		if normalized, err := config.Normalize(document, false); err == nil {
			prefs := config.AsObject(normalized.App["coordination"])
			if value, _ := prefs["default_launch_provider"].(string); value != "" {
				launch = value
			}
			if value, _ := prefs["default_view"].(string); value != "" {
				view = value
			}
		}
	}
	projects := []coordinationProject{}
	index := map[string]int{}
	for _, item := range s.sessions.List(owner) {
		if item.Agent() || item.Companion != "" {
			continue
		}
		name := item.ProjectLabel
		if name == "" {
			name = "WebMux"
		}
		at, ok := index[name]
		if !ok {
			at = len(projects)
			index[name] = at
			projects = append(projects, coordinationProject{Provider: "webmux", Name: name})
		}
		status := item.State
		if status == "" {
			status = "saved"
		}
		projects[at].Seats = append(projects[at].Seats, coordinationSeat{
			ID: item.ID, Title: item.Title, SessionID: item.ID, Minimized: item.Minimized, Status: status, External: item.Rig != nil,
		})
	}
	writeJSON(w, 200, coordinationInventory{
		EnabledProviders:      []string{},
		DefaultLaunchProvider: launch,
		DefaultView:           view,
		Projects:              projects,
		RemovedProviders:      []string{"agent-deck", "rig"},
		Message:               "Agent Deck and OpenRig are no longer launched by WebMux. Existing tmux sessions were left running. Reattach one from a WebMux terminal with tmux attach.",
	})
}

func (s *Server) attachCoordinatedSession(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.coordinationAdmin(w, owner) {
		return
	}
	var body struct {
		Provider  string `json:"provider"`
		SessionID string `json:"sessionId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider != "webmux" || body.SessionID == "" {
		writeError(w, http.StatusConflict, "Only built-in WebMux terminals can be reopened from Coordination. Agent Deck and OpenRig are no longer launched by WebMux. Attach an existing tmux session from a WebMux terminal with tmux attach. Nothing was stopped.")
		return
	}
	minimized := false
	value, err := s.sessions.Patch(owner, body.SessionID, session.Patch{Minimized: &minimized})
	if err != nil {
		s.sessionError(w, err, "Failed to reopen session")
		return
	}
	writeJSON(w, 200, value)
}
