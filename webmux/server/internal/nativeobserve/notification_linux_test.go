//go:build linux && (amd64 || arm64)

package nativeobserve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type notificationEvent struct {
	Stage string
	TID   uint32
	Errno uint32
}

// The pipes only exist in the test binary. This changes ordering, not the
// notification, filter, executable, arguments, signal handler or broker choice.
func notificationBarrier() func(uintptr, uint32, notification) {
	ready := os.NewFile(3, "notification-ready")
	commands := os.NewFile(4, "notification-commands")
	unix.CloseOnExec(3)
	unix.CloseOnExec(4)
	used := false
	return func(listener uintptr, tid uint32, n notification) {
		if used {
			return
		}
		valid := func() unix.Errno {
			for {
				id := n.ID
				_, _, errno := unix.Syscall(unix.SYS_IOCTL, listener, unix.SECCOMP_IOCTL_NOTIF_ID_VALID, uintptr(unsafe.Pointer(&id)))
				if errno != unix.EINTR {
					return errno
				}
			}
		}
		initial := valid()
		if initial == unix.ENOENT {
			// Let the actual broker handle an earlier, naturally withdrawn
			// request; pause only a live one for this controlled signal.
			return
		}
		used = true
		defer ready.Close()
		defer commands.Close()
		encoder := json.NewEncoder(ready)
		if err := encoder.Encode(notificationEvent{Stage: "received", TID: tid, Errno: uint32(initial)}); err != nil {
			panic(err)
		}
		scanner := bufio.NewScanner(commands)
		for scanner.Scan() {
			switch scanner.Text() {
			case "check":
				if err := encoder.Encode(notificationEvent{Stage: "checked", TID: tid, Errno: uint32(valid())}); err != nil {
					panic(err)
				}
			case "resume":
				return
			default:
				panic("unexpected notification test command")
			}
		}
		panic("notification test command pipe closed")
	}
}

func TestInterruptedInitialExecNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyRead.Close()
	defer readyWrite.Close()
	commandRead, commandWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer commandRead.Close()
	defer commandWrite.Close()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestObserverProcess$")
	cmd.Env = append(os.Environ(), "WEBMUX_TEST_OBSERVER_ROLE=guard", "WEBMUX_TEST_NOTIFICATION_BARRIER=1", "GOMAXPROCS=1")
	cmd.ExtraFiles = []*os.File{readyWrite, commandRead}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	readyWrite.Close()
	commandRead.Close()
	decoder := json.NewDecoder(readyRead)
	var event notificationEvent
	if err := decoder.Decode(&event); err != nil {
		t.Fatalf("notification barrier admission: %v", err)
	}
	if event.Stage != "received" || event.TID == 0 || event.Errno != 0 {
		t.Fatalf("initial notification was not live before interruption: %+v", event)
	}
	tid := event.TID
	// Target only this child's actual exec thread, with Go's installed signal
	// handler. The broker remains paused until the kernel proves ID withdrawal.
	if err := unix.Tgkill(cmd.Process.Pid, int(event.TID), unix.SIGURG); err != nil {
		t.Fatal(err)
	}
	withdrawn := false
	for i := 0; i < 100; i++ {
		if _, err := fmt.Fprintln(commandWrite, "check"); err != nil {
			t.Fatal(err)
		}
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("notification validity result: %v", err)
		}
		if event.Stage != "checked" || event.TID != tid {
			t.Fatalf("notification validity used a different stage/thread: %+v", event)
		}
		if event.Errno == uint32(unix.ENOENT) {
			withdrawn = true
			break
		}
		if event.Errno != 0 {
			t.Fatalf("unexpected notification validity: %+v", event)
		}
		time.Sleep(time.Millisecond)
	}
	if !withdrawn {
		t.Fatal("signal did not withdraw the live notification; no interrupted-exec acceptance")
	}
	if _, err := fmt.Fprintln(commandWrite, "resume"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("live notification withdrawn by signal, broker failed: %v; stderr=%s", err, stderr.String())
	}
	if strings.Count(stderr.String(), "native_observer_exec_notification_retried\n") != 1 || strings.Contains(stderr.String(), "native_observer_broker_failed") {
		t.Fatalf("retry diagnostics missing, repeated or fatal: %s", stderr.String())
	}
	var v observation
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		t.Fatalf("native probe result: %v %s", err, stdout.String())
	}
	if v.Threads != 4 || !v.SocketPair || !v.SocketPairSendRecv || !v.NoListener || !v.SamePID ||
		!v.MessageIODenied || !v.AddressedIODenied || !v.NetworkDenied || !v.ChildDenied || !v.SameDenied ||
		!v.ReplaceDenied || !v.SeccompDenied || !v.ReexecDenied || !v.ExecAtDenied ||
		v.Network || v.Child || v.SameExecutable || v.ReplaceFilter || v.Reexec {
		t.Fatalf("interrupted exec changed existing process/network/exec isolation: %+v", v)
	}
	t.Log("kernel withdrew the live notification; replacement exec retained all process/network/exec denials, listener cleanup and one bounded retry diagnostic")
}
