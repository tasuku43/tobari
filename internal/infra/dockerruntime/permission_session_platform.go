package dockerruntime

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func permissionSessionTransportForGOOS(goos string) tobari.PermissionSessionTransport {
	switch goos {
	case "linux":
		return tobari.PermissionSessionTransportUnix
	case "darwin":
		return tobari.PermissionSessionTransportTCP
	default:
		return ""
	}
}

func (r *Runtime) permissionSessionComposeFileArgs(runtimeDirectory string) ([]string, error) {
	return permissionSessionComposeFileArgsForTransport(runtimeDirectory, r.permissionIngestionTransport)
}

func permissionSessionComposeFileArgsForTransport(
	runtimeDirectory string, transport tobari.PermissionSessionTransport,
) ([]string, error) {
	if transport == "" {
		// Retained state written before the platform contract used base compose only.
		return nil, nil
	}
	if err := transport.Validate(); err != nil {
		return nil, fmt.Errorf("select permission ingestion support profile: %w", err)
	}
	name := "compose.permission-" + string(transport) + ".yaml"
	return []string{"-f", filepath.Join(runtimeDirectory, name)}, nil
}

func (r *Runtime) listenPermissionSession(epochID string) (net.Listener, string, error) {
	switch r.permissionIngestionTransport {
	case tobari.PermissionSessionTransportUnix:
		endpoint := "pws_" + epochID[len("att_"):] + ".sock"
		path := filepath.Join(r.interactiveAttachmentSocketDirectory(), endpoint)
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			return nil, "", err
		}
		listener.SetUnlinkOnClose(true)
		return listener, endpoint, nil
	case tobari.PermissionSessionTransportTCP:
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
		if err != nil {
			return nil, "", err
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok || !address.IP.Equal(net.IPv4(127, 0, 0, 1)) || address.Port < 1 || address.Port > 65535 {
			_ = listener.Close()
			return nil, "", fmt.Errorf("permission ingestion listener is not IPv4 loopback")
		}
		return listener, net.JoinHostPort("127.0.0.1", strconv.Itoa(address.Port)), nil
	default:
		return nil, "", fmt.Errorf("permission ingestion support profile is unsupported")
	}
}

func (r *Runtime) dialPermissionSession(session tobari.InteractiveAttachmentSession, timeout time.Duration) (net.Conn, error) {
	if session.IngestionTransport != r.permissionIngestionTransport {
		return nil, fmt.Errorf("permission ingestion transport does not match the trusted host profile")
	}
	switch session.IngestionTransport {
	case tobari.PermissionSessionTransportUnix:
		if err := requireOwnerOnlyPath(r.interactiveAttachmentSocketDirectory(), true); err != nil {
			return nil, err
		}
		path := r.interactiveAttachmentSocketPath(session)
		if err := requireOwnerOnlyPath(path, false); err != nil {
			return nil, err
		}
		return net.DialTimeout("unix", path, timeout)
	case tobari.PermissionSessionTransportTCP:
		return net.DialTimeout("tcp4", session.IngestionEndpoint, timeout)
	default:
		return nil, fmt.Errorf("permission ingestion transport is unsupported")
	}
}
