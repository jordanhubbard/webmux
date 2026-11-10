package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

type managedNativeProvider interface {
	PlanNativeRecovery(context.Context, session.RigIdentity) (func(context.Context) ([]string, error), error)
}

func managedRecoveryError(message string) error {
	message = recovery.PlainText(message)
	if len(message) > 1024 {
		message = strings.ToValidUTF8(message[:1024], "�")
	}
	return &config.ValidationError{Message: message + " Retained logs are preserved."}
}

func (s *Server) planManagedRecovery(ctx context.Context, owner string, saved session.RigIdentity) (func(context.Context) ([]string, error), error) {
	if s.openRigPermission(owner) != nil {
		return nil, managedRecoveryError("Access to this lifecycle manager is no longer available.")
	}
	provider := s.coordinationProvider(owner, saved.Provider)
	native, ok := provider.(managedNativeProvider)
	if !ok {
		return nil, managedRecoveryError("This lifecycle manager cannot resume the recorded native conversation; recover through the manager.")
	}
	execute, err := native.PlanNativeRecovery(ctx, saved)
	if err != nil {
		var validation *config.ValidationError
		if errors.As(err, &validation) {
			return nil, err
		}
		return nil, managedRecoveryError("Agent Deck could not prepare recovery: " + err.Error())
	}
	return func(ctx context.Context) ([]string, error) {
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, saved.Provider) == nil {
			return nil, managedRecoveryError("Access to this lifecycle manager changed.")
		}
		return execute(ctx)
	}, nil
}

// No shell commands, prompt contents or launch flags are accepted from the
// browser. The original lifecycle manager owns its saved session command.
func (p *agentDeckProvider) PlanNativeRecovery(ctx context.Context, saved session.RigIdentity) (func(context.Context) ([]string, error), error) {
	if _, _, err := p.prepareManagedClaude(ctx, saved); err != nil {
		return nil, err
	}
	return func(ctx context.Context) ([]string, error) {
		launch, live, err := p.prepareManagedClaude(ctx, saved)
		if err != nil {
			return nil, err
		}
		if live {
			return launch.Argv, nil
		}
		_, id, err := splitAgentDeckTarget(launch.Target)
		if err != nil {
			return nil, err
		}
		args := agentDeckLocalArgs(launch.Profile, "session", "start", id, "--json")
		var result struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := p.runJSON(ctx, &result, args...); err != nil {
			return nil, err
		}
		if result.ID != id || result.Status == "queued" {
			return nil, managedRecoveryError("Agent Deck did not confirm a started conversation.")
		}
		resumed, live, err := p.prepareManagedClaude(ctx, saved)
		if err != nil {
			return nil, err
		}
		if !live || resumed.NativeSession != saved.NativeSessionID || resumed.NativeStateFile != saved.NativeStateFile {
			return nil, managedRecoveryError("Agent Deck did not confirm the recorded native conversation. Inspect it before reconnecting.")
		}
		return resumed.Argv, nil
	}, nil
}

func agentDeckLocalArgs(profile string, args ...string) []string {
	if profile != "" && profile != "default" {
		return append([]string{"-p", profile}, args...)
	}
	return args
}

func (p *agentDeckProvider) prepareManagedClaude(ctx context.Context, saved session.RigIdentity) (coordinationLaunch, bool, error) {
	target := strings.TrimPrefix(saved.Target, "agent-deck:")
	remote, id, err := splitAgentDeckTarget(target)
	if err != nil || remote != "" || saved.Provider != "agent-deck" || (saved.Runtime != "claude" && saved.Runtime != "claude-code") || saved.NativeSource != "agent-deck-inventory" {
		return coordinationLaunch{}, false, managedRecoveryError("Native managed recovery is not yet supported for this destination.")
	}
	launch, err := p.Attach(ctx, target)
	if err != nil {
		return launch, false, err
	}
	normalize := func(profile string) string {
		if profile == "" {
			return "default"
		}
		return profile
	}
	if launch.Provider != saved.Provider || launch.Target != target || launch.Host != saved.Host || launch.Workspace != saved.Workspace || launch.Runtime != saved.Runtime || normalize(launch.Profile) != normalize(saved.ManagerProfile) {
		return launch, false, managedRecoveryError("The managed session's host, project, runtime or profile changed. Review it in Agent Deck.")
	}
	var receipt struct {
		InstanceID   string            `json:"instance_id"`
		State        string            `json:"state"`
		Verdict      string            `json:"verdict"`
		Admissible   bool              `json:"admissible"`
		PaneAttached bool              `json:"pane_attached"`
		ReceiptError string            `json:"receipt_error"`
		Survivors    []json.RawMessage `json:"survivors"`
	}
	if err := p.runJSON(ctx, &receipt, agentDeckLocalArgs(launch.Profile, "session", "ownership", "inspect", id, "--json")...); err != nil {
		return launch, false, err
	}
	if receipt.InstanceID != id || !receipt.Admissible || receipt.ReceiptError != "" || len(receipt.Survivors) != 0 {
		return launch, false, managedRecoveryError("Agent Deck cannot prove that this session has no escaped or unverified processes. Inspect its ownership receipt.")
	}
	if receipt.PaneAttached && receipt.State == "live" && receipt.Verdict == "owned" {
		return launch, true, nil
	}
	if receipt.PaneAttached || (receipt.Verdict != "clear" && (receipt.Verdict != "" || receipt.State != "")) {
		return launch, false, managedRecoveryError("Agent Deck has not confirmed that the original process is gone.")
	}
	if launch.Host != "localhost" {
		return launch, false, managedRecoveryError("Resume this SSH-managed session through Agent Deck on its execution host.")
	}
	version, err := exec.CommandContext(ctx, p.executable, "version").Output()
	fields := strings.Fields(string(version))
	if err != nil || len(fields) < 3 || fields[0] != "Agent" || fields[1] != "Deck" || fields[2] != "v1.16.34" {
		return launch, false, managedRecoveryError("This Agent Deck version has not been qualified for native recovery. Resume through its lifecycle manager.")
	}
	if launch.NativeSession != saved.NativeSessionID || launch.NativeStateFile != saved.NativeStateFile || !managedClaudeState(saved.NativeStateFile, saved.NativeSessionID, saved.Workspace) {
		return launch, false, managedRecoveryError("The recorded native conversation is missing or its project/session changed. Recover it through Agent Deck.")
	}
	return launch, false, nil
}

// Only bounded native JSON records establish the ID/project association. A
// WebMux transcript or a path containing a UUID is insufficient.
func managedClaudeState(path, id, cwd string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(cwd) {
		return false
	}
	directory, err := os.Stat(cwd)
	if err != nil || !directory.IsDir() {
		return false
	}
	recorded, err := os.Stat(path)
	if err != nil || !recorded.Mode().IsRegular() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(recorded, info) {
		return false
	}
	scanner := bufio.NewScanner(io.LimitReader(file, 1<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for n := 0; n < 64 && scanner.Scan(); n++ {
		var record struct {
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.SessionID != id {
			continue
		}
		original, err := os.Stat(record.Cwd)
		if err == nil && os.SameFile(directory, original) {
			return true
		}
	}
	return false
}
