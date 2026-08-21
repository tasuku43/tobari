//go:build windows

package dockerruntime

import (
	"errors"
	"net"
)

func servicePeerIdentity(_ *net.UnixConn) (int, int, error) {
	return 0, 0, errors.New("Workspace service peer validation is unsupported on Windows")
}
