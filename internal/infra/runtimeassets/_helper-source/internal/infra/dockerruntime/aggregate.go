package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const aggregateSchemaVersion = 2

var (
	regoPackagePattern     = regexp.MustCompile(`(?m)^package[ \t]+tobari\.http[ \t]*$`)
	regoInputSchemaPattern = regexp.MustCompile(`input\.schema_version[ \t]*==[ \t]*([0-9]+)`)
)

type aggregateProjection struct {
	Revision           string
	PolicyDirectory    string
	GatewayConfig      string
	ManifestCount      int
	EvaluatorIdentity  tobari.PolicyEvaluatorIdentity
	PolicyDataIdentity tobari.PolicyDataIdentity
}

type aggregatePolicyDataDocument struct {
	Contexts map[string]json.RawMessage `json:"tobari_contexts"`
	Tobari   aggregatePolicyMetadata    `json:"tobari"`
}

type aggregatePolicyMetadata struct {
	AggregateSchemaVersion int                            `json:"aggregate_schema_version"`
	AggregateRevision      string                         `json:"aggregate_revision"`
	EvaluatorIdentity      tobari.PolicyEvaluatorIdentity `json:"evaluator_identity"`
	PolicyDataIdentity     tobari.PolicyDataIdentity      `json:"policy_data_identity"`
}

type aggregateGatewayProjectionDocument struct {
	Version  string                     `json:"version"`
	Contexts map[string]json.RawMessage `json:"contexts"`
}

type aggregateGatewayContextDocument struct {
	Name                string          `json:"name"`
	GraphQLEndpoints    json.RawMessage `json:"graphql_endpoints"`
	MCPEndpoints        json.RawMessage `json:"mcp_endpoints"`
	KubernetesEndpoints json.RawMessage `json:"kubernetes_endpoints"`
}

type aggregateContextBoundaryDocument struct {
	GraphQLEndpoints    json.RawMessage `json:"graphql_endpoints"`
	MCPEndpoints        json.RawMessage `json:"mcp_endpoints"`
	KubernetesEndpoints json.RawMessage `json:"kubernetes_endpoints"`
}

type aggregatePolicyDataEntry struct {
	ContextID string
	Data      map[string]any
}

type aggregateContext struct {
	contextID           string
	presentation        string
	finalAuthority      *tobari.WorkspacePolicyProjectionContext
	manifest            tobari.WorkspaceManifest
	paths               tobari.ManifestStorePaths
	data                map[string]any
	policy              policyDataFile
	graphqlEndpoints    []tobari.GraphQLEndpoint
	mcpEndpoints        []tobari.MCPEndpoint
	kubernetesEndpoints []tobari.GraphQLEndpoint
	contextPolicy       tobari.ManifestPolicy
}

func (c aggregateContext) resolvedIdentity() aggregateContext {
	if c.contextID == "" {
		c.contextID = c.manifest.ID
	}
	if c.presentation == "" {
		c.presentation = c.manifest.Name
	}
	return c
}

func (c aggregateContext) validateIdentity() error {
	c = c.resolvedIdentity()
	if c.contextID == "" || c.presentation == "" {
		return fmt.Errorf("aggregate Context identity is incomplete")
	}
	if err := tobari.ValidateWorkspaceManifestID(c.contextID); err != nil {
		return err
	}
	if err := tobari.ValidateName(c.presentation); err != nil {
		return err
	}
	if c.finalAuthority != nil {
		if err := c.finalAuthority.Validate(); err != nil {
			return err
		}
		if c.contextID != string(c.finalAuthority.ContextID) || c.presentation != c.finalAuthority.Presentation ||
			!reflect.DeepEqual(c.manifest, tobari.WorkspaceManifest{}) || c.paths != (tobari.ManifestStorePaths{}) || !reflect.DeepEqual(c.policy, policyDataFile{}) {
			return fmt.Errorf("final aggregate Context crosses typed authority or legacy state")
		}
		expected, err := finalAggregateContext(c.finalAuthority.Clone())
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(c.data, expected.data) ||
			!reflect.DeepEqual(c.graphqlEndpoints, expected.graphqlEndpoints) || !reflect.DeepEqual(c.mcpEndpoints, expected.mcpEndpoints) ||
			!reflect.DeepEqual(c.kubernetesEndpoints, expected.kubernetesEndpoints) || !reflect.DeepEqual(c.contextPolicy, expected.contextPolicy) {
			return fmt.Errorf("final aggregate rendered content does not match its typed authority")
		}
		return nil
	}
	return c.manifest.Validate()
}

func (c aggregateContext) authorityBytes() ([]byte, error) {
	if err := c.validateIdentity(); err != nil {
		return nil, err
	}
	if c.finalAuthority != nil {
		return json.Marshal(c.finalAuthority)
	}
	return json.Marshal(c.manifest)
}

func (r *Runtime) aggregateRoot() string {
	return filepath.Join(r.stateDirectory, "cluster-projections")
}

func (r *Runtime) readAggregateContexts(ctx context.Context) ([]aggregateContext, error) {
	return r.readAggregateContextsWithTransactions(ctx, nil)
}

func (r *Runtime) readAggregateContextsWithTransactions(
	ctx context.Context, transactions map[string]*policySourceTransaction,
) ([]aggregateContext, error) {
	list, err := r.ListContexts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]aggregateContext, 0, len(list.Items))
	for _, summary := range list.Items {
		manifest, paths, err := r.resolveContext(summary.Name)
		if err != nil {
			return nil, err
		}
		var policySource policyDataFile
		if transaction := transactions[paths.PolicyDirectory]; transaction != nil {
			journal, exists, journalErr := readPolicySourceJournal(paths.PolicyDirectory)
			if journalErr != nil || !exists || !reflect.DeepEqual(journal, transaction.journal) {
				return nil, fmt.Errorf("Context %q policy transaction changed during aggregate generation", manifest.Name)
			}
			policySource, err = readPolicyDataDuringTransaction(paths.PolicyDirectory)
		} else {
			policySource, err = readPolicyData(paths.PolicyDirectory)
		}
		if err != nil {
			return nil, fmt.Errorf("Context %q policy: %w", manifest.Name, err)
		}
		if err := validateContextPolicyLayout(paths.PolicyDirectory); err != nil {
			return nil, fmt.Errorf("Context %q policy layout: %w", manifest.Name, err)
		}
		policySnapshot, err := r.readContextPolicy(manifest)
		if err != nil {
			return nil, fmt.Errorf("Context %q context policy: %w", manifest.Name, err)
		}
		readiness, err := tobari.ResolveContextNativeReadiness(manifest.NativeReadiness)
		if err != nil {
			return nil, fmt.Errorf("Context %q native readiness selection: %w", manifest.Name, err)
		}
		effectivePolicy, err := tobari.ApplyNativeToolAuthReadiness(readiness == tobari.ManifestNativeReadinessEnabled, true, policySnapshot)
		if err != nil {
			return nil, fmt.Errorf("Context %q native readiness: %w", manifest.Name, err)
		}
		var document map[string]any
		if err := json.Unmarshal(policySource.source, &document); err != nil {
			return nil, err
		}
		contextData, ok := document["tobari"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Context %q policy data has no tobari object", manifest.Name)
		}
		graphqlEndpoints, err := aggregateGraphQLEndpoints(policySource.graphqlEndpoints, effectivePolicy.GraphQLEndpoints)
		if err != nil {
			return nil, fmt.Errorf("Context %q GraphQL endpoints: %w", manifest.Name, err)
		}
		mcpEndpoints, err := aggregateMCPEndpoints(effectivePolicy.MCPEndpoints)
		if err != nil {
			return nil, fmt.Errorf("Context %q MCP endpoints: %w", manifest.Name, err)
		}
		kubernetesEndpoints, err := aggregateKubernetesEndpoints(manifest)
		if err != nil {
			return nil, fmt.Errorf("Context %q Kubernetes endpoint: %w", manifest.Name, err)
		}
		contextData["boundary"] = map[string]any{
			"graphql_endpoints": graphqlEndpoints, "mcp_endpoints": mcpEndpoints,
			"kubernetes_endpoints": kubernetesEndpoints,
		}
		contextData["policy"] = map[string]any{
			"destination_mode": effectivePolicy.DestinationCeiling.Mode, "authorities": effectivePolicy.DestinationCeiling.Authorities,
			"method_default": effectivePolicy.MethodPolicy.Default, "method_overrides": effectivePolicy.MethodPolicy.Overrides,
			"baseline_grants": effectivePolicy.BaselineGrants, "baseline_templates": effectivePolicy.BaselineTemplates,
			"mcp_baseline_grants": effectivePolicy.MCPBaselineGrants, "baseline_denies": effectivePolicy.BaselineDenies,
		}
		items = append(items, aggregateContext{
			contextID: manifest.ID, presentation: manifest.Name,
			manifest: manifest, paths: paths, data: contextData,
			policy:              policySource,
			graphqlEndpoints:    graphqlEndpoints,
			mcpEndpoints:        mcpEndpoints,
			kubernetesEndpoints: kubernetesEndpoints,
			contextPolicy:       effectivePolicy,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].contextID < items[j].contextID })
	return items, nil
}

func aggregateKubernetesEndpoints(manifest tobari.WorkspaceManifest) ([]tobari.GraphQLEndpoint, error) {
	if manifest.Bootstrap == nil || manifest.Bootstrap.EKS == nil {
		return []tobari.GraphQLEndpoint{}, nil
	}
	if err := manifest.Bootstrap.EKS.Validate(); err != nil {
		return nil, err
	}
	endpoint := tobari.GraphQLEndpoint{
		Scheme: "https", Host: strings.TrimPrefix(manifest.Bootstrap.EKS.Server, "https://"), Port: 443, Path: "/",
	}
	if err := endpoint.Validate(); err != nil {
		return nil, err
	}
	return []tobari.GraphQLEndpoint{endpoint}, nil
}

func aggregateGraphQLEndpoints(
	policyEndpoints []tobari.GraphQLEndpoint, contextPolicyEndpoints []tobari.ManifestPolicyExactRule,
) ([]tobari.GraphQLEndpoint, error) {
	seen := make(map[tobari.GraphQLEndpoint]struct{}, len(policyEndpoints)+len(contextPolicyEndpoints))
	result := make([]tobari.GraphQLEndpoint, 0, len(policyEndpoints)+len(contextPolicyEndpoints))
	appendEndpoint := func(endpoint tobari.GraphQLEndpoint) error {
		if err := endpoint.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[endpoint]; duplicate {
			return nil
		}
		seen[endpoint] = struct{}{}
		result = append(result, endpoint)
		return nil
	}
	for _, endpoint := range policyEndpoints {
		if err := appendEndpoint(endpoint); err != nil {
			return nil, err
		}
	}
	for _, endpoint := range contextPolicyEndpoints {
		if endpoint.Method != "POST" {
			return nil, fmt.Errorf("Context policy GraphQL endpoint method must be POST")
		}
		if err := appendEndpoint(tobari.GraphQLEndpoint{
			Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: endpoint.Path,
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		return fmt.Sprintf("%s\x00%s\x00%05d\x00%s", left.Scheme, left.Host, left.Port, left.Path) <
			fmt.Sprintf("%s\x00%s\x00%05d\x00%s", right.Scheme, right.Host, right.Port, right.Path)
	})
	return result, nil
}

func aggregateMCPEndpoints(contextPolicyEndpoints []tobari.ManifestPolicyExactRule) ([]tobari.MCPEndpoint, error) {
	return aggregateFinalMCPEndpoints(nil, contextPolicyEndpoints)
}

func aggregateFinalMCPEndpoints(policyEndpoints []tobari.MCPEndpoint, contextPolicyEndpoints []tobari.ManifestPolicyExactRule) ([]tobari.MCPEndpoint, error) {
	result := make([]tobari.MCPEndpoint, 0, len(policyEndpoints)+len(contextPolicyEndpoints))
	seen := map[tobari.MCPEndpoint]struct{}{}
	for _, value := range policyEndpoints {
		if err := value.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, endpoint := range contextPolicyEndpoints {
		if endpoint.Method != "POST" {
			return nil, fmt.Errorf("Context policy MCP endpoint method must be POST")
		}
		value := tobari.MCPEndpoint{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: endpoint.Path}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return fmt.Sprintf("%s/%s/%05d/%s", result[i].Scheme, result[i].Host, result[i].Port, result[i].Path) < fmt.Sprintf("%s/%s/%05d/%s", result[j].Scheme, result[j].Host, result[j].Port, result[j].Path)
	})
	return result, nil
}

func canonicalEvaluatorModule() ([]byte, error) {
	canonical, err := runtimeassets.Read("opa/policy/tobari.rego")
	if err != nil {
		return nil, err
	}
	if !regoPackagePattern.Match(canonical) {
		return nil, fmt.Errorf("Tobari evaluator must declare package tobari.http")
	}
	if bytes.Contains(canonical, []byte("data.tobari_contexts")) || bytes.Contains(canonical, []byte("package tobari.system")) || bytes.Contains(canonical, []byte("package tobari.contexts")) {
		return nil, fmt.Errorf("Tobari evaluator crosses the reserved routing namespace")
	}
	schemaMatches := regoInputSchemaPattern.FindAllSubmatch(canonical, -1)
	if len(schemaMatches) != 1 || string(schemaMatches[0][1]) != "2" {
		return nil, fmt.Errorf("Tobari evaluator must target source input schema 2")
	}
	return canonical, nil
}

// fixedEvaluatorModule binds the canonical evaluator to the private aggregate
// namespace. The canonical package is kept separately for the evaluator's
// own preflight tests, whose package deliberately exercises that source
// contract before the aggregate namespace rewrite.
func fixedEvaluatorModule() ([]byte, error) {
	canonical, err := canonicalEvaluatorModule()
	if err != nil {
		return nil, err
	}
	transformed := regoPackagePattern.ReplaceAll(canonical, []byte("package tobari.system.guided"))
	transformed = bytes.ReplaceAll(transformed, []byte("data.tobari"), []byte("data.tobari_contexts[input.principal.context_id]"))
	return transformed, nil
}

// aggregateRouter returns the fixed, Tobari-owned routing module. It has no
// Context input: Context membership and policy data are supplied as OPA data,
// so the mounted evaluator bytes cannot vary with user state.
func aggregateRouter() ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("package tobari.http\n\nimport rego.v1\n\n")
	builder.WriteString("default decision := {\"allow\": false, \"reason\": \"unknown or invalid Context authority\", \"status_code\": 403, \"learnable\": false}\n\n")
	builder.WriteString("host_loopback_request if { object.get(input, \"destination\", {}).kind == \"host_loopback\"; input.request.authority.scheme == \"http\"; input.request.authority.host == \"host.tobari.internal\"; input.request.authority.port >= 1024; input.request.authority.port <= 65535 }\n")
	builder.WriteString("host_loopback_identity_valid if { regex.match(\"^att_[0-9a-f]{32}$\", input.destination.attachment_epoch_id) }\n")
	builder.WriteString("attachment_grant_matches(grant) if { grant.lifetime == \"attachment\"; grant.destination_kind == \"host_loopback\"; grant.context_id == input.principal.context_id; grant.project_id == input.principal.project_id; grant.attachment_epoch_id == input.destination.attachment_epoch_id; grant.host == input.request.authority.host; grant.target_port == input.request.authority.port; grant.method == input.request.method; grant.path == input.request.path.raw }\n")
	builder.WriteString("attachment_allowed if { some grant in object.get(input.authorization, \"attachment_grants\", []); grant.decision == \"allow\"; attachment_grant_matches(grant) }\n")
	builder.WriteString("attachment_denied if { some grant in object.get(input.authorization, \"attachment_grants\", []); grant.decision == \"deny\"; attachment_grant_matches(grant) }\n\n")
	builder.WriteString("path_template_request_segment_valid(segment) if { is_string(segment); segment != \"\"; segment != \".\"; segment != \"..\"; not contains(segment, \"\\\\\"); not contains(segment, \"%\") }\n")
	builder.WriteString("path_template_segment_matches(template, actual) if { template == \"{id}\"; path_template_request_segment_valid(actual) }\n")
	builder.WriteString("path_template_segment_matches(template, actual) if { template != \"{id}\"; template == actual }\n")
	builder.WriteString("path_template_matches(template_segments, raw_path) if { is_string(raw_path); startswith(raw_path, \"/\"); parts := split(raw_path, \"/\"); actual_segments := array.slice(parts, 1, count(parts)); count(actual_segments) == count(template_segments); every index, template in template_segments { path_template_segment_matches(template, actual_segments[index]) } }\n\n")
	builder.WriteString("context_policy_destination_allowed if { some authority in data.tobari_contexts[input.principal.context_id].policy.authorities; authority.scheme == input.request.authority.scheme; authority.host == input.request.authority.host; authority.port == input.request.authority.port }\n")
	builder.WriteString("context_policy_method_override_exists if { some override in data.tobari_contexts[input.principal.context_id].policy.method_overrides; override.method == input.request.method }\n")
	builder.WriteString("context_policy_method_decision := override.decision if { some override in data.tobari_contexts[input.principal.context_id].policy.method_overrides; override.method == input.request.method }\n")
	builder.WriteString("context_policy_method_decision := data.tobari_contexts[input.principal.context_id].policy.method_default if { not context_policy_method_override_exists }\n")
	builder.WriteString("terminal_policy if { data.tobari_contexts[input.principal.context_id].policy.destination_mode == \"public_https\"; input.request.authority.scheme != \"https\" }\n")
	builder.WriteString("terminal_policy if { data.tobari_contexts[input.principal.context_id].policy.destination_mode == \"exact\"; not context_policy_destination_allowed }\n")
	builder.WriteString("terminal_policy if { context_policy_method_decision == \"deny\" }\n")
	builder.WriteString("method_policy_granted if { context_policy_method_decision == \"allow\" }\n\n")
	builder.WriteString("context_policy_exact_denied if { some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_denies; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw }\n")
	builder.WriteString("learned_exact_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; rule.protocol == \"http\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw }\n")
	builder.WriteString("learned_graphql_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; some root_field in input.request.graphql.root_fields; rule.protocol == \"graphql\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.graphql_operation_type == input.request.graphql.operation_type; rule.graphql_root_field == root_field }\n")
	builder.WriteString("learned_mcp_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; rule.protocol == \"mcp\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.mcp_method == input.request.mcp.method; object.get(rule, \"mcp_tool_name\", \"\") == object.get(input.request.mcp, \"tool_name\", \"\") }\n")
	builder.WriteString("learned_kubernetes_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; rule.protocol == \"kubernetes\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.kubernetes_verb == input.request.kubernetes.verb; rule.kubernetes_resource == input.request.kubernetes.resource; rule.kubernetes_dry_run == input.request.kubernetes.dry_run }\n")
	builder.WriteString("learned_git_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; rule.protocol == \"git\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.git_service == input.request.git.service; rule.git_repository == input.request.git.repository }\n")
	builder.WriteString("learned_oci_denied if { some rule in data.tobari_contexts[input.principal.context_id].rules.learned_denies; rule.protocol == \"oci\"; rule.scheme == input.request.authority.scheme; rule.context_id == input.principal.context_id; rule.project_id == input.principal.project_id; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.oci_action == input.request.oci.action; rule.oci_repository == input.request.oci.repository; rule.oci_object == input.request.oci.object }\n")
	builder.WriteString("exact_denied if { context_policy_exact_denied }\nexact_denied if { learned_exact_denied }\nexact_denied if { learned_graphql_denied }\nexact_denied if { learned_mcp_denied }\nexact_denied if { learned_kubernetes_denied }\nexact_denied if { learned_git_denied }\nexact_denied if { learned_oci_denied }\n")
	builder.WriteString("context_policy_exact_granted if { object.get(input.request, \"graphql\", null) == null; object.get(input.request, \"mcp\", null) == null; object.get(input.request, \"aws\", null) == null; object.get(input.request, \"kubernetes\", null) == null; object.get(input.request, \"git\", null) == null; object.get(input.request, \"oci\", null) == null; some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants; object.get(rule, \"protocol\", \"http\") == \"http\"; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw }\n")
	builder.WriteString("context_policy_template_granted if { object.get(input.request, \"graphql\", null) == null; object.get(input.request, \"mcp\", null) == null; object.get(input.request, \"aws\", null) == null; object.get(input.request, \"kubernetes\", null) == null; object.get(input.request, \"git\", null) == null; object.get(input.request, \"oci\", null) == null; some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_templates; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; path_template_matches(rule.segments, input.request.path.raw) }\n")
	builder.WriteString("context_policy_graphql_root_granted(root_field) if { some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants; rule.protocol == \"graphql\"; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.graphql_operation_type == input.request.graphql.operation_type; rule.graphql_root_field == root_field }\n")
	builder.WriteString("context_policy_graphql_granted if { is_array(input.request.graphql.root_fields); count(input.request.graphql.root_fields) > 0; every root_field in input.request.graphql.root_fields { context_policy_graphql_root_granted(root_field) } }\n")
	builder.WriteString("context_policy_mcp_granted if { some rule in data.tobari_contexts[input.principal.context_id].policy.mcp_baseline_grants; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.mcp_method == input.request.mcp.method; object.get(rule, \"mcp_tool_name\", \"\") == object.get(input.request.mcp, \"tool_name\", \"\") }\n")
	builder.WriteString("context_policy_granted if { context_policy_exact_granted }\ncontext_policy_granted if { context_policy_template_granted }\ncontext_policy_granted if { context_policy_graphql_granted }\ncontext_policy_granted if { context_policy_mcp_granted }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by attachment policy\", \"status_code\": 403, \"learnable\": false} if { input.schema_version == 2; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; attachment_denied }\n\n")
	builder.WriteString("decision := {\"allow\": true, \"reason\": \"allowed by attachment policy\", \"status_code\": 403, \"learnable\": false} if { input.schema_version == 2; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; not attachment_denied; attachment_allowed }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"Host Loopback requires attachment policy review\", \"status_code\": 403, \"learnable\": true} if { input.schema_version == 2; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; not attachment_denied; not attachment_allowed }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by Context policy ceiling\", \"status_code\": 403, \"learnable\": false} if {\n")
	builder.WriteString("  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  terminal_policy\n}\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by exact policy\", \"status_code\": 403, \"learnable\": false} if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  exact_denied\n}\n\n")
	builder.WriteString("decision := {\"allow\": true, \"reason\": \"allowed by Context policy\", \"status_code\": 403, \"learnable\": false} if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  context_policy_granted\n}\n\n")
	builder.WriteString("decision := result if {\n")
	builder.WriteString("  input.schema_version == 2\n")
	builder.WriteString("  input.principal.cluster == \"default\"\n")
	builder.WriteString("  data.tobari_contexts[input.principal.context_id]\n")
	builder.WriteString("  not host_loopback_request\n")
	builder.WriteString("  not terminal_policy\n")
	builder.WriteString("  not exact_denied\n  not context_policy_granted\n")
	builder.WriteString("  object.get(input.request, \"graphql\", null) != null\n")
	builder.WriteString("  result := data.tobari.system.guided.decision\n")
	builder.WriteString("}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"kubernetes\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"git\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"oci\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 2\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"aws\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString("decision := result if {\n")
	builder.WriteString("  input.schema_version == 2\n")
	builder.WriteString("  input.principal.cluster == \"default\"\n")
	builder.WriteString("  data.tobari_contexts[input.principal.context_id]\n")
	builder.WriteString("  not host_loopback_request\n")
	builder.WriteString("  not terminal_policy\n")
	builder.WriteString("  not exact_denied\n  not context_policy_granted\n")
	builder.WriteString("  object.get(input.request, \"graphql\", null) == null\n")
	builder.WriteString("  object.get(input.request, \"aws\", null) == null\n")
	builder.WriteString("  object.get(input.request, \"kubernetes\", null) == null\n")
	builder.WriteString("  object.get(input.request, \"git\", null) == null\n")
	builder.WriteString("  object.get(input.request, \"oci\", null) == null\n")
	builder.WriteString("  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString(`router_authority_valid if {
  input.schema_version == 2
  input.principal.cluster == "default"
  data.tobari_contexts[input.principal.context_id]
}

router_evidence_available if {
  router_authority_valid
  not host_loopback_request
}

router_evidence_available if {
  router_authority_valid
  host_loopback_request
  host_loopback_identity_valid
}

attachment_rule_refs := sort({grant.id |
  some grant in object.get(input.authorization, "attachment_grants", [])
  is_string(object.get(grant, "id", null))
  attachment_grant_matches(grant)
})

terminal_rule_ref_set contains "destination_ceiling:public_https" if {
  data.tobari_contexts[input.principal.context_id].policy.destination_mode == "public_https"
  input.request.authority.scheme != "https"
}

terminal_rule_ref_set contains "destination_ceiling:exact" if {
  data.tobari_contexts[input.principal.context_id].policy.destination_mode == "exact"
  not context_policy_destination_allowed
}

terminal_rule_ref_set contains ref if {
  context_policy_method_decision == "deny"
  context_policy_method_override_exists
  ref = sprintf("method_override:%s:deny", [input.request.method])
}

terminal_rule_ref_set contains "method_default:deny" if {
  context_policy_method_decision == "deny"
  not context_policy_method_override_exists
}

terminal_rule_refs := sort(terminal_rule_ref_set)

baseline_deny_rule_refs := sort({ref |
  some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_denies
  rule.scheme == input.request.authority.scheme
  rule.host == input.request.authority.host
  rule.port == input.request.authority.port
  rule.method == input.request.method
  rule.path == input.request.path.raw
  ref := sprintf("baseline_deny:%s:%s:%d:%s:%s", [rule.scheme, rule.host, rule.port, rule.method, rule.path])
})

exact_deny_rule_refs := sort(array.concat(baseline_deny_rule_refs, data.tobari.system.guided.matching_learned_deny_rule_refs))

context_policy_rule_ref_set contains ref if {
  some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants
  object.get(rule, "protocol", "http") == "http"
  rule.scheme == input.request.authority.scheme
  rule.host == input.request.authority.host
  rule.port == input.request.authority.port
  rule.method == input.request.method
  rule.path == input.request.path.raw
  ref = sprintf("baseline:http:%s:%s:%d:%s:%s", [rule.scheme, rule.host, rule.port, rule.method, rule.path])
}

context_policy_rule_ref_set contains ref if {
  some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_templates
  rule.scheme == input.request.authority.scheme
  rule.host == input.request.authority.host
  rule.port == input.request.authority.port
  rule.method == input.request.method
  path_template_matches(rule.segments, input.request.path.raw)
  ref = sprintf("baseline:http_template:%s:%s:%d:%s:%s", [rule.scheme, rule.host, rule.port, rule.method, rule.path])
}

context_policy_rule_ref_set contains ref if {
  some root_field in input.request.graphql.root_fields
  some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants
  rule.protocol == "graphql"
  rule.scheme == input.request.authority.scheme
  rule.host == input.request.authority.host
  rule.port == input.request.authority.port
  rule.method == input.request.method
  rule.path == input.request.path.raw
  rule.graphql_operation_type == input.request.graphql.operation_type
  rule.graphql_root_field == root_field
  ref = sprintf("baseline:graphql:%s:%s:%d:%s:%s:%s:%s", [rule.scheme, rule.host, rule.port, rule.method, rule.path, rule.graphql_operation_type, root_field])
}

context_policy_rule_ref_set contains ref if {
  some rule in data.tobari_contexts[input.principal.context_id].policy.mcp_baseline_grants
  rule.scheme == input.request.authority.scheme
  rule.host == input.request.authority.host
  rule.port == input.request.authority.port
  rule.method == input.request.method
  rule.path == input.request.path.raw
  rule.mcp_method == input.request.mcp.method
  object.get(rule, "mcp_tool_name", "") == object.get(input.request.mcp, "tool_name", "")
  ref = sprintf("baseline:mcp:%s:%s:%d:%s:%s:%s:%s", [rule.scheme, rule.host, rule.port, rule.method, rule.path, rule.mcp_method, object.get(rule, "mcp_tool_name", "")])
}

context_policy_rule_refs := sort(context_policy_rule_ref_set)

decision_evidence := {
  "decision": decision,
  "policy_layer": "attachment_deny",
  "rule_refs": attachment_rule_refs,
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": true,
} if {
  router_authority_valid
  host_loopback_request
  host_loopback_identity_valid
  attachment_denied
}

decision_evidence := {
  "decision": decision,
  "policy_layer": "attachment_allow",
  "rule_refs": attachment_rule_refs,
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": true,
} if {
  router_authority_valid
  host_loopback_request
  host_loopback_identity_valid
  not attachment_denied
  attachment_allowed
}

decision_evidence := {
  "decision": decision,
  "policy_layer": "default_posture",
  "rule_refs": [],
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": false,
} if {
  router_authority_valid
  host_loopback_request
  host_loopback_identity_valid
  not attachment_denied
  not attachment_allowed
}

decision_evidence := {
  "decision": decision,
  "policy_layer": "terminal_ceiling",
  "rule_refs": terminal_rule_refs,
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": true,
} if {
  router_authority_valid
  not host_loopback_request
  terminal_policy
}

decision_evidence := {
  "decision": decision,
  "policy_layer": "exact_deny",
  "rule_refs": exact_deny_rule_refs,
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": true,
} if {
  router_authority_valid
  not host_loopback_request
  not terminal_policy
  exact_denied
}

decision_evidence := {
  "decision": decision,
  "policy_layer": "static_allow",
  "rule_refs": context_policy_rule_refs,
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": true,
} if {
  router_authority_valid
  not host_loopback_request
  not terminal_policy
  not exact_denied
  context_policy_granted
}

decision_evidence := data.tobari.system.guided.decision_evidence if {
  router_authority_valid
  not host_loopback_request
  not terminal_policy
  not exact_denied
  not context_policy_granted
}

decision_evidence := {
  "decision": {"allow": false, "reason": "unknown or invalid Context authority", "status_code": 403, "learnable": false},
  "policy_layer": "default_posture",
  "rule_refs": [],
  "semantic_effect": data.tobari.system.guided.semantic_effect,
  "default_overridden": false,
} if {
  not router_evidence_available
}

`)
	builder.WriteString("permission_wait_observation := {\"revision\": data.tobari.aggregate_revision, \"decision\": decision}\n")
	return []byte(builder.String()), nil
}

// fixedAggregateEvaluatorModules is the one source of the Rego bytes mounted
// into an aggregate policy. Identity calculation and materialization both use
// this helper so they cannot silently drift apart.
func fixedAggregateEvaluatorModules() (router, module []byte, err error) {
	router, err = aggregateRouter()
	if err != nil {
		return nil, nil, err
	}
	module, err = fixedEvaluatorModule()
	if err != nil {
		return nil, nil, err
	}
	return router, module, nil
}

func buildAggregateGatewayDocument(items []aggregateContext) map[string]any {
	contexts := make(map[string]any, len(items))
	for _, item := range items {
		item = item.resolvedIdentity()
		contexts[item.contextID] = rewriteGatewayProjection(item)
	}
	return map[string]any{"version": "v2", "contexts": contexts}
}

func rewriteGatewayProjection(item aggregateContext) map[string]any {
	item = item.resolvedIdentity()
	endpoints := append([]tobari.GraphQLEndpoint{}, item.graphqlEndpoints...)
	mcpEndpoints := append([]tobari.MCPEndpoint{}, item.mcpEndpoints...)
	kubernetesEndpoints := append([]tobari.GraphQLEndpoint{}, item.kubernetesEndpoints...)
	return map[string]any{
		"name":                 item.presentation,
		"graphql_endpoints":    endpoints,
		"mcp_endpoints":        mcpEndpoints,
		"kubernetes_endpoints": kubernetesEndpoints,
	}
}

func (r *Runtime) buildAggregateProjection(ctx context.Context) (aggregateProjection, error) {
	return r.buildAggregateProjectionWithTransactions(ctx, nil)
}

func (r *Runtime) buildAggregateProjectionWithTransactions(
	ctx context.Context, transactions map[string]*policySourceTransaction,
) (aggregateProjection, error) {
	items, err := r.readAggregateContextsWithTransactions(ctx, transactions)
	if err != nil {
		return aggregateProjection{}, err
	}
	if err := r.ensurePrivateDirectory(r.aggregateRoot()); err != nil {
		return aggregateProjection{}, err
	}
	validationReused := false
	for _, item := range items {
		preflight, err := prepareContextPolicyPreflight(r.aggregateRoot(), item.manifest, item.paths.PolicyDirectory, item.policy)
		if err != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy preflight: %w", item.manifest.Name, err)
		}
		preflightDigest, digestErr := policyPreflightDigest(preflight)
		if digestErr != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy preflight digest: %w", item.manifest.Name, digestErr)
		}
		transaction := transactions[item.paths.PolicyDirectory]
		reuseValidation := !validationReused && transaction != nil && transaction.consumeCandidateValidation(
			item.paths.PolicyDirectory, policySourceDigest(item.policy.sources), preflightDigest,
		)
		var testErr error
		if reuseValidation {
			validationReused = true
		} else {
			testErr = r.testPolicyPreflight(ctx, preflight)
		}
		if testErr != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy tests: %w", item.manifest.Name, testErr)
		}
	}
	return r.materializeAggregateProjection(ctx, items, func() error {
		return verifyAggregatePolicySources(items, transactions)
	})
}

func (r *Runtime) materializeAggregateProjection(ctx context.Context, items []aggregateContext, verify func() error) (aggregateProjection, error) {
	for index := range items {
		items[index] = items[index].resolvedIdentity()
	}
	revision, err := aggregateRevision(items)
	if err != nil {
		return aggregateProjection{}, err
	}
	evaluatorIdentity, policyDataIdentity, err := aggregateIdentities(items)
	if err != nil {
		return aggregateProjection{}, err
	}
	dataContexts := map[string]any{}
	for _, item := range items {
		item = item.resolvedIdentity()
		if err := item.validateIdentity(); err != nil {
			return aggregateProjection{}, err
		}
		dataContexts[item.contextID] = item.data
	}
	directory := filepath.Join(r.aggregateRoot(), revision)
	result := aggregateProjection{
		Revision: revision, PolicyDirectory: filepath.Join(directory, "policy"),
		GatewayConfig: filepath.Join(directory, "gateway.json"), ManifestCount: len(items),
		EvaluatorIdentity: evaluatorIdentity, PolicyDataIdentity: policyDataIdentity,
	}
	if _, err := os.Lstat(directory); err == nil {
		if err := verifyAggregatePolicyDirectory(result.PolicyDirectory, items, result); err != nil {
			return aggregateProjection{}, fmt.Errorf("verify existing aggregate policy: %w", err)
		}
		if err := r.testPolicyDirectory(ctx, result.PolicyDirectory); err != nil {
			return aggregateProjection{}, fmt.Errorf("validate existing aggregate policy: %w", err)
		}
		if verify != nil {
			if err := verify(); err != nil {
				return aggregateProjection{}, err
			}
		}
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return aggregateProjection{}, err
	}
	if err := r.ensurePrivateDirectory(r.aggregateRoot()); err != nil {
		return aggregateProjection{}, err
	}
	temporary, err := os.MkdirTemp(r.aggregateRoot(), ".candidate-")
	if err != nil {
		return aggregateProjection{}, err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil { // #nosec G302 -- candidate projection is an owner-only directory.
		return aggregateProjection{}, err
	}
	policyDirectory := filepath.Join(temporary, "policy")
	if err := os.MkdirAll(policyDirectory, 0o700); err != nil {
		return aggregateProjection{}, err
	}
	dataDocument := map[string]any{"tobari_contexts": dataContexts, "tobari": map[string]any{
		"aggregate_schema_version": aggregateSchemaVersion,
		"aggregate_revision":       revision,
		"evaluator_identity":       evaluatorIdentity,
		"policy_data_identity":     policyDataIdentity,
	}}
	if err := writeAtomicJSON(filepath.Join(policyDirectory, "data.json"), dataDocument); err != nil {
		return aggregateProjection{}, err
	}
	gatewayDocument := buildAggregateGatewayDocument(items)
	if err := writeAtomicJSON(filepath.Join(temporary, "gateway.json"), gatewayDocument); err != nil {
		return aggregateProjection{}, err
	}
	candidatePolicy := filepath.Join(temporary, "policy")
	candidate := result
	candidate.PolicyDirectory = candidatePolicy
	candidate.GatewayConfig = filepath.Join(temporary, "gateway.json")
	if err := verifyAggregatePolicyDirectory(candidatePolicy, items, candidate); err != nil {
		return aggregateProjection{}, fmt.Errorf("verify aggregate policy candidate: %w", err)
	}
	if err := r.testPolicyDirectory(ctx, candidatePolicy); err != nil {
		return aggregateProjection{}, fmt.Errorf("validate aggregate policy: %w", err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return aggregateProjection{}, err
		}
	}
	if err := os.Rename(temporary, directory); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return aggregateProjection{}, err
		}
	}
	return result, nil
}

// aggregateRevision is the single content identity used by both projection
// construction and read-only freshness inspection. Items are already ordered
// by immutable Context ID by readAggregateContextsWithTransactions.
func aggregateRevision(items []aggregateContext) (string, error) {
	evaluatorIdentity, policyDataIdentity, err := aggregateIdentities(items)
	if err != nil {
		return "", err
	}
	return aggregateRevisionForIdentities(items, evaluatorIdentity, policyDataIdentity)
}

func aggregateRevisionForIdentities(items []aggregateContext, evaluatorIdentity tobari.PolicyEvaluatorIdentity, policyDataIdentity tobari.PolicyDataIdentity) (string, error) {
	hash := sha256.New()
	identityBytes, err := json.Marshal(struct {
		Evaluator  tobari.PolicyEvaluatorIdentity
		PolicyData tobari.PolicyDataIdentity
	}{evaluatorIdentity, policyDataIdentity})
	if err != nil {
		return "", err
	}
	hash.Write(identityBytes)
	hash.Write([]byte{0})
	for _, item := range items {
		authorityBytes, err := item.authorityBytes()
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(item.data)
		if err != nil {
			return "", err
		}
		hash.Write(authorityBytes)
		hash.Write([]byte{0})
		hash.Write(encoded)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func aggregateIdentities(items []aggregateContext) (tobari.PolicyEvaluatorIdentity, tobari.PolicyDataIdentity, error) {
	evaluatorBytes, err := aggregateEvaluatorMaterial()
	if err != nil {
		return tobari.PolicyEvaluatorIdentity{}, tobari.PolicyDataIdentity{}, err
	}
	evaluator := policyEvaluatorIdentityForBytes(evaluatorBytes)
	if err := evaluator.Validate(); err != nil {
		return tobari.PolicyEvaluatorIdentity{}, tobari.PolicyDataIdentity{}, err
	}
	data := make([]aggregatePolicyDataEntry, 0, len(items))
	for _, item := range items {
		item = item.resolvedIdentity()
		data = append(data, aggregatePolicyDataEntry{ContextID: item.contextID, Data: item.data})
	}
	policyData, err := policyDataIdentityForEntries(data)
	if err != nil {
		return tobari.PolicyEvaluatorIdentity{}, tobari.PolicyDataIdentity{}, err
	}
	return evaluator, policyData, nil
}

func policyDataIdentityForEntries(entries []aggregatePolicyDataEntry) (tobari.PolicyDataIdentity, error) {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return tobari.PolicyDataIdentity{}, err
	}
	canonical, err := canonicalJSONBytes(encoded)
	if err != nil {
		return tobari.PolicyDataIdentity{}, err
	}
	digest := sha256.Sum256(canonical)
	identity := tobari.PolicyDataIdentity{
		SchemaVersion: 1,
		Digest:        tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])),
	}
	if err := identity.Validate(); err != nil {
		return tobari.PolicyDataIdentity{}, err
	}
	return identity, nil
}

func policyDataIdentityForProjectedContexts(contexts map[string]json.RawMessage) (tobari.PolicyDataIdentity, error) {
	entries, err := aggregatePolicyDataEntriesForProjectedContexts(contexts)
	if err != nil {
		return tobari.PolicyDataIdentity{}, err
	}
	return policyDataIdentityForEntries(entries)
}

func aggregatePolicyDataEntriesForProjectedContexts(contexts map[string]json.RawMessage) ([]aggregatePolicyDataEntry, error) {
	ids := make([]string, 0, len(contexts))
	for contextID := range contexts {
		ids = append(ids, contextID)
	}
	sort.Strings(ids)
	entries := make([]aggregatePolicyDataEntry, 0, len(ids))
	for _, contextID := range ids {
		var data map[string]any
		if err := json.Unmarshal(contexts[contextID], &data); err != nil || data == nil {
			if err == nil {
				err = fmt.Errorf("Context data must be an object")
			}
			return nil, fmt.Errorf("Context %q data: %w", contextID, err)
		}
		entries = append(entries, aggregatePolicyDataEntry{ContextID: contextID, Data: data})
	}
	return entries, nil
}

// aggregateEvaluatorMaterial binds the identity to the complete fixed
// evaluator that is actually mounted for aggregate authorization: the
// embedded module plus the Tobari-owned router generated by this binary.
// Including both makes a binary evaluator change visible even when canonical
// policy data is unchanged.
func aggregateEvaluatorMaterial() ([]byte, error) {
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		return nil, err
	}
	return aggregateEvaluatorMaterialForModules(router, module), nil
}

func aggregateEvaluatorMaterialForModules(router, module []byte) []byte {
	var material bytes.Buffer
	for _, item := range []struct {
		name     string
		contents []byte
	}{
		{name: "router.rego", contents: router},
		{name: "guided.rego", contents: module},
	} {
		material.WriteString(item.name)
		material.WriteByte(0)
		material.Write(item.contents)
		material.WriteByte(0)
	}
	return material.Bytes()
}

func decodeAggregateJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("JSON contains trailing data")
		}
		return err
	}
	return nil
}

func canonicalJSONBytes(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("JSON contains trailing data")
		}
		return nil, err
	}
	return json.Marshal(value)
}

// verifyAggregatePolicyDirectory is the read-only integrity boundary for an
// aggregate candidate and the active/diagnostic projection. Host state owns
// only typed aggregate data; fixed evaluator modules are assembled from the
// embedded binary at the Docker bundle boundary. This verifies the exact host
// data set, actual projected typed data, fixed evaluator identity, metadata,
// revision, and the sibling Gateway projection together.
func verifyAggregatePolicyDirectory(
	policyDirectory string, items []aggregateContext, expected aggregateProjection,
) error {
	if expected.PolicyDirectory != "" && filepath.Clean(expected.PolicyDirectory) != filepath.Clean(policyDirectory) {
		return fmt.Errorf("aggregate policy path does not match expected projection")
	}
	if expected.GatewayConfig == "" || filepath.Clean(expected.GatewayConfig) != filepath.Join(filepath.Dir(policyDirectory), "gateway.json") {
		return fmt.Errorf("aggregate Gateway path does not match policy projection")
	}
	if err := requirePrivateDirectory(filepath.Dir(policyDirectory)); err != nil {
		return fmt.Errorf("aggregate revision directory: %w", err)
	}
	if err := requirePrivateDirectory(policyDirectory); err != nil {
		return fmt.Errorf("aggregate policy directory: %w", err)
	}
	revisionDirectory := filepath.Dir(policyDirectory)
	revisionEntries, err := os.ReadDir(revisionDirectory)
	if err != nil {
		return fmt.Errorf("read aggregate revision directory: %w", err)
	}
	if len(revisionEntries) != 2 {
		return fmt.Errorf("aggregate revision directory contains unexpected entries")
	}
	for _, entry := range revisionEntries {
		if entry.Name() != "policy" && entry.Name() != "gateway.json" {
			return fmt.Errorf("aggregate revision contains unexpected entry %q", entry.Name())
		}
		path := filepath.Join(revisionDirectory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if entry.Name() == "policy" {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("aggregate policy revision path is unsafe")
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("aggregate Gateway projection is unsafe")
		}
	}
	entries, err := os.ReadDir(policyDirectory)
	if err != nil {
		return err
	}
	allowed := map[string]struct{}{"data.json": {}}
	if len(entries) != len(allowed) {
		return fmt.Errorf("aggregate policy data set is incomplete or contains executable extras")
	}
	for _, entry := range entries {
		if _, ok := allowed[entry.Name()]; !ok {
			return fmt.Errorf("aggregate policy contains unexpected file %q", entry.Name())
		}
		info, err := os.Lstat(filepath.Join(policyDirectory, entry.Name()))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("aggregate policy file %q is unsafe", entry.Name())
		}
	}

	expectedRouter, expectedModule, err := fixedAggregateEvaluatorModules()
	if err != nil {
		return err
	}
	material := aggregateEvaluatorMaterialForModules(expectedRouter, expectedModule)
	currentEvaluator := policyEvaluatorIdentityForBytes(material)
	if expected.EvaluatorIdentity != currentEvaluator {
		return fmt.Errorf("expected aggregate evaluator identity is not current fixed evaluator")
	}

	dataBytes, err := readOwnerPolicyFile(filepath.Join(policyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil {
		return err
	}
	if err := validateNoDuplicateJSONKeys(dataBytes); err != nil {
		return fmt.Errorf("aggregate data has duplicate keys: %w", err)
	}
	var document aggregatePolicyDataDocument
	if err := decodeAggregateJSON(dataBytes, &document); err != nil {
		return fmt.Errorf("decode aggregate data: %w", err)
	}
	if document.Tobari.AggregateSchemaVersion != aggregateSchemaVersion ||
		document.Tobari.AggregateRevision != expected.Revision ||
		document.Tobari.EvaluatorIdentity != expected.EvaluatorIdentity ||
		document.Tobari.PolicyDataIdentity != expected.PolicyDataIdentity {
		return fmt.Errorf("aggregate data identity metadata does not match expected projection")
	}
	if err := document.Tobari.EvaluatorIdentity.Validate(); err != nil {
		return fmt.Errorf("aggregate data evaluator identity: %w", err)
	}
	if err := document.Tobari.PolicyDataIdentity.Validate(); err != nil {
		return fmt.Errorf("aggregate data policy-data identity: %w", err)
	}
	if expected.ManifestCount != 0 && expected.ManifestCount != len(items) {
		return fmt.Errorf("aggregate Context count does not match expected projection")
	}
	expectedEvaluator, expectedData, err := aggregateIdentities(items)
	if err != nil {
		return err
	}
	if expectedEvaluator != expected.EvaluatorIdentity || expectedData != expected.PolicyDataIdentity {
		return fmt.Errorf("expected aggregate identities do not match Context projection")
	}
	revision, err := aggregateRevisionForIdentities(items, expected.EvaluatorIdentity, expected.PolicyDataIdentity)
	if err != nil {
		return err
	}
	if revision != expected.Revision {
		return fmt.Errorf("aggregate revision does not match Context projection")
	}
	if len(document.Contexts) != len(items) {
		return fmt.Errorf("aggregate Context data count does not match expected projection")
	}
	for _, item := range items {
		item = item.resolvedIdentity()
		if err := item.validateIdentity(); err != nil {
			return err
		}
		actual, ok := document.Contexts[item.contextID]
		if !ok {
			return fmt.Errorf("aggregate Context data is missing %q", item.contextID)
		}
		expectedBytes, err := json.Marshal(item.data)
		if err != nil {
			return err
		}
		actualCanonical, err := canonicalJSONBytes(actual)
		if err != nil {
			return fmt.Errorf("aggregate Context %q data: %w", item.contextID, err)
		}
		expectedCanonical, err := canonicalJSONBytes(expectedBytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(actualCanonical, expectedCanonical) {
			return fmt.Errorf("aggregate Context %q data drifted", item.contextID)
		}
	}
	actualEntries, err := aggregatePolicyDataEntriesForProjectedContexts(document.Contexts)
	if err != nil {
		return err
	}
	actualDataIdentity, err := policyDataIdentityForEntries(actualEntries)
	if err != nil {
		return err
	}
	if actualDataIdentity != expected.PolicyDataIdentity {
		return fmt.Errorf("aggregate projected Context data does not match policy-data identity: actual=%+v expected=%+v", actualDataIdentity, expected.PolicyDataIdentity)
	}

	gatewayPath := filepath.Join(filepath.Dir(policyDirectory), "gateway.json")
	gatewayBytes, err := readOwnerPolicyFile(gatewayPath, maxPolicyPreflight)
	if err != nil {
		return fmt.Errorf("read aggregate Gateway projection: %w", err)
	}
	if err := validateNoDuplicateJSONKeys(gatewayBytes); err != nil {
		return fmt.Errorf("aggregate Gateway projection has duplicate keys: %w", err)
	}
	var gatewayDocument aggregateGatewayProjectionDocument
	if err := decodeAggregateJSON(gatewayBytes, &gatewayDocument); err != nil {
		return fmt.Errorf("decode aggregate Gateway projection: %w", err)
	}
	expectedGatewayBytes, err := json.Marshal(buildAggregateGatewayDocument(items))
	if err != nil {
		return err
	}
	actualGatewayCanonical, err := canonicalJSONBytes(gatewayBytes)
	if err != nil {
		return err
	}
	expectedGatewayCanonical, err := canonicalJSONBytes(expectedGatewayBytes)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualGatewayCanonical, expectedGatewayCanonical) {
		return fmt.Errorf("aggregate Gateway projection drifted")
	}
	return nil
}

// validatePersistedAggregateTarget binds a persisted aggregate target to the
// exact revision directory under this Runtime's private aggregate root. The
// resolved-path check rejects a revision/policy/Gateway symlink even when its
// lexical path is canonical.
func (r *Runtime) validatePersistedAggregateTarget(
	policyDirectory, gatewayConfig, revision string,
) error {
	if !aggregateRevisionPattern.MatchString(revision) {
		return fmt.Errorf("aggregate revision is invalid")
	}
	root := r.aggregateRoot()
	revisionDirectory := filepath.Join(root, revision)
	if policyDirectory != filepath.Join(revisionDirectory, "policy") ||
		gatewayConfig != filepath.Join(revisionDirectory, "gateway.json") {
		return fmt.Errorf("aggregate target does not match the owned revision")
	}
	if err := requirePrivateDirectory(root); err != nil {
		return fmt.Errorf("aggregate root: %w", err)
	}
	if err := requirePrivateDirectory(revisionDirectory); err != nil {
		return fmt.Errorf("aggregate revision directory: %w", err)
	}
	if err := requirePrivateDirectory(policyDirectory); err != nil {
		return fmt.Errorf("aggregate policy directory: %w", err)
	}
	if err := requireOwnerOnlyAggregateFile(gatewayConfig); err != nil {
		return fmt.Errorf("aggregate Gateway projection: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve aggregate root: %w", err)
	}
	for name, path := range map[string]string{
		"revision": revisionDirectory, "policy": policyDirectory, "Gateway": gatewayConfig,
	} {
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil || resolved != filepath.Join(rootReal, map[string]string{
			"revision": revision,
			"policy":   filepath.Join(revision, "policy"),
			"Gateway":  filepath.Join(revision, "gateway.json"),
		}[name]) {
			return fmt.Errorf("aggregate %s target has an unexpected symlink or parent", name)
		}
	}
	return nil
}

func requireOwnerOnlyAggregateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("path is not an owner-only regular file")
	}
	return nil
}

// verifyPersistedAggregateState is the state recovery/publication boundary.
// It checks the state identities against current typed Context data before the
// complete byte/data/Gateway verifier is allowed to authorize the target.
func (r *Runtime) verifyPersistedAggregateState(
	ctx context.Context, state tobari.State,
) ([]aggregateContext, aggregateProjection, error) {
	if err := state.ValidateAggregateProjectionRoot(r.aggregateRoot()); err != nil {
		return nil, aggregateProjection{}, err
	}
	if err := r.validatePersistedAggregateTarget(state.PolicyDirectory, state.GatewayConfig, state.AggregateRevision); err != nil {
		return nil, aggregateProjection{}, err
	}
	items, err := r.readAggregateContexts(ctx)
	if err != nil {
		if r.pendingPolicySourceTransactionExists() {
			if metadataErr := verifyAggregatePolicyDirectoryMetadata(
				state.PolicyDirectory, state.GatewayConfig, state.AggregateRevision,
				state.EvaluatorIdentity, state.PolicyDataIdentity, state.ManifestCount,
			); metadataErr != nil {
				return nil, aggregateProjection{}, metadataErr
			}
			return nil, aggregateProjection{
				Revision: state.AggregateRevision, PolicyDirectory: state.PolicyDirectory,
				GatewayConfig: state.GatewayConfig, ManifestCount: state.ManifestCount,
				EvaluatorIdentity: state.EvaluatorIdentity, PolicyDataIdentity: state.PolicyDataIdentity,
			}, nil
		}
		return nil, aggregateProjection{}, err
	}
	if len(items) != state.ManifestCount {
		return nil, aggregateProjection{}, fmt.Errorf("aggregate Context count does not match persisted state")
	}
	evaluatorIdentity, policyDataIdentity, err := aggregateIdentities(items)
	if err != nil {
		return nil, aggregateProjection{}, err
	}
	if state.EvaluatorIdentity != evaluatorIdentity || state.PolicyDataIdentity != policyDataIdentity {
		return nil, aggregateProjection{}, fmt.Errorf("persisted state aggregate identities do not match current Context data")
	}
	revision, err := aggregateRevisionForIdentities(items, evaluatorIdentity, policyDataIdentity)
	if err != nil {
		return nil, aggregateProjection{}, err
	}
	if revision != state.AggregateRevision {
		return nil, aggregateProjection{}, fmt.Errorf("persisted state aggregate revision does not match current Context data")
	}
	expected := aggregateProjection{
		Revision: state.AggregateRevision, PolicyDirectory: state.PolicyDirectory,
		GatewayConfig: state.GatewayConfig, ManifestCount: state.ManifestCount,
		EvaluatorIdentity: evaluatorIdentity, PolicyDataIdentity: policyDataIdentity,
	}
	if err := verifyAggregatePolicyDirectory(state.PolicyDirectory, items, expected); err != nil {
		return nil, aggregateProjection{}, err
	}
	return items, expected, nil
}

// verifyKnownGoodAggregateState validates a retained State for rollback. A
// failed forward reconciliation may have produced a newer canonical Context
// projection while the last-successful aggregate remains the only safe
// rollback target. The retained artifact therefore verifies itself (including
// actual projected data and the sibling Gateway projection) rather than being
// compared with the newer desired Context data.
func (r *Runtime) verifyKnownGoodAggregateState(
	ctx context.Context, state tobari.State,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := state.ValidateAggregateProjectionRoot(r.aggregateRoot()); err != nil {
		return err
	}
	if err := r.validatePersistedAggregateTarget(state.PolicyDirectory, state.GatewayConfig, state.AggregateRevision); err != nil {
		return err
	}
	if err := verifyAggregatePolicyDirectoryMetadata(
		state.PolicyDirectory, state.GatewayConfig, state.AggregateRevision,
		state.EvaluatorIdentity, state.PolicyDataIdentity, state.ManifestCount,
	); err != nil {
		return err
	}
	return nil
}

// validateRetainedAggregateBeforeClusterUp is the upgrade boundary for a
// schema-2 state written before the fixed-evaluator cutover. Such a state has
// no evaluator/data identities and its retained aggregate may contain the
// predecessor host Rego layout. We do not decode or execute that source, and
// we do not create a rollback snapshot to make it appear current. Instead the
// state is rejected before a reconcile journal or any live Docker mutation.
//
// The pre-platform profile is a separate, already-reviewed schema-1/2
// lifecycle and is deliberately handled by its existing migration path.
func (r *Runtime) validateRetainedAggregateBeforeClusterUp(
	ctx context.Context, state tobari.State,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if state.SchemaVersion != 2 || state.Applied.PermissionProfile == tobari.SharedClusterProfilePrePlatform {
		return nil
	}
	if state.EvaluatorIdentity != (tobari.PolicyEvaluatorIdentity{}) ||
		state.PolicyDataIdentity != (tobari.PolicyDataIdentity{}) {
		return r.verifyKnownGoodAggregateState(ctx, state)
	}
	if err := state.ValidateAggregateProjectionRoot(r.aggregateRoot()); err != nil {
		return err
	}
	if err := r.validatePersistedAggregateTarget(state.PolicyDirectory, state.GatewayConfig, state.AggregateRevision); err != nil {
		return fmt.Errorf("legacy aggregate target is unavailable: %w", err)
	}
	advanced, err := r.legacyAggregateContainsAdvancedAuthority(state)
	if err != nil {
		return fmt.Errorf("inspect legacy aggregate authority: %w", err)
	}
	if advanced {
		return fmt.Errorf("%w: %w: persisted legacy Advanced policy is unsupported; reset or recreate the shared cluster", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy)
	}
	return fmt.Errorf("%w: %w: persisted schema-2 Guided aggregate predates the fixed evaluator; reset or recreate the shared cluster", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy)
}

// legacyAggregateContainsAdvancedAuthority is a non-authorizing detector. It
// only observes bounded JSON keys and the predecessor module-name shape; it
// never deserializes Rego or constructs a current executable-policy value.
func (r *Runtime) legacyAggregateContainsAdvancedAuthority(state tobari.State) (bool, error) {
	data, err := readOwnerPolicyFile(filepath.Join(state.PolicyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil {
		return false, err
	}
	advanced, err := jsonContainsLegacyAdvancedMarker(data)
	if err != nil {
		return false, err
	}
	if advanced {
		return true, nil
	}
	entries, err := os.ReadDir(state.PolicyDirectory)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".rego") && entry.Name() != "router.rego" && entry.Name() != "guided.rego" {
			return true, nil
		}
	}
	return r.contextTreeContainsLegacyAdvancedAuthority()
}

func (r *Runtime) contextTreeContainsLegacyAdvancedAuthority() (bool, error) {
	_, err := os.ReadDir(r.contextsDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	visited := 0
	advanced := false
	err = filepath.WalkDir(r.contextsDirectory(), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != r.contextsDirectory() {
			visited++
			if visited > maxPolicyFiles*4 {
				return fmt.Errorf("Context legacy authority contains too many entries")
			}
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".rego") {
			advanced = true
			return filepath.SkipAll
		}
		if entry.Name() != "context.json" {
			return nil
		}
		data, readErr := readContextManifestFile(path)
		if readErr != nil {
			return readErr
		}
		marker, markerErr := jsonContainsLegacyAdvancedMarker(data)
		if markerErr != nil {
			return markerErr
		}
		if marker {
			advanced = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return advanced, nil
}

func jsonContainsLegacyAdvancedMarker(data []byte) (bool, error) {
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return false, fmt.Errorf("JSON contains trailing data")
		}
		return false, err
	}
	return legacyAdvancedMarkerValue(value), nil
}

func legacyAdvancedMarkerValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			switch key {
			case "advanced_policy":
				if nested != nil {
					return true
				}
			case "policy_mode", "mode":
				if mode, ok := nested.(string); ok && mode == "advanced" {
					return true
				}
			}
			if legacyAdvancedMarkerValue(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if legacyAdvancedMarkerValue(nested) {
				return true
			}
		}
	}
	return false
}

// verifyAggregateTargetForRevision repeats the complete read-only check at
// the archive/build boundary. Reconciliation can observe a filesystem change
// between its earlier state check and Docker publication, so existence or
// remembered metadata is never reused as proof.
func (r *Runtime) verifyAggregateTargetForRevision(
	ctx context.Context, policyDirectory, revision string,
) error {
	gatewayConfig := filepath.Join(filepath.Dir(policyDirectory), "gateway.json")
	if err := r.validatePersistedAggregateTarget(policyDirectory, gatewayConfig, revision); err != nil {
		return err
	}
	items, err := r.readAggregateContexts(ctx)
	if err != nil {
		if r.pendingPolicySourceTransactionExists() {
			return verifyAggregatePolicyDirectoryMetadata(
				policyDirectory, gatewayConfig, revision, tobari.PolicyEvaluatorIdentity{}, tobari.PolicyDataIdentity{}, -1,
			)
		}
		return err
	}
	evaluatorIdentity, policyDataIdentity, err := aggregateIdentities(items)
	if err != nil {
		return err
	}
	if desired, revisionErr := aggregateRevisionForIdentities(items, evaluatorIdentity, policyDataIdentity); revisionErr != nil {
		return revisionErr
	} else if desired != revision {
		return fmt.Errorf("aggregate target revision does not match current Context data")
	}
	expected := aggregateProjection{
		Revision: revision, PolicyDirectory: policyDirectory, GatewayConfig: gatewayConfig,
		ManifestCount: len(items), EvaluatorIdentity: evaluatorIdentity, PolicyDataIdentity: policyDataIdentity,
	}
	return verifyAggregatePolicyDirectory(policyDirectory, items, expected)
}

func verifyAggregatePolicyDirectoryMetadata(
	policyDirectory, gatewayConfig, revision string,
	evaluatorIdentity tobari.PolicyEvaluatorIdentity, policyDataIdentity tobari.PolicyDataIdentity,
	manifestCount int,
) error {
	if filepath.Clean(gatewayConfig) != filepath.Join(filepath.Dir(policyDirectory), "gateway.json") {
		return fmt.Errorf("aggregate Gateway path does not match policy projection")
	}
	revisionEntries, err := os.ReadDir(filepath.Dir(policyDirectory))
	if err != nil || len(revisionEntries) != 2 {
		return fmt.Errorf("aggregate revision directory contains unexpected entries")
	}
	for _, entry := range revisionEntries {
		if entry.Name() != "policy" && entry.Name() != "gateway.json" {
			return fmt.Errorf("aggregate revision contains unexpected entry %q", entry.Name())
		}
	}
	entries, err := os.ReadDir(policyDirectory)
	if err != nil || len(entries) != 1 {
		return fmt.Errorf("aggregate policy data set is incomplete or contains executable extras")
	}
	for _, entry := range entries {
		if entry.Name() != "data.json" {
			return fmt.Errorf("aggregate policy contains unexpected file %q", entry.Name())
		}
	}
	expectedRouter, expectedModule, err := fixedAggregateEvaluatorModules()
	if err != nil {
		return err
	}
	currentEvaluator := policyEvaluatorIdentityForBytes(aggregateEvaluatorMaterialForModules(expectedRouter, expectedModule))
	dataBytes, err := readOwnerPolicyFile(filepath.Join(policyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil {
		return err
	}
	if err := validateNoDuplicateJSONKeys(dataBytes); err != nil {
		return err
	}
	var document aggregatePolicyDataDocument
	if err := decodeAggregateJSON(dataBytes, &document); err != nil {
		return err
	}
	if document.Tobari.AggregateSchemaVersion != aggregateSchemaVersion ||
		document.Tobari.AggregateRevision != revision || document.Tobari.EvaluatorIdentity != currentEvaluator {
		return fmt.Errorf("aggregate data evaluator or revision metadata drifted")
	}
	if evaluatorIdentity != (tobari.PolicyEvaluatorIdentity{}) && document.Tobari.EvaluatorIdentity != evaluatorIdentity {
		return fmt.Errorf("aggregate data evaluator identity does not match state")
	}
	actualEntries, err := aggregatePolicyDataEntriesForProjectedContexts(document.Contexts)
	if err != nil {
		return err
	}
	actualDataIdentity, err := policyDataIdentityForEntries(actualEntries)
	if err != nil {
		return err
	}
	if actualDataIdentity != document.Tobari.PolicyDataIdentity {
		return fmt.Errorf("aggregate projected Context data identity drifted")
	}
	if policyDataIdentity != (tobari.PolicyDataIdentity{}) && actualDataIdentity != policyDataIdentity {
		return fmt.Errorf("aggregate projected Context data identity does not match state")
	}
	if manifestCount >= 0 && len(document.Contexts) != manifestCount {
		return fmt.Errorf("aggregate Context data count does not match state")
	}
	gatewayBytes, err := readOwnerPolicyFile(gatewayConfig, maxPolicyPreflight)
	if err != nil {
		return err
	}
	if err := validateNoDuplicateJSONKeys(gatewayBytes); err != nil {
		return err
	}
	return verifyAggregateGatewayProjectionData(gatewayBytes, document.Contexts)
}

// verifyAggregateGatewayProjectionData binds the sibling Gateway endpoint
// projection to the same canonical Context data used by the evaluator. Names
// remain presentation-only; every endpoint list that controls protocol
// classification must match the generated boundary data byte-canonically.
func verifyAggregateGatewayProjectionData(
	gatewayBytes []byte, contexts map[string]json.RawMessage,
) error {
	var gateway aggregateGatewayProjectionDocument
	if err := decodeAggregateJSON(gatewayBytes, &gateway); err != nil || gateway.Version != "v2" || gateway.Contexts == nil {
		return fmt.Errorf("aggregate Gateway projection is invalid")
	}
	if len(gateway.Contexts) != len(contexts) {
		return fmt.Errorf("aggregate Gateway Context count does not match policy data")
	}
	for contextID, contextBytes := range contexts {
		gatewayContextBytes, ok := gateway.Contexts[contextID]
		if !ok {
			return fmt.Errorf("aggregate Gateway projection is missing Context %q", contextID)
		}
		var contextData struct {
			SchemaVersion int             `json:"schema_version"`
			Boundary      json.RawMessage `json:"boundary"`
			Rules         json.RawMessage `json:"rules"`
			Policy        json.RawMessage `json:"policy"`
		}
		if err := decodeAggregateJSON(contextBytes, &contextData); err != nil || contextData.SchemaVersion != policySchemaVersion ||
			len(contextData.Boundary) == 0 || len(contextData.Rules) == 0 || len(contextData.Policy) == 0 {
			return fmt.Errorf("aggregate Context %q boundary projection is invalid", contextID)
		}
		var boundary aggregateContextBoundaryDocument
		if err := decodeAggregateJSON(contextData.Boundary, &boundary); err != nil {
			return fmt.Errorf("aggregate Context %q boundary projection: %w", contextID, err)
		}
		var projected aggregateGatewayContextDocument
		if err := decodeAggregateJSON(gatewayContextBytes, &projected); err != nil || projected.Name == "" ||
			len(projected.GraphQLEndpoints) == 0 || len(projected.MCPEndpoints) == 0 || len(projected.KubernetesEndpoints) == 0 {
			return fmt.Errorf("aggregate Gateway Context %q projection is invalid", contextID)
		}
		for field, pair := range map[string]struct {
			expected []byte
			actual   []byte
		}{
			"graphql_endpoints":    {expected: boundary.GraphQLEndpoints, actual: projected.GraphQLEndpoints},
			"mcp_endpoints":        {expected: boundary.MCPEndpoints, actual: projected.MCPEndpoints},
			"kubernetes_endpoints": {expected: boundary.KubernetesEndpoints, actual: projected.KubernetesEndpoints},
		} {
			expectedCanonical, err := canonicalJSONBytes(pair.expected)
			if err != nil {
				return fmt.Errorf("aggregate Context %q boundary %s: %w", contextID, field, err)
			}
			actualCanonical, err := canonicalJSONBytes(pair.actual)
			if err != nil || !bytes.Equal(expectedCanonical, actualCanonical) {
				return fmt.Errorf("aggregate Gateway Context %q %s drifted", contextID, field)
			}
		}
	}
	return nil
}

func (r *Runtime) pendingPolicySourceTransactionExists() bool {
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := policySourceJournalPath(r.contextPolicyDirectory(entry.Name()))
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	return false
}

// verifyFinalAggregateTarget is the bounded verifier for the dormant final
// authority publication port. Its publication receipt supplies the canonical
// Gateway artifact digest; this boundary still verifies the fixed evaluator,
// actual projected data identity, metadata, and exact owned target before OPA
// can use the directory.
func (r *Runtime) verifyFinalAggregateTarget(projection FinalAggregateProjection) error {
	if projection.EvaluatorIdentity.Validate() != nil || projection.PolicyDataIdentity.Validate() != nil {
		return fmt.Errorf("final aggregate evaluator or policy-data identity is invalid")
	}
	if err := r.validatePersistedAggregateTarget(
		projection.PolicyDirectory, projection.GatewayConfig, projection.AggregateRevision,
	); err != nil {
		return err
	}
	entries, err := os.ReadDir(projection.PolicyDirectory)
	if err != nil || len(entries) != 1 {
		return fmt.Errorf("final aggregate policy data set is incomplete or contains executable extras")
	}
	for _, entry := range entries {
		if entry.Name() != "data.json" {
			return fmt.Errorf("final aggregate policy contains unexpected file %q", entry.Name())
		}
	}
	_, _, err = fixedAggregateEvaluatorModules()
	if err != nil {
		return err
	}
	dataBytes, err := readOwnerPolicyFile(filepath.Join(projection.PolicyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil {
		return err
	}
	if err := validateNoDuplicateJSONKeys(dataBytes); err != nil {
		return err
	}
	var document aggregatePolicyDataDocument
	if err := decodeAggregateJSON(dataBytes, &document); err != nil {
		return err
	}
	if document.Tobari.AggregateSchemaVersion != aggregateSchemaVersion ||
		document.Tobari.AggregateRevision != projection.AggregateRevision ||
		document.Tobari.EvaluatorIdentity != projection.EvaluatorIdentity ||
		document.Tobari.PolicyDataIdentity != projection.PolicyDataIdentity {
		return fmt.Errorf("final aggregate data identity metadata does not match projection")
	}
	actualEntries, err := aggregatePolicyDataEntriesForProjectedContexts(document.Contexts)
	if err != nil {
		return err
	}
	actualDataIdentity, err := policyDataIdentityForEntries(actualEntries)
	if err != nil || actualDataIdentity != projection.PolicyDataIdentity {
		return fmt.Errorf("final aggregate projected Context data drifted")
	}
	gatewayBytes, err := readOwnerPolicyFile(projection.GatewayConfig, maxPolicyPreflight)
	if err != nil {
		return err
	}
	if err := validateNoDuplicateJSONKeys(gatewayBytes); err != nil {
		return err
	}
	return verifyAggregateGatewayProjectionData(gatewayBytes, document.Contexts)
}

func policyEvaluatorIdentityForBytes(evaluatorBytes []byte) tobari.PolicyEvaluatorIdentity {
	digest := sha256.Sum256(evaluatorBytes)
	return tobari.PolicyEvaluatorIdentity{
		SchemaVersion: 1, Version: runtimeassets.PolicyEvaluatorVersion,
		Digest: tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])),
	}
}

func verifyAggregatePolicySources(
	items []aggregateContext, transactions map[string]*policySourceTransaction,
) error {
	for _, item := range items {
		var current policyDataFile
		var err error
		if transactions[item.paths.PolicyDirectory] != nil {
			current, err = readPolicyDataDuringTransaction(item.paths.PolicyDirectory)
		} else {
			current, err = readPolicyData(item.paths.PolicyDirectory)
		}
		if err != nil || !reflect.DeepEqual(current.sources, item.policy.sources) {
			return fmt.Errorf("Context %q policy source changed during aggregate generation", item.presentation)
		}
	}
	return nil
}
