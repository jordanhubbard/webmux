package httpapi

import (
	"io"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/desktop"
	"github.com/jordanhubbard/webmux/server/internal/guacamole"
)

func rdpFixture(t *testing.T) (*Server, string, string, <-chan net.Conn) {
	t.Helper()
	s, handler := fixture(t, "none")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		if conn, err := listener.Accept(); err == nil {
			accepted <- conn
		}
	}()
	if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"guacd": map[string]any{"host": "127.0.0.1", "port": listener.Addr().(*net.TCPAddr).Port}}}); err != nil {
		t.Fatal(err)
	}
	username, domain := "user😀", "domain"
	value, err := s.rdp.Create("anonymous", desktop.CreateRequest{Hostname: "192.0.2.1", RDPPort: 3389, RDPUsername: username, RDPDomain: domain, RDPPassword: "password"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return s, "ws" + strings.TrimPrefix(server.URL, "http") + "/api/rdp/ws/" + value.ID, value.ID, accepted
}

func TestRdpHandshakeUnicodeBridgeAndDeletion(t *testing.T) {
	s, url, id, accepted := rdpFixture(t)
	dialer := websocket.Dialer{Subprotocols: []string{"guacamole"}}
	client, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.Subprotocol() != "guacamole" {
		t.Fatal("missing Guacamole subprotocol")
	}
	upstream := acceptedConn(t, accepted)
	_ = upstream.SetDeadline(time.Now().Add(5 * time.Second))
	d := guacamole.NewDecoder(upstream)
	selectInstruction, err := d.Read()
	if err != nil || selectInstruction.Raw != guacamole.Encode("select", "rdp") {
		t.Fatal(selectInstruction, err)
	}
	_, err = io.WriteString(upstream, guacamole.Encode("args", "hostname", "port", "username", "password", "domain"))
	if err != nil {
		t.Fatal(err)
	}
	connect, err := d.Read()
	if err != nil || connect.Opcode != "connect" || !reflect.DeepEqual(connect.Args, []string{"192.0.2.1", "3389", "user😀", "password", "domain"}) {
		t.Fatal("incorrect handshake credentials or destination", err)
	}
	ready := guacamole.Encode("ready", "connection;id")
	output := guacamole.Encode("name", "desktop😀;,é")
	for _, b := range []byte(ready + output) {
		if _, err := upstream.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	for _, want := range []string{ready, output} {
		kind, raw, err := client.ReadMessage()
		if err != nil || kind != websocket.TextMessage || string(raw) != want {
			t.Fatal(kind, string(raw), err)
		}
	}
	input := guacamole.Encode("key", "65293", "1")
	if err := client.WriteMessage(websocket.TextMessage, []byte(input)); err != nil {
		t.Fatal(err)
	}
	if got, err := d.Read(); err != nil || got.Raw != input {
		t.Fatal(got, err)
	}
	if err := s.rdp.Delete("anonymous", id); err != nil {
		t.Fatal(err)
	}
	socketClose(t, client, 1000)
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("upstream survived deletion")
	}
}

func TestRdpHandshakeErrorAndCancellation(t *testing.T) {
	for _, failure := range []string{"remote", "malformed", "deleted"} {
		t.Run(failure, func(t *testing.T) {
			s, url, id, accepted := rdpFixture(t)
			client := dialSocket(t, url)
			upstream := acceptedConn(t, accepted)
			_ = upstream.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err := guacamole.NewDecoder(upstream).Read(); err != nil {
				t.Fatal(err)
			}
			code := 1011
			switch failure {
			case "remote":
				_, _ = io.WriteString(upstream, guacamole.Encode("error", "private diagnostic", "256"))
			case "malformed":
				_, _ = io.WriteString(upstream, "invalid")
			case "deleted":
				if err := s.rdp.Delete("anonymous", id); err != nil {
					t.Fatal(err)
				}
				code = 1000
			}
			socketClose(t, client, code)
			if _, err := upstream.Read(make([]byte, 1)); err == nil {
				t.Fatal("upstream survived handshake failure")
			}
		})
	}
}
