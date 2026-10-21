package httpapi

import (
	"net/http"
	"os/exec"
	"runtime"

	"github.com/jordanhubbard/webmux/server/internal/auth"
)

func openRigExecutable() (string, string) {
	if runtime.GOOS == "windows" {
		return "", "Run WebMux and OpenRig together on Linux or macOS. Native Windows OpenRig is unsupported."
	}
	path, err := exec.LookPath("rig")
	if err != nil {
		return "", "Install OpenRig on this WebMux server and make rig available on the service PATH."
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return "", "Install tmux on this WebMux server before attaching the OpenRig dashboard."
	}
	return path, ""
}

func (s *Server) getOpenRig(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	path, message := openRigExecutable()
	writeJSON(w, 200, map[string]any{"available": path != "", "message": message, "session": s.sessions.OpenRigSession(owner)})
}

func (s *Server) attachOpenRig(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	path, message := openRigExecutable()
	if path == "" {
		writeError(w, 409, message)
		return
	}
	value, err := s.sessions.EnsureOpenRig(owner, path)
	if err != nil {
		s.sessionError(w, err, "Cannot attach the OpenRig dashboard. Check the host's OpenRig installation and shared dashboard.")
		return
	}
	writeJSON(w, 200, value)
}

// Match host Settings: trusted mode has no login; authenticated mode requires
// an administrator in addition to the normal owner/session checks.
func (s *Server) openRigAccess(w http.ResponseWriter, owner string) bool {
	if err := s.openRigPermission(owner); err != nil {
		s.changeError(w, err)
		return false
	}
	return true
}

func (s *Server) openRigPermission(owner string) error {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		return err
	}
	if config.Auth.Mode != "none" {
		return checkAdmin(&config, owner)
	}
	return nil
}
