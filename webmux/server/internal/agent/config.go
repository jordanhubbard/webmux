// Package agent discovers configured tmux agents and preserves their status files.
package agent

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type Definition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	PluralLabel string `json:"plural_label"`
	Badge       string `json:"badge"`
	TMUXSocket  string `json:"tmux_socket"`
	Workspace   string `json:"workspace"`
	Enabled     bool   `json:"enabled"`
}
type RuntimeConfig struct {
	Enabled                bool         `json:"enabled"`
	CombinedPane           bool         `json:"combined_pane"`
	DisableInMultiUserMode bool         `json:"disable_in_multi_user_mode"`
	Definitions            []Definition `json:"definitions"`
}

func (c RuntimeConfig) Find(id string) (Definition, bool) {
	for _, d := range c.Definitions {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

type AccessError struct {
	Status  int
	Message string
}

func (e *AccessError) Error() string { return e.Message }

type Service struct {
	store    *storage.Store
	logger   *slog.Logger
	run      runner
	now      func() time.Time
	statusMu sync.Mutex
}

func New(store *storage.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, logger: logger, run: runCommand, now: time.Now}
}

func (s *Service) Config() (RuntimeConfig, error) {
	var doc config.Document
	if err := s.store.ReadConfig("app.yaml", &doc); err != nil {
		return RuntimeConfig{}, err
	}
	normalized, err := config.NormalizeAgents(config.AsObject(doc.App["agents"]))
	if err != nil {
		return RuntimeConfig{}, err
	}
	result := RuntimeConfig{Enabled: normalized["enabled"].(bool), CombinedPane: normalized["combined_pane"].(bool), DisableInMultiUserMode: normalized["disable_in_multi_user_mode"].(bool), Definitions: []Definition{}}
	for _, value := range normalized["definitions"].([]any) {
		d := value.(map[string]any)
		result.Definitions = append(result.Definitions, Definition{ID: d["id"].(string), Label: d["label"].(string), PluralLabel: d["plural_label"].(string), Badge: d["badge"].(string), TMUXSocket: d["tmux_socket"].(string), Workspace: d["workspace"].(string), Enabled: d["enabled"].(bool)})
	}
	return result, nil
}

func (s *Service) Access(id string) (RuntimeConfig, error) {
	c, err := s.Config()
	if err != nil {
		return c, &AccessError{500, err.Error()}
	}
	if !c.Enabled || len(c.Definitions) == 0 {
		return c, &AccessError{404, "Agent sessions are not enabled"}
	}
	if id != "" {
		if _, ok := c.Find(id); !ok {
			return c, &AccessError{404, "Agent definition not found"}
		}
	}
	if c.DisableInMultiUserMode {
		authConfig, err := auth.LoadConfig(s.store)
		if err != nil || authConfig.Auth.Mode != "none" && len(authConfig.Auth.Users) > 1 {
			return c, &AccessError{403, "Agent sessions are disabled in multi-user mode"}
		}
	}
	return c, nil
}
func (s *Service) definition(id string) (Definition, error) {
	c, err := s.Config()
	if err != nil {
		return Definition{}, err
	}
	if definition, ok := c.Find(id); ok && c.Enabled {
		return definition, nil
	}
	return Definition{}, fmt.Errorf("Agent '%s' is not configured", id)
}
