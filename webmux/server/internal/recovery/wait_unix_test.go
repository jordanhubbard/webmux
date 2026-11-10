//go:build !windows

package recovery

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestWaitingSocketRPCConsumesExactlyOneIntent(t *testing.T) {
	request := waitingFixture(t)
	dir, err := os.MkdirTemp("/tmp", "wm-rpc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	state, err := updateHostState(testHostSession, testHostKey, false, func(state *HostState) error { state.Control.Endpoint = endpoint; return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.Action.Revision = state.Revision
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := serveWaiting(ctx, listener, testHostSession, testHostKey, "current-worker", preflightAction)
		_ = listener.Close()
		finished <- err
	}()
	response := HostRPC(ctx, request)
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatal(response)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	// The original worker no longer exists. A reconnect still sees consumption
	// and cannot submit the request to a replacement actor or a shell prompt.
	response = HostRPC(ctx, request)
	if response.Error != "" || response.Outcome != "consumed" || len(hostSnapshot(t).Consumed) != 1 {
		t.Fatal(response)
	}
}

// This subprocess owns a real controlling terminal. Keep it separate from the
// test runner's terminal and from the user's tmux server.
func TestRecoveryWaitingSubprocess(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_RECOVERY_WAIT") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := WaitForRecovery(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, "\r\nWM_FOLLOWING_READY\r\n")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		os.Exit(3)
	}
	fmt.Fprintf(os.Stdout, "WM_FOLLOWING_READ=%s", line)
	os.Exit(0)
}

func TestRealWaitingTTYDiscardsDraftBeforeStartingFollowingReader(t *testing.T) {
	request := waitingFixture(t)
	t.Setenv("WEBMUX_TEST_RECOVERY_WAIT", "1")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestRecoveryWaitingSubprocess$")
	terminal, err := pty.Start(child)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer child.Process.Kill()
	output := make(chan string, 64)
	go func() {
		defer close(output)
		buffer := make([]byte, 4096)
		for {
			n, err := terminal.Read(buffer)
			if n > 0 {
				output <- string(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	deadline := time.Now().Add(8 * time.Second)
	for {
		state := hostSnapshot(t)
		if state.Control != nil && state.Control.Endpoint != "" && state.Control.Mode == "waiting" {
			request.Action.Revision = state.Revision
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real waiting worker did not advertise its socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := terminal.Write([]byte("UNAPPROVED_DRAFT\npartial-draft")); err != nil {
		t.Fatal(err)
	}
	response := HostRPC(context.Background(), request)
	if response.Error != "" || response.Outcome != "consumed" {
		t.Fatal(response)
	}
	var transcript strings.Builder
	await := func(marker string) {
		t.Helper()
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		for !strings.Contains(transcript.String(), marker) {
			select {
			case chunk, ok := <-output:
				if !ok {
					t.Fatalf("worker output ended before %s: %q", marker, transcript.String())
				}
				transcript.WriteString(chunk)
			case <-timer.C:
				t.Fatalf("timed out waiting for %s: %q", marker, transcript.String())
			}
		}
	}
	await("WM_FOLLOWING_READY")
	if _, err := terminal.Write([]byte("approved-after-recovery\n")); err != nil {
		t.Fatal(err)
	}
	await("WM_FOLLOWING_READ=approved-after-recovery")
	if strings.Contains(transcript.String(), "UNAPPROVED_DRAFT") || strings.Contains(transcript.String(), "partial-draft") {
		t.Fatal("waiting input escaped into the restored terminal", transcript.String())
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
}
