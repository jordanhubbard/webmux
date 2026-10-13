//go:build !windows

package httpapi

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionManagerAuthorizationAndContract(t *testing.T) {
	s, handler := fixture(t, "local")
	token := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
	body := `{"version":1,"operation":"list","params":{}}`
	requireStatus(t, request(handler, "POST", "/api/session-manager", body, ""), 401)
	requireStatus(t, request(handler, "POST", "/api/session-manager", body, token), 403)
	s.secure = true
	dir, err := os.MkdirTemp("", "sm-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/session-manager" || r.Method != "POST" || r.Header.Get("Authorization") != "" || !strings.HasPrefix(r.Header.Get("X-Session-Manager-Principal"), "webmux-") {
			t.Error("unexpected provider request")
		}
		var value map[string]any
		if json.NewDecoder(r.Body).Decode(&value) != nil || value["operation"] != "list" {
			t.Error("invalid forwarded contract")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":1,"hostId":"fixture","generation":"epoch","result":{"sessions":[]}}`))
	})}
	go func() { _ = upstream.Serve(listener) }()
	defer upstream.Close()
	t.Setenv("SESSION_MANAGER_SOCKET", path)
	response := request(handler, "POST", "/api/session-manager", body, token)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"hostId":"fixture"`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("contract/cache mismatch")
	}
	req := httptest.NewRequest("POST", "/api/session-manager", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.invalid")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, req)
	requireStatus(t, denied, 403)
	if calls.Load() != 1 {
		t.Fatal("cross-origin request reached provider")
	}
	requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, token), 201)
	requireStatus(t, request(handler, "POST", "/api/session-manager", body, token), 403)
	if calls.Load() != 1 {
		t.Fatal("multi-user request reached provider")
	}
}

func TestSessionManagerRejectsUnsafeSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "sm-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_ = os.Chmod(path, 0600)
	if err := verifyManagerSocket(path); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0666)
	if verifyManagerSocket(path) == nil {
		t.Fatal("accepted world-accessible socket")
	}
	_ = os.Chmod(path, 0600)
	alias := filepath.Join(dir, "alias")
	_ = os.Symlink(path, alias)
	if verifyManagerSocket(alias) == nil {
		t.Fatal("accepted symlink socket")
	}
}

func TestSessionManagerRevocation(t *testing.T) {
	for _, phase := range []string{"body", "upstream"} {
		t.Run(phase, func(t *testing.T) {
			s, handler := fixture(t, "local")
			token := responseToken(t, request(handler, "POST", "/api/auth/bootstrap", `{"username":"owner","password":"owner-password"}`, ""))
			s.secure = true
			dir, err := os.MkdirTemp("", "sm-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			dir, err = filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "s")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			reached, cancelled := make(chan struct{}), make(chan struct{})
			upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(reached)
				<-r.Context().Done()
				close(cancelled)
			})}
			go func() { _ = upstream.Serve(listener) }()
			defer upstream.Close()
			t.Setenv("SESSION_MANAGER_SOCKET", path)
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			req := httptest.NewRequest("POST", "/api/session-manager", reader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			result, finished := httptest.NewRecorder(), make(chan struct{})
			go func() { handler.ServeHTTP(result, req); close(finished) }()
			// The first byte cannot be read until entry authorization has completed.
			if _, err := writer.Write([]byte("{")); err != nil {
				t.Fatal(err)
			}
			if phase == "upstream" {
				_, _ = writer.Write([]byte(`"version":1,"operation":"list","params":{}}`))
				_ = writer.Close()
				select {
				case <-reached:
				case <-time.After(3 * time.Second):
					t.Fatal("provider not reached")
				}
			}
			requireStatus(t, request(handler, "POST", "/api/auth/register", `{"username":"other","password":"other-password"}`, token), 201)
			if phase == "body" {
				_, _ = writer.Write([]byte(`"version":1,"operation":"list","params":{}}`))
				_ = writer.Close()
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("revoked request did not finish")
			}
			if phase == "body" {
				requireStatus(t, result, 403)
				select {
				case <-reached:
					t.Fatal("revoked request reached provider")
				default:
				}
			} else {
				select {
				case <-cancelled:
				case <-time.After(3 * time.Second):
					t.Fatal("upstream was not cancelled")
				}
				if result.Code == 200 {
					t.Fatal("revoked request succeeded")
				}
			}
		})
	}
}
