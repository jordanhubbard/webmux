package recovery

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type claudeSessionObservation struct {
	Event      string `json:"hook_event_name"`
	SessionID  string `json:"session_id"`
	Transcript string `json:"transcript_path"`
	Cwd        string `json:"cwd"`
	Source     string `json:"source"`
	AgentID    string `json:"agent_id"`
}

type claudeHookMessage struct {
	Token       string                   `json:"token"`
	Observation claudeSessionObservation `json:"observation"`
}

type claudeObserver struct {
	settings        string
	endpoint, token string
	start           func(int)
	close           func()
	observations    chan claudeSessionObservation
}

func claudeHookEligible(version string, args []string) bool {
	fields := strings.Fields(version)
	if len(fields) == 0 || fields[0] != "2.1.322" || os.Getenv("CLAUDE_CODE_SAFE_MODE") != "" {
		return false
	}
	for _, arg := range args {
		key, _, _ := strings.Cut(arg, "=")
		switch key {
		case "--safe-mode", "--bare", "--settings", "--setting-sources", "--print", "-p", "--agent":
			return false
		}
	}
	return true
}

func (o claudeSessionObservation) valid() bool {
	if o.Event != "SessionStart" || o.AgentID != "" || !ValidNativeID("claude", o.SessionID) || !filepath.IsAbs(o.Cwd) || !filepath.IsAbs(o.Transcript) {
		return false
	}
	switch o.Source {
	case "startup", "resume", "clear", "compact", "fork":
	default:
		return false
	}
	for _, value := range []string{o.Cwd, o.Transcript} {
		if len(value) > 4096 || strings.ContainsFunc(value, unicode.IsControl) {
			return false
		}
	}
	return true
}

// This command is invoked only by the scoped native hook. It writes no stdout
// (which Claude would interpret as model context), and a recording failure must
// never block the native CLI. The receiver authenticates the per-launch token
// and the calling process's ancestry independently of these JSON fields.
func ClaudeSessionHook(args []string, input io.Reader) error {
	if len(args) != 2 || len(args[0]) > 4096 || len(args[1]) != 32 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(input, (32<<10)+1))
	if err != nil || len(data) > 32<<10 {
		return nil
	}
	var observation claudeSessionObservation
	if json.Unmarshal(data, &observation) != nil || !observation.valid() {
		return nil
	}
	conn, err := net.DialTimeout("unix", args[0], 500*time.Millisecond)
	if err != nil {
		return nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if json.NewEncoder(conn).Encode(claudeHookMessage{Token: args[1], Observation: observation}) != nil {
		return nil
	}
	var ack [1]byte
	_, _ = io.ReadFull(conn, ack[:])
	return nil
}

func observeClaudeSession(event *Event, observation claudeSessionObservation, originalCwd string) {
	if !observation.valid() {
		return
	}
	if !sameDirectory(observation.Cwd, originalCwd) {
		// Relative invocation roots and cross-project policy need separate recovery
		// validation. Retain the last known conversation only as a manual choice.
		event.NativeEvidence = "unavailable"
		return
	}
	event.SessionID, event.StateFile, event.NativeEvidence = observation.SessionID, "", "native-hook"
	event.StateFile = claudeObservedStateFile(observation.SessionID, originalCwd, observation.Transcript)
}

func claudeObservedStateFile(id, cwd, expected string) string {
	verified := claudeStateFile(id, cwd)
	if verified != "" {
		left, e1 := os.Stat(verified)
		right, e2 := os.Stat(expected)
		if e1 == nil && e2 == nil && os.SameFile(left, right) {
			return verified
		}
	}
	return ""
}
