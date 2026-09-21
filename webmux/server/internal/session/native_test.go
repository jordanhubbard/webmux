package session

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestSessionChild(t *testing.T) {
	if os.Getenv("WEBMUX_SESSION_CHILD") != "1" {
		return
	}
	fmt.Printf("session-ready:%d\n", os.Getpid())
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if scanner.Text() == "exit" {
			os.Exit(0)
		}
		fmt.Printf("session-reply:%s:λ😀\n", scanner.Text())
	}
	os.Exit(0)
}

func TestNativeSessionInitialCommandReconnectAndShutdown(t *testing.T) {
	t.Setenv("WEBMUX_SESSION_CHILD", "1")
	unused, store, _ := fixture(t)
	if err := unused.Close(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launcher := terminal.Launcher{Store: store}
	b, err := newBroker(store, slog.New(slog.NewTextHandler(io.Discard, nil)), func(request terminal.LaunchRequest, password string) (process, error) {
		request.ExecArgv = []string{executable, "-test.run=^TestSessionChild$"}
		return launcher.Launch(request, password)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	s, err := b.Create("owner", CreateRequest{Hostname: "localhost", Username: "fixture", Transport: "exec", InitialCommand: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.Contains(text, "session-reply:initial:λ😀")
	})
	if err := b.Resize("owner", s.ID, 110, 40); err != nil {
		t.Fatal(err)
	}
	if err := b.Input("owner", s.ID, "interactive\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		text, _ := b.Scrollback("owner", s.ID)
		return strings.Contains(text, "session-reply:interactive:λ😀")
	})
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return strings.Contains(text, "session-ready:") })
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(text, "session-reply:") {
		t.Fatalf("replayed stale output: %q", text)
	}
	if err := b.Input("owner", s.ID, "exit\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { value, _ := b.Get("owner", s.ID); return value.State == "disconnected" })
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create("owner", CreateRequest{Username: "fixture"}); err != ErrClosed {
		t.Fatalf("create after shutdown: %v", err)
	}
	if err := b.Input("owner", s.ID, "ignored"); err != ErrClosed {
		t.Fatalf("input after shutdown: %v", err)
	}
}
