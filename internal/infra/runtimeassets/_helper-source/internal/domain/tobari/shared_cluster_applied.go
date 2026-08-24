package tobari

import "fmt"

const (
	SharedClusterProfilePrePlatform SharedClusterAppliedProfile = "pre_platform"
	SharedClusterProfileUnix        SharedClusterAppliedProfile = "unix"
	SharedClusterProfileLoopbackTCP SharedClusterAppliedProfile = "loopback_tcp"
)

// SharedClusterAppliedProfile records the exact permission-ingestion Compose
// shape in the last successfully published shared-cluster state.
type SharedClusterAppliedProfile string

func (p SharedClusterAppliedProfile) Validate() error {
	switch p {
	case SharedClusterProfilePrePlatform, SharedClusterProfileUnix, SharedClusterProfileLoopbackTCP:
		return nil
	default:
		return fmt.Errorf("shared-cluster applied profile is invalid")
	}
}

func (p SharedClusterAppliedProfile) PermissionTransport() (PermissionSessionTransport, bool) {
	switch p {
	case SharedClusterProfileUnix:
		return PermissionSessionTransportUnix, true
	case SharedClusterProfileLoopbackTCP:
		return PermissionSessionTransportTCP, true
	default:
		return "", false
	}
}

func SharedClusterProfileForTransport(transport PermissionSessionTransport) (SharedClusterAppliedProfile, error) {
	switch transport {
	case PermissionSessionTransportUnix:
		return SharedClusterProfileUnix, nil
	case PermissionSessionTransportTCP:
		return SharedClusterProfileLoopbackTCP, nil
	default:
		return "", fmt.Errorf("permission session transport is invalid")
	}
}
