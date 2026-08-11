package tobari

import (
	"strings"
	"testing"
)

func TestNetworkGuardStateRequiresOneCompleteRevision(t *testing.T) {
	t.Parallel()
	complete := NetworkGuardState{
		Revision: NetworkGuardRevision, WorkspaceOutputClosed: true,
		TransparentHTTPReady: true, SyntheticDNSReady: true,
		GatewayForwardingDisabled: true, GatewayForwardPolicyClosed: true,
	}
	if err := complete.Validate(); err != nil {
		t.Fatalf("complete guard state was rejected: %v", err)
	}

	tests := map[string]func(*NetworkGuardState){
		"revision":               func(state *NetworkGuardState) { state.Revision = "v0" },
		"Workspace output":       func(state *NetworkGuardState) { state.WorkspaceOutputClosed = false },
		"transparent HTTP":       func(state *NetworkGuardState) { state.TransparentHTTPReady = false },
		"synthetic DNS":          func(state *NetworkGuardState) { state.SyntheticDNSReady = false },
		"Gateway forwarding":     func(state *NetworkGuardState) { state.GatewayForwardingDisabled = false },
		"Gateway forward policy": func(state *NetworkGuardState) { state.GatewayForwardPolicyClosed = false },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			state := complete
			mutate(&state)
			if err := state.Validate(); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("Validate() error = %v, want %q", err, name)
			}
		})
	}
}
