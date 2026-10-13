package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type moshProcess struct {
	*fakeProcess
	code int
}

func TestMoshFallbackDoesNotReplayInitialCommand(t *testing.T) {
	b, _, _ := fixture(t)
	mosh, ssh := fake(), fake()
	b.launch = func(r terminal.LaunchRequest, _ string) (process, error) {
		if r.Transport == "mosh" {
			return mosh, nil
		}
		return ssh, nil
	}
	s, err := b.Create("owner", CreateRequest{Hostname: "host.example", Username: "alice", Transport: "mosh", InitialCommand: "run-once"})
	if err != nil {
		t.Fatal(err)
	}
	mosh.output(t, "connecting\r\n")
	select {
	case value := <-mosh.input:
		if value != "run-once\r" {
			t.Fatalf("initial input: %q", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("initial command was not sent")
	}
	mosh.exit()
	eventually(t, func() bool { v, _ := b.Get("owner", s.ID); return v.Transport == "ssh" })
	ssh.output(t, "SSH_READY\r\n")
	select {
	case value := <-ssh.input:
		t.Fatalf("replayed command: %q", value)
	case <-time.After(time.Second):
	}
	if err := b.Input("owner", s.ID, "manual\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-ssh.input:
		if value != "manual\r" {
			t.Fatalf("SSH input: %q", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fallback terminal rejected input")
	}
}

func TestStaleMoshFailureCannotReplaceReconnectedSession(t *testing.T) {
	b, _, _ := fixture(t)
	first, second := fake(), fake()
	first.delayClose = true
	calls := 0
	b.launch = func(r terminal.LaunchRequest, _ string) (process, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	s, err := b.Create("owner", CreateRequest{Hostname: "host.example", Username: "alice", Transport: "mosh"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	first.output(t, "mosh did not make a successful connection")
	first.exit()
	second.output(t, "\x1b[?1049hREMOTE_READY")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return strings.Contains(text, "REMOTE_READY") })
	second.exit()
	eventually(t, func() bool { v, _ := b.Get("owner", s.ID); return v.State == "disconnected" })
	b.mu.Lock()
	defer b.mu.Unlock()
	if calls != 2 || b.entries[s.ID].value.Transport != "mosh" {
		t.Fatal("stale failure triggered SSH fallback")
	}
}

func (p *moshProcess) Wait() terminal.Exit { <-p.done; return terminal.Exit{Code: p.code} }

func TestMoshFallbackLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		output               []string
		code                 int
		disabled, launchFail bool
		sshFail, fallback    bool
	}{
		{name: "missing client", launchFail: true, fallback: true},
		{name: "server missing", output: []string{"mosh-server: command not found\r\n"}, code: 1, fallback: true},
		{name: "locale failure", output: []string{"mosh-client needs a UTF-8 native locale to run.\r\n"}, code: 1, fallback: true},
		{name: "UDP timeout", output: []string{"\x1b[?104", "9h\x1b[HConnecting...", "\x1b[?1049l\r\nmosh did not make a success", "ful connection to host:60001.\r\n"}, code: 1, fallback: true},
		{name: "disabled startup", launchFail: true, disabled: true},
		{name: "disabled timeout", output: []string{"mosh did not make a successful connection"}, code: 1, disabled: true},
		{name: "normal logout", output: []string{"\x1b[?1049hWelcome\r\n$ "}},
		{name: "established failure", output: []string{"\x1b[?1049hWelcome\r\n$ "}, code: 1},
		{name: "SSH cannot launch", launchFail: true, sshFail: true, fallback: true},
		{name: "SSH fails after timeout", output: []string{"mosh did not make a successful connection"}, code: 1, sshFail: true, fallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, _ := fixture(t)
			if tc.disabled {
				if err := store.WriteConfig("app.yaml", config.Document{App: map[string]any{"transport": map[string]any{"ssh_fallback": false}}}); err != nil {
					t.Fatal(err)
				}
			}
			calls := make(chan terminal.LaunchRequest, 10)
			mosh := &moshProcess{fakeProcess: fake(), code: tc.code}
			ssh := fake()
			t.Cleanup(func() { mosh.exit(); ssh.exit() })
			b.launch = func(r terminal.LaunchRequest, password string) (process, error) {
				calls <- r
				if r.Transport == "mosh" {
					if tc.launchFail {
						return nil, errors.New("mosh is not installed")
					}
					return mosh, nil
				}
				if tc.sshFail {
					return nil, errors.New("ssh is not installed")
				}
				return ssh, nil
			}
			s, err := b.Create("owner", CreateRequest{Hostname: "host.example", Username: "alice", Port: 2222, KeyID: "key", Transport: "mosh"})
			if err != nil {
				t.Fatal(err)
			}
			if r := <-calls; r.Transport != "mosh" {
				t.Fatalf("first transport: %s", r.Transport)
			}
			if !tc.launchFail {
				for _, data := range tc.output {
					mosh.output(t, data)
				}
				mosh.exit()
			}
			eventually(t, func() bool {
				v, _ := b.Get("owner", s.ID)
				if tc.fallback {
					return v.Transport == "ssh"
				}
				return v.State == "disconnected" || v.State == "error"
			})
			if !tc.fallback {
				select {
				case r := <-calls:
					t.Fatalf("unexpected fallback: %+v", r)
				default:
				}
				return
			}
			r := <-calls
			if r.Transport != "ssh" || r.Hostname != "host.example" || r.Username != "alice" || r.Port != 2222 || r.KeyID != "key" {
				t.Fatalf("fallback changed destination: %+v", r)
			}
			if !tc.sshFail {
				ssh.output(t, "SSH_READY\r\n")
				eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return strings.Contains(text, "SSH_READY") })
			}
			text, _ := b.Scrollback("owner", s.ID)
			if !strings.Contains(text, "falling back to SSH") || !strings.Contains(text, strings.Join(tc.output, "")) {
				t.Fatalf("lost diagnostics: %q", text)
			}
			var saved document
			if err := store.ReadSessions(&saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Sessions) != 1 || saved.Sessions[0].Transport != "ssh" {
				t.Fatalf("fallback not persisted: %+v", saved)
			}
			if !tc.sshFail {
				ssh.exit()
				eventually(t, func() bool { v, _ := b.Get("owner", s.ID); return v.State == "disconnected" })
			}
			select {
			case r := <-calls:
				t.Fatalf("fallback loop: %+v", r)
			default:
			}
		})
	}
}
