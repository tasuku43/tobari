package tobari

import "fmt"

const NetworkGuardRevision = "v1"

// NetworkGuardState is the transport-independent proof required before a
// Workspace may be entered. Infrastructure may obtain these facts from a
// container runtime and packet filter, but callers cannot weaken a missing
// dimension into partial readiness.
type NetworkGuardState struct {
	Revision                   string
	WorkspaceOutputClosed      bool
	TransparentHTTPReady       bool
	SyntheticDNSReady          bool
	GatewayForwardingDisabled  bool
	GatewayForwardPolicyClosed bool
}

// Validate rejects partial guard observations. A routine entry is safe only
// when all dimensions belong to the same known revision.
func (s NetworkGuardState) Validate() error {
	if s.Revision != NetworkGuardRevision {
		return fmt.Errorf("network guard revision is invalid")
	}
	checks := map[string]bool{
		"Workspace output":       s.WorkspaceOutputClosed,
		"transparent HTTP":       s.TransparentHTTPReady,
		"synthetic DNS":          s.SyntheticDNSReady,
		"Gateway forwarding":     s.GatewayForwardingDisabled,
		"Gateway forward policy": s.GatewayForwardPolicyClosed,
	}
	for name, ready := range checks {
		if !ready {
			return fmt.Errorf("network guard %s is not ready", name)
		}
	}
	return nil
}
