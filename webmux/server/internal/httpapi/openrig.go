package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
	"github.com/jordanhubbard/webmux/server/internal/openrigproof"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/config"
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
	Target    string `json:"target"`
	Project   string `json:"project"`
	RigID     string `json:"rigId"`
	RigName   string `json:"rigName"`
	Pod       string `json:"pod"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Cwd       string `json:"cwd"`
	Runtime   string `json:"runtime"`
	Tmux      string `json:"tmux"`
	SessionID string `json:"sessionId,omitempty"`
	Connected bool   `json:"connected,omitempty"`
}

func (s *Server) attachManagedRig(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Target   string `json:"target"`
		Row      *int   `json:"row"`
		Col      *int   `json:"col"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body) != nil || body.Target == "" || (body.Row == nil) != (body.Col == nil) {
		writeError(w, 400, "Invalid agent target.")
		return
	}
	if body.Provider == "" {
		body.Provider = "rig"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	provider := s.coordinationProvider(owner, body.Provider)
	if provider == nil || !provider.Descriptor().Available {
		writeError(w, 409, "The selected coordination provider is unavailable.")
		return
	}
	launch, err := provider.Attach(ctx, body.Target)
	if err != nil {
		writeError(w, 502, "Coordination: "+err.Error())
		return
	}
	value, err := s.attachCoordinationTerminal(r.Context(), owner, launch, body.Row, body.Col)
	if err != nil {
		s.sessionError(w, err, "Cannot open coordinated session terminal.")
		return
	}
	writeJSON(w, 200, value)
}

func (s *Server) getCoordination(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	defaultProvider := any("choose")
	if os.Getenv("WEBMUX_COORDINATION_DISABLED") == "1" {
		writeJSON(w, 200, map[string]any{"contract": "webmux-coordination/v1", "defaultProvider": defaultProvider, "providers": []coordinationProviderDescriptor{}, "projects": []coordinationProject{}, "seats": []coordinationSeat{}, "messages": []coordinationMessage{}, "errors": map[string]string{}, "attentionCount": 0})
		return
	}
	settings, err := s.loadSettings()
	if err != nil {
		s.catalogError(w, err, "Failed to load coordination settings")
		return
	}
	defaultProvider = config.AsObject(settings.App["coordination"])["default_provider"]
	providers := s.coordinationProviders(owner)
	descriptors := make([]coordinationProviderDescriptor, 0, len(providers))
	projects := []coordinationProject{}
	seats := []coordinationSeat{}
	messages := []coordinationMessage{}
	errors := map[string]string{}
	attention := 0
	for _, provider := range providers {
		descriptor := provider.Descriptor()
		descriptors = append(descriptors, descriptor)
		if !descriptor.Available {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		status, err := provider.Status(ctx)
		cancel()
		if err != nil {
			errors[descriptor.ID] = err.Error()
			continue
		}
		projects = append(projects, status.Projects...)
		seats = append(seats, status.Seats...)
		messages = append(messages, status.Messages...)
		attention += status.Attention
		if len(status.Errors) > 0 {
			errors[descriptor.ID] = strings.Join(status.Errors, "; ")
		}
	}
	writeJSON(w, 200, map[string]any{"contract": "webmux-coordination/v1", "defaultProvider": defaultProvider, "providers": descriptors, "projects": projects, "seats": seats, "messages": messages, "errors": errors, "attentionCount": attention})
}

func (s *Server) sendCoordinationMessage(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		coordinationSendRequest
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 264*1024)).Decode(&body) != nil || body.Provider == "" || body.Target == "" || strings.TrimSpace(body.Message) == "" {
		writeError(w, 400, "Invalid coordination message.")
		return
	}
	provider := s.coordinationProvider(owner, body.Provider)
	if provider == nil || !provider.Descriptor().Available {
		writeError(w, 409, "The selected coordination provider is unavailable.")
		return
	}
	messenger, ok := provider.(coordinationMessenger)
	if !ok {
		writeError(w, 409, "The selected coordination provider does not support structured messaging.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	result, err := messenger.Send(ctx, body.coordinationSendRequest)
	if err != nil {
		writeError(w, 502, "Coordination message: "+err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) coordinationSendStatus(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Target   string `json:"target"`
		SendID   string `json:"sendId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.Provider == "" || body.Target == "" || body.SendID == "" {
		writeError(w, 400, "Invalid coordination send status request.")
		return
	}
	provider := s.coordinationProvider(owner, body.Provider)
	messenger, ok := provider.(coordinationMessenger)
	if provider == nil || !provider.Descriptor().Available || !ok {
		writeError(w, 409, "The selected coordination provider does not support send status.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := messenger.SendStatus(ctx, body.Target, body.SendID)
	if err != nil {
		writeError(w, 502, "Coordination send status: "+err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) drainCoordinationRemote(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Remote   string `json:"remote"`
		Target   string `json:"target"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.Provider == "" || body.Remote == "" || body.Target == "" {
		writeError(w, 400, "Invalid remote completion request.")
		return
	}
	provider := s.coordinationProvider(owner, body.Provider)
	drainer, ok := provider.(coordinationDrainer)
	if provider == nil || !provider.Descriptor().Available || !ok {
		writeError(w, 409, "The selected coordination provider does not support remote completion collection.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	result, err := drainer.Drain(ctx, body.Remote, body.Target)
	if err != nil {
		writeError(w, 502, "Remote completions: "+err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) createCoordinatedAgent(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		writeError(w, 400, "Invalid coordination request.")
		return
	}
	var request struct {
		coordinationLaunchRequest
		Row *int `json:"row"`
		Col *int `json:"col"`
	}
	if json.Unmarshal(body, &request) != nil || request.Provider == "" || (request.Row == nil) != (request.Col == nil) {
		writeError(w, 400, "Invalid coordination request.")
		return
	}
	provider := s.coordinationProvider(owner, request.Provider)
	if provider == nil || !provider.Descriptor().Available {
		writeError(w, 409, "The selected coordination provider is unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	launch, err := provider.Launch(ctx, request.coordinationLaunchRequest)
	if err != nil {
		writeError(w, 502, "Coordination: "+err.Error())
		return
	}
	value, err := s.attachCoordinationTerminal(r.Context(), owner, launch, request.Row, request.Col)
	if err != nil {
		s.sessionError(w, err, "Session was created. Open it from Coordination after making room in the grid.")
		return
	}
	writeJSON(w, 200, map[string]any{"target": launch.Target, "session": value})
}
func (s *Server) browseCoordinationWorkspaces(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil || !json.Valid(body) {
		writeError(w, 400, "Invalid workspace request.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var browse struct {
		Provider string `json:"provider"`
		Host     string `json:"host"`
		Path     string `json:"path"`
	}
	if json.Unmarshal(body, &browse) != nil {
		writeError(w, 400, "Invalid workspace request.")
		return
	}
	if browse.Provider == "" {
		browse.Provider = "rig"
	}
	provider := s.coordinationProvider(owner, browse.Provider)
	if provider == nil || !provider.Descriptor().Available {
		writeError(w, 409, "The selected coordination provider is unavailable.")
		return
	}
	if browse.Provider == "agent-deck" && browse.Host != "" && browse.Host != "localhost" {
		resolved, err := provider.(*agentDeckProvider).remoteHost(ctx, browse.Host)
		if err != nil {
			writeError(w, 502, "Workspace browser: "+err.Error())
			return
		}
		browse.Host = resolved
		body, _ = json.Marshal(browse)
	}
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, "browse", string(body))
	if err != nil {
		writeError(w, 502, "Workspace browser: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

func (s *Server) coordinationProviders(owner string) []coordinationProvider {
	return []coordinationProvider{&rigProvider{home: s.store.Home, owner: owner}, newAgentDeckProvider()}
}

func (s *Server) coordinationProvider(owner, id string) coordinationProvider {
	if os.Getenv("WEBMUX_COORDINATION_DISABLED") == "1" {
		return nil
	}
	for _, provider := range s.coordinationProviders(owner) {
		if provider.Descriptor().ID == id {
			return provider
		}
	}
	return nil
}

func (s *Server) attachCoordinationTerminal(parent context.Context, owner string, launch coordinationLaunch, row, col *int) (session.Session, error) {
	if launch.Provider == "rig" && len(launch.Argv) >= 4 {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, launch.Argv[0], "set-option", "-t", strings.TrimPrefix(launch.Argv[3], "="), "mouse", "on").Run(); err != nil {
			return session.Session{}, fmt.Errorf("enable coordinated terminal scrollback: %w", err)
		}
	}
	viewerTarget := launch.Target
	if launch.Provider != "rig" {
		viewerTarget = launch.Provider + ":" + launch.Target
	}
	identityProvider := launch.Provider
	if identityProvider == "rig" {
		identityProvider = ""
	}
	identity := session.RigIdentity{Provider: identityProvider, Target: viewerTarget, Project: launch.Project, Name: launch.Name, Host: launch.Host, Runtime: launch.Runtime, Workspace: launch.Workspace}
	return s.sessions.EnsureRigTerminalAt(owner, viewerTarget, launch.Title, launch.Argv, row, col, identity)
}
