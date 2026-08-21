//go:build linux

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
	err = raw.Control(func(fd uintptr) {
		credential, credentialErr := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if credentialErr != nil {
			controlErr = credentialErr
			return
		}
		pid, uid = int(credential.Pid), int(credential.Uid)
	})
	if err != nil {
		return 0, -1, err
	}
	return pid, uid, controlErr
}
