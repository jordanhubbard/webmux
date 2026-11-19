package browser

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestRemoteWorkerStaysOnTerminalHost(t *testing.T) {
	for _, transport := range []string{"ssh", "mosh"} {
		cmd, err := command(session.Session{Transport: transport, Hostname: "target.example", Username: "alice", Port: 2222}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-p", "2222", "-l", "alice", "--", "target.example", "webmux --browser-worker"}
		if !reflect.DeepEqual(cmd.Args, want) {
			t.Fatalf("unsafe/wrong-host launch: %q", cmd.Args)
		}
	}
	if _, err := command(session.Session{Transport: "exec", Hostname: "localhost"}, nil); err == nil {
		t.Fatal("exec must not silently open a local browser")
	}
}

func TestNavigationSchemes(t *testing.T) {
	for _, raw := range []string{"https://login.example/authorize?state=abc", "http://127.0.0.1:1234/callback", "http://[::1]:1234"} {
		if !ValidURL(raw) {
			t.Errorf("rejected %q", raw)
		}
	}
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "data:text/html,test", "chrome://settings", "https://user:secret@example.com", "https:///missing-host"} {
		if ValidURL(raw) {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestCallbackWorkerUsesTrustedTransportWithoutSecrets(t *testing.T) {
	for _, transport := range []string{"ssh", "mosh"} {
		cmd, err := WorkerCommand(context.Background(), session.Session{Transport: transport, Hostname: "target.example", Username: "alice", Port: 2222}, nil, "--auth-callback")
		if err != nil {
			t.Fatal(err)
		}
		if cmd.Args[len(cmd.Args)-1] != "webmux --auth-callback" {
			t.Fatal("wrong worker")
		}
		if cmd.Args[len(cmd.Args)-2] != "target.example" {
			t.Fatal("wrong host")
		}
	}
	if _, err := WorkerCommand(context.Background(), session.Session{Transport: "local"}, nil, "--auth-callback; evil"); err == nil {
		t.Fatal("accepted shell text")
	}
}

func TestEndControlledRetainsAuthorizationAndReportsWorkerExitFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		in, out := io.Pipe()
		t.Cleanup(func() { in.Close(); out.Close() })
		c := &client{in: out, done: make(chan struct{}), stopped: make(chan struct{})}
		if failed {
			c.exitErr = errors.New("private-provider-url?credential=do-not-relay")
		}
		close(c.done)
		viewer := strings.Repeat("a", 32)
		control, err := c.control.control(viewer, "", true, false, false)
		if err != nil {
			t.Fatal(err)
		}
		m := &Manager{clients: map[string]*client{"fixture": c}}
		if err := m.EndControlled("fixture", strings.Repeat("b", 32), control.Token); !errors.Is(err, ErrControl) || m.clients["fixture"] != c {
			t.Fatal("controller denial did not preserve the browser")
		}
		err = m.EndControlled("fixture", viewer, control.Token)
		if (err != nil) != failed {
			t.Fatalf("worker exit status lost: failed=%t err=%v", failed, err)
		}
		if err != nil && (strings.Contains(err.Error(), "credential") || strings.Contains(err.Error(), "provider-url")) {
			t.Fatal("private worker error relayed")
		}
	}
}
