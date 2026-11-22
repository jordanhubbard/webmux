// Package coordination owns advisory session coordination, independently of
// optional provider lifecycle managers. It never writes terminal input or runs jobs.
package coordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const Contract = "webmux-session-coordination/v1"
const DefaultLease = 120

var (
	ErrUnauthorized = errors.New("session coordination capability is invalid or inactive")
	ErrDisabled     = errors.New("session coordination is disabled by server policy")
	ErrClosed       = errors.New("session coordination is closed")
	ErrInvalid      = errors.New("invalid session coordination request")
	ErrClock        = errors.New("coordination authority clock moved backwards; leases remain held")
	ErrIdentity     = errors.New("coordination node identity differs from its retained database")
)

type Ref struct {
	Node        string `json:"node_id"`
	Owner       string `json:"owner"`
	Session     string `json:"session_id"`
	Incarnation string `json:"incarnation"`
}

func (r Ref) Key() string       { data, _ := json.Marshal(r); return digest(data) }
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func validText(s string, max int) bool {
	return s != "" && len(s) <= max && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
}
func validHex(s string, n int) bool {
	if len(s) != n*2 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func validRef(r Ref) bool {
	return validHex(r.Node, 16) && validText(r.Owner, 256) && validText(r.Session, 128) && validHex(r.Incarnation, 16)
}

// Binding is supplied by a verified execution-host resolver or an explicit
// administrator mapping. Agent requests cannot assign their own project identity.
type Binding struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Source     string `json:"source"`
	CaseFold   bool   `json:"case_fold"`
}

func validBinding(p *Binding) bool {
	return p == nil || (validText(p.ID, 256) && validText(p.Repository, 2048) && validText(p.Worktree, 4096) && (p.Source == "verified-git" || p.Source == "explicit-map"))
}

type MemberInput struct {
	Interface     string   `json:"interface"`
	ProjectStatus string   `json:"project_status"`
	Ref           Ref      `json:"ref"`
	Active        bool     `json:"active"`
	Transport     string   `json:"transport"`
	ExecutionHost string   `json:"execution_host"`
	Project       *Binding `json:"project,omitempty"`
}
type Member struct {
	MemberInput
	Present   bool       `json:"present"`
	Awareness string     `json:"interface_state"`
	Task      string     `json:"task"`
	Joined    time.Time  `json:"joined"`
	Observed  time.Time  `json:"observed"`
	Revoked   *time.Time `json:"revoked,omitempty"`
}
type Claim struct {
	ID       string    `json:"id"`
	Actor    Ref       `json:"actor"`
	Project  string    `json:"project"`
	Kind     string    `json:"kind"`
	Resource string    `json:"resource"`
	CaseFold bool      `json:"case_fold"`
	Expires  time.Time `json:"expires"`
}
type Message struct {
	ID           string     `json:"id"`
	Index        uint64     `json:"index"`
	From         Ref        `json:"from"`
	To           Ref        `json:"to"`
	Project      string     `json:"project"`
	Kind         string     `json:"kind"`
	Text         string     `json:"text"`
	State        string     `json:"state"`
	Created      time.Time  `json:"created"`
	Delivered    *time.Time `json:"delivered,omitempty"`
	Acknowledged *time.Time `json:"acknowledged,omitempty"`
	Completed    *time.Time `json:"completed,omitempty"`
}
type Reservation struct {
	ID       string    `json:"id"`
	Resource string    `json:"resource"`
	Actor    Ref       `json:"actor"`
	Project  string    `json:"project"`
	Fence    uint64    `json:"fence"`
	Expires  time.Time `json:"expires"`
}
type ResourceView struct {
	ID        string     `json:"id"`
	Authority string     `json:"authority_node_id"`
	Occupied  bool       `json:"occupied"`
	Fence     uint64     `json:"fence"`
	Expires   *time.Time `json:"expires,omitempty"`
	Holder    *Ref       `json:"holder,omitempty"`
}
type Request struct {
	ID           string `json:"request_id"`
	Action       string `json:"action"`
	Kind         string `json:"kind,omitempty"`
	Resource     string `json:"resource,omitempty"`
	Text         string `json:"text,omitempty"`
	Target       *Ref   `json:"target,omitempty"`
	Record       string `json:"record_id,omitempty"`
	Fence        uint64 `json:"fence,omitempty"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}
type Result struct {
	Status    string     `json:"status"`
	Record    string     `json:"record_id,omitempty"`
	State     string     `json:"state,omitempty"`
	Fence     uint64     `json:"fence,omitempty"`
	Expires   *time.Time `json:"expires,omitempty"`
	Conflicts []Claim    `json:"conflicts,omitempty"`
}
type View struct {
	Peers      []PeerView     `json:"peers,omitempty"`
	Scope      string         `json:"scope"`
	Contract   string         `json:"contract"`
	Enabled    bool           `json:"enabled"`
	Self       *Member        `json:"self,omitempty"`
	Members    []Member       `json:"members"`
	Claims     []Claim        `json:"claims"`
	Inbox      []Message      `json:"inbox"`
	Outbox     []Message      `json:"outbox"`
	NextCursor uint64         `json:"next_cursor"`
	Resources  []ResourceView `json:"resources"`
}
type receipt struct {
	Hash   string `json:"hash"`
	Result Result `json:"result"`
}

func canonicalResource(kind, resource string) (string, bool) {
	if !validText(resource, 1024) {
		return "", false
	}
	if kind != "file" && kind != "directory" {
		return resource, kind == "task" || kind == "branch"
	}
	resource = strings.ReplaceAll(resource, "\\", "/")
	if strings.HasPrefix(resource, "/") || strings.Contains(resource, ":") {
		return "", false
	}
	for _, part := range strings.Split(resource, "/") {
		if part == ".." || part == "." || part == "" {
			return "", false
		}
	}
	clean := path.Clean(resource)
	return clean, clean != "." && !strings.HasPrefix(clean, "../")
}
func overlaps(a, b Claim) bool {
	if a.Project != b.Project {
		return false
	}
	paths := func(kind string) bool { return kind == "file" || kind == "directory" }
	if !paths(a.Kind) || !paths(b.Kind) {
		return a.Kind == b.Kind && a.Resource == b.Resource
	}
	left, right := a.Resource, b.Resource
	if a.CaseFold || b.CaseFold {
		left, right = strings.ToLower(left), strings.ToLower(right)
	}
	return left == right || (a.Kind == "directory" && strings.HasPrefix(right, left+"/")) || (b.Kind == "directory" && strings.HasPrefix(left, right+"/"))
}
