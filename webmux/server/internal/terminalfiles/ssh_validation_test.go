package terminalfiles

import (
	"context"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestTransferRejectsInvalidStoredSSHAddress(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*session.Session)
	}{
		{"hostname-meta", func(s *session.Session) { s.Hostname = "host$variable" }},
		{"option-host", func(s *session.Session) { s.Hostname = "-oProxyCommand=bad" }},
		{"empty-host", func(s *session.Session) { s.Hostname = "" }},
		{"long-host", func(s *session.Session) { s.Hostname = strings.Repeat("a", 256) }},
		{"username-dollar", func(s *session.Session) { s.Username = "invalid$user" }},
		{"username-backslash", func(s *session.Session) { s.Username = `domain\user` }},
		{"long-user", func(s *session.Session) { s.Username = strings.Repeat("a", 65) }},
		{"negative-port", func(s *session.Session) { s.Port = -1 }},
		{"large-port", func(s *session.Session) { s.Port = 65536 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			s := session.Session{Transport: "ssh", Hostname: "owned.invalid", Username: "fixture", Port: 22}
			change.apply(&s)
			if cmd, err := Command(context.Background(), s, nil, "", "owned.png"); err == nil || cmd != nil {
				t.Fatalf("invalid stored address yielded transfer SSH command: %#v %v", cmd, err)
			}
		})
	}
}

func TestTransferDefaultPortAndEmptyUserRemainUsable(t *testing.T) {
	cmd, err := Command(context.Background(), session.Session{Transport: "ssh", Hostname: "[::1]"}, nil, "", "owned.png")
	if err != nil {
		t.Fatal("valid default-port transfer refused", err)
	}
	joined := strings.Join(cmd.Args, "\x00")
	if !strings.Contains(joined, "-p\x0022\x00--\x00[::1]\x00") || strings.Contains(joined, "\x00-l\x00") {
		t.Fatalf("default port/user changed: %q", cmd.Args)
	}
}
