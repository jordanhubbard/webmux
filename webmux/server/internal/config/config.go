// Package config normalizes the existing extensible app.yaml format without
// persisting environment overrides or API-only fields.
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"
)

type Object = map[string]any

type Document struct {
	App   Object `yaml:"app" json:"app"`
	Extra Object `yaml:",inline" json:"-"`
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func invalid(message string) error       { return &ValidationError{message} }

func AsObject(value any) Object {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return Object{}
}

func clone(value Object) Object {
	if value == nil {
		return Object{}
	}
	return maps.Clone(value)
}

func text(value any, fallback string) string {
	if value, ok := value.(string); ok && trim(value) != "" {
		return trim(value)
	}
	return fallback
}

// JavaScript trim/\s includes BOM and excludes the Unicode NEXT LINE character.
func whitespace(r rune) bool {
	return (r >= '\t' && r <= '\r') || r == ' ' || r == '\u00a0' || r == '\u1680' ||
		(r >= '\u2000' && r <= '\u200a') || r == '\u2028' || r == '\u2029' ||
		r == '\u202f' || r == '\u205f' || r == '\u3000' || r == '\ufeff'
}

func trim(value string) string { return strings.TrimFunc(value, whitespace) }

// TrimSpace follows JavaScript String.trim for browser-facing text fields.
func TrimSpace(value string) string { return trim(value) }

func boolean(value any, fallback bool) bool {
	if value, ok := value.(bool); ok {
		return value
	}
	return fallback
}

func length(value string) int { return len(utf16.Encode([]rune(value))) }

var agentID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var socketName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
var moshPath = regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`)

func ValidateMoshServerPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return invalid("Invalid mosh_server_path: must be an absolute path")
	}
	if length(path) > 4096 {
		return invalid("Invalid mosh_server_path: too long")
	}
	if !moshPath.MatchString(path) || slices.Contains(strings.Split(path, "/"), "..") {
		return invalid("Invalid mosh_server_path: " + path)
	}
	return nil
}

// Normalize returns public settings without modifying the input document.
func Normalize(document Document, environment bool) (Document, error) {
	if document.App == nil {
		return Document{}, errors.New("app configuration is missing")
	}
	app := clone(document.App)
	agents, err := normalizeAgents(AsObject(app["agents"]))
	if err != nil {
		return Document{}, err
	}
	app["agents"] = agents
	terminal := clone(AsObject(app["default_term"]))
	family, err := FontFamily(terminal["font_family"])
	if err != nil {
		return Document{}, err
	}
	terminal["font_family"] = family
	app["default_term"] = terminal
	faces, err := FontFaces(app["font_faces"])
	if err != nil {
		return Document{}, err
	}
	app["font_faces"] = faces
	app["session_logging"] = Object{"enabled": boolean(AsObject(app["session_logging"])["enabled"], false)}
	grid := clone(AsObject(app["terminal_grid"]))
	for _, field := range []string{"max_cols", "max_rows"} {
		limit, err := GridLimit(grid[field], "app.terminal_grid."+field)
		if err != nil {
			return Document{}, err
		}
		if environment {
			name := "WEBMUX_TERMINAL_GRID_" + strings.ToUpper(field)
			if value, ok := os.LookupEnv(name); ok {
				limit, err = GridLimit(value, name)
				if err != nil {
					return Document{}, err
				}
			}
		}
		grid[field] = nil
		if limit != nil {
			grid[field] = *limit
		}
	}
	app["terminal_grid"] = grid
	ui := AsObject(app["ui"])
	switcher := AsObject(ui["host_switcher"])
	suffixes := []any{}
	if values, ok := switcher["suffixes"].([]any); ok {
		for _, value := range values {
			if text(value, "") != "" {
				suffixes = append(suffixes, value)
			}
		}
	}
	hosts := []any{}
	if values, ok := switcher["hosts"].([]any); ok {
		for _, value := range values {
			host := AsObject(value)
			_, hasID := host["id"].(string)
			_, hasHostname := host["hostname"].(string)
			if hasID && hasHostname {
				hosts = append(hosts, value)
			}
		}
	}
	app["ui"] = Object{"default_pane": text(ui["default_pane"], "terminals"), "host_switcher": Object{
		"enabled": boolean(switcher["enabled"], false), "suffixes": suffixes, "hosts": hosts,
	}}
	return Document{App: app}, nil
}

func normalizeAgents(raw Object) (Object, error) {
	enabled := boolean(raw["enabled"], false)
	definitions := []any{}
	seen := map[string]bool{}
	if enabled {
		values, ok := raw["definitions"].([]any)
		if !ok && raw["definitions"] != nil {
			return nil, errors.New("invalid agent definitions")
		}
		for _, value := range values {
			definition := AsObject(value)
			if !boolean(definition["enabled"], true) {
				continue
			}
			id := text(definition["id"], "")
			if !agentID.MatchString(id) {
				return nil, fmt.Errorf("invalid agent id %q", id)
			}
			if seen[id] {
				return nil, fmt.Errorf("duplicate agent id %q", id)
			}
			seen[id] = true
			socket := text(definition["tmux_socket"], "")
			if socket == "" || strings.ContainsRune(socket, 0) || (!filepath.IsAbs(socket) && !socketName.MatchString(socket)) {
				return nil, fmt.Errorf("invalid tmux_socket for agent %q", id)
			}
			workspace := text(definition["workspace"], "agent-"+id)
			if strings.ContainsRune(workspace, 0) {
				return nil, fmt.Errorf("invalid workspace for agent %q", id)
			}
			label := text(definition["label"], id)
			badge := utf16.Encode([]rune(text(definition["badge"], strings.ToUpper(id))))
			if len(badge) > 16 {
				badge = badge[:16]
			}
			definitions = append(definitions, Object{
				"id": id, "label": label, "plural_label": text(definition["plural_label"], label+" Sessions"),
				"badge": string(utf16.Decode(badge)), "tmux_socket": socket, "workspace": workspace, "enabled": true,
			})
		}
	}
	return Object{"enabled": enabled, "combined_pane": boolean(raw["combined_pane"], true),
		"disable_in_multi_user_mode": boolean(raw["disable_in_multi_user_mode"], true), "definitions": definitions}, nil
}

// NormalizeAgents validates the independent agent section without requiring
// unrelated display or font settings to be valid.
func NormalizeAgents(raw Object) (Object, error) { return normalizeAgents(raw) }

// Update returns persisted settings; API-only fields and environment overrides
// are applied separately to the public response.
func Update(current Document, updates Object) (Document, error) {
	mutable := []string{"name", "default_term", "font_faces", "terminal_grid", "session_logging", "transport", "ui"}
	for _, key := range slices.Sorted(maps.Keys(updates)) {
		if !slices.Contains(mutable, key) {
			return Document{}, invalid(fmt.Sprintf("Field '%s' cannot be changed at runtime", key))
		}
	}
	if value := AsObject(updates["transport"])["mosh_server_path"]; value != nil && value != "" {
		path, ok := value.(string)
		if !ok {
			return Document{}, invalid("Invalid mosh_server_path: must be an absolute path")
		}
		if err := ValidateMoshServerPath(path); err != nil {
			return Document{}, err
		}
	}
	merged := clone(current.App)
	maps.Copy(merged, updates)
	for _, field := range []string{"default_term", "terminal_grid", "session_logging", "transport", "ui"} {
		if value := updates[field]; value != nil {
			patch, ok := value.(map[string]any)
			if !ok {
				return Document{}, invalid("Invalid app." + field)
			}
			object := clone(AsObject(current.App[field]))
			maps.Copy(object, patch)
			merged[field] = object
		} else if original, ok := current.App[field]; ok {
			merged[field] = original
		} else {
			delete(merged, field)
		}
	}
	normalized, err := Normalize(Document{App: merged}, false)
	if err != nil {
		return Document{}, err
	}
	for _, field := range []string{"default_term", "font_faces", "session_logging"} {
		merged[field] = normalized.App[field]
	}
	return Document{App: merged, Extra: current.Extra}, nil
}
