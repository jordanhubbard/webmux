//go:build windows

package recovery_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
	"golang.org/x/sys/windows"
)

// The child owns a real ConPTY, never the runner's/user's console.
func TestWindowsRecoveryWorkerChild(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_WINDOWS_WAIT") != "1" {
		return
	}
	var before uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &before); err != nil {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if file := os.Getenv("WEBMUX_TEST_WINDOWS_WAIT_CANCEL"); file != "" {
		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if _, err := os.Stat(file); err == nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	err := recovery.WaitForRecovery(ctx)
	if os.Getenv("WEBMUX_TEST_WINDOWS_WAIT_CANCEL") == "" && err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if os.Getenv("WEBMUX_TEST_WINDOWS_WAIT_CANCEL") != "" && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	var after uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &after); err != nil || before != after {
		fmt.Fprintf(os.Stderr, "mode not restored: %x -> %x, %v", before, after, err)
		os.Exit(5)
	}
	fmt.Fprint(os.Stdout, "\r\nWM_FOLLOWING_READY\r\n")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		os.Exit(6)
	}
	fmt.Fprintf(os.Stdout, "WM_FOLLOWING_READ=%s", line)
	os.Exit(0)
}

type waitingOutput struct {
	sync.Mutex
	text strings.Builder
}

func (o *waitingOutput) Write(b []byte) (int, error) {
	o.Lock()
	defer o.Unlock()
	return o.text.Write(b)
}
func (o *waitingOutput) String() string { o.Lock(); defer o.Unlock(); return o.text.String() }

func TestWindowsRecoveryWorkerConsoleHandoff(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled=%v", cancelled), func(t *testing.T) {
			const id = "978af6ab-33a5-4502-85ef-68dfb1a6c845"
			const key = "0123456789abcdef0123456789abcdef"
			const instance = "native-windows-worker"
			t.Setenv("WEBMUX_RECOVERY_CACHE_HOME", t.TempDir())
			t.Setenv("WEBMUX_RECOVERY_SESSION", id)
			t.Setenv("WEBMUX_RECOVERY_KEY", key)
			t.Setenv("WEBMUX_RECOVERY_INSTANCE", instance)
			t.Setenv("WEBMUX_TEST_WINDOWS_WAIT", "1")
			cancelFile := filepath.Join(t.TempDir(), "cancel")
			if cancelled {
				t.Setenv("WEBMUX_TEST_WINDOWS_WAIT_CANCEL", cancelFile)
			} else {
				t.Setenv("WEBMUX_TEST_WINDOWS_WAIT_CANCEL", "")
			}
			if err := recovery.InitializeHostState(id, key, true); err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			p, err := terminal.Start(terminal.Command{Path: self, Args: []string{"-test.run=^TestWindowsRecoveryWorkerChild$"}, Env: os.Environ(), Cols: 160, Rows: 24})
			if err != nil {
				t.Fatal(err)
			}
			var output waitingOutput
			done := make(chan struct{})
			go func() { _, _ = io.Copy(&output, p); close(done) }()
			t.Cleanup(func() {
				_ = p.Close()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Error("ConPTY output did not close")
				}
			})
			deadline := time.Now().Add(10 * time.Second)
			request := recovery.RPCRequest{Protocol: 1, SessionID: id, Key: key, Operation: "snapshot"}
			var snapshot recovery.RPCResponse
			for {
				snapshot = recovery.HostRPC(context.Background(), request)
				waiting := false
				for _, event := range snapshot.Events {
					if event.Kind == "control" && event.Mode == "waiting" {
						waiting = true
					}
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("no Windows waiting socket: %s, %s", snapshot.Error, output.String())
				}
				time.Sleep(10 * time.Millisecond)
			}
			if _, err := p.Write([]byte("UNAPPROVED_DRAFT\rpartial-draft")); err != nil {
				t.Fatal(err)
			}
			request.Operation = "action"
			request.Action = &recovery.RecoveryAction{ID: "5394ee76-3322-4382-a34c-83bf82497566", Instance: instance, Revision: snapshot.Revision, Kind: "shell"}
			if cancelled {
				if err := os.WriteFile(cancelFile, []byte("cancel"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				wrong := request
				wrong.Key = "ffffffffffffffffffffffffffffffff"
				if response := recovery.HostRPC(context.Background(), wrong); response.Error == "" {
					t.Fatal("unauthenticated worker request accepted")
				}
				response := recovery.HostRPC(context.Background(), request)
				if response.Error != "" || response.Outcome != "consumed" {
					t.Fatal(response)
				}
				response = recovery.HostRPC(context.Background(), request)
				if response.Error != "" || response.Outcome != "consumed" {
					t.Fatal("reconnect lost once-only receipt", response)
				}
			}
			await := func(marker string) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for !strings.Contains(output.String(), marker) {
					select {
					case <-done:
						t.Fatalf("worker ended before %s: %s", marker, output.String())
					default:
					}
					if time.Now().After(deadline) {
						t.Fatalf("missing %s: %s", marker, output.String())
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			await("WM_FOLLOWING_READY")
			if _, err := p.Write([]byte("approved-after-recovery\r")); err != nil {
				t.Fatal(err)
			}
			await("WM_FOLLOWING_READ=approved-after-recovery")
			if text := output.String(); strings.Contains(text, "UNAPPROVED_DRAFT") || strings.Contains(text, "partial-draft") {
				t.Fatal("waiting draft reached following reader", text)
			}
			select {
			case <-p.Done():
				if exit := p.Wait(); exit.Code != 0 || exit.Err != nil {
					t.Fatal(exit)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("worker child did not exit")
			}
		})
	}
}
