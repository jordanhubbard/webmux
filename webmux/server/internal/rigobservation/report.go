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
	"regexp"
	"strings"
	"time"
	"unicode"
)

const Contract = "webmux-rig-observation/v1"
const Source = "openrig-cli/0.6.14"
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

var target = regexp.MustCompile(`^[a-zA-Z0-9_.:@-]{1,256}$`)
var attach = regexp.MustCompile(`^tmux attach(?:-session)? -t ([a-zA-Z0-9_.:@-]{1,256})$`)

func Validate(r *Report) error {
	if r == nil {
		return nil
	}
	if r.Contract != Contract || r.Source != Source || (r.State != "ok" && r.State != "unavailable") || len(r.Seats) > MaxSeats || (r.State != "ok" && len(r.Seats) != 0) {
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
	cmd := exec.CommandContext(check, "rig", "ps", "--nodes", "-A", "--no-cleanup", "--json", "--fields", "rigId,rigName,logicalId,podNamespace,canonicalSessionName,runtime,lifecycleState,sessionStatus,tmuxAttachCommand")
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "OPENRIG_HOST_SELECTED", "OPENRIG_URL", "RIGGED_URL", "OPENRIG_HOST", "RIGGED_HOST", "OPENRIG_SESSION_NAME", "RIGGED_SESSION_NAME":
			continue
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "OPENRIG_HOST_SELECTED=local")
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
		TmuxAttachCommand string `json:"tmuxAttachCommand"`
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
		if match := attach.FindStringSubmatch(row.TmuxAttachCommand); match != nil {
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
