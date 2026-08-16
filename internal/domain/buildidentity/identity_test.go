package buildidentity

import (
	"testing"

	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
)

func TestIdentityRequiresCompleteTruthfulMetadataForCompatibility(t *testing.T) {
	t.Parallel()
	identity := Identity{
		Version: "1.2.3", Commit: UnknownCommit, ResolverChannel: ResolverEmbedded,
		CapabilityProfile: capabilityprofile.ProfileStandard,
		Gateway:           Component{RequiredAPI: 4, SelectedAPI: 4},
	}
	if err := identity.Validate(); err != nil {
		t.Fatal(err)
	}
	if identity.Compatible() {
		t.Fatal("missing commit metadata was presented as compatible")
	}
	identity.Commit = "0123456789abcdef0123456789abcdef01234567"
	if !identity.Compatible() {
		t.Fatal("complete matching identity was not compatible")
	}
	identity.Gateway.SelectedAPI = 3
	if identity.APIsCompatible() || identity.Compatible() {
		t.Fatal("mismatched Gateway API was presented as compatible")
	}
}

func TestDevelopmentRecoveryRequiresDevelopmentResolverMetadata(t *testing.T) {
	t.Parallel()
	identity := Identity{ResolverChannel: ResolverEmbedded, CapabilityProfile: capabilityprofile.ProfileStandard}
	if build, binary, ok := identity.DevelopmentRecovery(); ok || build != "" || binary != "" {
		t.Fatalf("published recovery = %q %q %t", build, binary, ok)
	}
	identity = Identity{ResolverChannel: ResolverDevelopment, DevelopmentSource: true, CapabilityProfile: capabilityprofile.ProfileStandard}
	build, binary, ok := identity.DevelopmentRecovery()
	if !ok || build != "task build" || binary != "bin/tobari" {
		t.Fatalf("development recovery = %q %q %t", build, binary, ok)
	}
}

func TestIdentityRejectsCrossedChannelMetadata(t *testing.T) {
	t.Parallel()
	identity := Identity{
		Version: "dev", Commit: UnknownCommit, ResolverChannel: ResolverEmbedded, DevelopmentSource: true,
		CapabilityProfile: capabilityprofile.ProfileStandard,
		Gateway:           Component{RequiredAPI: 4, SelectedAPI: 3},
	}
	if err := identity.Validate(); err == nil {
		t.Fatal("embedded resolver accepted development source metadata")
	}
}
