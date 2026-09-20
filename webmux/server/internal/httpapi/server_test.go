package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func fixture(t *testing.T, mode string) (*Server, http.Handler) {
	t.Helper()
	defaults := t.TempDir()
	if err := os.WriteFile(filepath.Join(defaults, "auth.yaml"), []byte("auth:\n  mode: "+mode+"\n  users: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.TempDir(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(store, Options{Name: "fixture", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	return s, s.Handler()
}

func request(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func requireStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("HTTP %d, want %d: %s", w.Code, want, w.Body.String())
	}
}

func responseToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	requireStatus(t, w, 200)
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Token == "" {
		t.Fatalf("missing token: %v", err)
	}
	return response.Token
}

func TestAccountLifecycleAndDeletedTokenRevocation(t *testing.T) {
	s, handler := fixture(t, "local")
	requireStatus(t, request(handler, "GET", "/api/auth/me", "", ""), 401)
	owner := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	requireStatus(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"other","password":"password"}`, ""), 403)
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"member","password":"member-password"}`, owner), 201)
	member := responseToken(t, request(handler, "POST", "/api/auth/login", `{"username":"member","password":"member-password"}`, ""))
	requireStatus(t, request(handler, "GET", "/api/auth/users", "", member), 403)
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"intruder","password":"password","admin":true}`, member), 403)
	requireStatus(t, request(handler, "DELETE", "/api/auth/users/owner", "", owner), 400)
	users := request(handler, "GET", "/api/auth/users", "", owner)
	requireStatus(t, users, 200)
	if strings.Contains(users.Body.String(), "password") || strings.Contains(users.Body.String(), "jwt_secret") {
		t.Fatal("credentials exposed")
	}
	requireStatus(t, request(handler, "DELETE", "/api/auth/users/member", "", owner), 204)
	requireStatus(t, request(handler, "GET", "/api/auth/me", "", member), 401)
	requireStatus(t, request(handler, "POST", "/api/auth/refresh", "", member), 401)
	requireStatus(t, request(handler, "POST", "/api/auth/ticket", "", member), 401)
	config, err := auth.LoadConfig(s.store)
	if err != nil || len(config.Auth.Users) != 1 {
		t.Fatalf("account update not persisted: %v", err)
	}
	if config.Auth.Users[0].PasswordHash == "owner-password" {
		t.Fatal("password persisted in plaintext")
	}
}

func TestBootstrapRaceHasOnlyOneWinner(t *testing.T) {
	s, handler := fixture(t, "local")
	var workers sync.WaitGroup
	codes := make(chan int, 4)
	for range 4 {
		workers.Go(func() {
			codes <- request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"password"}`, "").Code
		})
	}
	workers.Wait()
	close(codes)
	winners := 0
	for code := range codes {
		if code == 200 {
			winners++
		} else if code != 403 {
			t.Fatalf("unexpected bootstrap result: %d", code)
		}
	}
	if winners != 1 {
		t.Fatalf("bootstrap succeeded %d times", winners)
	}
	config, err := auth.LoadConfig(s.store)
	if err != nil || len(config.Auth.Users) != 1 || !config.Auth.Users[0].Admin {
		t.Fatalf("bad bootstrap state: %v", err)
	}
}

func TestTrustedModeAndCorruptConfigFailClosed(t *testing.T) {
	s, handler := fixture(t, "none")
	me := request(handler, "GET", "/api/auth/me", "", "")
	requireStatus(t, me, 200)
	if strings.TrimSpace(me.Body.String()) != `{"username":"anonymous","admin":false}` {
		t.Fatal(me.Body.String())
	}
	requireStatus(t, request(handler, "GET", "/api/auth/users", "", ""), 401)
	responseToken(t, request(handler, "POST", "/api/auth/refresh", "", ""))
	requireStatus(t, request(handler, "POST", "/api/auth/ticket", "", ""), 200)
	if err := os.WriteFile(s.store.ConfigPath("auth.yaml"), []byte("auth: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, request(handler, "GET", "/api/auth/me", "", ""), 500)
}

func TestMalformedAndOversizedRequests(t *testing.T) {
	_, handler := fixture(t, "local")
	for _, body := range []string{`{`, `{"username":7,"password":true}`, `{} {}`, `null`, `[]`} {
		requireStatus(t, request(handler, "POST", "/api/auth/login", body, ""), 400)
	}
	requireStatus(t, request(handler, "POST", "/api/auth/login", `{"username":"owner","password":"`+strings.Repeat("a", 1<<20)+`"}`, ""), 413)
}

func TestRateLimitIgnoresSpoofedProxyHeadersAndResets(t *testing.T) {
	limiter := newLimiter(2, time.Minute)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	handler := limiter.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := range 3 {
		req := httptest.NewRequest("GET", "/api/auth/status", nil)
		req.RemoteAddr = "[2001:db8:1234:ab00::1]:1234"
		if i == 2 {
			req.RemoteAddr = "[2001:db8:1234:abff::2]:5678"
		}
		req.Header.Set("X-Forwarded-For", "192.0.2.10")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		want := 204
		if i == 2 {
			want = 429
		}
		requireStatus(t, w, want)
	}
	now = now.Add(time.Minute)
	resetRequest := httptest.NewRequest("GET", "/api/auth/status", nil)
	resetRequest.RemoteAddr = "[2001:db8:1234:ab00::1]:1234"
	resetResponse := httptest.NewRecorder()
	handler.ServeHTTP(resetResponse, resetRequest)
	requireStatus(t, resetResponse, 204)
	for range 4 {
		requireStatus(t, request(handler, "GET", "/api/health", "", ""), 204)
	}
}
