package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

type Session struct {
	Name         string  `json:"name"`
	AgentID      string  `json:"agent_id"`
	DisplayName  string  `json:"display_name"`
	Windows      float64 `json:"windows"`
	Attached     float64 `json:"attached"`
	CreatedAt    string  `json:"created_at,omitempty"`
	LastOutputAt string  `json:"last_output_at,omitempty"`
	Status       string  `json:"status"`
	StatusSource string  `json:"status_source"`
}

var timestampSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}$`)

func displayBase(id, name string) string {
	base := timestampSuffix.ReplaceAllString(strings.TrimPrefix(name, id+"-"), "")
	if base == "" {
		return name
	}
	return base
}

// Assign labels without changing tmux's session order. English collation matches
// the legacy server's default en-US locale; other host locales need parity gates.
func assignDisplayNames(sessions []Session) {
	groups := map[string][]int{}
	for i := range sessions {
		base := displayBase(sessions[i].AgentID, sessions[i].Name)
		sessions[i].DisplayName = base
		groups[base] = append(groups[base], i)
	}
	collation := collate.New(language.English)
	for base, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		sort.SliceStable(indices, func(i, j int) bool {
			a, b := sessions[indices[i]], sessions[indices[j]]
			at, bt := isoTime(a.CreatedAt), isoTime(b.CreatedAt)
			if at != bt {
				return at < bt
			}
			if c := collation.CompareString(a.AgentID, b.AgentID); c != 0 {
				return c < 0
			}
			return collation.CompareString(a.Name, b.Name) < 0
		})
		for n, i := range indices {
			sessions[i].DisplayName = fmt.Sprintf("%s (%d)", base, n+1)
		}
	}
}

func number(raw string) float64 {
	v, err := strconv.ParseFloat(config.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
func epochISO(raw string) string {
	v := number(raw)
	if v <= 0 || v > 253402300799 {
		return ""
	}
	return time.UnixMilli(int64(v * 1000)).UTC().Format(timeFormat)
}
func socketArgs(d Definition) []string {
	flag := "-L"
	if filepath.IsAbs(d.TMUXSocket) {
		flag = "-S"
	}
	return []string{flag, d.TMUXSocket}
}

func (s *Service) parse(output string, d Definition) []Session {
	sessions := []Session{}
	for _, line := range strings.Split(output, "\n") {
		line = config.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		field := func(i int) string {
			if i < len(fields) {
				return fields[i]
			}
			return ""
		}
		item := Session{Name: field(0), AgentID: d.ID, Windows: number(field(1)), Attached: number(field(2)), CreatedAt: epochISO(field(3)), LastOutputAt: epochISO(field(4))}
		s.mergeStatus(&item, nil)
		sessions = append(sessions, item)
	}
	assignDisplayNames(sessions)
	return sessions
}

func (s *Service) List(ctx context.Context, id string) ([]Session, error) {
	d, err := s.definition(id)
	if err != nil {
		return nil, err
	}
	args := append(socketArgs(d), "list-sessions", "-F", "#S\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}")
	output, stderr, err := s.run(ctx, "tmux", args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("tmux is not installed")
		}
		message := strings.ToLower(err.Error() + "\n" + stderr)
		if strings.Contains(message, "no server running") || strings.Contains(message, "error connecting to") && strings.Contains(message, "no such file or directory") {
			return []Session{}, nil
		}
		return nil, err
	}
	sessions := s.parse(output, d)
	for i := range sessions {
		s.mergeStatus(&sessions[i], s.readStatus(id, sessions[i].Name))
	}
	assignDisplayNames(sessions)
	return sessions, nil
}
func (s *Service) ListAll(ctx context.Context) ([]Session, error) {
	c, err := s.Config()
	if err != nil {
		return nil, err
	}
	sessions := []Session{}
	if !c.Enabled {
		return sessions, nil
	}
	for _, d := range c.Definitions {
		items, err := s.List(ctx, d.ID)
		if err != nil {
			s.logger.Warn("Failed to list combined agent sessions", "agent_id", d.ID, "error", err)
			continue
		}
		sessions = append(sessions, items...)
	}
	assignDisplayNames(sessions)
	return sessions, nil
}
func (s *Service) HasSession(ctx context.Context, id, name string) (bool, error) {
	items, err := s.List(ctx, id)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Name == name {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) AttachArgv(id, name string) ([]string, error) {
	d, err := s.definition(id)
	if err != nil {
		return nil, err
	}
	return append(append([]string{"tmux"}, socketArgs(d)...), "attach-session", "-t", name), nil
}
func (s *Service) PaneCurrentPath(ctx context.Context, id, name string) string {
	d, err := s.definition(id)
	if err != nil {
		return ""
	}
	output, _, err := s.run(ctx, "tmux", append(socketArgs(d), "display-message", "-p", "-t", name, "#{pane_current_path}")...)
	if err != nil {
		return ""
	}
	cwd := config.TrimSpace(output)
	if !filepath.IsAbs(cwd) {
		return ""
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return ""
	}
	return cwd
}
