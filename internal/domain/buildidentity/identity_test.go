package buildidentity

import "testing"

func TestIdentityRequiresCompleteTruthfulMetadataForCompatibility(t *testing.T) {
	t.Parallel()
	identity := Identity{
		Version: "1.2.3", Commit: UnknownCommit, ResolverChannel: ResolverPublished,
		Gateway:    Component{RequiredAPI: 4, SelectedAPI: 4},
		AuthBroker: Component{RequiredAPI: 3, SelectedAPI: 3},
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
	identity := Identity{ResolverChannel: ResolverPublished}
	if build, binary, ok := identity.DevelopmentRecovery(); ok || build != "" || binary != "" {
		t.Fatalf("published recovery = %q %q %t", build, binary, ok)
	}
	identity = Identity{ResolverChannel: ResolverDevelopment, DevelopmentSource: true}
	build, binary, ok := identity.DevelopmentRecovery()
	if !ok || build != "task build:dev" || binary != "bin/tobari-dev" {
		t.Fatalf("development recovery = %q %q %t", build, binary, ok)
	}
}

func TestIdentityRejectsCrossedChannelMetadata(t *testing.T) {
	t.Parallel()
	identity := Identity{
		Version: "dev", Commit: UnknownCommit, ResolverChannel: ResolverPublished, DevelopmentSource: true,
		Gateway:    Component{RequiredAPI: 4, SelectedAPI: 3},
		AuthBroker: Component{RequiredAPI: 3, SelectedAPI: 2},
	}
	if err := identity.Validate(); err == nil {
		t.Fatal("published resolver accepted development source metadata")
	}
}
