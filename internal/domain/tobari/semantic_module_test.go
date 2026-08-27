package tobari

import (
	"reflect"
	"strings"
	"testing"
)

func TestSemanticModuleRegistryIsClosedAndCallerImmutable(t *testing.T) {
	want := []string{
		SemanticModuleHTTPGeneric,
		SemanticModuleGraphQL,
		SemanticModuleMCP,
		SemanticModuleAWS,
		SemanticModuleKubernetes,
		SemanticModuleGit,
		SemanticModuleOCI,
	}
	if err := ValidateSemanticModuleRegistry(); err != nil {
		t.Fatalf("ValidateSemanticModuleRegistry() error = %v", err)
	}
	got := SemanticModuleIDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SemanticModuleIDs() = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if next := SemanticModuleIDs(); !reflect.DeepEqual(next, want) {
		t.Fatalf("caller mutated registry inventory: %v", next)
	}
}

func TestSelectSemanticModuleRequiresUniqueMostSpecificCandidate(t *testing.T) {
	for _, candidates := range [][]string{
		{SemanticModuleHTTPGeneric, SemanticModuleGraphQL},
		{SemanticModuleGraphQL, SemanticModuleHTTPGeneric},
		{SemanticModuleHTTPGeneric, SemanticModuleGraphQL, SemanticModuleHTTPGeneric},
	} {
		got, err := SelectSemanticModule(candidates)
		if err != nil {
			t.Fatalf("SelectSemanticModule(%v) error = %v", candidates, err)
		}
		if got != SemanticModuleGraphQL {
			t.Fatalf("SelectSemanticModule(%v) = %q, want %q", candidates, got, SemanticModuleGraphQL)
		}
	}

	for name, candidates := range map[string][]string{
		"empty":     nil,
		"unknown":   {"providers.future"},
		"ambiguous": {SemanticModuleGraphQL, SemanticModuleMCP},
	} {
		t.Run(name, func(t *testing.T) {
			if selected, err := SelectSemanticModule(candidates); err == nil {
				t.Fatalf("SelectSemanticModule(%v) = %q, want error", candidates, selected)
			}
		})
	}
}

func TestSelectSemanticModuleClaimsMakesMalformedClassificationTerminal(t *testing.T) {
	if selected, err := SelectSemanticModuleClaims([]SemanticClassificationClaim{
		{ModuleID: SemanticModuleHTTPGeneric, State: SemanticClassificationMatched},
		{ModuleID: SemanticModuleGraphQL, State: SemanticClassificationMalformed},
	}); err == nil {
		t.Fatalf("malformed GraphQL classification selected %q", selected)
	}
	selected, err := SelectSemanticModuleClaims([]SemanticClassificationClaim{
		{ModuleID: SemanticModuleGraphQL, State: SemanticClassificationMatched},
		{ModuleID: SemanticModuleHTTPGeneric, State: SemanticClassificationMatched},
	})
	if err != nil || selected != SemanticModuleGraphQL {
		t.Fatalf("matched refinement selection = %q, %v", selected, err)
	}
}

func TestPolicyIdentitySelectsOneSemanticModule(t *testing.T) {
	identities := []struct {
		identity PolicyProtocolIdentity
		moduleID string
	}{
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}, SemanticModuleHTTPGeneric},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "viewer"}, SemanticModuleGraphQL},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "issues.get"}, SemanticModuleMCP},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolAWS, AWSWireProtocol: AWSWireProtocolQuery, AWSService: "sts", AWSProtocolVersion: "2011-06-15", AWSOperation: "GetCallerIdentity"}, SemanticModuleAWS},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolKubernetes, KubernetesKind: KubernetesRequestResource, KubernetesVerb: "get", KubernetesGroup: "", KubernetesVersion: "v1", KubernetesResource: "pods", KubernetesNamespace: "default", KubernetesName: "example", KubernetesDryRun: "none"}, SemanticModuleKubernetes},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolGit, GitService: "upload-pack", GitRepository: "/owner/repo.git"}, SemanticModuleGit},
		{PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolOCI, OCIAction: "pull", OCIRepository: "owner/image", OCIObject: "manifest:latest"}, SemanticModuleOCI},
	}
	for _, test := range identities {
		if err := test.identity.Validate(); err != nil {
			t.Fatalf("%s identity validation error = %v", test.moduleID, err)
		}
		if got := test.identity.SemanticModuleID(); got != test.moduleID {
			t.Fatalf("SemanticModuleID() = %q, want %q", got, test.moduleID)
		}
	}
}

func TestHTTPModuleRejectsRefinementFields(t *testing.T) {
	for _, identity := range []PolicyProtocolIdentity{
		{Scheme: "https", Protocol: PolicyProtocolHTTP, GraphQLRootField: "viewer"},
		{Scheme: "https", Protocol: PolicyProtocolHTTP, MCPMethod: "tools/list"},
		{Scheme: "https", Protocol: PolicyProtocolHTTP, AWSService: "sts"},
		{Scheme: "https", Protocol: PolicyProtocolHTTP, KubernetesVerb: "get"},
		{Scheme: "https", Protocol: PolicyProtocolHTTP, GitService: "upload-pack"},
		{Scheme: "https", Protocol: PolicyProtocolHTTP, OCIAction: "pull"},
	} {
		if err := identity.Validate(); err == nil || !strings.Contains(err.Error(), "cannot contain semantic refinement") {
			t.Fatalf("Validate(%+v) error = %v, want HTTP refinement rejection", identity, err)
		}
	}
}
