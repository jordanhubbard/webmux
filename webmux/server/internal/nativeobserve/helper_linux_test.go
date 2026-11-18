//go:build linux && (amd64 || arm64)

package nativeobserve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

type observation struct {
	Threads            int
	AddressedIOControl bool
	MessageIODenied    bool
	MessageIOControl   bool
	SocketPairSendRecv bool
	AddressedIODenied  bool
	SocketPair         bool
	Network            bool
	NetworkDenied      bool
	Child              bool
	ChildDenied        bool
	SameExecutable     bool
	SameDenied         bool
	ReplaceFilter      bool
	ReplaceDenied      bool
	SeccompDenied      bool
	Reexec             bool
	ReexecDenied       bool
	ExecAtDenied       bool
	NoListener         bool
	SamePID            bool
}

func TestObserverProcess(t *testing.T) {
	role := os.Getenv("WEBMUX_TEST_OBSERVER_ROLE")
	if role == "" {
		return
	}
	if role == "guard" {
		body, e := os.ReadFile(os.Args[0])
		if e != nil {
			panic(e)
		}
		digest := sha256.Sum256(body)
		expected := hex.EncodeToString(digest[:])
		if os.Getenv("WEBMUX_TEST_BAD_DIGEST") == "1" {
			expected = strings.Repeat("0", 64)
		}
		os.Setenv("WEBMUX_TEST_OBSERVER_ROLE", "probe")
		os.Setenv("WEBMUX_TEST_INITIAL_PID", strconv.Itoa(os.Getpid()))
		err := Run([]string{hex.EncodeToString(digest[:]), os.Args[0], expected, "-test.run=^TestObserverProcess$"})
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if role == "witness" {
		os.Exit(0)
	}
	if role == "exec-witness" {
		var v observation
		if json.Unmarshal([]byte(os.Getenv("WEBMUX_TEST_OBSERVATION")), &v) != nil {
			os.Exit(2)
		}
		v.Reexec = true
		_ = json.NewEncoder(os.Stdout).Encode(v)
		os.Exit(0)
	}
	if role != "probe" {
		os.Exit(2)
	}
	var v observation
	var wg sync.WaitGroup
	ready := make(chan int, 4)
	release := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer wg.Done()
			ready <- unix.Gettid()
			<-release
		}()
	}
	tids := map[int]bool{}
	for i := 0; i < 4; i++ {
		tids[<-ready] = true
	}
	close(release)
	wg.Wait()
	v.Threads = len(tids)
	if pair, e := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0); e == nil {
		_, e = unix.Write(pair[0], []byte{'x'})
		var b [1]byte
		_, r := unix.Read(pair[1], b[:])
		v.SocketPair = e == nil && r == nil && b[0] == 'x'
		sent := unix.Sendto(pair[0], []byte{'s'}, unix.MSG_NOSIGNAL, nil)
		var n uintptr
		received := unix.EIO
		if sent == nil {
			n, _, received = unix.Syscall6(unix.SYS_RECVFROM, uintptr(pair[1]), uintptr(unsafe.Pointer(&b[0])), 1, 0, 0, 0)
		}
		_, _, empty := unix.Syscall6(unix.SYS_RECVFROM, uintptr(pair[1]), uintptr(unsafe.Pointer(&b[0])), 1, unix.MSG_DONTWAIT, 0, 0)
		runtime.KeepAlive(b)
		v.SocketPairSendRecv = sent == nil && received == 0 && n == 1 && b[0] == 's' && empty == unix.EAGAIN
		// No endpoint addresses, address-output pointers, noncanonical flag
		// words or out-of-contract flags may enter these two exceptions.
		v.AddressedIODenied = true
		v.AddressedIOControl = true
		for _, nr := range []uintptr{unix.SYS_SENDTO, unix.SYS_RECVFROM} {
			for _, args := range [][3]uintptr{{unix.MSG_OOB, 0, 0}, {1 << 32, 0, 0}, {0, 1, 0}, {0, 1 << 32, 0}, {0, 0, 1}, {0, 0, 1 << 32}} {
				_, _, errno := unix.Syscall6(nr, ^uintptr(0), 0, 0, args[0], args[1], args[2])
				v.AddressedIODenied = v.AddressedIODenied && errno == unix.EPERM
				v.AddressedIOControl = v.AddressedIOControl && errno == unix.EBADF
			}
		}
		v.MessageIODenied, v.MessageIOControl = true, true
		for _, nr := range []uintptr{unix.SYS_SENDMSG, unix.SYS_RECVMSG, unix.SYS_SENDMMSG, unix.SYS_RECVMMSG} {
			_, _, errno := unix.Syscall6(nr, ^uintptr(0), 0, 1, 0, 0, 0)
			v.MessageIODenied = v.MessageIODenied && errno == unix.EPERM
			v.MessageIOControl = v.MessageIOControl && errno == unix.EBADF
		}
		unix.Close(pair[0])
		unix.Close(pair[1])
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	v.Network = e == nil
	v.NetworkDenied = errors.Is(e, unix.EPERM)
	if l != nil {
		l.Close()
	}
	e = exec.Command("/bin/true").Run()
	v.Child = e == nil
	v.ChildDenied = errors.Is(e, unix.EPERM)
	child := exec.Command(os.Args[0], "-test.run=^TestObserverProcess$")
	child.Env = append(os.Environ(), "WEBMUX_TEST_OBSERVER_ROLE=witness")
	e = child.Run()
	v.SameExecutable = e == nil
	v.SameDenied = errors.Is(e, unix.EPERM)
	runtime.LockOSThread() // no-new-privs and seccomp installation are per-thread.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	allow := []unix.SockFilter{{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW}}
	program := unix.SockFprog{Len: 1, Filter: &allow[0]}
	_, _, errno := unix.Syscall6(unix.SYS_PRCTL, unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0, 0)
	v.ReplaceFilter = errno == 0
	v.ReplaceDenied = errno == unix.EPERM
	_, _, errno = unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, 0, uintptr(unsafe.Pointer(&program)))
	v.SeccompDenied = errno == unix.EPERM
	runtime.KeepAlive(allow)
	runtime.KeepAlive(program)
	_, _, errno = unix.Syscall6(unix.SYS_EXECVEAT, ^uintptr(0), 0, 0, 0, unix.AT_EMPTY_PATH, 0)
	v.ExecAtDenied = errno == unix.ENOSYS
	v.NoListener = true
	entries, _ := os.ReadDir("/proc/self/fd")
	for _, entry := range entries {
		target, _ := os.Readlink("/proc/self/fd/" + entry.Name())
		if strings.Contains(target, "seccomp") {
			v.NoListener = false
		}
	}
	v.SamePID = os.Getenv("WEBMUX_TEST_INITIAL_PID") == strconv.Itoa(os.Getpid())
	data, _ := json.Marshal(v)
	os.Setenv("WEBMUX_TEST_OBSERVATION", string(data))
	os.Setenv("WEBMUX_TEST_OBSERVER_ROLE", "exec-witness")
	e = unix.Exec(os.Args[0], []string{os.Args[0], "-test.run=^TestObserverProcess$"}, os.Environ())
	v.ReexecDenied = errors.Is(e, unix.ENOSYS)
	_ = json.NewEncoder(os.Stdout).Encode(v)
	os.Exit(0)
}
func TestNativeGuardEffects(t *testing.T) {
	for _, guarded := range []bool{false, true} {
		t.Run(strconv.FormatBool(guarded), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestObserverProcess$")
			role := "probe"
			if guarded {
				role = "guard"
			}
			cmd.Env = append(os.Environ(), "WEBMUX_TEST_OBSERVER_ROLE="+role, "GOMAXPROCS=1")
			out, e := cmd.CombinedOutput()
			if e != nil {
				t.Fatalf("probe: %v %s", e, out)
			}
			var v observation
			if e = json.Unmarshal(out, &v); e != nil {
				t.Fatalf("probe response: %v %s", e, out)
			}
			if v.Threads != 4 || !v.SocketPair || !v.SocketPairSendRecv || !v.NoListener {
				t.Fatalf("native runtime or descriptor cleanup: %+v", v)
			}
			if guarded {
				if !v.MessageIODenied || !v.AddressedIODenied || !v.NetworkDenied || !v.ChildDenied || !v.SameDenied || !v.ReplaceDenied || !v.SeccompDenied || !v.ReexecDenied || !v.ExecAtDenied || !v.SamePID || v.Network || v.Child || v.SameExecutable || v.ReplaceFilter || v.Reexec {
					t.Fatalf("guard escaped or changed PID: %+v", v)
				}
			} else if !v.MessageIOControl || !v.AddressedIOControl || !v.Network || !v.Child || !v.SameExecutable || !v.ReplaceFilter || !v.Reexec {
				t.Fatalf("positive controls unavailable: %+v", v)
			}
		})
	}
}
func TestChangedExecutableRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestObserverProcess$")
	cmd.Env = append(os.Environ(), "WEBMUX_TEST_OBSERVER_ROLE=guard", "WEBMUX_TEST_BAD_DIGEST=1")
	out, e := cmd.CombinedOutput()
	if e == nil || !strings.Contains(string(out), "executable changed") {
		t.Fatalf("bad digest was not refused: %v %s", e, out)
	}
}
