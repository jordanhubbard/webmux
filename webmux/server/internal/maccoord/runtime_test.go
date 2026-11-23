package maccoord

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func runtimeFiles(t *testing.T, origin string) (string, string, map[string]any) {
	t.Helper()
	directory := t.TempDir()
	credential := filepath.Join(directory, "native-read-enrollment.json")
	manifest := map[string]any{
		"schema": "mac.client_enrollment.v1", "client_id": "webmux-fixture",
		"connection": map[string]any{"api_url": origin},
		"credential": map[string]any{"token": accessToken, "scopes": []string{"read"}, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)},
	}
	writeRuntimeJSON(t, credential, manifest)
	document := bindingFixture()
	document.Authorities[0].Origin = origin
	policy := map[string]any{"schema": RuntimeSchema, "bindings": document, "credentials": []any{map[string]any{"authority_id": "fleet-a", "client_id": "webmux-fixture", "enrollment_file": credential}}}
	path := filepath.Join(directory, "mac-observation.json")
	writeRuntimeJSON(t, path, policy)
	return path, credential, manifest
}

func writeRuntimeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	protectRuntimeFixture(t, path)
}

func runtimeServer(t *testing.T, fixture *projectFixture, broad bool) *httptest.Server {
	t.Helper()
	backend := projectServer(t, fixture)
	t.Cleanup(backend.Close)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/__webmux_read_scope_probe_no_handler__" {
			if broad {
				w.WriteHeader(404)
			} else {
				w.WriteHeader(403)
			}
			return
		}
		backend.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRuntimeUsesExplicitPrivateReadClientAndDetectsPolicyChanges(t *testing.T) {
	fixture := &projectFixture{}
	server := runtimeServer(t, fixture, false)
	path, _, _ := runtimeFiles(t, server.URL)
	roots := server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	r := OpenRuntime(path, roots)
	if mode, err := r.Backend("alice", repository()); err != nil || mode != "mac" {
		t.Fatal("configured backend unavailable", mode, err)
	}
	view, err := r.ObserveProject(t.Context(), "alice", repository(), 0, 1)
	if err != nil || view.Schema != ProjectSchema || len(view.Tasks) != 1 {
		t.Fatal("explicit observer failed", err)
	}
	data, _ := json.Marshal(view)
	if strings.Contains(string(data), accessToken) || strings.Contains(string(data), path) || strings.Contains(string(data), server.URL) {
		t.Fatal("runtime leaked native credentials or local policy")
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if mode, err := r.Backend("alice", repository()); !errors.Is(err, ErrPolicy) || mode != "" {
		t.Fatal("changed policy fell back to standalone", mode, err)
	}
	if view, err := r.ObserveProject(t.Context(), "alice", repository(), 0, 1); !errors.Is(err, ErrPolicy) || view.Schema != "" {
		t.Fatal("changed policy released old data", err)
	}
}

func TestRuntimeDoesNotFallbackOnUnknownOrDriftingRepository(t *testing.T) {
	path, _, _ := runtimeFiles(t, "https://authority.example")
	r := OpenRuntime(path, nil)
	for _, change := range []string{"unknown", "root", "remote", "unverified"} {
		t.Run(change, func(t *testing.T) {
			repo := repository()
			switch change {
			case "unknown":
				repo = nil
			case "root":
				repo.ID = "changed-root"
			case "remote":
				repo.Repository = "example.test/changed"
			case "unverified":
				repo.Source = "display-name"
			}
			if mode, err := r.Backend("alice", repo); !errors.Is(err, ErrUnmapped) || mode != "" {
				t.Fatal("identity drift selected a competing backend", mode, err)
			}
		})
	}
	other := repository()
	other.ID = "other-root"
	other.Repository = "example.test/other/project"
	if mode, err := r.Backend("alice", other); err != nil || mode != "standalone" {
		t.Fatal("unconfigured verified project changed mode", mode, err)
	}
	if mode, err := r.Backend("unconfigured-owner", nil); err != nil || mode != "standalone" {
		t.Fatal("unconfigured account changed mode", mode, err)
	}
	missing := filepath.Join(t.TempDir(), "absent.json")
	disabled := OpenRuntime(missing, nil)
	if mode, err := disabled.Backend("alice", nil); err != nil || mode != "standalone" {
		t.Fatal("absent optional configuration changed existing mode", mode, err)
	}
	writeRuntimeJSON(t, missing, map[string]any{})
	if mode, err := disabled.Backend("alice", nil); !errors.Is(err, ErrPolicy) || mode != "" {
		t.Fatal("new policy silently activated without reconciliation", mode, err)
	}
}

func TestRuntimeRejectsWrongExpiredAndBroadCredentials(t *testing.T) {
	for _, test := range []string{"wrong client", "expired", "write scope", "wrong authority", "broad actual token", "unqualified loopback"} {
		t.Run(test, func(t *testing.T) {
			fixture := &projectFixture{}
			server := runtimeServer(t, fixture, test == "broad actual token")
			origin := server.URL
			if test == "unqualified loopback" {
				origin = "http://127.0.0.1:8789"
			}
			path, credential, manifest := runtimeFiles(t, origin)
			switch test {
			case "wrong client":
				manifest["client_id"] = "another-client"
			case "expired":
				manifest["credential"].(map[string]any)["expires_at"] = "2000-01-01T00:00:00Z"
			case "write scope":
				manifest["credential"].(map[string]any)["scopes"] = []string{"read", "write"}
			case "wrong authority":
				manifest["connection"].(map[string]any)["api_url"] = "https://different.example"
			}
			writeRuntimeJSON(t, credential, manifest)
			r := OpenRuntime(path, server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs)
			got, err := r.ObserveProject(t.Context(), "alice", repository(), 0, 1)
			if !errors.Is(err, ErrReadAccess) || got.Schema != "" || fixture.calls.Load() != 0 || strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), accessToken) {
				t.Fatal("unqualified credential reached project reads or exposed private data", err)
			}
		})
	}
}

func TestRuntimeDiscardsInFlightPolicyAndCredentialChanges(t *testing.T) {
	for _, target := range []string{"policy", "credential"} {
		t.Run(target, func(t *testing.T) {
			var path, credential string
			var once sync.Once
			fixture := &projectFixture{editTask: func(_ string, _ map[string]any) {
				once.Do(func() {
					name := path
					if target == "credential" {
						name = credential
					}
					data, err := os.ReadFile(name)
					if err != nil {
						t.Error(err)
						return
					}
					if err := os.WriteFile(name, append(data, '\n'), 0600); err != nil {
						t.Error(err)
					}
				})
			}}
			server := runtimeServer(t, fixture, false)
			path, credential, _ = runtimeFiles(t, server.URL)
			r := OpenRuntime(path, server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs)
			got, err := r.ObserveProject(t.Context(), "alice", repository(), 0, 1)
			want := ErrPolicy
			if target == "credential" {
				want = ErrReadAccess
			}
			if !errors.Is(err, want) || got.Schema != "" || got.Tasks != nil {
				t.Fatal("in-flight configuration change released old data", err)
			}
		})
	}
}

func TestRuntimeStrictPolicyAndReceivingScope(t *testing.T) {
	path, _, _ := runtimeFiles(t, "https://authority.example")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{bytesReplace(data, `"schema":`, `"Schema":`), append(append([]byte(nil), data...), []byte(` {}`)...), bytesReplace(data, `"client_id":`, `"Client_id":`)} {
		if _, _, err := parseRuntime(bad); err == nil {
			t.Fatal("ambiguous operator policy accepted")
		}
	}
	r := OpenRuntime(path, nil)
	if got, err := r.ObserveProject(t.Context(), "unconfigured-owner", repository(), 0, 1); !errors.Is(err, ErrUnmapped) || got.Schema != "" {
		t.Fatal("foreign owner reached credentials", err)
	}
	if got, err := r.ObserveProject(t.Context(), "alice", repository(), 0, 9); !errors.Is(err, ErrInvalid) || got.Schema != "" {
		t.Fatal("invalid page reached transport", err)
	}
}

func bytesReplace(data []byte, from, to string) []byte {
	return []byte(strings.Replace(string(data), from, to, 1))
}
