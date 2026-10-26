package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLocalHostnameDialUsesInjectedResolver(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			acceptErr = connection.Close()
		}
		accepted <- acceptErr
	}()

	var resolved string
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		resolved = host
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	port := listener.Addr().(*net.TCPAddr).Port
	dial := dialResolvedHost(&net.Dialer{}, lookup)
	connection, err := dial(context.Background(), "tcp", net.JoinHostPort("fixture.local", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	if resolved != "fixture.local" {
		t.Fatalf("resolved %q instead of the original mDNS hostname", resolved)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
}

func TestOrdinaryHostnameDialBypassesMulticastResolver(t *testing.T) {
	called := false
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		called = true
		return nil, errors.New("unexpected lookup")
	}
	dial := dialResolvedHost(&net.Dialer{}, lookup)
	if _, err := dial(context.Background(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("closed TCP port unexpectedly accepted a connection")
	}
	if called {
		t.Fatal("ordinary address was sent to the multicast resolver")
	}
}

func TestClientIdentityAndServerURL(t *testing.T) {
	if server, err := normalizeServer("webmux.example:8443", false); err != nil || server != "https://webmux.example:8443" {
		t.Fatalf("host:port shortcut: %q %v", server, err)
	}
	if _, err := normalizeServer("http://remote.example:8080", false); err == nil {
		t.Fatal("cleartext server accepted without explicit opt-in")
	}
	for _, invalid := range []string{"https://example.test/path", "https://user:pass@example.test", "ftp://example.test"} {
		if _, err := normalizeServer(invalid, true); err == nil {
			t.Fatalf("invalid server accepted: %s", invalid)
		}
	}
	path := filepath.Join(t.TempDir(), "client.json")
	if err := saveIdentity(path, identity{Server: "https://example.test", ID: "client-id", Credential: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := saveIdentity(path, identity{}); err == nil {
		t.Fatal("existing identity overwritten")
	}
	loaded, err := loadIdentity(path)
	if err != nil || loaded.ID != "client-id" {
		t.Fatalf("identity not retained: %+v %v", loaded, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadIdentity(path); err == nil {
		t.Fatal("public credential file accepted")
	}
}

func TestOutboundClientRespondsToHostFactRequest(t *testing.T) {
	connected := make(chan struct{})
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/clients/connect" || r.Header.Get("Authorization") != "Bearer client.secret" {
			http.Error(w, "unauthorized", 401)
			return
		}
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		if err := connection.WriteJSON(frame{Type: "hello", Protocol: protocol}); err != nil {
			return
		}
		var initial frame
		if err := connection.ReadJSON(&initial); err != nil || initial.Type != "snapshot" || initial.Snapshot == nil || initial.Snapshot.OS == "" {
			return
		}
		close(connected)
		if err := connection.WriteJSON(frame{Type: "request", ID: "nonce", Command: "system.snapshot"}); err != nil {
			return
		}
		var reply frame
		if err := connection.ReadJSON(&reply); err == nil && reply.Type == "snapshot" && reply.ID == "nonce" && reply.Snapshot != nil && reply.Snapshot.CPUs > 0 {
			close(done)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- connect(ctx, server.URL, "client.secret") }()
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("client did not check in")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("client did not respond to refresh")
	}
	if err := <-result; err == nil || strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("client did not observe ordinary socket close: %v", err)
	}
}

func TestProbeRequiresMatchingProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer client.secret" {
			http.Error(w, "unauthorized", 401)
			return
		}
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.WriteJSON(frame{Type: "hello", Protocol: protocol + 1})
	}))
	defer server.Close()
	if err := probeConnection(server.URL, "bad"); !errors.Is(err, errRevoked) {
		t.Fatalf("invalid credential passed probe: %v", err)
	}
	if err := probeConnection(server.URL, "client.secret"); err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("unsupported protocol passed probe: %v", err)
	}
}
