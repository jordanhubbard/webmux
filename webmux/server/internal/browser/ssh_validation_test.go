package browser

import (
	"context"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestRemoteWorkerRejectsInvalidStoredSSHAddress(t *testing.T) {
	for _, mode := range []string{"--browser-worker", "--auth-callback", "--browser-profile-remove"} {
		for _, change := range []struct {
			name  string
			apply func(*session.Session)
		}{
			{"option-host", func(s *session.Session) { s.Hostname = "-oProxyCommand=bad" }},
			{"hostname-meta", func(s *session.Session) { s.Hostname = "host$variable" }},
			{"empty-host", func(s *session.Session) { s.Hostname = "" }},
			{"long-host", func(s *session.Session) { s.Hostname = strings.Repeat("a", 256) }},
			{"username-dollar", func(s *session.Session) { s.Username = "invalid$user" }},
			{"username-backslash", func(s *session.Session) { s.Username = `domain\user` }},
			{"username-newline", func(s *session.Session) { s.Username = "user\nother" }},
			{"long-user", func(s *session.Session) { s.Username = strings.Repeat("a", 65) }},
			{"negative-port", func(s *session.Session) { s.Port = -1 }},
			{"large-port", func(s *session.Session) { s.Port = 65536 }},
		} {
			t.Run(mode+"/"+change.name, func(t *testing.T) {
				s := session.Session{Transport: "ssh", Hostname: "owned.invalid", Username: "fixture", Port: 22}
				change.apply(&s)
				if cmd, err := WorkerCommand(context.Background(), s, nil, mode); err == nil || cmd != nil {
					t.Fatalf("invalid stored address yielded SSH command: %#v %v", cmd, err)
				}
			})
		}
	}
}

func TestRemoteWorkerDefaultPortAndEmptyUserRemainUsable(t *testing.T) {
	for _, transport := range []string{"ssh", "mosh"} {
		for _, mode := range []string{"--browser-worker", "--auth-callback", "--browser-profile-remove"} {
			cmd, err := WorkerCommand(context.Background(), session.Session{Transport: transport, Hostname: "[::1]"}, nil, mode)
			if err != nil {
				t.Fatal("valid default-port helper refused", err)
			}
			joined := strings.Join(cmd.Args, "\x00")
			if !strings.Contains(joined, "-p\x0022\x00--\x00[::1]\x00webmux "+mode) || strings.Contains(joined, "\x00-l\x00") {
				t.Fatalf("default port/user changed: %q", cmd.Args)
			}
		}
	}
}
