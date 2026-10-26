package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

// coordinationProvider is the server-side adapter boundary for session
// coordinators. Provider implementations return stable, host-qualified IDs;
// browser code never infers identity from tmux names or transcript paths.
type coordinationProvider interface {
	Descriptor() coordinationProviderDescriptor
	Status(context.Context) (coordinationProviderStatus, error)
	Launch(context.Context, coordinationLaunchRequest) (coordinationLaunch, error)
	Attach(context.Context, string) (coordinationLaunch, error)
}

type coordinationProviderDescriptor struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Available    bool     `json:"available"`
	Message      string   `json:"message,omitempty"`
	Capabilities []string `json:"capabilities"`
	Contract     string   `json:"contract"`
}

type coordinationProject struct {
	Provider     string   `json:"provider"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Host         string   `json:"host"`
	State        string   `json:"state"`
	Historical   bool     `json:"historical"`
	SeatCount    int      `json:"seatCount"`
	RunningCount int      `json:"runningCount"`
	Detail       string   `json:"detail,omitempty"`
	Contexts     []string `json:"contexts,omitempty"`
}

type coordinationSeat struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	ProjectName    string `json:"projectName"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Workspace      string `json:"workspace,omitempty"`
	Runtime        string `json:"runtime"`
	State          string `json:"state"`
	Openable       bool   `json:"openable"`
	Managed        bool   `json:"managed"`
	NativeSession  string `json:"nativeSessionId,omitempty"`
	ProviderTarget string `json:"-"`
	Context        string `json:"context,omitempty"`
}

type coordinationMessage struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Body  string `json:"body"`
	Time  string `json:"time"`
	State string `json:"state"`
}

type coordinationProviderStatus struct {
	Projects []coordinationProject `json:"projects"`
	Seats    []coordinationSeat    `json:"seats"`
	Messages []coordinationMessage `json:"messages,omitempty"`
	Errors   []string              `json:"errors,omitempty"`
}

type coordinationLaunchRequest struct {
	Provider    string `json:"provider"`
	Mode        string `json:"mode"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Workspace   string `json:"workspace"`
	Runtime     string `json:"runtime"`
	Context     string `json:"context"`
}

type coordinationLaunch struct {
	Provider  string
	Target    string
	Title     string
	Project   string
	Name      string
	Host      string
	Workspace string
	Runtime   string
	Argv      []string
}

type agentDeckProvider struct {
	executable string
}

type rigProvider struct {
	home  string
	owner string
}

func (p *rigProvider) Descriptor() coordinationProviderDescriptor {
	path, message := openRigExecutable()
	return coordinationProviderDescriptor{
		ID: "rig", Label: "Rig", Available: path != "", Message: message,
		Capabilities: []string{"projects", "topology", "sessions", "local", "remote-launch", "launch", "attach", "peer-messaging"},
		Contract:     "openrig-cli/0.6.14",
	}
}

type rigStatusEnvelope struct {
	Rigs []struct {
		RigID          string  `json:"rigId"`
		Name           string  `json:"name"`
		RigName        string  `json:"rigName"`
		NodeCount      int     `json:"nodeCount"`
		RunningCount   int     `json:"runningCount"`
		Status         string  `json:"status"`
		LifecycleState string  `json:"lifecycleState"`
		LatestSnapshot *string `json:"latestSnapshot"`
	} `json:"rigs"`
	Seats []struct {
		RigID                string `json:"rigId"`
		RigName              string `json:"rigName"`
		LogicalID            string `json:"logicalId"`
		PodNamespace         string `json:"podNamespace"`
		CanonicalSessionName string `json:"canonicalSessionName"`
		Runtime              string `json:"runtime"`
		SessionStatus        string `json:"sessionStatus"`
		LifecycleState       string `json:"lifecycleState"`
		Managed              bool   `json:"managed"`
		Host                 string `json:"host"`
		Cwd                  string `json:"cwd"`
		Name                 string `json:"name"`
		Openable             bool   `json:"openable"`
		TmuxTarget           string `json:"tmuxTarget"`
	} `json:"seats"`
	Agents   []managedRigAgent     `json:"agents"`
	Messages []coordinationMessage `json:"messages"`
}

func (p *rigProvider) rawStatus(ctx context.Context) (rigStatusEnvelope, error) {
	output, err := openrigcoord.Run(ctx, p.home, p.owner, "status", "")
	if err != nil {
		return rigStatusEnvelope{}, err
	}
	var status rigStatusEnvelope
	if err := json.Unmarshal(output, &status); err != nil {
		return rigStatusEnvelope{}, fmt.Errorf("invalid Rig coordinator response: %w", err)
	}
	return status, nil
}

func (p *rigProvider) Status(ctx context.Context) (coordinationProviderStatus, error) {
	raw, err := p.rawStatus(ctx)
	if err != nil {
		return coordinationProviderStatus{}, err
	}
	status := coordinationProviderStatus{Projects: []coordinationProject{}, Seats: []coordinationSeat{}, Messages: raw.Messages, Errors: []string{}}
	contexts := map[string]map[string]bool{}
	managed := map[string]managedRigAgent{}
	for _, agent := range raw.Agents {
		managed[agent.Target] = agent
	}
	for _, seat := range raw.Seats {
		if contexts[seat.RigID] == nil {
			contexts[seat.RigID] = map[string]bool{}
		}
		if seat.PodNamespace != "" {
			contexts[seat.RigID][seat.PodNamespace] = true
		}
		host := seat.Host
		if host == "" {
			host = "localhost"
		}
		name := seat.Name
		if name == "" {
			name = seat.LogicalID
		}
		state := seat.LifecycleState
		openable := seat.Openable
		nativeSession := ""
		if agent, ok := managed[seat.CanonicalSessionName]; ok {
			nativeSession = agent.SessionID
			if !agent.Connected {
				state = "stopped"
				openable = false
			}
		}
		status.Seats = append(status.Seats, coordinationSeat{
			Provider: "rig", ID: seat.CanonicalSessionName, ProjectID: seat.RigID, ProjectName: seat.RigName,
			Name: name, Host: host, Workspace: seat.Cwd, Runtime: seat.Runtime, State: state,
			Openable: openable, Managed: seat.Managed, NativeSession: nativeSession, ProviderTarget: seat.CanonicalSessionName, Context: seat.PodNamespace,
		})
	}
	for _, rig := range raw.Rigs {
		name := rig.RigName
		if name == "" {
			name = rig.Name
		}
		project := coordinationProject{
			Provider: "rig", ID: rig.RigID, Name: name, Host: "localhost", State: rig.LifecycleState,
			Historical: rig.LifecycleState == "stopped", SeatCount: rig.NodeCount, RunningCount: rig.RunningCount,
		}
		for context := range contexts[rig.RigID] {
			project.Contexts = append(project.Contexts, context)
		}
		sort.Strings(project.Contexts)
		if rig.LatestSnapshot != nil {
			project.Detail = "snapshot " + *rig.LatestSnapshot
		}
		status.Projects = append(status.Projects, project)
	}
	return status, nil
}

func (p *rigProvider) Launch(ctx context.Context, request coordinationLaunchRequest) (coordinationLaunch, error) {
	input := map[string]string{
		"mode": request.Mode, "rigId": request.ProjectID, "rigName": request.ProjectName, "pod": request.Context,
		"name": request.Name, "host": request.Host, "cwd": request.Workspace, "runtime": request.Runtime,
	}
	body, _ := json.Marshal(input)
	output, err := openrigcoord.Run(ctx, p.home, p.owner, "create", string(body))
	if err != nil {
		return coordinationLaunch{}, err
	}
	var agent managedRigAgent
	if err := json.Unmarshal(output, &agent); err != nil || agent.Tmux == "" || agent.Target == "" {
		return coordinationLaunch{}, fmt.Errorf("invalid managed Rig agent response")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return coordinationLaunch{}, err
	}
	return coordinationLaunch{
		Provider: "rig", Target: agent.Target, Title: agent.Name + " · " + agent.Host + " · " + agent.Project,
		Project: agent.Project, Name: agent.Name, Host: agent.Host, Workspace: agent.Cwd, Runtime: agent.Runtime,
		Argv: []string{tmux, "attach-session", "-t", "=" + agent.Tmux},
	}, nil
}

func (p *rigProvider) Attach(ctx context.Context, target string) (coordinationLaunch, error) {
	raw, err := p.rawStatus(ctx)
	if err != nil {
		return coordinationLaunch{}, err
	}
	for _, agent := range raw.Agents {
		if agent.Target != target {
			continue
		}
		tmux, err := exec.LookPath("tmux")
		if err != nil {
			return coordinationLaunch{}, err
		}
		return coordinationLaunch{Provider: "rig", Target: agent.Target, Title: agent.Name + " · " + agent.Host + " · " + agent.Project, Project: agent.Project, Name: agent.Name, Host: agent.Host, Workspace: agent.Cwd, Runtime: agent.Runtime, Argv: []string{tmux, "attach-session", "-t", "=" + agent.Tmux}}, nil
	}
	for _, seat := range raw.Seats {
		if seat.CanonicalSessionName != target {
			continue
		}
		if !seat.Openable || seat.TmuxTarget == "" {
			return coordinationLaunch{}, fmt.Errorf("this canonical Rig seat has no local tmux viewer; open it on its execution host or start a managed seat in this topology")
		}
		tmux, err := exec.LookPath("tmux")
		if err != nil {
			return coordinationLaunch{}, err
		}
		name := seat.Name
		if name == "" {
			name = seat.LogicalID
		}
		return coordinationLaunch{Provider: "rig", Target: target, Title: name + " · localhost · " + seat.RigName, Project: seat.RigName, Name: name, Host: "localhost", Runtime: seat.Runtime, Argv: []string{tmux, "attach-session", "-t", "=" + seat.TmuxTarget}}, nil
	}
	return coordinationLaunch{}, fmt.Errorf("unknown Rig seat; refresh the canonical project list")
}

func newAgentDeckProvider() *agentDeckProvider {
	path, _ := exec.LookPath("agent-deck")
	return &agentDeckProvider{executable: path}
}

func (p *agentDeckProvider) Descriptor() coordinationProviderDescriptor {
	value := coordinationProviderDescriptor{
		ID: "agent-deck", Label: "Agent Deck", Available: p.executable != "",
		Capabilities: []string{"projects", "sessions", "local", "remote", "launch", "attach", "native-session-id"},
		Contract:     "agent-deck-cli/v1",
	}
	if p.executable == "" {
		value.Message = "Install Agent Deck on the WebMux server to inventory and launch its local and registered-remote sessions."
	}
	return value
}

type agentDeckSession struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Path            string `json:"path"`
	Group           string `json:"group"`
	Tool            string `json:"tool"`
	Status          string `json:"status"`
	Archived        bool   `json:"archived"`
	SSHHost         string `json:"ssh_host"`
	SSHRemotePath   string `json:"ssh_remote_path"`
	CodexSessionID  string `json:"codex_session_id"`
	ClaudeSessionID string `json:"claude_session_id"`
}

type agentDeckRemote struct {
	Name              string `json:"name"`
	Host              string `json:"host"`
	LastPollStatus    string `json:"last_poll_status"`
	LastPollError     string `json:"last_poll_error"`
	AgentDeckPath     string `json:"agent_deck_path"`
	ConfiguredProfile string `json:"profile"`
}

type agentDeckGroup struct {
	Name         string           `json:"name"`
	Path         string           `json:"path"`
	SessionCount int              `json:"session_count"`
	Children     []agentDeckGroup `json:"children"`
}

type agentDeckGroups struct {
	Groups []agentDeckGroup `json:"groups"`
}

func flattenAgentDeckGroups(values []agentDeckGroup) []agentDeckGroup {
	result := []agentDeckGroup{}
	for _, value := range values {
		result = append(result, value)
		result = append(result, flattenAgentDeckGroups(value.Children)...)
	}
	return result
}

func (p *agentDeckProvider) runJSON(ctx context.Context, destination any, args ...string) error {
	if p.executable == "" {
		return fmt.Errorf("Agent Deck is not installed")
	}
	command := exec.CommandContext(ctx, p.executable, args...)
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s", strings.TrimSpace(string(exit.Stderr)))
		}
		return err
	}
	if len(output) > 8*1024*1024 {
		return fmt.Errorf("Agent Deck response is too large")
	}
	if err := json.Unmarshal(output, destination); err != nil {
		return fmt.Errorf("invalid Agent Deck JSON: %w", err)
	}
	return nil
}

func (p *agentDeckProvider) remoteHost(ctx context.Context, name string) (string, error) {
	var remotes []agentDeckRemote
	if err := p.runJSON(ctx, &remotes, "remote", "list", "--json"); err != nil {
		return "", err
	}
	for _, remote := range remotes {
		if remote.Name == name {
			return remote.Host, nil
		}
	}
	return "", fmt.Errorf("Agent Deck remote %q is no longer configured", name)
}

func agentDeckQualified(remote, id string) string {
	if remote == "" {
		remote = "local"
	}
	return remote + ":" + id
}

func agentDeckProjectID(remote, group string) string {
	if group == "" {
		group = "ungrouped"
	}
	return agentDeckQualified(remote, group)
}

func (p *agentDeckProvider) Status(ctx context.Context) (coordinationProviderStatus, error) {
	if p.executable == "" {
		return coordinationProviderStatus{}, fmt.Errorf("Agent Deck is not installed")
	}
	var local []agentDeckSession
	if err := p.runJSON(ctx, &local, "list", "--all", "--json"); err != nil {
		return coordinationProviderStatus{}, err
	}
	var localGroups agentDeckGroups
	if err := p.runJSON(ctx, &localGroups, "group", "list", "--json"); err != nil {
		return coordinationProviderStatus{}, err
	}
	type located struct {
		remote  string
		session agentDeckSession
	}
	type locatedGroup struct {
		remote string
		host   string
		group  agentDeckGroup
	}
	locatedSessions := make([]located, 0, len(local))
	locatedGroups := []locatedGroup{}
	for _, value := range local {
		locatedSessions = append(locatedSessions, located{session: value})
	}
	for _, value := range flattenAgentDeckGroups(localGroups.Groups) {
		locatedGroups = append(locatedGroups, locatedGroup{host: "localhost", group: value})
	}
	var remotes []agentDeckRemote
	if err := p.runJSON(ctx, &remotes, "remote", "list", "--json"); err != nil {
		return coordinationProviderStatus{}, err
	}
	status := coordinationProviderStatus{Projects: []coordinationProject{}, Seats: []coordinationSeat{}, Messages: []coordinationMessage{}, Errors: []string{}}
	for _, remote := range remotes {
		var sessions []agentDeckSession
		remoteCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := p.runJSON(remoteCtx, &sessions, "remote", "sessions", remote.Name, "--json")
		cancel()
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("%s (%s): %v", remote.Name, remote.Host, err))
			continue
		}
		for _, value := range sessions {
			locatedSessions = append(locatedSessions, located{remote: remote.Name, session: value})
		}
		var groups agentDeckGroups
		groupCtx, groupCancel := context.WithTimeout(ctx, 12*time.Second)
		err = p.runJSON(groupCtx, &groups, "remote", remote.Name, "group", "list", "--json")
		groupCancel()
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("%s groups (%s): %v", remote.Name, remote.Host, err))
			continue
		}
		for _, value := range flattenAgentDeckGroups(groups.Groups) {
			locatedGroups = append(locatedGroups, locatedGroup{remote: remote.Name, host: remote.Name, group: value})
		}
	}
	type aggregate struct {
		project  coordinationProject
		archived int
	}
	projects := map[string]*aggregate{}
	for _, located := range locatedGroups {
		groupPath := located.group.Path
		if groupPath == "" {
			groupPath = located.group.Name
		}
		id := agentDeckProjectID(located.remote, groupPath)
		projects[id] = &aggregate{project: coordinationProject{
			Provider: "agent-deck", ID: id, Name: groupPath, Host: located.host,
			State: "idle",
		}}
	}
	for _, located := range locatedSessions {
		value := located.session
		host := located.remote
		if host == "" {
			host = value.SSHHost
			if host == "" {
				host = "localhost"
			}
		}
		workspace := value.Path
		if value.SSHRemotePath != "" {
			workspace = value.SSHRemotePath
		}
		group := value.Group
		if group == "" {
			group = "Ungrouped"
		}
		projectID := agentDeckProjectID(located.remote, value.Group)
		item := projects[projectID]
		if item == nil {
			item = &aggregate{project: coordinationProject{Provider: "agent-deck", ID: projectID, Name: group, Host: host, State: "idle"}}
			projects[projectID] = item
		}
		item.project.SeatCount++
		if value.Status == "running" || value.Status == "waiting" || value.Status == "starting" {
			item.project.RunningCount++
			item.project.State = "active"
		}
		if value.Archived {
			item.archived++
		}
		native := value.CodexSessionID
		if native == "" {
			native = value.ClaudeSessionID
		}
		status.Seats = append(status.Seats, coordinationSeat{
			Provider: "agent-deck", ID: agentDeckQualified(located.remote, value.ID), ProjectID: projectID, ProjectName: group,
			Name: value.Title, Host: host, Workspace: workspace, Runtime: value.Tool, State: value.Status,
			Openable: !value.Archived, Managed: true, NativeSession: native, ProviderTarget: value.ID,
		})
	}
	for _, value := range projects {
		value.project.Historical = value.project.SeatCount > 0 && value.archived == value.project.SeatCount
		if value.project.Historical {
			value.project.State = "archived"
		}
		status.Projects = append(status.Projects, value.project)
	}
	sort.Slice(status.Projects, func(i, j int) bool { return status.Projects[i].Name < status.Projects[j].Name })
	sort.Slice(status.Seats, func(i, j int) bool { return status.Seats[i].Name < status.Seats[j].Name })
	return status, nil
}

func splitAgentDeckTarget(value string) (remote, id string, err error) {
	remote, id, found := strings.Cut(value, ":")
	if !found || remote == "" || id == "" || strings.ContainsAny(remote+id, "\x00\r\n") {
		return "", "", fmt.Errorf("invalid Agent Deck session identity")
	}
	if remote == "local" {
		remote = ""
	}
	return remote, id, nil
}

func (p *agentDeckProvider) Attach(ctx context.Context, target string) (coordinationLaunch, error) {
	status, err := p.Status(ctx)
	if err != nil {
		return coordinationLaunch{}, err
	}
	var seat *coordinationSeat
	for i := range status.Seats {
		if status.Seats[i].ID == target {
			seat = &status.Seats[i]
			break
		}
	}
	if seat == nil || !seat.Openable {
		return coordinationLaunch{}, fmt.Errorf("Agent Deck session is unavailable; refresh the session list")
	}
	remote, id, err := splitAgentDeckTarget(target)
	if err != nil {
		return coordinationLaunch{}, err
	}
	argv := []string{p.executable, "session", "attach", id}
	if remote != "" {
		argv = []string{p.executable, "remote", "attach", remote, id}
	}
	return coordinationLaunch{Provider: "agent-deck", Target: target, Title: seat.Name + " · " + seat.Host + " · " + seat.ProjectName, Project: seat.ProjectName, Name: seat.Name, Host: seat.Host, Workspace: seat.Workspace, Runtime: seat.Runtime, Argv: argv}, nil
}

func (p *agentDeckProvider) Launch(ctx context.Context, request coordinationLaunchRequest) (coordinationLaunch, error) {
	if p.executable == "" {
		return coordinationLaunch{}, fmt.Errorf("Agent Deck is not installed")
	}
	if request.Name == "" || request.ProjectName == "" || request.Workspace == "" {
		return coordinationLaunch{}, fmt.Errorf("project, session name and workspace are required")
	}
	if request.Runtime != "codex" && request.Runtime != "claude-code" {
		return coordinationLaunch{}, fmt.Errorf("Agent Deck supports Codex or Claude Code in this flow")
	}
	tool := request.Runtime
	if tool == "claude-code" {
		tool = "claude"
	}
	remote := ""
	if request.ProjectID != "" {
		remote, _, _ = strings.Cut(request.ProjectID, ":")
		if remote == "local" {
			remote = ""
		}
	} else if request.Host != "" && request.Host != "localhost" {
		remote = request.Host
	}
	args := []string{"launch", request.Workspace, "-t", request.Name, "-g", request.ProjectName, "-c", tool, "--json"}
	if remote != "" {
		args = append([]string{"remote", remote}, args...)
	}
	var launched struct {
		ID string `json:"id"`
	}
	if err := p.runJSON(ctx, &launched, args...); err != nil {
		return coordinationLaunch{}, err
	}
	if launched.ID == "" {
		return coordinationLaunch{}, fmt.Errorf("Agent Deck did not return a session ID")
	}
	target := agentDeckQualified(remote, launched.ID)
	argv := []string{p.executable, "session", "attach", launched.ID}
	if remote != "" {
		argv = []string{p.executable, "remote", "attach", remote, launched.ID}
	}
	host := request.Host
	if host == "" {
		host = "localhost"
	}
	return coordinationLaunch{Provider: "agent-deck", Target: target, Title: request.Name + " · " + host + " · " + request.ProjectName, Project: request.ProjectName, Name: request.Name, Host: host, Workspace: request.Workspace, Runtime: request.Runtime, Argv: argv}, nil
}
