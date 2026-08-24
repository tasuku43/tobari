package tobari

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	hostLoopbackTestEpoch   = "att_0123456789abcdef0123456789abcdef"
	hostLoopbackTestContext = "01912345-6789-7abc-8def-0123456789ad"
	hostLoopbackTestProject = "01912345-6789-7abc-8def-0123456789ab"
)

func testAttachmentHostLoopbackRoute(t *testing.T) AttachmentHostLoopbackRoute {
	t.Helper()
	route, err := NewAttachmentHostLoopbackRouteForPrincipal(
		hostLoopbackTestEpoch, hostLoopbackTestContext, "default",
		hostLoopbackTestProject, "/workspace/project", 43179, strings.Repeat("3", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func TestAttachmentHostLoopbackRouteIdentityBindsEpochContextWorkspaceAndHostname(t *testing.T) {
	route := testAttachmentHostLoopbackRoute(t)
	for name, mutate := range map[string]func(*AttachmentHostLoopbackRoute){
		"epoch":     func(r *AttachmentHostLoopbackRoute) { r.EpochID = "att_abcdef0123456789abcdef0123456789" },
		"context":   func(r *AttachmentHostLoopbackRoute) { r.ContextID = "01912345-6789-7abc-8def-0123456789ac" },
		"workspace": func(r *AttachmentHostLoopbackRoute) { r.WorkspaceID = "01912345-6789-7abc-8def-0123456789ac" },
		"host":      func(r *AttachmentHostLoopbackRoute) { r.Hostname = "example.com" },
	} {
		changed := route
		mutate(&changed)
		if err := changed.Validate(); err == nil {
			t.Errorf("%s mutation remained valid", name)
		}
	}
}

func TestHostLoopbackSchemaAndAuthorityHardCut(t *testing.T) {
	if HostLoopbackCapabilitySchema != 1 || HostLoopbackRegistrySchema != 2 {
		t.Fatalf("capability/registry schemas = %d/%d, want 1/2", HostLoopbackCapabilitySchema, HostLoopbackRegistrySchema)
	}
	if HostLoopbackHostname != "host.tobari.internal" || HostLoopbackURLTemplate != "http://host.tobari.internal:{port}" {
		t.Fatalf("current Host Loopback authority = %q %q", HostLoopbackHostname, HostLoopbackURLTemplate)
	}
	if RetiredHostLoopbackHostname != "host.tobari.test" {
		t.Fatalf("retired Host Loopback authority = %q", RetiredHostLoopbackHostname)
	}
	route := testAttachmentHostLoopbackRoute(t)
	oldID := route.ID
	route.Hostname = RetiredHostLoopbackHostname
	if route.Validate() == nil {
		t.Fatal("retired hostname remained valid route authority")
	}
	route.ID = hostLoopbackRouteID(route.EpochID, route.ContextID, route.WorkspaceID, route.Hostname)
	if route.ID == oldID || route.Validate() == nil {
		t.Fatal("hostname-bound fresh route ID authorized the retired hostname")
	}
}

func TestHostLoopbackWireEmitsOnlyFrozenGatewayIdentityKeys(t *testing.T) {
	route := testAttachmentHostLoopbackRoute(t)
	grant, err := NewAttachmentGrantFromCandidate(PolicyDecisionAllow, testHostLoopbackCandidate(t, 3000))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"route": route, "grant": grant} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		wire := string(encoded)
		for _, required := range []string{`"project_id"`, `"context_id"`} {
			if !strings.Contains(wire, required) {
				t.Errorf("%s Gateway wire %s lacks %s", name, wire, required)
			}
		}
		for _, forbidden := range []string{`"workspace_id"`, `"workspace_manifest_id"`} {
			if strings.Contains(wire, forbidden) {
				t.Errorf("%s Gateway wire %s contains renamed alias %s", name, wire, forbidden)
			}
		}
	}
}

func TestHostLoopbackRegistryAllowsOneRoutePerWorkspace(t *testing.T) {
	route := testAttachmentHostLoopbackRoute(t)
	registry := HostLoopbackRegistry{SchemaVersion: HostLoopbackRegistrySchema, Routes: []AttachmentHostLoopbackRoute{route}}
	if err := registry.Validate(); err != nil {
		t.Fatal(err)
	}
	registry.Routes = append(registry.Routes, route)
	if err := registry.Validate(); err == nil {
		t.Fatal("duplicate Workspace route was accepted")
	}
}

func TestHostLoopbackCapabilityProjectionIsConstantAndSecretFree(t *testing.T) {
	projection := NewHostLoopbackCapabilityProjection()
	if err := projection.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{HostLoopbackURLTemplate, `"minimum_port":1024`, `"maximum_port":65535`, `"lifetime":"attachment"`, `"audience":"workspace"`, `"host_docker_control":"unavailable"`} {
		if !strings.Contains(text, required) {
			t.Errorf("projection %s lacks %q", text, required)
		}
	}
	for _, forbidden := range []string{"relay", "token", hostLoopbackTestEpoch, "127.0.0.1"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("capability projection leaked %q: %s", forbidden, text)
		}
	}
}

func testHostLoopbackCandidate(t *testing.T, port int) PolicyCandidate {
	t.Helper()
	denial := PolicyDenial{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "http", Protocol: PolicyProtocolHTTP},
		Timestamp:              "2026-08-17T12:00:00Z", RequestID: "0123456789abcdef0123456789abcdef",
		WorkspaceManifestID: hostLoopbackTestContext, WorkspaceManifestName: "default", ProjectID: hostLoopbackTestProject,
		ProjectRoot: "/workspace/project", Host: HostLoopbackHostname, Port: port,
		Method: "GET", Path: "/health", Reason: "review", StatusCode: 403, Learnable: true,
		DestinationKind: PolicyDestinationHostLoopback, AuthorityLifetime: AuthorityLifetimeAttachment,
		AttachmentEpochID: hostLoopbackTestEpoch,
	}
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestAttachmentGrantBindsReviewedPortAndCannotBecomePersistent(t *testing.T) {
	grant, err := NewAttachmentGrantFromCandidate(PolicyDecisionAllow, testHostLoopbackCandidate(t, 3000))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*AttachmentGrant){
		"persistent": func(g *AttachmentGrant) { g.Lifetime = AuthorityLifetimePersistent },
		"epoch":      func(g *AttachmentGrant) { g.EpochID = "att_abcdef0123456789abcdef0123456789" },
		"external":   func(g *AttachmentGrant) { g.DestinationKind = PolicyDestinationExternal },
		"port":       func(g *AttachmentGrant) { g.TargetPort = 3001 },
		"host":       func(g *AttachmentGrant) { g.Hostname = "example.com" },
	} {
		changed := grant
		mutate(&changed)
		if err := changed.Validate(); err == nil {
			t.Errorf("%s mutation remained valid", name)
		}
	}
}

func TestHostLoopbackCandidateProducesOnlyAttachmentGrantAndExactReviewItem(t *testing.T) {
	candidate := testHostLoopbackCandidate(t, 3000)
	if _, err := NewExactLearnedPolicyRule(candidate); err == nil {
		t.Fatal("host loopback candidate became a persistent Allow")
	}
	if _, err := NewExactPolicyDenyRule(candidate); err == nil {
		t.Fatal("host loopback candidate became a persistent Deny")
	}
	items, err := PolicyReviewItems([]PolicyCandidate{candidate}, []LearnedPolicyRule{})
	if err != nil || len(items) != 1 || items[0].Match != PolicyMatchExact || items[0].Candidate == nil {
		t.Fatalf("review items = %+v, %v", items, err)
	}
	grant, err := NewAttachmentGrantFromCandidate(PolicyDecisionAllow, candidate)
	if err != nil || grant.ContextID != candidate.WorkspaceManifestID || grant.WorkspaceID != candidate.ProjectID || grant.TargetPort != 3000 || grant.EpochID != candidate.AttachmentEpochID {
		t.Fatalf("attachment grant = %+v, %v", grant, err)
	}
}

func TestHostLoopbackRejectsPrivilegedPort(t *testing.T) {
	if err := ValidateHostLoopbackPort(80); err == nil {
		t.Fatal("privileged Host Loopback port was accepted")
	}
}
