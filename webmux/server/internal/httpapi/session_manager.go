package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
)

// Session Manager v1 is a local service contract, independent of mobile clients
// and runtime providers. No runtime credentials are sent to the browser.
func (s *Server) sessionManagerAuthorized(r *http.Request, owner string) bool {
	c, err := auth.LoadConfig(s.store)
	if err != nil || !s.secure || c.Auth.Mode == "none" || len(c.Auth.Users) != 1 || !c.HasUser(owner) || !auth.IsAdmin(c.Auth.Users, owner) {
		return false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	user, err := s.auth.Verify(strings.TrimPrefix(header, "Bearer "))
	return err == nil && user == owner
}

func (s *Server) sessionManager(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.sessionManagerAuthorized(r, owner) {
		writeError(w, 403, "Session manager requires secure mode and one authenticated administrator")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			writeError(w, 403, "Cross-origin session-manager access is not allowed")
			return
		}
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		writeError(w, 403, "Cross-site session-manager access is not allowed")
		return
	}
	path := os.Getenv("SESSION_MANAGER_SOCKET")
	if path == "" || verifyManagerSocket(path) != nil {
		writeError(w, 503, "Configure SESSION_MANAGER_SOCKET to an available owner-only session-manager socket")
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, 415, "JSON content type required")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128*1024))
	if err != nil || !json.Valid(data) {
		writeError(w, 400, "Invalid or oversized session-manager request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		if err := verifyManagerSocket(path); err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://local/v1/session-manager", bytes.NewReader(data))
	if err != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Session-Manager-Principal", fmt.Sprintf("webmux-%x", sha256.Sum256([]byte(owner))))
	request.Header.Set("X-Session-Manager-Label", "WebMux")
	if !s.sessionManagerAuthorized(r, owner) {
		writeError(w, 403, "Session-manager access changed")
		return
	}
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !s.sessionManagerAuthorized(r, owner) {
					cancel()
					return
				}
			}
		}
	}()
	response, err := client.Do(request)
	if err != nil {
		writeError(w, 503, "Session manager unavailable. Refresh before repeating any change.")
		return
	}
	defer response.Body.Close()
	result, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(result) > 8*1024*1024 || !json.Valid(result) {
		writeError(w, 502, "Invalid session-manager response")
		return
	}
	if !s.sessionManagerAuthorized(r, owner) {
		writeError(w, 403, "Session-manager access changed")
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(result, &failure)
		if failure.Error.Message == "" {
			failure.Error.Message = "Session-manager operation failed"
		}
		writeError(w, response.StatusCode, failure.Error.Message)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(result)
}
