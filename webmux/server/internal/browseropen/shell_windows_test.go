//go:build windows

package browseropen_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/browseropen"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestMain(m *testing.M) {
	if os.Getenv("WEBMUX_TEST_SHELL_HELPER") == "1" {
		self, _ := os.Executable()
		if strings.EqualFold(self, os.Getenv("WEBMUX_TEST_ORIGINAL_CLI")) {
			if len(os.Args) == 2 && os.Args[1] == "--version" {
				fmt.Println("codex-cli 0.153.0")
				os.Exit(0)
			}
			data, _ := json.Marshal(os.Args[1:])
			fmt.Printf("NATIVE_ARGS:%s;\n", data)
			os.Exit(0)
		}
		var err error
		switch {
		case recovery.CLIName(filepath.Base(self)) != "":
			err = recovery.RunCLI(filepath.Base(self), os.Args[1:])
		case len(os.Args) == 2 && os.Args[1] == "--browser-shell":
			err = browseropen.Shell()
		case len(os.Args) == 2 && os.Args[1] == "--recovery-wait":
			if os.Getenv("WEBMUX_TEST_WAIT_FAILURE") == "1" {
				os.Exit(10)
			}
			err = recovery.WaitForRecovery(context.Background())
		case len(os.Args) >= 2 && os.Args[1] == "--recovery-event":
			err = recovery.ShellEvent(os.Args[2:], os.Stdout)
		default:
			fmt.Fprintln(os.Stderr, "unexpected shell helper invocation")
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type shellCapture struct {
	sync.Mutex
	text string
}

func (c *shellCapture) Write(p []byte) (int, error) {
	c.Lock()
	defer c.Unlock()
	c.text += string(p)
	return len(p), nil
}
func (c *shellCapture) contains(s string) bool {
	c.Lock()
	defer c.Unlock()
	return strings.Contains(c.text, s)
}
func waitShell(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("shell fixture did not reach its expected state")
}

const fixtureSession = "19e0a099-1787-40fe-9016-00ccbd7e96e7"
const fixtureKey = "f1649178eed423eda95d303ac3ef8199"

func shellFixture(t *testing.T, waiting bool) (*terminal.Process, *shellCapture, func() recovery.RPCResponse) {
	t.Helper()
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	bin := filepath.Join(root, "original")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "codex.exe")
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(fake, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_RECOVERY_CACHE_HOME", cache)
	t.Setenv("WEBMUX_TEST_SHELL_HELPER", "1")
	t.Setenv("WEBMUX_TEST_ORIGINAL_CLI", fake)
	t.Setenv("PATH", bin)
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", bin)
	if err := os.WriteFile(filepath.Join(bin, "opencode.cmd"), []byte("@echo SCRIPT_SHIM_PRESERVED\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_RECOVERY_SESSION", fixtureSession)
	t.Setenv("WEBMUX_RECOVERY_KEY", fixtureKey)
	t.Setenv("WEBMUX_RECOVERY_WAIT", "0")
	if waiting {
		t.Setenv("WEBMUX_RECOVERY_WAIT", "1")
	}
	if err := recovery.InitializeHostEvidence(fixtureSession, fixtureKey, true, nil); err != nil {
		t.Fatal(err)
	}
	p, err := terminal.Start(terminal.Command{Path: self, Args: []string{"--browser-shell"}, Dir: root, Env: os.Environ(), Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	c := &shellCapture{}
	drained := make(chan struct{})
	go func() { defer close(drained); _, _ = io.Copy(c, p) }()
	t.Cleanup(func() {
		_ = p.Close()
		select {
		case <-drained:
		case <-time.After(5 * time.Second):
			t.Error("shell reader did not stop")
		}
	})
	snapshot := func() recovery.RPCResponse {
		return recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: fixtureSession, Key: fixtureKey, Operation: "snapshot"})
	}
	return p, c, snapshot
}

func TestWindowsShellCapturesNativeLaunch(t *testing.T) {
	p, c, snapshot := shellFixture(t, false)
	waitShell(t, func() bool {
		for _, e := range snapshot().Events {
			if e.Kind == "shell" {
				return true
			}
		}
		return false
	})
	id := "12345678-1234-4234-a234-123456789abc"
	if _, err := p.Write([]byte("codex resume " + id + " --model \"fixture model\"\r\n")); err != nil {
		t.Fatal(err)
	}
	waitShell(t, func() bool { return c.contains(`NATIVE_ARGS:["resume","` + id + `","--model","fixture model"];`) })
	waitShell(t, func() bool {
		for _, e := range snapshot().Events {
			if e.Kind == "cli" && e.SessionID == id && e.NativeEvidence == "launch-selector" && e.Version == "codex-cli 0.153.0" && e.Configuration != nil {
				return true
			}
		}
		return false
	})
	if _, err := p.Write([]byte("opencode\r\n")); err != nil {
		t.Fatal(err)
	}
	waitShell(t, func() bool { return c.contains("SCRIPT_SHIM_PRESERVED") })
	if _, err := p.Write([]byte("exit\r\n")); err != nil {
		t.Fatal(err)
	}
	done := make(chan terminal.Exit, 1)
	go func() { done <- p.Wait() }()
	select {
	case exit := <-done:
		if exit.Code != 0 || exit.Err != nil {
			t.Fatalf("shell exit: %+v", exit)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shell did not exit")
	}
}

func TestWindowsShellWaitsForDeliberateChoice(t *testing.T) {
	p, c, snapshot := shellFixture(t, true)
	var state recovery.RPCResponse
	var control recovery.Event
	waitShell(t, func() bool {
		state = snapshot()
		for _, e := range state.Events {
			if e.Kind == "control" && e.Mode == "waiting" {
				control = e
				return true
			}
		}
		return false
	})
	// Bytes typed while the worker owns input must not become a later cmd command.
	if _, err := p.Write([]byte("echo QUEUED_INPUT_EXECUTED\r\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	reply := recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: fixtureSession, Key: fixtureKey, Operation: "action", Action: &recovery.RecoveryAction{ID: "aab7cddf-e248-44d1-8513-f7e4ac1dbf24", Instance: control.Instance, Revision: state.Revision, Kind: "shell", Confirm: true}})
	if reply.Error != "" {
		t.Fatal(reply.Error)
	}
	waitShell(t, func() bool {
		for _, e := range snapshot().Events {
			if e.Kind == "shell" {
				return true
			}
		}
		return false
	})
	if _, err := p.Write([]byte("@echo SHELL_AFTER_CHOICE\r\n")); err != nil {
		t.Fatal(err)
	}
	waitShell(t, func() bool { return c.contains("SHELL_AFTER_CHOICE") })
	if c.contains("QUEUED_INPUT_EXECUTED") {
		t.Fatal("waiting input reached shell output")
	}
	_, _ = p.Write([]byte("exit\r\n"))
}

func TestWindowsShellRefusesFailedWaiter(t *testing.T) {
	t.Setenv("WEBMUX_TEST_WAIT_FAILURE", "1")
	p, c, snapshot := shellFixture(t, true)
	done := make(chan terminal.Exit, 1)
	go func() { done <- p.Wait() }()
	select {
	case exit := <-done:
		if exit.Code == 0 {
			t.Fatal("failed waiter started a shell")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("failed waiter did not exit")
	}
	waitShell(t, func() bool { return c.contains("Windows recovery worker stopped") })
	for _, event := range snapshot().Events {
		if event.Kind == "shell" {
			t.Fatal("failed waiter initialized a shell")
		}
	}
}
