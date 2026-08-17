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
	project := ProjectInstance{
		SchemaVersion: ProjectStateSchemaVersion, ID: hostLoopbackTestProject,
		Root: "/workspace/project", ContextID: hostLoopbackTestContext, ContextName: "default",
		Profile: DefaultProfile, Runtime: ProjectRuntime{},
	}
	route, err := NewAttachmentHostLoopbackRoute(hostLoopbackTestEpoch, project, 43179, strings.Repeat("3", 64))
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func TestAttachmentHostLoopbackRouteIdentityBindsEpochContextAndProject(t *testing.T) {
	route := testAttachmentHostLoopbackRoute(t)
	for name, mutate := range map[string]func(*AttachmentHostLoopbackRoute){
		"epoch":   func(r *AttachmentHostLoopbackRoute) { r.EpochID = "att_abcdef0123456789abcdef0123456789" },
		"context": func(r *AttachmentHostLoopbackRoute) { r.ContextID = "01912345-6789-7abc-8def-0123456789ac" },
		"project": func(r *AttachmentHostLoopbackRoute) { r.ProjectID = "01912345-6789-7abc-8def-0123456789ac" },
		"host":    func(r *AttachmentHostLoopbackRoute) { r.Hostname = "example.com" },
	} {
		changed := route
		mutate(&changed)
		if err := changed.Validate(); err == nil {
			t.Errorf("%s mutation remained valid", name)
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
		ContextID: hostLoopbackTestContext, ContextName: "default", ProjectID: hostLoopbackTestProject,
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
	if err != nil || grant.TargetPort != 3000 || grant.EpochID != candidate.AttachmentEpochID {
		t.Fatalf("attachment grant = %+v, %v", grant, err)
	}
}

func TestHostLoopbackRejectsPrivilegedPort(t *testing.T) {
	if err := ValidateHostLoopbackPort(80); err == nil {
		t.Fatal("privileged Host Loopback port was accepted")
	}
}
