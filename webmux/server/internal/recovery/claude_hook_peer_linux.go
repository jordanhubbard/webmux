package recovery

import (
	"golang.org/x/sys/unix"
	"net"
)

func claudeHookPeerPID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var peerErr error
	err = raw.Control(func(fd uintptr) {
		credential, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		peerErr = e
		if e == nil {
			pid = int(credential.Pid)
		}
	})
	if err != nil {
		return 0, err
	}
	return pid, peerErr
}
