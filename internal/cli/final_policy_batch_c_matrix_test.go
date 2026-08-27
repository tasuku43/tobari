package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// batchCB3ReviewPort deliberately returns a pre-effect stale-snapshot fault.
// That makes the test observe the exact immutable set crossing the application
// port without granting authority or needing a second test mutation engine.
type batchCB3ReviewPort struct {
	*finalPolicyPortFixture
	candidateReads int
	ruleReads      int
	reviewReads    int
	applyCalls     int
	applied        tobari.PolicyMemoryReviewedDecisionSet
	refreshEmpty   bool
}

func (p *batchCB3ReviewPort) ReadPolicyMemoryReviewSnapshot(context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	p.reviewReads++
	if p.refreshEmpty && p.reviewReads > 1 {
		return tobari.NewPolicyMemoryReviewSnapshot(tobari.WorkspaceAuthorityCollection{}, false)
	}
	return p.review.Clone(), nil
}

func (p *batchCB3ReviewPort) ListPendingPolicyCandidateAuthority(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	p.candidateReads++
	if p.refreshEmpty && p.candidateReads > 1 {
		return tobari.NewPolicyCandidateAuthorityList(tobari.WorkspaceAuthorityCollection{}, false)
	}
	return p.finalPolicyPortFixture.ListPendingPolicyCandidateAuthority(ctx)
}

func (p *batchCB3ReviewPort) ListPolicyMemoryRuleAuthority(ctx context.Context) (tobari.PolicyMemoryRuleList, error) {
	p.ruleReads++
	return p.finalPolicyPortFixture.ListPolicyMemoryRuleAuthority(ctx)
}

func (p *batchCB3ReviewPort) ApplyReviewedPolicyMemory(_ context.Context, set tobari.PolicyMemoryReviewedDecisionSet) (tobari.PolicyMemoryReviewedSetPublication, error) {
	p.applyCalls++
	p.applied = set.Clone()
	return tobari.PolicyMemoryReviewedSetPublication{}, tobari.ErrPolicyReviewChanged
}

func batchCB3FinalReviewCLI(t *testing.T, input string, refreshEmpty bool) (*CLI, *batchCB3ReviewPort, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	base, _, _ := finalPolicyCLIFixture(t)
	port := &batchCB3ReviewPort{finalPolicyPortFixture: base, refreshEmpty: refreshEmpty}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(input), stdout, stderr, DefaultCatalog(), nil)
	command.finalPolicy = workspaceauthoritycmd.NewPolicyMemoryService(port)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	return command, port, stdout, stderr
}

func TestBatchCB3FinalTTYSubmitsOneImmutableReviewedSetToFinalPort(t *testing.T) {
	command, port, stdout, stderr := batchCB3FinalReviewCLI(t, "1\na\np\ny\n", false)
	code := command.RunContext(context.Background(), []string{"review", "permissions"})
	if code != ExitRejected || port.reviewReads != 1 || port.candidateReads != 0 || port.ruleReads != 0 || port.applyCalls != 1 {
		t.Fatalf("code=%d reviewReads=%d candidateReads=%d ruleReads=%d applyCalls=%d stdout=%q stderr=%q", code, port.reviewReads, port.candidateReads, port.ruleReads, port.applyCalls, stdout.String(), stderr.String())
	}
	if err := port.applied.Validate(); err != nil || len(port.applied.Decisions) != 1 ||
		port.applied.Decisions[0].Decision != tobari.PolicyMemoryAllow ||
		port.applied.Decisions[0].ReviewItemID != port.review.Items[0].ID {
		t.Fatalf("reviewed set=%#v validate=%v", port.applied, err)
	}
	if !humanOutputHasRow(stderr.String(), "Code", "policy_review_changed") {
		t.Fatalf("stale final snapshot fault=%q", stderr.String())
	}
}

func TestBatchCB3FinalRefreshInvalidatesStagingWithoutApply(t *testing.T) {
	command, port, stdout, stderr := batchCB3FinalReviewCLI(t, "1\na\nr\np\nq\n", true)
	code := command.RunContext(context.Background(), []string{"review", "permissions"})
	if code != ExitCanceled || port.reviewReads != 2 || port.candidateReads != 0 || port.ruleReads != 0 || port.applyCalls != 0 {
		t.Fatalf("code=%d reviewReads=%d candidateReads=%d ruleReads=%d applyCalls=%d stdout=%q stderr=%q", code, port.reviewReads, port.candidateReads, port.ruleReads, port.applyCalls, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stale staged decision removed") {
		t.Fatalf("refresh did not explain invalidation: %q", stdout.String())
	}
}

func TestBatchCB3FinalSchemaAndReferenceBoundary(t *testing.T) {
	catalog := DefaultCatalog()
	apply, found := catalog.lookupRegistered("policy apply-reviewed")
	if !found {
		t.Fatal("policy apply-reviewed is not registered")
	}
	if !reflect.DeepEqual(apply.Agent.Output.Formats, []OutputFormat{OutputFormatText, OutputFormatJSON}) ||
		apply.Agent.Output.JSONSchemaVersion != tobari.WorkspaceAuthorityPolicyReadSchemaVersion ||
		apply.Agent.Output.JSONEnvelope != "result" || apply.Agent.Output.JSONEnvelopeType != OutputFieldTypeObject {
		t.Errorf("apply-reviewed output contract=%+v", apply.Agent.Output)
	}
	if got := apply.ProducedRefs(); !reflect.DeepEqual(got, []ProducedRef{{Kind: tobari.PolicyRuleKind, Field: "decisions[].rule_id"}}) {
		t.Errorf("apply-reviewed producers=%+v", got)
	}
	encoded, _ := json.Marshal(apply.Agent.Output.Fields)
	for _, forbidden := range []string{"context_ref", "template_ref", "observing_workspace_ref"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("apply-reviewed exposes owner reference %q: %s", forbidden, encoded)
		}
	}

	base, _, _ := finalPolicyCLIFixture(t)
	for _, args := range [][]string{{"review", "permissions", "--format=json"}, {"review", "permissions"}} {
		var stdout, stderr bytes.Buffer
		command := newCLI(strings.NewReader("1\na\np\ny\n"), &stdout, &stderr, catalog, nil)
		command.finalPolicy = workspaceauthoritycmd.NewPolicyMemoryService(base)
		command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: false})
		if code := command.RunContext(context.Background(), args); code != ExitOK || stderr.Len() != 0 {
			t.Errorf("redirected %v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
			continue
		}
		if args[len(args)-1] == "--format=json" && (!json.Valid(stdout.Bytes()) || !strings.Contains(stdout.String(), `"schema_version":3`)) {
			t.Errorf("redirected JSON=%q", stdout.String())
		}
	}
	if base.allowCalls != 0 || base.denyCalls != 0 || base.resetCalls != 0 {
		t.Fatalf("redirected Permission Inbox mutated final policy: %+v", base)
	}
}

func TestPolicyOutputsDeclareEveryClosedModuleCoordinate(t *testing.T) {
	required := []string{
		"graphql_operation_type", "graphql_root_field", "mcp_method", "mcp_tool_name",
		"aws_wire_protocol", "aws_service", "aws_protocol_version", "aws_target_namespace", "aws_operation",
		"kubernetes_kind", "kubernetes_verb", "kubernetes_group", "kubernetes_version", "kubernetes_resource",
		"kubernetes_namespace", "kubernetes_name", "kubernetes_subresource", "kubernetes_dry_run", "kubernetes_non_resource_path",
		"git_service", "git_repository", "oci_action", "oci_repository", "oci_object",
	}
	for _, path := range []string{"policy candidates", "review permissions", "policy rules", "cluster denials"} {
		command, found := DefaultCatalog().lookupRegistered(path)
		if !found {
			t.Fatalf("missing %s", path)
		}
		encoded, err := json.Marshal(command.Agent.Output.Fields)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range required {
			if !bytes.Contains(encoded, []byte(`"name":"`+field+`"`)) {
				t.Errorf("%s output omits closed coordinate %s", path, field)
			}
		}
	}
}

func TestFinalPolicyTextRetainsSelectedModuleCoordinate(t *testing.T) {
	tests := []struct {
		name string
		id   tobari.PolicyProtocolIdentity
		want []string
	}{
		{name: "graphql", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolGraphQL, GraphQLOperationType: "mutation", GraphQLRootField: "updateItem"}, want: []string{"GraphQL", "mutation.updateItem"}},
		{name: "mcp", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "deploy"}, want: []string{"MCP", "tools/call deploy"}},
		{name: "aws", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolAWS, AWSWireProtocol: "json", AWSService: "dynamodb", AWSTargetNamespace: "DynamoDB_20120810", AWSOperation: "PutItem"}, want: []string{"AWS", "json dynamodb/DynamoDB_20120810/PutItem"}},
		{name: "kubernetes", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolKubernetes, KubernetesKind: tobari.KubernetesRequestResource, KubernetesVerb: "update", KubernetesGroup: "apps", KubernetesVersion: "v1", KubernetesResource: "deployments", KubernetesNamespace: "team", KubernetesName: "api", KubernetesSubresource: "scale", KubernetesDryRun: "all"}, want: []string{"Kubernetes", "apps/v1/deployments", "namespace=team", "name=api", "subresource=scale", "dry-run=all"}},
		{name: "git", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolGit, GitService: "receive-pack", GitRepository: "/team/repo.git"}, want: []string{"Git", "receive-pack /team/repo.git"}},
		{name: "oci", id: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolOCI, OCIAction: "push", OCIRepository: "team/app", OCIObject: "manifest:latest"}, want: []string{"OCI", "push team/app manifest:latest"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := finalPolicyEffectSummary(test.id, "POST", "api.example.dev", 443, "/effect")
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("summary %q lacks %q", got, want)
				}
			}
		})
	}
}

func TestFinalPolicyJSONRetainsMeaningfulEmptyCoordinates(t *testing.T) {
	value := []any{
		map[string]any{"protocol": tobari.PolicyProtocolKubernetes, "kubernetes_kind": tobari.KubernetesRequestResource},
		map[string]any{"protocol": tobari.PolicyProtocolOCI, "oci_action": "list", "oci_object": "catalog"},
	}
	projected, err := finalPolicyJSONProjection(value)
	if err != nil {
		t.Fatal(err)
	}
	items := projected.([]any)
	if group, present := items[0].(map[string]any)["kubernetes_group"]; !present || group != "" {
		t.Fatalf("core Kubernetes group = %#v, present=%t", group, present)
	}
	if repository, present := items[1].(map[string]any)["oci_repository"]; !present || repository != "" {
		t.Fatalf("OCI catalog repository = %#v, present=%t", repository, present)
	}
}

func TestBatchCB3ResearchServeUsesFinalReviewedPolicyPort(t *testing.T) {
	source, err := os.ReadFile("serve.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "*workspaceauthoritycmd.PolicyMemoryService") ||
		strings.Contains(text, "service *tobaricmd.Service") ||
		!strings.Contains(text, ".ApplyReviewed(") {
		t.Fatalf("research serve does not share the final reviewed-policy application port")
	}
}
