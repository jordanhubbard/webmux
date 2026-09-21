package httpapi

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/desktop"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func vncFixture(t *testing.T, mode string) (*Server, string, string, <-chan net.Conn) {
	t.Helper()
	t.Setenv("WEBMUX_ALLOW_LOCAL_TARGETS", "1")
	s, handler := fixture(t, mode)
	if mode == "local" {
		if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
			c.Auth.Users = []auth.User{{Username: "owner", PasswordHash: "fixture"}, {Username: "other", PasswordHash: "fixture"}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 4)
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			select {
			case accepted <- c:
			default:
				_ = c.Close()
			}
		}
	}()
	owner := "anonymous"
	if mode == "local" {
		owner = "owner"
	}
	value, err := s.vnc.Create(owner, desktop.CreateRequest{Hostname: "127.0.0.1", VNCPort: listener.Addr().(*net.TCPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return s, "ws" + strings.TrimPrefix(server.URL, "http") + "/api/vnc/ws/" + value.ID, value.ID, accepted
}
func acceptedConn(t *testing.T, accepted <-chan net.Conn) net.Conn {
	t.Helper()
	select {
	case c := <-accepted:
		t.Cleanup(func() { _ = c.Close() })
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not connect")
		return nil
	}
}

func TestVncBinaryBridgeDeletionAndTicketReuse(t *testing.T) {
	s, url, id, accepted := vncFixture(t, "local")
	unauthorized := dialSocket(t, url)
	socketClose(t, unauthorized, 1008)
	foreign := dialSocket(t, url+"?token="+token(t, s, "other"))
	socketClose(t, foreign, 1008)
	ticket, err := s.auth.IssueTicket("owner")
	if err != nil {
		t.Fatal(err)
	}
	client := dialSocket(t, url+"?ticket="+ticket)
	upstream := acceptedConn(t, accepted)
	reused := dialSocket(t, url+"?ticket="+ticket)
	socketClose(t, reused, 1008)
	payload := []byte{0, 255, 1, 128, 13, 10}
	if _, err := upstream.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	kind, got, err := client.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage || !bytes.Equal(payload, got) {
		t.Fatal(kind, got, err)
	}
	if err := client.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = upstream.SetReadDeadline(time.Now().Add(5 * time.Second))
	got = make([]byte, len(payload))
	if _, err := io.ReadFull(upstream, got); err != nil || !bytes.Equal(payload, got) {
		t.Fatal(got, err)
	}
	if err := s.vnc.Delete("owner", id); err != nil {
		t.Fatal(err)
	}
	socketClose(t, client, 1000)
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("upstream survived deletion")
	}
}

func TestVncAccountRevocationAndShutdown(t *testing.T) {
	s, url, _, accepted := vncFixture(t, "local")
	client := dialSocket(t, url+"?token="+token(t, s, "owner"))
	upstream := acceptedConn(t, accepted)
	if err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error { c.Auth.Users = c.Auth.Users[1:]; return nil }); err != nil {
		t.Fatal(err)
	}
	_ = client.WriteMessage(websocket.BinaryMessage, []byte("revoked input"))
	socketClose(t, client, 1008)
	_ = upstream.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("revoked input reached upstream")
	}
}

func TestVncGuardOriginAndSizeLimit(t *testing.T) {
	s, url, _, accepted := vncFixture(t, "none")
	s.targets.AllowLocal = false
	denied := dialSocket(t, url)
	socketClose(t, denied, 1008)
	select {
	case <-accepted:
		t.Fatal("blocked target was dialed")
	default:
	}
	s.targets.AllowLocal = true
	client, response, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"https://unrelated.invalid"}})
	if client != nil {
		client.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatal(response, err)
	}
	response.Body.Close()
	large := dialSocket(t, url)
	acceptedConn(t, accepted)
	_ = large.WriteMessage(websocket.BinaryMessage, bytes.Repeat([]byte{1}, (1<<20)+1))
	socketClose(t, large, 1009)
	live := dialSocket(t, url)
	upstream := acceptedConn(t, accepted)
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown stalled")
	}
	_ = upstream.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("upstream survived shutdown")
	}
	_ = live.Close()
}
