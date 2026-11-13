//go:build windows

package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/browseropen"
	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

type localLaunch struct {
	Request LaunchRequest
	Key     string
}

type localFrame struct {
	Kind       string
	Data       []byte
	Cols, Rows int
	Error      string
}

func TestMain(m *testing.M) {
	if os.Getenv("WEBMUX_TEST_LOCAL_PROVIDER_HELPER") != "1" {
		os.Exit(m.Run())
	}
	self, _ := os.Executable()
	var err error
	switch {
	case strings.EqualFold(self, os.Getenv("WEBMUX_TEST_LOCAL_CLI")):
		if len(os.Args) == 2 && os.Args[1] == "--version" {
			fmt.Println("codex-cli 0.153.0")
			os.Exit(0)
		}
		f, e := os.OpenFile(os.Getenv("WEBMUX_TEST_LOCAL_STARTS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			panic(e)
		}
		fmt.Fprintln(f, os.Getpid())
		f.Close()
		prior, e := term.MakeRaw(int(os.Stdin.Fd()))
		if e != nil {
			panic(e)
		}
		defer term.Restore(int(os.Stdin.Fd()), prior)
		fmt.Print("CODING_JOB_READY;")
		var b [256]byte
		pending := ""
		for {
			n, e := os.Stdin.Read(b[:])
			if e != nil {
				break
			}
			pending += string(b[:n])
			if strings.Contains(pending, "ALIVE") {
				fmt.Print("ALIVE_ACK;")
				pending = ""
			}
			if strings.Contains(pending, "SIZE") {
				cols, rows, _ := term.GetSize(int(os.Stdout.Fd()))
				fmt.Printf("SIZE:%dx%d;", cols, rows)
				pending = ""
			}
		}
	case len(os.Args) == 2 && (os.Args[1] == "--native-provider-bootstrap" || os.Args[1] == "--native-provider-owner"):
		err = RunNativeProviderHelper(context.Background(), os.Args[1], os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "--test-local-controller":
		err = localController()
	case recovery.CLIName(filepath.Base(self)) != "":
		err = recovery.RunCLI(filepath.Base(self), os.Args[1:])
	case len(os.Args) == 2 && os.Args[1] == "--browser-shell":
		err = browseropen.Shell()
	case len(os.Args) >= 2 && os.Args[1] == "--recovery-event":
		err = recovery.ShellEvent(os.Args[2:], os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "--recovery-wait":
		err = recovery.WaitForRecovery(context.Background())
	default:
		err = fmt.Errorf("unknown local fixture mode")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func localController() error {
	decoder := json.NewDecoder(os.Stdin)
	var launch localLaunch
	if err := decoder.Decode(&launch); err != nil {
		return err
	}
	request := launch.Request
	request.RecoveryKey = launch.Key
	p, err := (Launcher{}).Launch(request, "")
	if err != nil {
		return err
	}
	defer p.Close()
	var mu sync.Mutex
	emit := func(f localFrame) { mu.Lock(); defer mu.Unlock(); _ = json.NewEncoder(os.Stdout).Encode(f) }
	emit(localFrame{Kind: "ready"})
	go func() {
		var b [8192]byte
		for {
			n, err := p.Read(b[:])
			if n > 0 {
				emit(localFrame{Kind: "data", Data: b[:n]})
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		var f localFrame
		if err := decoder.Decode(&f); err != nil {
			return err
		}
		switch f.Kind {
		case "input":
			_, err = p.Write(f.Data)
		case "resize":
			err = p.Resize(f.Cols, f.Rows)
		case "close":
			err = p.Close()
			emit(localFrame{Kind: "closed"})
			return err
		default:
			return fmt.Errorf("invalid fixture request")
		}
		if err != nil {
			return err
		}
	}
}

type localControllerClient struct {
	cmd   *exec.Cmd
	input io.WriteCloser
	sync.Mutex
	output        string
	ready, closed bool
	failure       string
	done          chan error
}

func startLocalController(t *testing.T, request LaunchRequest) *localControllerClient {
	t.Helper()
	self, _ := os.Executable()
	cmd := exec.Command(self, "--test-local-controller")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic strings.Builder
	cmd.Stderr = &diagnostic
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &localControllerClient{cmd: cmd, input: input, done: make(chan error, 1)}
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var f localFrame
			if decoder.Decode(&f) != nil {
				break
			}
			c.Lock()
			if f.Kind == "ready" {
				c.ready = true
			}
			if f.Kind == "closed" {
				c.closed = true
			}
			c.output += string(f.Data)
			c.Unlock()
		}
		waitErr := cmd.Wait()
		c.Lock()
		c.failure = fmt.Sprintf("%v: %s", waitErr, diagnostic.String())
		c.Unlock()
		c.done <- waitErr
	}()
	t.Cleanup(func() {
		_ = input.Close()
		_ = cmd.Process.Kill()
	})
	if err = json.NewEncoder(input).Encode(localLaunch{Request: request, Key: request.RecoveryKey}); err != nil {
		t.Fatal(err)
	}
	waitLocal(t, func() bool { c.Lock(); defer c.Unlock(); return c.ready || c.failure != "" }, "controller ready")
	c.Lock()
	ready, failure := c.ready, c.failure
	c.Unlock()
	if !ready {
		t.Fatalf("controller failed: %s", failure)
	}
	return c
}
func (c *localControllerClient) send(t *testing.T, f localFrame) {
	t.Helper()
	if err := json.NewEncoder(c.input).Encode(f); err != nil {
		t.Fatal(err)
	}
}
func (c *localControllerClient) contains(s string) bool {
	c.Lock()
	defer c.Unlock()
	return strings.Contains(c.output, s)
}
func waitLocal(t *testing.T, condition func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

func localFixture(t *testing.T) (LaunchRequest, string, func() nativeprovider.Identity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "exit", "0")
	probe.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB}
	if err := probe.Run(); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Skip("outer runner job forbids independent providers; refusal is covered by nativeprovider tests")
		}
		t.Fatal(err)
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(bin, "codex.exe")
	if err = os.WriteFile(cli, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_TEST_LOCAL_PROVIDER_HELPER", "1")
	t.Setenv("WEBMUX_TEST_LOCAL_CLI", cli)
	counter := filepath.Join(root, "starts")
	t.Setenv("WEBMUX_TEST_LOCAL_STARTS", counter)
	t.Setenv("WEBMUX_RECOVERY_CACHE_HOME", cache)
	t.Setenv("USERPROFILE", root)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", os.Getenv("PATH"))
	token, _ := recovery.NewInstance()
	key, _ := recovery.NewInstance()
	id := token[:8] + "-" + token[8:12] + "-" + token[12:16] + "-" + token[16:20] + "-" + token[20:]
	request := LaunchRequest{SessionID: id, RecoveryKey: key, Transport: "local", Hostname: "localhost", Cols: 100, Rows: 30}
	identity := func() nativeprovider.Identity {
		data, err := os.ReadFile(filepath.Join(cache, "webmux", "native-providers", id, "identity.json"))
		if err != nil {
			t.Fatal(err)
		}
		var record struct{ Identity nativeprovider.Identity }
		if json.Unmarshal(data, &record) != nil {
			t.Fatal("invalid owner record")
		}
		return record.Identity
	}
	t.Cleanup(func() {
		path := filepath.Join(cache, "webmux", "native-providers", id, "identity.json")
		if _, err := os.Stat(path); err != nil {
			return
		}
		i := identity()
		handle, handleErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(i.OwnerPID))
		if handleErr == nil {
			defer windows.CloseHandle(handle)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := nativeprovider.Probe(ctx, i, key); err == nil {
			if err = nativeprovider.Stop(ctx, i, key); err != nil {
				t.Error(err)
			}
		}
		if handleErr == nil {
			if status, err := windows.WaitForSingleObject(handle, 10000); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Errorf("owned provider did not exit after explicit stop: %d %v", status, err)
			}
		}
	})
	return request, counter, identity
}

func TestWindowsLocalLauncherSurvivesControllerLoss(t *testing.T) {
	request, counter, identity := localFixture(t)
	first := startLocalController(t, request)
	waitLocal(t, func() bool { return first.contains("webmux-recovery;") }, "provider metadata")
	recording := true
	reply := recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: request.SessionID, Key: request.RecoveryKey, Operation: "policy", Recording: &recording})
	if reply.Error != "" {
		t.Fatal(reply.Error)
	}
	first.send(t, localFrame{Kind: "input", Data: []byte("codex resume 12345678-1234-4234-a234-123456789abc\r\n")})
	waitLocal(t, func() bool { return first.contains("CODING_JOB_READY;") }, "coding fixture")
	before := identity()
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.done:
	case <-time.After(5 * time.Second):
		t.Fatal("controller did not exit")
	}
	second := startLocalController(t, request)
	after := identity()
	if before.Instance != after.Instance || before.OwnerPID != after.OwnerPID || before.RootPID != after.RootPID {
		t.Fatal("controller replacement created another provider")
	}
	waitLocal(t, func() bool { return second.contains("CODING_JOB_READY;") }, "restored display")
	second.send(t, localFrame{Kind: "resize", Cols: 90, Rows: 28})
	second.send(t, localFrame{Kind: "input", Data: []byte("SIZE")})
	waitLocal(t, func() bool { return second.contains("SIZE:90x28;") }, "resized native job")
	second.send(t, localFrame{Kind: "input", Data: []byte("ALIVE")})
	waitLocal(t, func() bool { return second.contains("ALIVE_ACK;") }, "surviving job input")
	data, err := os.ReadFile(counter)
	if err != nil || len(strings.Fields(string(data))) != 1 {
		t.Fatal("native job was duplicated")
	}
	second.send(t, localFrame{Kind: "close"})
	select {
	case err := <-second.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("viewer did not detach")
	}
	if _, err := nativeprovider.Probe(context.Background(), after, request.RecoveryKey); err != nil {
		t.Fatal("viewer detach stopped the provider")
	}
}

func TestWindowsLocalProviderLossWaitsWithoutDuplicatingWork(t *testing.T) {
	request, counter, identity := localFixture(t)
	first := startLocalController(t, request)
	recording := true
	reply := recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: request.SessionID, Key: request.RecoveryKey, Operation: "policy", Recording: &recording})
	if reply.Error != "" {
		t.Fatal(reply.Error)
	}
	first.send(t, localFrame{Kind: "input", Data: []byte("codex resume 12345678-1234-4234-a234-123456789abc\r\n")})
	waitLocal(t, func() bool { return first.contains("CODING_JOB_READY;") }, "coding fixture")
	before := identity()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(before.OwnerPID))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err = nativeprovider.Stop(context.Background(), before, request.RecoveryKey); err != nil {
		t.Fatal(err)
	}
	if status, err := windows.WaitForSingleObject(handle, 10000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("stopped provider did not exit")
	}
	first.send(t, localFrame{Kind: "close"})
	select {
	case <-first.done:
	case <-time.After(5 * time.Second):
		t.Fatal("old viewer did not close")
	}
	second := startLocalController(t, request)
	after := identity()
	if before.Instance == after.Instance {
		t.Fatal("provider loss reused a dead identity")
	}
	snapshot := func() recovery.RPCResponse {
		return recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: request.SessionID, Key: request.RecoveryKey, Operation: "snapshot"})
	}
	var state recovery.RPCResponse
	var control recovery.Event
	waitLocal(t, func() bool {
		state = snapshot()
		for _, event := range state.Events {
			if event.Kind == "control" && event.Mode == "waiting" {
				control = event
				return true
			}
		}
		return false
	}, "new waiting worker")
	sawSelector := false
	for _, event := range state.Events {
		if event.Kind == "cli" && event.SessionID == "12345678-1234-4234-a234-123456789abc" && event.NativeEvidence == "launch-selector" {
			sawSelector = true
		}
	}
	if !sawSelector {
		t.Fatal("provider loss discarded the explicit native selector")
	}
	action := &recovery.RecoveryAction{ID: "d5d955b2-2826-4a64-b5b8-2acdfbfe1cb9", Instance: control.Instance, Revision: state.Revision, Kind: "replay", Confirm: true}
	rpc := recovery.RPCRequest{Protocol: 1, SessionID: request.SessionID, Key: request.RecoveryKey, Operation: "action", Action: action}
	reply = recovery.HostRPC(context.Background(), rpc)
	if !strings.Contains(reply.Error, "command replay is not supported") {
		t.Fatalf("Windows replay refusal: %s", reply.Error)
	}
	data, err := os.ReadFile(counter)
	if err != nil || len(strings.Fields(string(data))) != 1 {
		t.Fatal("provider loss automatically duplicated the coding job")
	}
	// A rejected action is not consumed; the same deliberate request can choose
	// the supported shell fallback without ever evaluating the old command.
	action.Kind = "shell"
	reply = recovery.HostRPC(context.Background(), rpc)
	if reply.Error != "" || reply.Outcome != "consumed" {
		t.Fatalf("shell fallback: %+v", reply)
	}
	waitLocal(t, func() bool {
		for _, event := range snapshot().Events {
			if event.Kind == "control" && event.Mode == "active" && event.Ready {
				return true
			}
		}
		return false
	}, "shell fallback ready")
	second.send(t, localFrame{Kind: "input", Data: []byte("set WM_FIXTURE=RECOVERY\r\necho %WM_FIXTURE%_SHELL_READY\r\n")})
	waitLocal(t, func() bool { return second.contains("RECOVERY_SHELL_READY") }, "fallback shell input")
	reply = recovery.HostRPC(context.Background(), rpc)
	if reply.Error != "" || reply.Outcome != "consumed" {
		t.Fatal("duplicate recovery request was not recognized")
	}
	data, err = os.ReadFile(counter)
	if err != nil || len(strings.Fields(string(data))) != 1 {
		t.Fatal("fallback repeated the coding job")
	}
}

func TestWindowsLocalRemoteEnsureDoesNotCreateUnattachableProvider(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("WEBMUX_RECOVERY_CACHE_HOME", cache)
	key, err := recovery.NewInstance()
	if err != nil {
		t.Fatal(err)
	}
	id := "21fb5fd0-2b83-4d5f-bf1a-b0b63f2c240e"
	recording := false
	reply := recovery.HostRPC(context.Background(), recovery.RPCRequest{Protocol: 1, SessionID: id, Key: key, Operation: "ensure", Recording: &recording})
	if !strings.Contains(reply.Error, "remote native attachment remains unsupported") {
		t.Fatalf("unqualified remote path was admitted: %s", reply.Error)
	}
	if _, err := os.Stat(filepath.Join(cache, "webmux", "native-providers", id)); !os.IsNotExist(err) {
		t.Fatal("remote ensure allocated an unattachable provider")
	}
}
