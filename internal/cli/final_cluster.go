package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type finalClusterCLIState struct {
	finalCluster interface {
		Reconcile(context.Context, operation.Intent) (workspaceauthoritycmd.FinalClusterReconciliation, error)
	}
	finalClusterLifecycle *workspaceauthoritycmd.FinalClusterLifecycleService
	finalClusterRead      *workspaceauthoritycmd.FinalClusterReadService
}

func configureFinalClusterCLI(command *CLI, mutator *workspaceauthoritystore.Mutator, lifecycle *workspaceauthoritystore.ClusterLifecycleAdapter, reads *workspaceauthoritystore.ClusterReadAdapter) error {
	if command == nil || mutator == nil || lifecycle == nil || reads == nil {
		return fmt.Errorf("final cluster CLI composition is unavailable")
	}
	cluster, err := workspaceauthoritystore.NewClusterAdapter(mutator)
	if err != nil {
		return err
	}
	command.finalCluster = workspaceauthoritycmd.NewFinalClusterService(cluster)
	command.finalClusterLifecycle = workspaceauthoritycmd.NewFinalClusterLifecycleService(lifecycle)
	command.finalClusterRead = workspaceauthoritycmd.NewFinalClusterReadService(reads)
	return nil
}

func runFinalClusterUp(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalCluster == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalCluster.Reconcile(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	output, err := renderFinalClusterUp(command.Path, result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalClusterStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalClusterLifecycle == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalClusterLifecycle.Status(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	output, err := renderFinalClusterStatus(command.Path, result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalClusterDown(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalClusterLifecycle == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalClusterLifecycle.Down(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	output, err := renderFinalClusterDown(command.Path, result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalClusterLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalClusterRead == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.finalClusterRead.Logs(ctx, tobari.LogRequest{Component: inputs.One("--component"), Tail: int(tail)})
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output, humanStyleAllowed(ctx, c, c.Out)))
}

func runFinalClusterDenials(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalClusterRead == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	result, err := c.finalClusterRead.Denials(ctx, int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	reviewCommand := ProgramName + " review permissions"
	items := make([]map[string]any, len(result.Items))
	for index, item := range result.Items {
		items[index] = finalClusterDenialValue(item)
	}
	if format == successFormatJSON {
		body, err := marshalCommandJSON(command.Path, map[string]any{"schema_version": tobari.FinalClusterDenialSchemaVersion, "denials": map[string]any{
			"task": result.Task, "window_lines": result.WindowLines, "unparsed_lines": result.UnparsedLines, "items": items, "review_command": reviewCommand,
		}})
		if err != nil {
			return c.fail(ctx, fault.Wrap(fault.KindContract, "output_encoding_failed", "final cluster denials could not be encoded", false, err))
		}
		return c.emitResult(ctx, append(body, '\n'))
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Cluster denials %d · unparsed %d\n", len(items), result.UnparsedLines)
	for _, item := range result.Items {
		fmt.Fprintf(&output, "%s  %s  %s %s\n", item.ContextID, item.WorkspaceID, item.Denial.Method, safeExternalText(item.Denial.Path))
	}
	fmt.Fprintf(&output, "Review  %s\n", reviewCommand)
	return c.emitResult(ctx, []byte(output.String()))
}

func finalClusterDenialValue(item tobari.FinalClusterDenial) map[string]any {
	denial := item.Denial
	return map[string]any{
		"context_id": item.ContextID, "workspace_template_id": item.WorkspaceTemplateID, "template_name": item.TemplateName,
		"workspace_id": item.WorkspaceID, "project_root": item.ProjectRoot,
		"timestamp": denial.Timestamp, "request_id": denial.RequestID, "scheme": denial.Scheme, "host": denial.Host, "port": denial.Port,
		"method": denial.Method, "path": denial.Path, "protocol": denial.Protocol, "state_change": denial.StateChangePotential(),
		"graphql_operation_type": denial.GraphQLOperationType, "graphql_root_field": denial.GraphQLRootField,
		"mcp_method": denial.MCPMethod, "mcp_tool_name": denial.MCPToolName,
		"aws_wire_protocol": denial.AWSWireProtocol, "aws_service": denial.AWSService, "aws_operation": denial.AWSOperation,
		"kubernetes_verb": denial.KubernetesVerb, "kubernetes_resource": denial.KubernetesResource, "kubernetes_dry_run": denial.KubernetesDryRun,
		"git_service": denial.GitService, "git_repository": denial.GitRepository,
		"oci_action": denial.OCIAction, "oci_repository": denial.OCIRepository, "oci_object": denial.OCIObject,
		"reason": denial.Reason, "status_code": denial.StatusCode, "learnable": denial.Learnable,
		"destination_kind": denial.EffectiveDestinationKind(), "authority_lifetime": denial.EffectiveAuthorityLifetime(), "attachment_epoch_id": denial.AttachmentEpochID,
	}
}

func renderFinalClusterStatus(path string, status tobari.FinalClusterStatus, format successFormat) ([]byte, error) {
	if err := status.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_status_result", "final cluster status is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster", status)
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Cluster %s\n", status.Runtime)
	fmt.Fprintf(&output, "Authority  %s\n", status.Authority)
	fmt.Fprintf(&output, "Receipt    %s\n", status.Receipt)
	if status.Authority == tobari.FinalClusterAuthorityPresent {
		fmt.Fprintf(&output, "Collection generation %d · %s\n", status.Generation, status.CollectionRevision)
	}
	fmt.Fprintf(&output, "Templates %d · Contexts %d · Workspaces %d\n", status.TemplateCount, status.ContextCount, status.WorkspaceCount)
	for _, component := range status.Components {
		health := ""
		if component.Health != "" {
			health = " · " + safeExternalText(component.Health)
		}
		fmt.Fprintf(&output, "%s  %s%s · identity %s · topology %s\n", finalClusterComponentLabel(component.Name), component.State, health, component.Identity, component.Topology)
	}
	return []byte(output.String()), nil
}

func renderFinalClusterUp(path string, result workspaceauthoritycmd.FinalClusterReconciliation, format successFormat) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_reconciliation_result", "final cluster activation result is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster_up", newFinalClusterUpPublicResult(result))
	}
	return []byte(fmt.Sprintf("Cluster activated\nCollection generation %d · %s\nContexts %d · content %s\n", result.Generation, result.CollectionRevision, len(result.Contexts), result.ContentDigest)), nil
}

// finalClusterUpPublicResult keeps the task-owned validation version private.
// The public schema version belongs to the outer command envelope declared by
// Catalog; serializing the application result directly would create a second,
// undeclared nested schema_version field.
type finalClusterUpPublicResult struct {
	Task               string                                      `json:"task"`
	Generation         uint64                                      `json:"generation"`
	CollectionRevision tobari.SemanticDigest                       `json:"collection_revision"`
	ContentDigest      tobari.SemanticDigest                       `json:"content_digest"`
	PlanDigest         tobari.SemanticDigest                       `json:"plan_digest"`
	EnvelopeChanged    bool                                        `json:"envelope_changed"`
	Applied            bool                                        `json:"applied"`
	Contexts           []finalClusterContextActivationPublicResult `json:"contexts"`
}

type finalClusterContextActivationPublicResult struct {
	ContextID           tobari.ContextID                       `json:"context_id"`
	WorkspaceTemplateID tobari.WorkspaceTemplateID             `json:"workspace_template_id"`
	TemplatePolicy      tobari.TemplatePolicyActivationReceipt `json:"template_policy"`
	PolicyMemory        finalClusterPolicyMemoryPublicResult   `json:"policy_memory"`
}

type finalClusterPolicyMemoryPublicResult struct {
	ContextID tobari.ContextID      `json:"context_id"`
	Revision  tobari.SemanticDigest `json:"revision"`
}

func newFinalClusterUpPublicResult(result workspaceauthoritycmd.FinalClusterReconciliation) finalClusterUpPublicResult {
	contexts := make([]finalClusterContextActivationPublicResult, len(result.Contexts))
	for index, context := range result.Contexts {
		contexts[index] = finalClusterContextActivationPublicResult{
			ContextID: context.ContextID, WorkspaceTemplateID: context.WorkspaceTemplateID, TemplatePolicy: context.TemplatePolicy,
			PolicyMemory: finalClusterPolicyMemoryPublicResult{ContextID: context.PolicyMemory.ContextID, Revision: context.PolicyMemory.Revision},
		}
	}
	return finalClusterUpPublicResult{
		Task: result.Task, Generation: result.Generation, CollectionRevision: result.CollectionRevision,
		ContentDigest: result.ContentDigest, PlanDigest: result.PlanDigest, EnvelopeChanged: result.EnvelopeChanged,
		Applied: result.Applied, Contexts: contexts,
	}
}

func renderFinalClusterDown(path string, result workspaceauthoritycmd.FinalClusterDownResult, format successFormat) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_down_result", "final cluster retirement result is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster_down", finalClusterDownPublicResult{
			Task: result.Task, Stopped: result.Stopped, Generation: result.Generation,
			CollectionRevision: result.CollectionRevision, EnvelopeChanged: result.EnvelopeChanged,
		})
	}
	return []byte(fmt.Sprintf("Cluster stopped\nCollection generation %d · %s\nActive Context receipts cleared: %t\n", result.Generation, result.CollectionRevision, result.EnvelopeChanged)), nil
}

type finalClusterDownPublicResult struct {
	Task               string                `json:"task"`
	Stopped            bool                  `json:"stopped"`
	Generation         uint64                `json:"generation"`
	CollectionRevision tobari.SemanticDigest `json:"collection_revision"`
	EnvelopeChanged    bool                  `json:"envelope_changed"`
}

func finalClusterJSON(path, envelope string, value any) ([]byte, error) {
	encoded, err := marshalCommandJSON(path, map[string]any{"schema_version": tobari.FinalClusterLifecycleSchemaVersion, envelope: value})
	if err != nil {
		return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "final cluster JSON could not be encoded", false, err)
	}
	return append(encoded, '\n'), nil
}

func finalClusterComponentLabel(name string) string {
	switch name {
	case "gateway":
		return "Gateway"
	case "opa":
		return "OPA"
	case "auth-broker":
		return "Auth Broker"
	case "credential-companion":
		return "Companion"
	default:
		return safeExternalText(name)
	}
}
