package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func websocketFixture(t *testing.T, mode string) (*Server, string, string) {
	t.Helper()
	t.Setenv("WEBMUX_EXEC_COMMAND", "")
	s, handler := fixture(t, mode)
	if mode == "local" {
		if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
			c.Auth.Users = []auth.User{{Username: "owner", PasswordHash: "unused-fixture-hash"}, {Username: "other", PasswordHash: "unused-fixture-hash"}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	owner := "owner"
	if mode == "none" {
		owner = "anonymous"
	}
	value, err := s.sessions.Create(owner, session.CreateRequest{Hostname: "localhost", Username: "fixture", Transport: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return s, "ws" + strings.TrimPrefix(server.URL, "http") + "/api/term/" + value.ID, value.ID
}
func dialSocket(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func socketEvent(t *testing.T, c *websocket.Conn, kind string) session.Event {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	var value session.Event
	if err := c.ReadJSON(&value); err != nil {
		t.Fatal(err)
	}
	if value["type"] != kind {
		t.Fatalf("event: %v, wanted %s", value, kind)
	}
	return value
}
func socketClose(t *testing.T, c *websocket.Conn, want int) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		_, _, err := c.ReadMessage()
		if err == nil {
			continue
		}
		var closed *websocket.CloseError
		if !errors.As(err, &closed) || closed.Code != want {
			t.Fatalf("close: %v, expected %d", err, want)
		}
		return
	}
}
func token(t *testing.T, s *Server, owner string) string {
	t.Helper()
	value, err := s.auth.Sign(owner)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTerminalSocketAuthenticationAndTicketReuse(t *testing.T) {
	s, url, _ := websocketFixture(t, "local")
	for _, query := range []string{"", "?token=invalid", "?token=" + token(t, s, "other")} {
		c := dialSocket(t, url+query)
		socketEvent(t, c, "error")
		socketClose(t, c, 1008)
	}
	ticket, err := s.auth.IssueTicket("owner")
	if err != nil {
		t.Fatal(err)
	}
	c := dialSocket(t, url+"?ticket="+ticket)
	socketEvent(t, c, "viewer_join")
	socketEvent(t, c, "status")
	reused := dialSocket(t, url+"?ticket="+ticket)
	socketEvent(t, reused, "error")
	socketClose(t, reused, 1008)
	deletedTicket, err := s.auth.IssueTicket("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error { c.Auth.Users = c.Auth.Users[:1]; return nil }); err != nil {
		t.Fatal(err)
	}
	deleted := dialSocket(t, url+"?ticket="+deletedTicket)
	if message := socketEvent(t, deleted, "error"); message["message"] != "Unauthorized" {
		t.Fatal("deleted ticket identity accepted")
	}
	socketClose(t, deleted, 1008)
	if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error { c.Auth.Users = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]string{"type": "focus"}); err != nil {
		t.Fatal(err)
	}
	socketClose(t, c, 1008)
}

func TestTerminalSocketFocusMalformedMessagesAndDelete(t *testing.T) {
	s, url, id := websocketFixture(t, "none")
	first := dialSocket(t, url)
	joined := socketEvent(t, first, "viewer_join")
	socketEvent(t, first, "status")
	second := dialSocket(t, url)
	socketEvent(t, first, "viewer_join")
	other := socketEvent(t, second, "viewer_join")
	socketEvent(t, second, "status")
	if other["focus_owner"] != joined["viewer_id"] {
		t.Fatal("join took existing focus")
	}
	for _, payload := range []string{"not json", `null`, `{"type":"input","data":{}}`, `{"type":"resize","cols":1.5,"rows":24}`} {
		if err := second.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := second.WriteJSON(map[string]string{"type": "focus"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*websocket.Conn{first, second} {
		if focused := socketEvent(t, c, "focus"); focused["focus_owner"] != other["viewer_id"] {
			t.Fatal("wrong focus owner")
		}
	}
	if err := s.sessions.Delete("anonymous", id); err != nil {
		t.Fatal(err)
	}
	socketClose(t, first, 1000)
	socketClose(t, second, 1000)
}

func TestTerminalSocketOriginSizeAndShutdown(t *testing.T) {
	s, url, _ := websocketFixture(t, "none")
	c, response, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"https://unrelated.invalid"}})
	if c != nil {
		c.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("cross-origin upgrade: %v %v", response, err)
	}
	response.Body.Close()
	// Reverse proxies (including Vite) preserve the browser-facing Host even
	// when the upstream network address and TLS termination are different.
	proxied, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"https://webmux.example:5173"}, "Host": []string{"webmux.example:5173"}})
	if err != nil {
		t.Fatal(err)
	}
	socketEvent(t, proxied, "viewer_join")
	socketEvent(t, proxied, "status")
	_ = proxied.Close()
	oversized := dialSocket(t, url)
	socketEvent(t, oversized, "viewer_join")
	socketEvent(t, oversized, "status")
	_ = oversized.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", (1<<20)+1)))
	socketClose(t, oversized, 1009)
	live := dialSocket(t, url)
	socketEvent(t, live, "viewer_join")
	socketEvent(t, live, "status")
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left socket handlers running")
	}
	_ = live.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := live.ReadMessage(); err == nil {
		t.Fatal("socket survived shutdown")
	}
}
