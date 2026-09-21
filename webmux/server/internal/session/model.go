package session

import (
	"errors"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

var ErrNotFound = errors.New("Session not found")
var ErrClosed = errors.New("session broker is shutting down")

type Session struct {
	ID               string   `json:"id" yaml:"id"`
	Kind             string   `json:"kind" yaml:"kind"`
	Owner            string   `json:"owner" yaml:"owner"`
	Transport        string   `json:"transport" yaml:"transport"`
	HostID           string   `json:"host_id" yaml:"host_id"`
	Hostname         string   `json:"hostname" yaml:"hostname"`
	Port             int      `json:"port" yaml:"port"`
	Username         string   `json:"username" yaml:"username"`
	KeyID            string   `json:"key_id" yaml:"key_id"`
	ExecCommand      string   `json:"exec_command,omitempty" yaml:"exec_command,omitempty"`
	ExecArgv         []string `json:"exec_argv,omitempty" yaml:"exec_argv,omitempty"`
	ExecCwd          string   `json:"exec_cwd,omitempty" yaml:"exec_cwd,omitempty"`
	Cols             int      `json:"cols" yaml:"cols"`
	Rows             int      `json:"rows" yaml:"rows"`
	Row              int      `json:"row" yaml:"row"`
	Col              int      `json:"col" yaml:"col"`
	State            string   `json:"state" yaml:"state"`
	CreatedAt        string   `json:"created_at" yaml:"created_at"`
	UpdatedAt        string   `json:"updated_at" yaml:"updated_at"`
	Title            string   `json:"title" yaml:"title"`
	Persistent       bool     `json:"persistent" yaml:"persistent"`
	Minimized        bool     `json:"minimized" yaml:"minimized"`
	Workspace        string   `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	AgentID          string   `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	AgentRole        string   `json:"agent_role,omitempty" yaml:"agent_role,omitempty"`
	AgentSessionName string   `json:"agent_session_name,omitempty" yaml:"agent_session_name,omitempty"`
}

func (s Session) Agent() bool    { return s.AgentID != "" && s.AgentRole != "" }
func (s Session) clone() Session { s.ExecArgv = append([]string(nil), s.ExecArgv...); return s }
func (s Session) launchRequest() terminal.LaunchRequest {
	return terminal.LaunchRequest{Hostname: s.Hostname, Username: s.Username, Transport: s.Transport, KeyID: s.KeyID, Port: s.Port, Cols: s.Cols, Rows: s.Rows, ExecCommand: s.ExecCommand, ExecArgv: append([]string(nil), s.ExecArgv...), ExecCwd: s.ExecCwd}
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
	Row       *int    `json:"row"`
	Col       *int    `json:"col"`
	Title     *string `json:"title"`
	Minimized *bool   `json:"minimized"`
}

type document struct {
	Sessions []Session `yaml:"sessions"`
}
