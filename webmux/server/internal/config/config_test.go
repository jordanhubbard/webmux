package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizationDoesNotPersistOverridesOrDiscardDisabledAgents(t *testing.T) {
	raw := Document{App: Object{
		"default_term":  Object{"font_family": "Custom Font,monospace", "cols": 80},
		"terminal_grid": Object{"max_cols": 2, "max_rows": "unlimited"},
		"agents":        Object{"enabled": false, "definitions": []any{Object{"id": "stored", "tmux_socket": "stored"}}},
	}}
	before, _ := json.Marshal(raw)
	t.Setenv("WEBMUX_TERMINAL_GRID_MAX_COLS", "5")
	normalized, err := Normalize(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := AsObject(normalized.App["terminal_grid"])["max_cols"]; got != float64(5) {
		t.Fatal(got)
	}
	if got := AsObject(normalized.App["default_term"])["font_family"]; got != `"Custom Font", monospace` {
		t.Fatal(got)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("normalization mutated persisted config")
	}
	persisted, err := Update(raw, Object{"session_logging": Object{"enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.App["terminal_grid"], raw.App["terminal_grid"]) {
		t.Fatal("environment override persisted")
	}
	if !reflect.DeepEqual(persisted.App["agents"], raw.App["agents"]) {
		t.Fatal("disabled agent definitions discarded")
	}
}

func TestFontNormalizationAndInvalidInputs(t *testing.T) {
	for input, want := range map[string]string{
		"":                               DefaultFontFamily,
		"Custom Font, monospace":         `"Custom Font", monospace`,
		`"Family, with comma",monospace`: `"Family, with comma", monospace`,
		`'Quoted Family', monospace`:     `'Quoted Family', monospace`,
	} {
		got, err := FontFamily(input)
		if err != nil || got != want {
			t.Fatalf("%q: got %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"unclosed'", "a,,b", "a; color: red", strings.Repeat("a", 257), "a\x00b", `unquoted\escape`} {
		if _, err := FontFamily(input); err == nil {
			t.Fatalf("accepted invalid font %q", input)
		}
	}
	faces, err := FontFaces([]any{Object{"family": "Fixture Font", "source": "fonts/a.woff2", "weight": 400, "style": "ITALIC", "display": "SWAP", "url": "discard"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) != 1 || faces[0].Family != "Fixture Font" || faces[0].Weight != "400" || faces[0].Style != "italic" || faces[0].Display != "swap" || faces[0].URL != "" {
		t.Fatal(faces)
	}
	for _, source := range []string{"../outside.ttf", `fonts\..\outside.ttf`, "/etc/font.ttf", `\fonts\rooted.ttf`, `\\server\share\font.ttf`, "https://example.invalid/font.ttf", "C:/font.ttf", "font.js"} {
		if _, err := FontFaces([]any{Object{"family": "Font", "source": source}}); err == nil {
			t.Fatalf("accepted source %q", source)
		}
	}
}

func TestGridLimitFormsAndInvalidValues(t *testing.T) {
	for _, value := range []any{nil, 0, float64(0), "", "0", "none", "unlimited", " infinite "} {
		if got, err := GridLimit(value, "limit"); err != nil || got != nil {
			t.Fatalf("%v: %v %v", value, got, err)
		}
	}
	for _, value := range []any{16, float64(16), "16", "1.6e1", "0x10", "0b10000", "0o20"} {
		if got, err := GridLimit(value, "limit"); err != nil || got == nil || *got != 16 {
			t.Fatalf("%v: %v %v", value, got, err)
		}
	}
	for _, value := range []any{-1, 1.5, true, "-1", "00", "NaN", "Infinity", "1_0"} {
		if _, err := GridLimit(value, "limit"); err == nil {
			t.Fatalf("invalid limit accepted: %v", value)
		}
	}
}

func TestMutableFieldsAndTransportValidation(t *testing.T) {
	for _, field := range []string{"listen_host", "http_port", "https_port", "secure_mode", "agents", "exec_command"} {
		if _, err := Update(Document{App: Object{}}, Object{field: "changed"}); err == nil {
			t.Fatalf("runtime field allowed: %s", field)
		}
	}
	for _, path := range []string{"relative", "/path with spaces", "/usr/../other", "/usr/bin;other"} {
		if _, err := Update(Document{App: Object{}}, Object{"transport": Object{"mosh_server_path": path}}); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
}
