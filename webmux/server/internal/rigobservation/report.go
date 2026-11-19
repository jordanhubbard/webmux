// Package rigobservation defines the bounded, read-only host-client Rig capability.
package rigobservation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const Contract = "webmux-rig-observation/v1"
const Source = "openrig-cli/0.6.14"
const StructuredSource = "openrig-cli/0.7.18"
const MaxBytes = 128 * 1024
const MaxSeats = 256

type Seat struct {
	RigID                string `json:"rigId" yaml:"rigId"`
	RigName              string `json:"rigName" yaml:"rigName"`
	LogicalID            string `json:"logicalId" yaml:"logicalId"`
	PodNamespace         string `json:"podNamespace,omitempty" yaml:"podNamespace,omitempty"`
	CanonicalSessionName string `json:"canonicalSessionName,omitempty" yaml:"canonicalSessionName,omitempty"`
	Runtime              string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	LifecycleState       string `json:"lifecycleState,omitempty" yaml:"lifecycleState,omitempty"`
	SessionStatus        string `json:"sessionStatus,omitempty" yaml:"sessionStatus,omitempty"`
	TmuxTarget           string `json:"tmuxTarget,omitempty" yaml:"tmuxTarget,omitempty"`
	TmuxServerFlag       string `json:"tmuxServerFlag,omitempty" yaml:"tmuxServerFlag,omitempty"`
	TmuxServerValue      string `json:"tmuxServerValue,omitempty" yaml:"tmuxServerValue,omitempty"`
}

type Report struct {
	Contract string `json:"contract" yaml:"contract"`
	Source   string `json:"source" yaml:"source"`
	State    string `json:"state" yaml:"state"`
	Seats    []Seat `json:"seats" yaml:"seats"`
}

// Key binds a stable logical seat to its canonical rig, independently of the
// current tmux attachment name. The server adds authenticated client identity.
func (s Seat) Key() string {
	sum := sha256.Sum256([]byte(s.RigID + "\x00" + s.LogicalID))
	return hex.EncodeToString(sum[:])
}

var tmuxLabel = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)
var target = regexp.MustCompile(`^[a-zA-Z0-9_.:@-]{1,256}$`)
var attach = regexp.MustCompile(`^tmux attach(?:-session)? -t ([a-zA-Z0-9_.:@-]{1,256})$`)

func Validate(r *Report) error {
	if r == nil {
		return nil
	}
	if r.Contract != Contract || (r.Source != Source && r.Source != StructuredSource) || (r.State != "ok" && r.State != "unavailable") || len(r.Seats) > MaxSeats || (r.State != "ok" && len(r.Seats) != 0) {
		return errors.New("invalid Rig observation contract")
	}
	seen := map[string]bool{}
	sessions := map[string]bool{}
	for _, s := range r.Seats {
		if s.RigID == "" || s.LogicalID == "" || s.RigName == "" || seen[s.Key()] || (s.CanonicalSessionName != "" && sessions[s.CanonicalSessionName]) {
			return errors.New("invalid or duplicate Rig identity")
		}
		seen[s.Key()] = true
		if s.CanonicalSessionName != "" {
			sessions[s.CanonicalSessionName] = true
		}
		for _, value := range []string{s.RigID, s.RigName, s.LogicalID, s.PodNamespace, s.CanonicalSessionName, s.Runtime, s.LifecycleState, s.SessionStatus} {
			if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return errors.New("invalid Rig observation field")
			}
		}
		if s.TmuxTarget != "" && !target.MatchString(s.TmuxTarget) {
			return errors.New("invalid Rig attachment target")
		}
		selected := s.TmuxServerFlag != "" || s.TmuxServerValue != ""
		if selected {
			if r.Source != StructuredSource || s.TmuxTarget == "" || s.TmuxServerFlag != "-S" || !path.IsAbs(s.TmuxServerValue) || len(s.TmuxServerValue) > 2048 || strings.ContainsFunc(s.TmuxServerValue, unicode.IsControl) {
				return errors.New("invalid Rig tmux server selection")
			}
		} else if r.Source == StructuredSource && s.TmuxTarget != "" {
			return errors.New("invalid Rig tmux server selection")
		}
	}
	data, err := json.Marshal(r)
	if err != nil || len(data) > MaxBytes {
		return errors.New("Rig observation exceeds bound")
	}
	return nil
}

type boundedOutput struct{ buffer bytes.Buffer }

func (b *boundedOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > MaxBytes {
		return 0, errors.New("Rig output exceeds bound")
	}
	return b.buffer.Write(p)
}

// Observe never runs start/up/attach or cleanup. The version-qualified ps path
// only probes the existing daemon. Ignore persisted cross-host selection and
// inherited agent routing so the report describes this client's own instance.
func Observe(ctx context.Context) *Report {
	report := &Report{Contract: Contract, Source: Source, State: "unavailable", Seats: []Seat{}}
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var env []string
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "OPENRIG_HOST_SELECTED", "OPENRIG_URL", "RIGGED_URL", "OPENRIG_HOST", "RIGGED_HOST", "OPENRIG_SESSION_NAME", "RIGGED_SESSION_NAME":
			continue
		}
		env = append(env, value)
	}
	env = append(env, "OPENRIG_HOST_SELECTED=local")
	version := exec.CommandContext(check, "rig", "--version")
	version.Env = env
	version.Stderr = io.Discard
	version.WaitDelay = 250 * time.Millisecond
	var versionOutput boundedOutput
	version.Stdout = &versionOutput
	structured := version.Run() == nil && strings.TrimSpace(string(versionOutput.Bytes())) == "0.7.18 (3bf87c62)"
	args := []string{"ps", "--nodes", "-A", "--no-cleanup", "--json"}
	if structured {
		report.Source = StructuredSource
		args = append(args, "--full")
	} else {
		args = append(args, "--fields", "rigId,rigName,logicalId,podNamespace,canonicalSessionName,runtime,lifecycleState,sessionStatus,tmuxAttachCommand")
	}
	cmd := exec.CommandContext(check, "rig", args...)
	cmd.Env = env
	var out boundedOutput
	cmd.Stdout = &out
	// Never retain arbitrary CLI diagnostics or credentials from its environment.
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 250 * time.Millisecond
	if err := cmd.Run(); err != nil {
		return report
	}
	type row struct {
		Seat
		TmuxAttachCommand string          `json:"tmuxAttachCommand"`
		AttachmentType    string          `json:"attachmentType"`
		ServerArgs        json.RawMessage `json:"tmuxServerArgs"`
	}
	var rows []row
	data := bytes.TrimSpace(out.Bytes())
	if len(data) > 0 && data[0] == '[' {
		if json.Unmarshal(data, &rows) != nil {
			return report
		}
	} else {
		var envelope struct {
			Entries []row `json:"entries"`
		}
		if json.Unmarshal(data, &envelope) != nil || envelope.Entries == nil {
			return report
		}
		rows = envelope.Entries
	}
	for _, row := range rows {
		// Do not accept an executable command or an unverified wire tmuxTarget.
		row.TmuxTarget = ""
		row.TmuxServerFlag, row.TmuxServerValue = "", ""
		if structured {
			var args []string
			valid := len(row.ServerArgs) == 0
			if len(row.ServerArgs) > 0 && string(row.ServerArgs) != "null" {
				valid = json.Unmarshal(row.ServerArgs, &args) == nil
			}
			if valid && row.AttachmentType == "tmux" && target.MatchString(row.CanonicalSessionName) {
				flag, value := "", ""
				if len(args) == 0 && os.Getuid() >= 0 {
					flag, value = "-S", "/tmp/tmux-"+strconv.Itoa(os.Getuid())+"/default"
				} else if len(args) == 2 {
					flag, value = args[0], args[1]
					if flag == "-L" && tmuxLabel.MatchString(value) && value != "." && value != ".." && os.Getuid() >= 0 {
						flag, value = "-S", "/tmp/tmux-"+strconv.Itoa(os.Getuid())+"/"+value
					}
				}
				if flag == "-S" && path.IsAbs(value) && len(value) <= 2048 && !strings.ContainsFunc(value, unicode.IsControl) {
					row.TmuxTarget, row.TmuxServerFlag, row.TmuxServerValue = row.CanonicalSessionName, flag, value
				}
			}
		} else if match := attach.FindStringSubmatch(row.TmuxAttachCommand); match != nil {
			row.TmuxTarget = match[1]
		}
		report.Seats = append(report.Seats, row.Seat)
	}
	report.State = "ok"
	if Validate(report) != nil {
		report.State = "unavailable"
		report.Seats = []Seat{}
	}
	return report
}
