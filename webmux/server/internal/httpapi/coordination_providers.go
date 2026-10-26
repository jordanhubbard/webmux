package httpapi

import (
	"bytes"
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
	Attention    int      `json:"attentionCount,omitempty"`
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
	ParentID       string `json:"parentId,omitempty"`
	ParentName     string `json:"parentName,omitempty"`
	Role           string `json:"role,omitempty"`
	Substate       string `json:"substate,omitempty"`
	LastActivity   string `json:"lastActivityAt,omitempty"`
	Attention      bool   `json:"attention,omitempty"`
	CanReceive     bool   `json:"canReceive,omitempty"`
	ExitCode       *int   `json:"exitCode,omitempty"`
}

type coordinationMessage struct {
	Provider     string `json:"provider,omitempty"`
	From         string `json:"from"`
	To           string `json:"to"`
	Body         string `json:"body"`
	Time         string `json:"time"`
	State        string `json:"state"`
	Kind         string `json:"kind,omitempty"`
	Level        string `json:"level,omitempty"`
	SourceRemote string `json:"sourceRemote,omitempty"`
}

type coordinationProviderStatus struct {
	Projects  []coordinationProject `json:"projects"`
	Seats     []coordinationSeat    `json:"seats"`
	Messages  []coordinationMessage `json:"messages,omitempty"`
	Errors    []string              `json:"errors,omitempty"`
	Attention int                   `json:"attentionCount,omitempty"`
}

type coordinationSendRequest struct {
	Target  string `json:"target"`
	Message string `json:"message"`
	Queue   bool   `json:"queue"`
}

type coordinationSendResult struct {
	Target       string `json:"target"`
	SendID       string `json:"sendId,omitempty"`
	State        string `json:"state,omitempty"`
	Verdict      string `json:"verdict,omitempty"`
	Delivery     string `json:"delivery,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Queued       bool   `json:"queued"`
	Submitted    bool   `json:"submitted"`
}

type coordinationDrainResult struct {
	Remote        string                `json:"remote"`
	Target        string                `json:"target"`
	Fetched       int                   `json:"fetched"`
	Written       int                   `json:"written"`
	Duplicates    int                   `json:"duplicates"`
	Unknown       int                   `json:"unknown"`
	Woke          bool                  `json:"woke"`
	WriterRunning *bool                 `json:"writerRunning,omitempty"`
	WriterDetail  string                `json:"writerDetail,omitempty"`
	Messages      []coordinationMessage `json:"messages"`
}

type coordinationMessenger interface {
	Send(context.Context, coordinationSendRequest) (coordinationSendResult, error)
	SendStatus(context.Context, string, string) (map[string]any, error)
}

type coordinationDrainer interface {
	Drain(context.Context, string, string) (coordinationDrainResult, error)
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
		Capabilities: []string{"projects", "sessions", "local", "remote", "launch", "attach", "native-session-id", "parent-child", "send", "queue", "send-status", "completion-inbox", "remote-drain"},
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
	ParentSessionID string `json:"parent_session_id"`
	ConductorName   string `json:"conductor_name"`
	Substate        string `json:"substate"`
	LastActivityAt  string `json:"last_activity_at"`
	ExitCode        *int   `json:"exit_code"`
}

type agentDeckInboxEvent struct {
	ChildSessionID string `json:"child_session_id"`
	ChildTitle     string `json:"child_title"`
	FromStatus     string `json:"from_status"`
	ToStatus       string `json:"to_status"`
	Timestamp      string `json:"timestamp"`
	Substate       string `json:"substate"`
	ExitCode       *int   `json:"exit_code"`
	Kind           string `json:"kind"`
	DoneStatus     string `json:"done_status"`
	DoneSummary    string `json:"done_summary"`
	Tier           string `json:"tier"`
	Text           string `json:"text"`
	DeliveryResult string `json:"delivery_result"`
	SourceRemote   string `json:"source_remote"`
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
	return p.runJSONInput(ctx, destination, nil, args...)
}

func (p *agentDeckProvider) runJSONInput(ctx context.Context, destination any, input []byte, args ...string) error {
	if p.executable == "" {
		return fmt.Errorf("Agent Deck is not installed")
	}
	command := exec.CommandContext(ctx, p.executable, args...)
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
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
	sessionsByID := map[string]located{}
	parents := map[string]bool{}
	for _, item := range locatedSessions {
		qualified := agentDeckQualified(item.remote, item.session.ID)
		sessionsByID[qualified] = item
		if item.session.ParentSessionID != "" {
			parents[agentDeckQualified(item.remote, item.session.ParentSessionID)] = true
		}
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
		attention := value.Status == "waiting" || value.Status == "error" || value.Status == "failed"
		if attention {
			item.project.Attention++
			item.project.State = "attention_required"
			status.Attention++
		}
		if value.Archived {
			item.archived++
		}
		native := value.CodexSessionID
		if native == "" {
			native = value.ClaudeSessionID
		}
		qualified := agentDeckQualified(located.remote, value.ID)
		parentID := ""
		parentName := ""
		if value.ParentSessionID != "" {
			parentID = agentDeckQualified(located.remote, value.ParentSessionID)
			if parent, ok := sessionsByID[parentID]; ok {
				parentName = parent.session.Title
			}
		}
		role := "session"
		if value.ConductorName != "" {
			role = "conductor"
		} else if parentID != "" {
			role = "worker"
		}
		status.Seats = append(status.Seats, coordinationSeat{
			Provider: "agent-deck", ID: qualified, ProjectID: projectID, ProjectName: group,
			Name: value.Title, Host: host, Workspace: workspace, Runtime: value.Tool, State: value.Status,
			Openable: !value.Archived, Managed: true, NativeSession: native, ProviderTarget: value.ID,
			ParentID: parentID, ParentName: parentName, Role: role, Substate: value.Substate,
			LastActivity: value.LastActivityAt, Attention: attention, CanReceive: role == "conductor" || parents[qualified], ExitCode: value.ExitCode,
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
	peeked := 0
	for _, seat := range status.Seats {
		if !seat.CanReceive || !strings.HasPrefix(seat.ID, "local:") || peeked >= 16 {
			continue
		}
		peeked++
		var records []agentDeckInboxEvent
		if err := p.runJSON(ctx, &records, "inbox", "peek", "--json", seat.ProviderTarget); err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("%s inbox: %v", seat.Name, err))
			continue
		}
		for _, record := range records {
			from := agentDeckQualified(record.SourceRemote, record.ChildSessionID)
			body := record.Text
			if body == "" {
				body = record.DoneSummary
			}
			state := record.DeliveryResult
			if state == "" {
				state = record.ToStatus
			}
			kind := record.Kind
			if kind == "" {
				kind = "transition"
			}
			level := record.Tier
			if level == "" {
				level = "urgent"
			}
			status.Messages = append(status.Messages, coordinationMessage{
				Provider: "agent-deck", From: from, To: seat.ID, Body: body, Time: record.Timestamp,
				State: state, Kind: kind, Level: level, SourceRemote: record.SourceRemote,
			})
			if level == "urgent" || record.DoneStatus == "fail" {
				status.Attention++
			}
		}
	}
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

func (p *agentDeckProvider) agentDeckSeat(ctx context.Context, target string) (coordinationSeat, error) {
	status, err := p.Status(ctx)
	if err != nil {
		return coordinationSeat{}, err
	}
	for _, seat := range status.Seats {
		if seat.ID == target {
			return seat, nil
		}
	}
	return coordinationSeat{}, fmt.Errorf("Agent Deck session is unavailable; refresh the session list")
}

func (p *agentDeckProvider) Send(ctx context.Context, request coordinationSendRequest) (coordinationSendResult, error) {
	if strings.TrimSpace(request.Message) == "" || len(request.Message) > 256*1024 {
		return coordinationSendResult{}, fmt.Errorf("message must contain between 1 and 262144 bytes")
	}
	seat, err := p.agentDeckSeat(ctx, request.Target)
	if err != nil || !seat.Openable {
		if err != nil {
			return coordinationSendResult{}, err
		}
		return coordinationSendResult{}, fmt.Errorf("Agent Deck session is not openable")
	}
	remote, id, err := splitAgentDeckTarget(request.Target)
	if err != nil {
		return coordinationSendResult{}, err
	}
	args := []string{"session", "send", id, "--message-file", "-", "--json", "--require-input-prompt"}
	if request.Queue {
		args = append(args, "--queue")
	} else {
		args = append(args, "--no-wait")
	}
	if remote != "" {
		args = append([]string{"remote", remote}, args...)
	}
	var output struct {
		SendID       string `json:"send_id"`
		State        string `json:"state"`
		Verdict      string `json:"verdict"`
		Delivery     string `json:"delivery"`
		Confirmation string `json:"confirmation"`
		Reason       string `json:"reason"`
		Submitted    bool   `json:"submitted"`
	}
	if err := p.runJSONInput(ctx, &output, []byte(request.Message), args...); err != nil {
		return coordinationSendResult{}, err
	}
	return coordinationSendResult{
		Target: request.Target, SendID: output.SendID, State: output.State, Verdict: output.Verdict,
		Delivery: output.Delivery, Confirmation: output.Confirmation, Reason: output.Reason,
		Queued:    request.Queue || output.State == "queued" || output.Verdict == "queued" || output.Delivery == "queued",
		Submitted: output.Submitted,
	}, nil
}

func (p *agentDeckProvider) SendStatus(ctx context.Context, target, sendID string) (map[string]any, error) {
	if _, err := p.agentDeckSeat(ctx, target); err != nil {
		return nil, err
	}
	if sendID == "" || len(sendID) > 256 || strings.ContainsAny(sendID, "\x00\r\n") {
		return nil, fmt.Errorf("invalid Agent Deck send identity")
	}
	remote, _, err := splitAgentDeckTarget(target)
	if err != nil {
		return nil, err
	}
	args := []string{"session", "send-status", sendID, "--json"}
	if remote != "" {
		args = append([]string{"remote", remote}, args...)
	}
	result := map[string]any{}
	if err := p.runJSON(ctx, &result, args...); err != nil {
		return nil, err
	}
	delete(result, "message")
	result["target"] = target
	return result, nil
}

func agentDeckEventMessage(record agentDeckInboxEvent, target string) coordinationMessage {
	from := agentDeckQualified(record.SourceRemote, record.ChildSessionID)
	body := record.Text
	if body == "" {
		body = record.DoneSummary
	}
	state := record.DeliveryResult
	if state == "" {
		state = record.ToStatus
	}
	kind := record.Kind
	if kind == "" {
		kind = "transition"
	}
	level := record.Tier
	if level == "" {
		level = "urgent"
	}
	return coordinationMessage{Provider: "agent-deck", From: from, To: target, Body: body, Time: record.Timestamp, State: state, Kind: kind, Level: level, SourceRemote: record.SourceRemote}
}

func (p *agentDeckProvider) Drain(ctx context.Context, remoteName, target string) (coordinationDrainResult, error) {
	if remoteName == "" || remoteName == "local" || strings.ContainsAny(remoteName, "\x00\r\n") {
		return coordinationDrainResult{}, fmt.Errorf("choose a registered remote Agent Deck host")
	}
	seat, err := p.agentDeckSeat(ctx, target)
	if err != nil {
		return coordinationDrainResult{}, err
	}
	remote, id, err := splitAgentDeckTarget(target)
	if err != nil || remote != "" || !seat.CanReceive {
		return coordinationDrainResult{}, fmt.Errorf("remote completions must be collected into a local conductor or parent session")
	}
	var output struct {
		Remote     string                `json:"remote"`
		Fetched    int                   `json:"fetched"`
		Written    int                   `json:"written"`
		Duplicates int                   `json:"duplicates"`
		Unknown    int                   `json:"unknown"`
		Woke       bool                  `json:"woke"`
		Records    []agentDeckInboxEvent `json:"records"`
		Writer     *struct {
			Running bool   `json:"running"`
			Detail  string `json:"detail"`
		} `json:"writer"`
	}
	if err := p.runJSON(ctx, &output, "remote", "drain", remoteName, "--into", id, "--json"); err != nil {
		return coordinationDrainResult{}, err
	}
	result := coordinationDrainResult{Remote: output.Remote, Target: target, Fetched: output.Fetched, Written: output.Written, Duplicates: output.Duplicates, Unknown: output.Unknown, Woke: output.Woke, Messages: []coordinationMessage{}}
	if result.Remote == "" {
		result.Remote = remoteName
	}
	if output.Writer != nil {
		result.WriterRunning = &output.Writer.Running
		result.WriterDetail = output.Writer.Detail
	}
	for _, record := range output.Records {
		result.Messages = append(result.Messages, agentDeckEventMessage(record, target))
	}
	return result, nil
}

func (p *agentDeckProvider) Attach(ctx context.Context, target string) (coordinationLaunch, error) {
	seat, err := p.agentDeckSeat(ctx, target)
	if err != nil || !seat.Openable {
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
