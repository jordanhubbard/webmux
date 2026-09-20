package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/config"
)

func settingsFixture(t *testing.T, mode string) (*Server, http.Handler) {
	t.Helper()
	s, handler := fixture(t, mode)
	if err := os.WriteFile(s.store.ConfigPath("app.yaml"), []byte("app:\n  name: fixture\n  default_term:\n    cols: 80\n    rows: 24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, handler
}

func TestConcurrentSettingsUpdatesAndInvalidWritePreservation(t *testing.T) {
	s, handler := settingsFixture(t, "none")
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Go(func() {
			response := request(handler, "PUT", "/api/config", fmt.Sprintf(`{"app":{"transport":{"custom_%d":true}}}`, i), "")
			if response.Code != 200 {
				t.Errorf("settings update failed: %s", response.Body.String())
			}
		})
	}
	workers.Wait()
	var document config.Document
	if err := s.store.ReadConfig("app.yaml", &document); err != nil {
		t.Fatal(err)
	}
	if len(config.AsObject(document.App["transport"])) != 20 {
		t.Fatal("lost settings updates")
	}
	before, err := os.ReadFile(s.store.ConfigPath("app.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "PUT", "/api/config", `{"app":{"name":"must not save","font_faces":[{"family":"Bad","source":"../bad.ttf"}]}}`, ""), 400)
	after, err := os.ReadFile(s.store.ConfigPath("app.yaml"))
	if err != nil || string(before) != string(after) {
		t.Fatal("invalid update modified app.yaml")
	}
	// A missing layout can be recreated, as with the existing Node server.
	requireStatus(t, request(handler, "PUT", "/api/config/layout", `{"layout":{"font_size":16,"tiles":[]}}`, ""), 200)
	requireStatus(t, request(handler, "GET", "/api/config/layout", "", ""), 200)
}

func TestFontServingRequiresAuthAndHonorsLinkedConfigRoot(t *testing.T) {
	s, handler := settingsFixture(t, "local")
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"password"}`, ""))
	directory := t.TempDir()
	font := filepath.Join(directory, "fixture.woff2")
	if err := os.WriteFile(font, []byte("fixture-font-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(directory, "linked-app.yaml")
	if err := os.WriteFile(app, []byte("app:\n  font_faces:\n    - family: Fixture\n      source: fixture.woff2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.store.ConfigPath("app.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(app, s.store.ConfigPath("app.yaml")); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	requireStatus(t, request(handler, "GET", "/api/config/fonts/0", "", ""), 401)
	response := request(handler, "GET", "/api/config/fonts/0", "", owner)
	requireStatus(t, response, 200)
	if response.Body.String() != "fixture-font-bytes" || response.Header().Get("Content-Type") != "font/woff2" || response.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatal("font response differs")
	}
	partialRequest := httptest.NewRequest("GET", "/api/config/fonts/0", nil)
	partialRequest.Header.Set("Authorization", "Bearer "+owner)
	partialRequest.Header.Set("Range", "bytes=0-6")
	partial := httptest.NewRecorder()
	handler.ServeHTTP(partial, partialRequest)
	requireStatus(t, partial, 206)
	if partial.Body.String() != "fixture" {
		t.Fatal(partial.Body.String())
	}
	outside := filepath.Join(t.TempDir(), "outside.woff2")
	if err := os.WriteFile(outside, []byte("must-not-be-served"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(font); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, font); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "GET", "/api/config/fonts/0", "", owner), 404)
}
