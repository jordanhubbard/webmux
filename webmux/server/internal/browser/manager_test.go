package browser

import (
	"reflect"
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
