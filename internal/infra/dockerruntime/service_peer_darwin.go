//go:build darwin

package dockerruntime

import (
	"net"
	"syscall"
)

func servicePeerIdentity(connection *net.UnixConn) (pid int, uid int, err error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, -1, err
	}
	var controlErr error
	err = raw.Control(func(fd uintptr) { pid, controlErr = syscall.GetsockoptInt(int(fd), 0, 2) })
	if err != nil {
		return 0, -1, err
	}
	return pid, -1, controlErr
}
