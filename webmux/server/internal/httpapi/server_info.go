package httpapi

import (
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/buildinfo"
)

// This handler describes compiled contracts without invoking their discovery
// paths, observing provider availability, or authorizing any operation.
func (s *Server) serverInfo(w http.ResponseWriter, _ *http.Request, _ string) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	identity, coordination, administration := "account-jwt", "administrator", "administrator"
	if config.Auth.Mode == "none" {
		identity, coordination, administration = "trusted-anonymous", "trusted-anonymous", "unavailable"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"contract": "webmux-server-info/v1",
		"build":    buildinfo.Current(),
		"contracts": map[string]string{
			"coordination": "webmux-coordination/v1", "serverDirectory": "webmux-server-directory/v1",
		},
		"access": map[string]any{
			"mode": config.Auth.Mode, "identity": identity,
			"coordination": coordination, "accountAdministration": administration,
			"federation": map[string]any{
				"credentialScope": "one-owner-terminal", "serverInfo": false, "coordination": false, "osSandbox": false,
			},
		},
		"limits": map[string]any{
			"providerAvailability": "not-observed", "dynamicProviderRegistration": false,
			"codexConversationHistory": false, "codexGoals": false,
			"terminalMessageReceipt": "queued-not-processed", "terminalMessageIdempotent": false,
		},
	})
}
