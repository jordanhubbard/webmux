package session

import (
	"errors"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

var ErrNotFound = errors.New("Session not found")
var ErrClosed = errors.New("session broker is shutting down")

// RigIdentity is recorded only from the server coordinator, never a terminal title.
type RigIdentity struct {
	ProjectID             string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	NativeRigRecovery     string `json:"native_rig_recovery,omitempty" yaml:"native_rig_recovery,omitempty"`
	TmuxServerFlag        string `json:"tmux_server_flag,omitempty" yaml:"tmux_server_flag,omitempty"`
	TmuxServerValue       string `json:"tmux_server_value,omitempty" yaml:"tmux_server_value,omitempty"`
	AttachmentTarget      string `json:"attachment_target,omitempty" yaml:"attachment_target,omitempty"`
	SeatID                string `json:"seat_id,omitempty" yaml:"seat_id,omitempty"`
	RigHostID             string `json:"rig_host_id,omitempty" yaml:"rig_host_id,omitempty"`
	ManagerProcess        string `json:"manager_process,omitempty" yaml:"manager_process,omitempty"`
	ManagerRigID          string `json:"manager_rig_id,omitempty" yaml:"manager_rig_id,omitempty"`
	ManagerNodeID         string `json:"manager_node_id,omitempty" yaml:"manager_node_id,omitempty"`
	ManagerLogicalID      string `json:"manager_logical_id,omitempty" yaml:"manager_logical_id,omitempty"`
	ManagerSnapshotID     string `json:"manager_snapshot_id,omitempty" yaml:"manager_snapshot_id,omitempty"`
	ManagerSnapshotHash   string `json:"manager_snapshot_hash,omitempty" yaml:"manager_snapshot_hash,omitempty"`
	ManagerPolicyHash     string `json:"manager_policy_hash,omitempty" yaml:"manager_policy_hash,omitempty"`
	ManagerRecordIdentity string `json:"manager_record_identity,omitempty" yaml:"manager_record_identity,omitempty"`
	ManagerProfile        string `json:"manager_profile,omitempty" yaml:"manager_profile,omitempty"`
	NativeSessionID       string `json:"native_session_id,omitempty" yaml:"native_session_id,omitempty"`
	NativeStateFile       string `json:"native_state_file,omitempty" yaml:"native_state_file,omitempty"`
	NativeSource          string `json:"native_source,omitempty" yaml:"native_source,omitempty"`
	NativeObservedAt      string `json:"native_observed_at,omitempty" yaml:"native_observed_at,omitempty"`
	Provider              string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Target                string `json:"target" yaml:"target"`
	Project               string `json:"project" yaml:"project"`
	Name                  string `json:"name" yaml:"name"`
	Host                  string `json:"host" yaml:"host"`
	Runtime               string `json:"runtime" yaml:"runtime"`
	Workspace             string `json:"workspace,omitempty" yaml:"workspace,omitempty"`
}

// CoordinationObservation is live routing metadata, independent of transcript
// recording. It is never persisted as recovery history or returned in session JSON.
type CoordinationObservation struct{ Directory, CLI, Version, Interface string }

// CoordinationRemote identifies ordinary sessions whose repository must be
// inspected through their authenticated execution-host channel. Provider-owned
// viewers must not inherit the authority of the local attachment process.
func (s Session) CoordinationRemote() bool {
	return (s.Transport == "ssh" || s.Transport == "mosh") && s.Rig == nil && !s.Agent() && s.Companion == "" && s.OpenRigView == ""
}

type Session struct {
	CoordinationChannel  string                     `json:"-" yaml:"-"`
	CoordinationActivity *CoordinationObservation   `json:"-" yaml:"-"`
	CoordinationRun      uint64                     `json:"-" yaml:"-"` // Live broker generation, never persisted as authority.
	CoordinationLive     bool                       `json:"-" yaml:"-"` // Broker-observed viewer/run state, never restored as authority.
	LocalOwnerManaged    bool                       `json:"-" yaml:"local_owner_managed,omitempty"`
	LocalScope           string                     `json:"-" yaml:"local_scope,omitempty"`
	LocalOwnerAvailable  bool                       `json:"-" yaml:"-"`
	LocalOwner           *terminal.LocalOwnerTarget `json:"-" yaml:"local_owner,omitempty"`
	LocalStop            *LocalStopIntent           `json:"-" yaml:"local_stop,omitempty"`
	LocalLifecycle       *LocalLifecycle            `json:"local_lifecycle,omitempty" yaml:"-"`
	LastBrowserAck       string                     `json:"-" yaml:"last_browser_ack,omitempty"`
	TranscriptEnabled    *bool                      `json:"transcript_enabled,omitempty" yaml:"transcript_enabled,omitempty"`
	RecoveryKey          string                     `json:"-" yaml:"recovery_key,omitempty"`
	Recovery             *recovery.State            `json:"recovery,omitempty" yaml:"recovery,omitempty"`
	Rig                  *RigIdentity               `json:"rig,omitempty" yaml:"rig,omitempty"`
	OpenRigView          string                     `json:"openrig_view,omitempty" yaml:"openrig_view,omitempty"`
	Companion            string                     `json:"companion,omitempty" yaml:"companion,omitempty"`
	BrowserLocal         bool                       `json:"browser_local,omitempty" yaml:"browser_local,omitempty"`
	ID                   string                     `json:"id" yaml:"id"`
	Kind                 string                     `json:"kind" yaml:"kind"`
	Owner                string                     `json:"owner" yaml:"owner"`
	Transport            string                     `json:"transport" yaml:"transport"`
	HostID               string                     `json:"host_id" yaml:"host_id"`
	Hostname             string                     `json:"hostname" yaml:"hostname"`
	Port                 int                        `json:"port" yaml:"port"`
	Username             string                     `json:"username" yaml:"username"`
	KeyID                string                     `json:"key_id" yaml:"key_id"`
	ExecCommand          string                     `json:"exec_command,omitempty" yaml:"exec_command,omitempty"`
	ExecArgv             []string                   `json:"exec_argv,omitempty" yaml:"exec_argv,omitempty"`
	ExecCwd              string                     `json:"exec_cwd,omitempty" yaml:"exec_cwd,omitempty"`
	Cols                 int                        `json:"cols" yaml:"cols"`
	Rows                 int                        `json:"rows" yaml:"rows"`
	Row                  int                        `json:"row" yaml:"row"`
	Col                  int                        `json:"col" yaml:"col"`
	State                string                     `json:"state" yaml:"state"`
	CreatedAt            string                     `json:"created_at" yaml:"created_at"`
	UpdatedAt            string                     `json:"updated_at" yaml:"updated_at"`
	Title                string                     `json:"title" yaml:"title"`
	ProjectLabel         string                     `json:"project_label,omitempty" yaml:"project_label,omitempty"`
	Persistent           bool                       `json:"persistent" yaml:"persistent"`
	Minimized            bool                       `json:"minimized" yaml:"minimized"`
	Workspace            string                     `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	AgentID              string                     `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	AgentRole            string                     `json:"agent_role,omitempty" yaml:"agent_role,omitempty"`
	AgentSessionName     string                     `json:"agent_session_name,omitempty" yaml:"agent_session_name,omitempty"`
}

func (s Session) Agent() bool { return s.AgentID != "" && s.AgentRole != "" }
func (s Session) clone() Session {
	if s.CoordinationActivity != nil {
		copy := *s.CoordinationActivity
		s.CoordinationActivity = &copy
	}
	if s.LocalOwner != nil {
		copy := *s.LocalOwner
		s.LocalOwner = &copy
	}
	if s.LocalStop != nil {
		copy := *s.LocalStop
		s.LocalStop = &copy
	}
	s.LocalLifecycle = localLifecycle(s)
	if s.TranscriptEnabled != nil {
		enabled := *s.TranscriptEnabled
		s.TranscriptEnabled = &enabled
	}
	s.Recovery = s.Recovery.Clone()
	s.ExecArgv = append([]string(nil), s.ExecArgv...)
	if s.Rig != nil {
		identity := *s.Rig
		s.Rig = &identity
	}
	return s
}
func (s Session) launchRequest() terminal.LaunchRequest {
	request := terminal.LaunchRequest{SessionID: s.ID, RecoveryKey: s.RecoveryKey, Hostname: s.Hostname, Username: s.Username, Transport: s.Transport, KeyID: s.KeyID, Port: s.Port, Cols: s.Cols, Rows: s.Rows, ExecCommand: s.ExecCommand, ExecArgv: append([]string(nil), s.ExecArgv...), ExecCwd: s.ExecCwd}
	if s.Recovery != nil {
		request.RecoverySeed = s.Recovery.Clone().Activity
	}
	return request
}

type CreateRequest struct {
	HostID         string `json:"host_id"`
	Hostname       string `json:"hostname"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	KeyID          string `json:"key_id"`
	Transport      string `json:"transport"`
	ExecCommand    string `json:"exec_command"`
	Cols           int    `json:"cols"`
	Rows           int    `json:"rows"`
	Row            *int   `json:"row"`
	Col            *int   `json:"col"`
	InitialCommand string `json:"initial_cmd"`
	TemplateID     string `json:"template_id"`
}

type Patch struct {
	Row          *int    `json:"row"`
	Col          *int    `json:"col"`
	Title        *string `json:"title"`
	ProjectLabel *string `json:"project_label"`
	Minimized    *bool   `json:"minimized"`
	HostID       *string `json:"host_id"`
}

type document struct {
	Sessions []Session `yaml:"sessions"`
}
