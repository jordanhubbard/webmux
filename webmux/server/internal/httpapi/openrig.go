package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/openrigproof"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/session"
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
	views := s.sessions.OpenRigViews(owner)
	proof := openrigproof.Status(s.store.Home, owner)
	if phase, _ := proof["phase"].(string); phase == "preparing" || phase == "waiting" {
		console, exists := views["proof"]
		if !exists || (console.State != "connected" && console.State != "connecting") {
			proof["phase"] = "interrupted"
			proof["message"] = "The live test console is no longer running. Inspect the agent terminals and run a fresh challenge."
		}
	}
	writeJSON(w, 200, map[string]any{"available": path != "", "message": message, "session": s.sessions.OpenRigSession(owner), "views": views, "proof": proof})
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
	var body struct {
		View   string `json:"view"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, 400, "Invalid OpenRig view.")
		return
	}
	if body.View == "" {
		body.View = "dashboard"
	}
	title := "OpenRig dashboard"
	argv := []string{path, "tui"}
	switch body.View {
	case "dashboard":
	case "agent":
		agents, err := readOpenRigAgents(r.Context(), path)
		if err != nil {
			writeError(w, 502, "Cannot read OpenRig agents. Check the daemon and refresh the agent list.")
			return
		}
		found := false
		for _, agent := range agents {
			if agent.Target == body.Target {
				found = true
				break
			}
		}
		if !found {
			writeError(w, 404, "This agent is no longer listed by OpenRig. Refresh the agent list.")
			return
		}
		tmux, _ := exec.LookPath("tmux")
		argv = []string{tmux, "attach-session", "-t", "=" + body.Target}
		title = body.Target
		body.View = "agent:" + body.Target
	case "proof", "alpha", "beta":
		location, err := openrigproof.Prepare(s.store.Home, owner)
		if err != nil {
			writeError(w, 500, "Cannot prepare the live test directory.")
			return
		}
		if body.View == "proof" {
			node, err := exec.LookPath("node")
			if err != nil {
				writeError(w, 409, "Node.js must be available on the WebMux service PATH to run the live test.")
				return
			}
			current, exists := s.sessions.OpenRigViews(owner)["proof"]
			if !exists || (current.State != "connected" && current.State != "connecting") {
				if err := openrigproof.Starting(location); err != nil {
					writeError(w, 500, "Cannot start a fresh proof record.")
					return
				}
			}
			title = "OpenRig live test console"
			argv = []string{node, location.Runner, "run", location.Directory, location.Team}
		} else {
			tmux, _ := exec.LookPath("tmux")
			target := "proof-" + body.View + "@" + location.Team
			title = "OpenRig " + body.View
			argv = []string{tmux, "attach-session", "-t", "=" + target}
		}
	default:
		writeError(w, 400, "Unknown OpenRig view.")
		return
	}
	value, err := s.sessions.EnsureOpenRigView(owner, body.View, title, argv)
	if err != nil {
		s.sessionError(w, err, "Cannot attach the OpenRig view. Check the host's OpenRig installation and daemon.")
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

// Stop only the owner-namespaced acceptance pair. Closing a view never calls it.
func (s *Server) stopOpenRigProof(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	if path, message := openRigExecutable(); path == "" {
		writeError(w, 409, message)
		return
	}
	node, err := exec.LookPath("node")
	if err != nil {
		writeError(w, 409, "Node.js is unavailable on the service PATH.")
		return
	}
	location, err := openrigproof.Prepare(s.store.Home, owner)
	if err != nil {
		writeError(w, 500, "Cannot access the live test directory.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, node, location.Runner, "stop", location.Directory, location.Team).Run(); err != nil {
		writeError(w, 502, "Could not confirm demo shutdown. Inspect the live test console and OpenRig dashboard.")
		return
	}
	writeJSON(w, 200, openrigproof.Status(s.store.Home, owner))
}

type openRigAgent struct {
	Target  string `json:"target"`
	Runtime string `json:"runtime"`
	Rig     string `json:"rig"`
	Status  string `json:"status"`
}

func readOpenRigAgents(parent context.Context, executable string) ([]openRigAgent, error) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, executable, "ps", "--nodes", "-A", "--fields", "canonicalSessionName,runtime,rigName,sessionStatus", "--json").Output()
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Entries []struct {
			Target  string `json:"canonicalSessionName"`
			Runtime string `json:"runtime"`
			Rig     string `json:"rigName"`
			Status  string `json:"sessionStatus"`
		} `json:"entries"`
	}
	if len(output) > 4*1024*1024 {
		return nil, fmt.Errorf("OpenRig agent listing too large")
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		return nil, err
	}
	result := make([]openRigAgent, 0, len(envelope.Entries))
	for _, entry := range envelope.Entries {
		// Only exact tmux targets obtained from the daemon, never shell expressions.
		if entry.Target == "" || len(entry.Target) > 256 || strings.ContainsAny(entry.Target, "\x00\n\r") {
			continue
		}
		result = append(result, openRigAgent{entry.Target, entry.Runtime, entry.Rig, entry.Status})
	}
	return result, nil
}

func (s *Server) getOpenRigAgents(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	path, message := openRigExecutable()
	if path == "" {
		writeError(w, 409, message)
		return
	}
	value, err := readOpenRigAgents(r.Context(), path)
	if err != nil {
		writeError(w, 502, "Cannot list OpenRig agents. Check the host's OpenRig daemon.")
		return
	}
	writeJSON(w, 200, value)
}

// Managed agents attach through a server-verified transport, never arbitrary argv.
type managedRigAgent struct {
	Target  string `json:"target"`
	Project string `json:"project"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Cwd     string `json:"cwd"`
	Runtime string `json:"runtime"`
	Tmux    string `json:"tmux"`
}

func (s *Server) rigAction(w http.ResponseWriter, r *http.Request, owner, action string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	if executable, message := openRigExecutable(); executable == "" {
		writeError(w, 409, message)
		return
	}
	input := ""
	if action == "create" {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil || !json.Valid(body) {
			writeError(w, 400, "Invalid agent details.")
			return
		}
		input = string(body)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, action, input)
	if err != nil {
		writeError(w, 502, "Coordination: "+err.Error())
		return
	}
	if action == "create" {
		var agent managedRigAgent
		if json.Unmarshal(output, &agent) != nil || agent.Tmux == "" {
			writeError(w, 502, "Invalid managed agent response.")
			return
		}
		if _, err := s.attachManagedRigTerminal(owner, agent); err != nil {
			s.sessionError(w, err, "Agent was created. Open its terminal from Coordination after making room in the grid.")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

func (s *Server) attachManagedRigTerminal(owner string, agent managedRigAgent) (session.Session, error) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return session.Session{}, err
	}
	return s.sessions.EnsureRigTerminal(owner, agent.Target, agent.Name+" · "+agent.Host+" · "+agent.Project, []string{tmux, "attach-session", "-t", "=" + agent.Tmux})
}

func (s *Server) attachManagedRig(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var body struct {
		Target string `json:"target"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
		writeError(w, 400, "Invalid agent target.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, "status", "")
	if err != nil {
		writeError(w, 502, "Cannot read coordinator state.")
		return
	}
	var status struct {
		Agents []managedRigAgent `json:"agents"`
	}
	if json.Unmarshal(output, &status) != nil {
		writeError(w, 502, "Invalid coordinator response.")
		return
	}
	for _, agent := range status.Agents {
		if agent.Target == body.Target {
			value, err := s.attachManagedRigTerminal(owner, agent)
			if err != nil {
				s.sessionError(w, err, "Cannot open agent terminal.")
				return
			}
			writeJSON(w, 200, value)
			return
		}
	}
	writeError(w, 404, "Unknown managed agent.")
}

func (s *Server) getCoordination(w http.ResponseWriter, r *http.Request, owner string) {
	s.rigAction(w, r, owner, "status")
}
func (s *Server) createCoordinatedAgent(w http.ResponseWriter, r *http.Request, owner string) {
	s.rigAction(w, r, owner, "create")
}
