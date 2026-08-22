package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const aggregateSchemaVersion = 1

var (
	regoPackagePattern     = regexp.MustCompile(`(?m)^package[ \t]+tobari\.http[ \t]*$`)
	regoInputSchemaPattern = regexp.MustCompile(`input\.schema_version[ \t]*==[ \t]*([0-9]+)`)
)

type aggregateProjection struct {
	Revision        string
	PolicyDirectory string
	GatewayConfig   string
	ContextCount    int
}

type aggregateContext struct {
	manifest            tobari.ContextManifest
	paths               tobari.ContextStorePaths
	data                map[string]any
	policy              policyDataFile
	rego                []byte
	graphqlEndpoints    []tobari.GraphQLEndpoint
	mcpEndpoints        []tobari.MCPEndpoint
	kubernetesEndpoints []tobari.GraphQLEndpoint
	contextPolicy       tobari.ContextPolicy
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
		if err := validateContextPolicyLayout(paths.PolicyDirectory, manifest.PolicyMode); err != nil {
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
		effectivePolicy, err := tobari.ApplyNativeToolAuthReadiness(readiness == tobari.ContextNativeReadinessEnabled, true, policySnapshot)
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
		var rego []byte
		if manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			rego, err = runtimeassets.Read("opa/policy/tobari.rego")
		} else {
			rego, err = readOwnerPolicyFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), maxPolicyPreflight)
		}
		if err != nil {
			return nil, fmt.Errorf("Context %q policy evaluator: %w", manifest.Name, err)
		}
		items = append(items, aggregateContext{
			manifest: manifest, paths: paths, data: contextData,
			policy: policySource, rego: rego,
			graphqlEndpoints:    graphqlEndpoints,
			mcpEndpoints:        mcpEndpoints,
			kubernetesEndpoints: kubernetesEndpoints,
			contextPolicy:       effectivePolicy,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].manifest.ID < items[j].manifest.ID })
	return items, nil
}

func aggregateKubernetesEndpoints(manifest tobari.ContextManifest) ([]tobari.GraphQLEndpoint, error) {
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
	policyEndpoints []tobari.GraphQLEndpoint, contextPolicyEndpoints []tobari.ContextPolicyExactRule,
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

func aggregateMCPEndpoints(contextPolicyEndpoints []tobari.ContextPolicyExactRule) ([]tobari.MCPEndpoint, error) {
	result := make([]tobari.MCPEndpoint, 0, len(contextPolicyEndpoints))
	seen := map[tobari.MCPEndpoint]struct{}{}
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

func aggregateNamespace(id string) string {
	return "c" + strings.ReplaceAll(id, "-", "")
}

func transformContextRego(item aggregateContext) ([]byte, error) {
	if !regoPackagePattern.Match(item.rego) {
		return nil, fmt.Errorf("Context %q policy must declare package tobari.http", item.manifest.Name)
	}
	if bytes.Contains(item.rego, []byte("data.tobari_contexts")) || bytes.Contains(item.rego, []byte("package tobari.system")) || bytes.Contains(item.rego, []byte("package tobari.contexts")) {
		return nil, fmt.Errorf("Context %q policy crosses the reserved routing namespace", item.manifest.Name)
	}
	schemaMatches := regoInputSchemaPattern.FindAllSubmatch(item.rego, -1)
	if len(schemaMatches) != 1 || string(schemaMatches[0][1]) != "1" {
		return nil, fmt.Errorf("Context %q policy must target source input schema 1", item.manifest.Name)
	}
	packageName := "package tobari.contexts." + aggregateNamespace(item.manifest.ID) + ".http"
	if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
		packageName = "package tobari.system.guided"
	}
	transformed := regoPackagePattern.ReplaceAll(item.rego, []byte(packageName))
	transformed = bytes.ReplaceAll(transformed, []byte("data.tobari"), []byte("data.tobari_contexts[input.principal.context_id]"))
	return transformed, nil
}

func aggregateRouter(items []aggregateContext) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("package tobari.http\n\nimport rego.v1\n\n")
	builder.WriteString("default decision := {\"allow\": false, \"reason\": \"unknown or invalid Context authority\", \"status_code\": 403, \"learnable\": false}\n\n")
	builder.WriteString("host_loopback_request if { object.get(input, \"destination\", {}).kind == \"host_loopback\" }\n")
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
	builder.WriteString("exact_denied if { context_policy_exact_denied }\nexact_denied if { learned_exact_denied }\nexact_denied if { learned_graphql_denied }\nexact_denied if { learned_mcp_denied }\nexact_denied if { learned_kubernetes_denied }\nexact_denied if { learned_git_denied }\n")
	builder.WriteString("context_policy_exact_granted if { object.get(input.request, \"graphql\", null) == null; object.get(input.request, \"mcp\", null) == null; object.get(input.request, \"kubernetes\", null) == null; object.get(input.request, \"git\", null) == null; some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants; object.get(rule, \"protocol\", \"http\") == \"http\"; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw }\n")
	builder.WriteString("context_policy_template_granted if { object.get(input.request, \"graphql\", null) == null; object.get(input.request, \"mcp\", null) == null; object.get(input.request, \"kubernetes\", null) == null; object.get(input.request, \"git\", null) == null; some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_templates; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; path_template_matches(rule.segments, input.request.path.raw) }\n")
	builder.WriteString("context_policy_graphql_root_granted(root_field) if { some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants; rule.protocol == \"graphql\"; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.graphql_operation_type == input.request.graphql.operation_type; rule.graphql_root_field == root_field }\n")
	builder.WriteString("context_policy_graphql_granted if { is_array(input.request.graphql.root_fields); count(input.request.graphql.root_fields) > 0; every root_field in input.request.graphql.root_fields { context_policy_graphql_root_granted(root_field) } }\n")
	builder.WriteString("context_policy_mcp_granted if { some rule in data.tobari_contexts[input.principal.context_id].policy.mcp_baseline_grants; rule.scheme == input.request.authority.scheme; rule.host == input.request.authority.host; rule.port == input.request.authority.port; rule.method == input.request.method; rule.path == input.request.path.raw; rule.mcp_method == input.request.mcp.method; object.get(rule, \"mcp_tool_name\", \"\") == object.get(input.request.mcp, \"tool_name\", \"\") }\n")
	builder.WriteString("context_policy_granted if { method_policy_granted; object.get(input.request, \"kubernetes\", null) == null; object.get(input.request, \"git\", null) == null }\ncontext_policy_granted if { context_policy_exact_granted }\ncontext_policy_granted if { context_policy_template_granted }\ncontext_policy_granted if { context_policy_graphql_granted }\ncontext_policy_granted if { context_policy_mcp_granted }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by attachment policy\", \"status_code\": 403, \"learnable\": false} if { input.schema_version == 1; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; attachment_denied }\n\n")
	builder.WriteString("decision := {\"allow\": true, \"reason\": \"allowed by attachment policy\", \"status_code\": 403, \"learnable\": false} if { input.schema_version == 1; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; not attachment_denied; attachment_allowed }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"Host Loopback requires attachment policy review\", \"status_code\": 403, \"learnable\": true} if { input.schema_version == 1; input.principal.cluster == \"default\"; data.tobari_contexts[input.principal.context_id]; host_loopback_request; host_loopback_identity_valid; not attachment_denied; not attachment_allowed }\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by Context policy ceiling\", \"status_code\": 403, \"learnable\": false} if {\n")
	builder.WriteString("  input.schema_version == 1\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  terminal_policy\n}\n\n")
	builder.WriteString("decision := {\"allow\": false, \"reason\": \"denied by exact policy\", \"status_code\": 403, \"learnable\": false} if {\n  input.schema_version == 1\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  exact_denied\n}\n\n")
	builder.WriteString("decision := {\"allow\": true, \"reason\": \"allowed by Context policy\", \"status_code\": 403, \"learnable\": false} if {\n  input.schema_version == 1\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  context_policy_granted\n}\n\n")
	builder.WriteString("decision := result if {\n")
	builder.WriteString("  input.schema_version == 1\n")
	builder.WriteString("  input.principal.cluster == \"default\"\n")
	builder.WriteString("  data.tobari_contexts[input.principal.context_id]\n")
	builder.WriteString("  not host_loopback_request\n")
	builder.WriteString("  not terminal_policy\n")
	builder.WriteString("  not exact_denied\n  not context_policy_granted\n")
	builder.WriteString("  object.get(input.request, \"graphql\", null) != null\n")
	builder.WriteString("  result := data.tobari.system.guided.decision\n")
	builder.WriteString("}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 1\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"kubernetes\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	builder.WriteString("decision := result if {\n  input.schema_version == 1\n  input.principal.cluster == \"default\"\n  data.tobari_contexts[input.principal.context_id]\n  not host_loopback_request\n  not terminal_policy\n  not exact_denied\n  not context_policy_granted\n  object.get(input.request, \"git\", null) != null\n  result := data.tobari.system.guided.decision\n}\n\n")
	for _, item := range items {
		if err := item.manifest.Validate(); err != nil {
			return nil, err
		}
		builder.WriteString("decision := result if {\n")
		builder.WriteString("  input.schema_version == 1\n")
		builder.WriteString("  input.principal.cluster == \"default\"\n")
		builder.WriteString("  input.principal.context_id == \"")
		builder.WriteString(item.manifest.ID)
		builder.WriteString("\"\n")
		builder.WriteString("  not host_loopback_request\n")
		builder.WriteString("  not terminal_policy\n")
		builder.WriteString("  not exact_denied\n  not context_policy_granted\n")
		builder.WriteString("  object.get(input.request, \"graphql\", null) == null\n")
		builder.WriteString("  object.get(input.request, \"kubernetes\", null) == null\n")
		builder.WriteString("  object.get(input.request, \"git\", null) == null\n")
		builder.WriteString("  result := data.")
		if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			builder.WriteString("tobari.system.guided")
		} else {
			builder.WriteString("tobari.contexts.")
			builder.WriteString(aggregateNamespace(item.manifest.ID))
			builder.WriteString(".http")
		}
		builder.WriteString(".decision\n}\n\n")
	}
	return []byte(builder.String()), nil
}

func rewriteGatewayProjection(item aggregateContext) map[string]any {
	endpoints := append([]tobari.GraphQLEndpoint{}, item.graphqlEndpoints...)
	mcpEndpoints := append([]tobari.MCPEndpoint{}, item.mcpEndpoints...)
	kubernetesEndpoints := append([]tobari.GraphQLEndpoint{}, item.kubernetesEndpoints...)
	return map[string]any{
		"name":                 item.manifest.Name,
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
	revision, err := aggregateRevision(items)
	if err != nil {
		return aggregateProjection{}, err
	}
	dataContexts := map[string]any{}
	gatewayContexts := map[string]any{}
	validationReused := false
	for _, item := range items {
		preflight, err := prepareContextPolicyPreflight(item.manifest, item.paths.PolicyDirectory, item.policy)
		if err != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy preflight: %w", item.manifest.Name, err)
		}
		preflightDigest, digestErr := policyPreflightDigest(preflight)
		if digestErr != nil {
			_ = os.RemoveAll(preflight)
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
			testErr = r.testPolicyDirectory(ctx, preflight)
		}
		_ = os.RemoveAll(preflight)
		if testErr != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy tests: %w", item.manifest.Name, testErr)
		}
		dataContexts[item.manifest.ID] = item.data
		gatewayContexts[item.manifest.ID] = rewriteGatewayProjection(item)
	}
	directory := filepath.Join(r.aggregateRoot(), revision)
	result := aggregateProjection{
		Revision: revision, PolicyDirectory: filepath.Join(directory, "policy"),
		GatewayConfig: filepath.Join(directory, "gateway.json"), ContextCount: len(items),
	}
	if _, err := os.Lstat(directory); err == nil {
		if err := r.testPolicyDirectory(ctx, result.PolicyDirectory); err != nil {
			return aggregateProjection{}, fmt.Errorf("validate existing aggregate policy: %w", err)
		}
		if err := verifyAggregatePolicySources(items, transactions); err != nil {
			return aggregateProjection{}, err
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
	router, err := aggregateRouter(items)
	if err != nil {
		return aggregateProjection{}, err
	}
	if err := os.WriteFile(filepath.Join(policyDirectory, "router.rego"), router, 0o600); err != nil {
		return aggregateProjection{}, err
	}
	canonicalGuided, err := runtimeassets.Read("opa/policy/tobari.rego")
	if err != nil {
		return aggregateProjection{}, err
	}
	guidedModule, err := transformContextRego(aggregateContext{
		manifest: tobari.ContextManifest{Name: "system", PolicyMode: tobari.ContextPolicyModeGuided, SourceAccess: tobari.ContextSourceAccessReadWrite},
		rego:     canonicalGuided,
	})
	if err != nil {
		return aggregateProjection{}, err
	}
	if err := os.WriteFile(filepath.Join(policyDirectory, "guided.rego"), guidedModule, 0o600); err != nil {
		return aggregateProjection{}, err
	}
	for _, item := range items {
		rego, err := transformContextRego(item)
		if err != nil {
			return aggregateProjection{}, err
		}
		regoName := aggregateNamespace(item.manifest.ID) + ".rego"
		if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			regoName = "guided.rego"
			if !bytes.Equal(guidedModule, rego) {
				return aggregateProjection{}, fmt.Errorf("guided Context policy logic diverged from the shared system module")
			}
		}
		if item.manifest.PolicyMode != tobari.ContextPolicyModeGuided {
			if _, err := os.Lstat(filepath.Join(policyDirectory, regoName)); errors.Is(err, os.ErrNotExist) {
				if err := os.WriteFile(filepath.Join(policyDirectory, regoName), rego, 0o600); err != nil {
					return aggregateProjection{}, err
				}
			} else if err != nil {
				return aggregateProjection{}, err
			}
		}
	}
	dataDocument := map[string]any{"tobari_contexts": dataContexts, "tobari": map[string]any{
		"aggregate_schema_version": aggregateSchemaVersion,
		"aggregate_revision":       revision,
	}}
	if err := writeAtomicJSON(filepath.Join(policyDirectory, "data.json"), dataDocument); err != nil {
		return aggregateProjection{}, err
	}
	gatewayDocument := map[string]any{"version": "v1", "contexts": gatewayContexts}
	if err := writeAtomicJSON(filepath.Join(temporary, "gateway.json"), gatewayDocument); err != nil {
		return aggregateProjection{}, err
	}
	candidatePolicy := filepath.Join(temporary, "policy")
	if err := r.testPolicyDirectory(ctx, candidatePolicy); err != nil {
		return aggregateProjection{}, fmt.Errorf("validate aggregate policy: %w", err)
	}
	if err := verifyAggregatePolicySources(items, transactions); err != nil {
		return aggregateProjection{}, err
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
	hash := sha256.New()
	for _, item := range items {
		manifestBytes, err := json.Marshal(item.manifest)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(item.data)
		if err != nil {
			return "", err
		}
		hash.Write(manifestBytes)
		hash.Write([]byte{0})
		hash.Write(encoded)
		hash.Write([]byte{0})
		hash.Write(item.rego)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
			return fmt.Errorf("Context %q policy source changed during aggregate generation", item.manifest.Name)
		}
	}
	return nil
}
