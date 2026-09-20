package agent

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

var agentIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type metadata map[string]any

func (m metadata) text(key string) string { v, _ := m[key].(string); return v }
func isoTime(value string) int64 {
	return timestampInLocation(value, time.Local)
}
func timestampInLocation(value string, local *time.Location) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UnixMilli()
	}
	// ISO date-only values use UTC; datetimes without a zone use host local
	// time. Preserve the original spelling in metadata and API responses.
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UnixMilli()
	}
	for _, layout := range []string{"2006-01-02T15:04:05Z0700", "2006-01-02T15:04Z07:00", "2006-01-02T15:04Z0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UnixMilli()
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "1/2/2006 15:04:05", "1/2/2006"} {
		if parsed, err := time.ParseInLocation(layout, value, local); err == nil {
			return parsed.UnixMilli()
		}
	}
	// Date.toString() includes a descriptive zone name in parentheses. Its
	// numeric GMT offset determines the instant, not the host's zone database.
	legacy := strings.TrimSpace(value)
	if i := strings.LastIndex(legacy, " ("); i >= 0 && strings.HasSuffix(legacy, ")") {
		legacy = legacy[:i]
	}
	for _, layout := range []string{
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"Mon Jan 02 2006 15:04:05 GMT-0700",
		"January 2, 2006 15:04:05 GMT",
	} {
		if parsed, err := time.Parse(layout, legacy); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
func latestISO(values ...string) string {
	latest := int64(0)
	result := ""
	for _, v := range values {
		if n := isoTime(v); n > latest {
			latest = n
			result = v
		}
	}
	return result
}
func statusFilename(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name)) + ".json"
}

func (s *Service) statusRoot(id string, create bool) (*os.Root, error) {
	if !agentIDPattern.MatchString(id) {
		return nil, errors.New("invalid agent ID")
	}
	dir := filepath.Join(s.store.Home, "data", "agent-status", id)
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(dir)
}
func readMetadata(root *os.Root, id, name string) metadata {
	file, err := root.Open(statusFilename(name))
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	var value metadata
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	if v := value.text("agent_id"); v != "" && v != id {
		return nil
	}
	if v := value.text("name"); v != "" && v != name {
		return nil
	}
	return value
}
func (s *Service) readStatus(id, name string) metadata {
	root, err := s.statusRoot(id, false)
	if err != nil {
		return nil
	}
	defer root.Close()
	return readMetadata(root, id, name)
}

type StatusUpdate struct {
	Status           string
	Source           string
	LastInputAt      *string
	LastOutputAt     *string
	LastOutputSource *string
	LastReadyAt      *string
}

// RecordStatus preserves hook-owned fields and atomically replaces the JSON file.
// Updates within this service serialize; external hooks remain last-writer-wins,
// as in the original server.
func (s *Service) RecordStatus(id, name string, update StatusUpdate) error {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	root, err := s.statusRoot(id, true)
	if err != nil {
		return err
	}
	defer root.Close()
	next := readMetadata(root, id, name)
	if next == nil {
		next = metadata{}
	}
	next["agent_id"], next["name"], next["status"], next["source"], next["updated_at"] = id, name, update.Status, update.Source, s.now().UTC().Format(timeFormat)
	for key, value := range map[string]*string{"last_input_at": update.LastInputAt, "last_output_at": update.LastOutputAt, "last_ready_at": update.LastReadyAt} {
		if value != nil {
			next[key] = *value
		}
	}
	if update.LastOutputSource != nil {
		next["last_output_source"] = *update.LastOutputSource
	} else if update.LastOutputAt != nil {
		delete(next, "last_output_source")
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp := ".status-" + rand.Text() + ".tmp"
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return root.Rename(temp, statusFilename(name))
}

func (s *Service) mergeStatus(item *Session, m metadata) {
	output := m.text("last_output_at")
	if m.text("source") == "webmux" && m.text("last_output_source") != "live" {
		output = ""
	}
	item.LastOutputAt = latestISO(item.LastOutputAt, output, m.text("last_ready_at"))
	last := isoTime(item.LastOutputAt)
	status := m.text("status")
	waiting := status == "waiting" && (last == 0 || isoTime(m.text("updated_at"))+1500 >= last)
	switch {
	case waiting:
		item.Status = "waiting"
	case status == "waiting" || status == "working":
		item.Status = "working"
	case last != 0 && s.now().UnixMilli()-last <= int64(5*time.Minute/time.Millisecond):
		item.Status = "working"
	case status == "" && last != 0 && s.now().UnixMilli()-last >= int64(24*time.Hour/time.Millisecond):
		item.Status = "stale"
	default:
		item.Status = "unknown"
	}
	switch {
	case waiting:
		item.StatusSource = "hook"
		if source, ok := m["source"].(string); ok {
			item.StatusSource = source
		}
	case status == "working":
		item.StatusSource = "webmux"
		if source, ok := m["source"].(string); ok {
			item.StatusSource = source
		}
	case item.LastOutputAt != "":
		item.StatusSource = "tmux"
	default:
		item.StatusSource = "none"
	}
}
