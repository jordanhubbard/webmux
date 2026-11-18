//go:build linux && (amd64 || arm64)

package nativeobserve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

type data struct {
	Nr   int32
	Arch uint32
	IP   uint64
	Args [6]uint64
}
type notification struct {
	ID    uint64
	PID   uint32
	Flags uint32
	Data  data
}
type response struct {
	ID    uint64
	Value int64
	Error int32
	Flags uint32
}

func stmt(c uint16, k uint32) unix.SockFilter { return unix.SockFilter{Code: c, K: k} }
func jump(c uint16, k uint32, t, f uint8) unix.SockFilter {
	return unix.SockFilter{Code: c, K: k, Jt: t, Jf: f}
}

// Run verifies an ELF executable before installing the filter on the locked
// exec thread. Call it only in a dedicated helper process: installed restrictions
// are irreversible. The native program replaces this helper in the same PID.
func Run(args []string) error {
	if len(args) < 3 || !filepath.IsAbs(args[1]) {
		return fmt.Errorf("invalid native observer invocation")
	}
	selfExpected, e := hex.DecodeString(args[0])
	if e != nil || len(selfExpected) != 32 {
		return fmt.Errorf("invalid native observer helper digest")
	}
	self, e := os.Open("/proc/self/exe")
	if e != nil {
		return fmt.Errorf("native observer helper unavailable")
	}
	selfDigest := sha256.New()
	selfBytes, copyErr := io.Copy(selfDigest, io.LimitReader(self, 512*1024*1024+1))
	self.Close()
	if copyErr != nil || selfBytes > 512*1024*1024 || hex.EncodeToString(selfDigest.Sum(nil)) != args[0] {
		return fmt.Errorf("native observer helper changed")
	}
	args = args[1:]
	expected, e := hex.DecodeString(args[1])
	if e != nil || len(expected) != 32 {
		return fmt.Errorf("invalid native observer digest")
	}
	fd, e := unix.Open(args[0], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return fmt.Errorf("native observer executable unavailable")
	}
	target := os.NewFile(uintptr(fd), "native-observer-executable")
	defer target.Close()
	stat, e := target.Stat()
	if e != nil || !stat.Mode().IsRegular() || stat.Size() < 64 || stat.Size() > 512*1024*1024 {
		return fmt.Errorf("invalid native observer executable")
	}
	canonical, e := filepath.EvalSymlinks(args[0])
	if e != nil || canonical != args[0] {
		return fmt.Errorf("invalid native observer executable")
	}
	digest := sha256.New()
	if _, e = io.Copy(digest, target); e != nil || hex.EncodeToString(digest.Sum(nil)) != args[1] {
		return fmt.Errorf("native observer executable changed")
	}
	if _, e = target.Seek(0, 0); e != nil {
		return e
	}
	header := make([]byte, 20)
	if _, e = io.ReadFull(target, header); e != nil {
		return e
	}
	machine := byte(0xb7)
	if runtime.GOARCH == "amd64" {
		machine = 0x3e
	}
	if string(header[:4]) != "\x7fELF" || header[4] != 2 || header[5] != 1 || header[18] != machine || header[19] != 0 {
		return fmt.Errorf("unqualified native observer executable ABI")
	}
	file, e := unix.BytePtrFromString("")
	if e != nil {
		return e
	}
	argv, e := syscall.SlicePtrFromStrings(append([]string{args[0]}, args[2:]...))
	if e != nil {
		return e
	}
	env, e := syscall.SlicePtrFromStrings(os.Environ())
	if e != nil {
		return e
	}
	runtime.LockOSThread()
	var arch uint32
	switch runtime.GOARCH {
	case "arm64":
		arch = unix.AUDIT_ARCH_AARCH64
	case "amd64":
		arch = unix.AUDIT_ARCH_X86_64
	default:
		return fmt.Errorf("unqualified architecture")
	}
	deny := uint32(unix.SECCOMP_RET_ERRNO) | uint32(unix.EPERM)
	filters := []unix.SockFilter{stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 4), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, arch, 1, 0), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_PROCESS), stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0), jump(unix.BPF_JMP|unix.BPF_JGE|unix.BPF_K, 0x40000000, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, deny)}
	blocked := []uint32{unix.SYS_SOCKET, unix.SYS_CONNECT, unix.SYS_BIND, unix.SYS_LISTEN, unix.SYS_ACCEPT, unix.SYS_ACCEPT4, unix.SYS_SENDTO, unix.SYS_SENDMSG, unix.SYS_SENDMMSG, unix.SYS_RECVFROM, unix.SYS_RECVMSG, unix.SYS_RECVMMSG, unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER, unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV, unix.SYS_PIDFD_GETFD, unix.SYS_SECCOMP}
	if runtime.GOARCH == "amd64" {
		blocked = append(blocked, 57, 58)
	}
	for _, nr := range blocked {
		filters = append(filters, jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, nr, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, deny))
	}
	// Only unnamed Unix pairs for native signal delivery; no external endpoints.
	filters = append(filters, jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_SOCKETPAIR, 0, 4), stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.AF_UNIX, 1, 0), stmt(unix.BPF_RET|unix.BPF_K, deny), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
	// Deny replacement of the closed exec notification listener.
	filters = append(filters, jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_PRCTL, 0, 4), stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.PR_SET_SECCOMP, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, deny), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
	filters = append(filters, jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_CLONE3, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS)), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_EXECVE, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_USER_NOTIF), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_EXECVEAT, 0, 1), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_USER_NOTIF))
	mask := uint32(unix.CLONE_THREAD | unix.CLONE_VM | unix.CLONE_SIGHAND)
	filters = append(filters, jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_CLONE, 0, 4), stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16), stmt(unix.BPF_ALU|unix.BPF_AND|unix.BPF_K, mask), jump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, mask, 1, 0), stmt(unix.BPF_RET|unix.BPF_K, deny), stmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
	var sizes struct{ Notification, Response, Data uint16 }
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_GET_NOTIF_SIZES, 0, uintptr(unsafe.Pointer(&sizes)))
	if errno != 0 || int(sizes.Notification) != int(unsafe.Sizeof(notification{})) || int(sizes.Response) != int(unsafe.Sizeof(response{})) || int(sizes.Data) != int(unsafe.Sizeof(data{})) {
		return fmt.Errorf("unqualified notification ABI: %v", errno)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	listener, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_NEW_LISTENER, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filters)
	runtime.KeepAlive(program)
	if errno != 0 {
		return errno
	}
	defer unix.Close(int(listener))
	flags, err := unix.FcntlInt(listener, unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		return fmt.Errorf("listener is not close-on-exec")
	}
	tid := uint32(unix.Gettid())
	go func() {
		fatal := func() { _ = unix.Kill(os.Getpid(), unix.SIGKILL) }
		var n notification
		for {
			n = notification{} // The kernel requires a zeroed buffer on every receive.
			_, _, e := unix.Syscall(unix.SYS_IOCTL, listener, unix.SECCOMP_IOCTL_NOTIF_RECV, uintptr(unsafe.Pointer(&n)))
			if e == unix.EINTR {
				continue
			}
			if e != 0 {
				fatal()
				return
			}
			break
		}
		if n.PID != tid || n.Data.Arch != arch || n.Data.Nr != unix.SYS_EXECVEAT || n.Data.Args[0] != uint64(fd) || n.Data.Args[4] != unix.AT_EMPTY_PATH || n.Flags != 0 {
			fatal()
			return
		}
		r := response{ID: n.ID, Flags: unix.SECCOMP_USER_NOTIF_FLAG_CONTINUE}
		_, _, e := unix.Syscall(unix.SYS_IOCTL, listener, unix.SECCOMP_IOCTL_NOTIF_SEND, uintptr(unsafe.Pointer(&r)))
		if e != 0 {
			fatal()
		}
	}()
	// Enter a schedulable syscall so the broker runs even with GOMAXPROCS=1.
	// Successful exec kills helper threads and closes the only listener; later
	// exec requests then fail ENOSYS in the kernel, without a resident broker.
	_, _, errno = unix.Syscall6(unix.SYS_EXECVEAT, uintptr(fd), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(&argv[0])), uintptr(unsafe.Pointer(&env[0])), unix.AT_EMPTY_PATH, 0)
	runtime.KeepAlive(file)
	runtime.KeepAlive(argv)
	runtime.KeepAlive(env)
	runtime.KeepAlive(target)
	return fmt.Errorf("native policy observer exec failed: %w", errno)
}
