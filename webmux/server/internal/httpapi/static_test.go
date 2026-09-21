package httpapi

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStaticAssetsNavigationAndCache(t *testing.T) {
	s, _ := fixture(t, "local")
	s.webDir = t.TempDir()
	for name, text := range map[string]string{"index.html": "<!doctype html><title>WebMux</title>", "app.js": "console.log('fixture');", ".private": "private"} {
		if err := os.WriteFile(filepath.Join(s.webDir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(s.webDir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	for _, url := range []string{"/", "/index.html", "/workspace/terminals", "/assets/"} {
		response := request(handler, "GET", url, "", "")
		requireStatus(t, response, 200)
		if response.Body.String() != "<!doctype html><title>WebMux</title>" {
			t.Fatal(url, response.Body.String())
		}
	}
	response := request(handler, "GET", "/app.js", "", "")
	requireStatus(t, response, 200)
	if response.Header().Get("Content-Type") != "application/javascript; charset=UTF-8" || response.Header().Get("Cache-Control") != "public, max-age=0" {
		t.Fatal(response.Header())
	}
	for _, tc := range []struct {
		method, header, value string
		status                int
		body                  string
	}{
		{"HEAD", "", "", 200, ""},
		{"GET", "Range", "bytes=0-6", 206, "console"},
		{"GET", "If-None-Match", response.Header().Get("ETag"), 304, ""},
		{"GET", "If-Modified-Since", response.Header().Get("Last-Modified"), 304, ""},
	} {
		r := httptest.NewRequest(tc.method, "/app.js", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		requireStatus(t, w, tc.status)
		if w.Body.String() != tc.body {
			t.Fatal(w.Body.String())
		}
	}
	redirect := request(handler, "GET", "/assets?version=1", "", "")
	requireStatus(t, redirect, 301)
	if redirect.Header().Get("Location") != "/assets/?version=1" {
		t.Fatal(redirect.Header())
	}
	requireStatus(t, request(handler, "GET", "/api/sessions", "", ""), 401)
	for _, url := range []string{"/.private", "/%2eprivate", "/assets/%2e%2e/.private", "/assets%5c..%5c.private", "/app.js:stream"} {
		response := request(handler, "GET", url, "", "")
		if response.Code == 200 {
			t.Fatal("private path served", url)
		}
	}
}

func TestStaticFilesConfinedToBuild(t *testing.T) {
	s, _ := fixture(t, "none")
	s.webDir = t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("outside content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.webDir, "linked.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	requireStatus(t, request(s.Handler(), "GET", "/linked.txt", "", ""), 404)
	requireStatus(t, request(s.Handler(), "GET", "/", "", ""), 404)
	s.webDir = filepath.Join(s.webDir, "missing")
	requireStatus(t, request(s.Handler(), "GET", "/", "", ""), 404)
}

// Embedded files have zero modification times. Their cache identity must change
// across upgrades even when two versions contain the same number of bytes.
func TestEmbeddedAssetsCacheIdentityAndRanges(t *testing.T) {
	s, _ := fixture(t, "local")
	files := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>standalone</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("version-one")},
	}
	s.webFS = files
	handler := s.Handler()
	first := request(handler, "GET", "/assets/app.js", "", "")
	requireStatus(t, first, 200)
	etag := first.Header().Get("ETag")
	if etag == "" || first.Header().Get("Last-Modified") != "" {
		t.Fatal(first.Header())
	}
	for _, tc := range []struct {
		method, header, value string
		status                int
		body                  string
	}{
		{"HEAD", "", "", 200, ""},
		{"GET", "Range", "bytes=0-6", 206, "version"},
		{"GET", "If-None-Match", etag, 304, ""},
	} {
		r := httptest.NewRequest(tc.method, "/assets/app.js", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		requireStatus(t, w, tc.status)
		if w.Body.String() != tc.body {
			t.Fatal(w.Body.String())
		}
	}
	files["assets/app.js"].Data = []byte("version-two")
	r := httptest.NewRequest("GET", "/assets/app.js", nil)
	r.Header.Set("If-None-Match", etag)
	changed := httptest.NewRecorder()
	handler.ServeHTTP(changed, r)
	requireStatus(t, changed, 200)
	if changed.Header().Get("ETag") == etag || changed.Body.String() != "version-two" {
		t.Fatal(changed.Header(), changed.Body.String())
	}
	requireStatus(t, request(handler, "GET", "/workspace/terminals", "", ""), 200)
	requireStatus(t, request(handler, "GET", "/api/sessions", "", ""), 401)
}
