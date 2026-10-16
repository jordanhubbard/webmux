package httpapi

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/macsetup"
)

type macSetupPreferences struct {
	Skipped bool `json:"skipped" yaml:"skipped"`
}

func loopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback()
}

func (s *Server) macSetupAccess(w http.ResponseWriter, r *http.Request, actor string) bool {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return false
	}
	if config.Auth.Mode == "none" {
		if !loopbackRequest(r) {
			writeError(w, 403, "Mac setup requires a local connection when authentication is disabled")
			return false
		}
	} else if err := checkAdmin(&config, actor); err != nil {
		s.changeError(w, err)
		return false
	}
	// Settings actions affect the server's desktop. Do not allow cross-origin
	// browser requests, including deployments with authentication disabled.
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		// Match the browser-facing Host, as our WebSocket origin check does.
		// TLS may terminate at a reverse proxy before this request reaches Go.
		if err != nil || parsed.Host != r.Host || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			writeError(w, 403, "Mac setup requires the same origin")
			return false
		}
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, 403, "Mac setup requires the same origin")
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	return true
}
func (s *Server) getMacSetup(w http.ResponseWriter, r *http.Request, actor string) {
	if !s.macSetupAccess(w, r, actor) {
		return
	}
	status := s.macSetup.Status()
	status.CanOpenSettings = status.Supported && loopbackRequest(r)
	var prefs macSetupPreferences
	// Missing preferences are the first-run case. Other errors remain visible.
	if err := s.store.ReadConfig("macos-setup.yaml", &prefs); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": status, "skipped": prefs.Skipped})
}
func (s *Server) macSetupAction(w http.ResponseWriter, r *http.Request, actor string) {
	if !s.macSetupAccess(w, r, actor) {
		return
	}
	if runtime.GOOS != "darwin" {
		writeError(w, 400, "Mac setup is unavailable on this host")
		return
	}
	var body struct {
		Action    string `json:"action"`
		Transport string `json:"transport"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	switch body.Action {
	case "check":
		result, err := s.macSetup.Check(r.Context(), body.Transport)
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, result)
	case "skip":
		if err := s.store.WriteConfig("macos-setup.yaml", macSetupPreferences{Skipped: true}); err != nil {
			s.internalError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "privacy", "sharing", "reveal":
		if !loopbackRequest(r) {
			writeError(w, 403, "Open System Settings on the Mac running WebMux; this browser is connected remotely")
			return
		}
		if err := macsetup.Open(r.Context(), body.Action); err != nil {
			writeError(w, 500, "Could not open the Mac desktop. Follow the displayed System Settings instructions.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "restart":
		if s.restart == nil {
			writeError(w, 503, "Restart is unavailable; restart WebMux using your service manager")
			return
		}
		if err := s.restart(); err != nil {
			writeError(w, 409, err.Error())
			return
		}
		writeJSON(w, 202, map[string]bool{"restarting": true})
	default:
		writeError(w, 400, "Unknown Mac setup action")
	}
}
