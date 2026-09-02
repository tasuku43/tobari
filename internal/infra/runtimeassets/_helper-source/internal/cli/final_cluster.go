package cli

import (
	"context"
	"fmt"

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
	command.finalCluster = workspaceauthoritycmd.NewFinalClusterService(cluster, command.finalEntryReadiness)
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
	output, err := renderFinalClusterUpWithColor(command.Path, result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	output, err := renderFinalClusterStatusWithColor(command.Path, result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	purge, _ := inputs.Boolean("--purge")
	result, err := c.finalClusterLifecycle.Down(ctx, intent, purge)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	output, err := renderFinalClusterDownWithColor(command.Path, result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
			"aggregate_revision": result.AggregateRevision, "evaluator_identity": result.EvaluatorIdentity, "policy_data_identity": result.PolicyDataIdentity,
			"task": result.Task, "window_lines": result.WindowLines, "unparsed_lines": result.UnparsedLines, "items": items, "review_command": reviewCommand,
		}})
		if err != nil {
			return c.fail(ctx, fault.Wrap(fault.KindContract, "output_encoding_failed", "final cluster denials could not be encoded", false, err))
		}
		return c.emitResult(ctx, append(body, '\n'))
	}
	output := newHumanOutput(humanStyleAllowed(ctx, c, c.Out))
	if len(items) == 0 {
		output.heading("·", "No permission denials", styleMuted)
	} else {
		output.heading("!", fmt.Sprintf("%d permission denials", len(items)), styleWarning)
	}
	output.row("Unparsed", fmt.Sprintf("%d lines", result.UnparsedLines), humanStatusToken(fmt.Sprint(result.UnparsedLines)))
	output.row("Aggregate", string(result.AggregateRevision), styleText)
	output.row("Evaluator", result.EvaluatorIdentity.Version+"@"+string(result.EvaluatorIdentity.Digest), styleText)
	output.row("Policy data", string(result.PolicyDataIdentity.Digest), styleText)
	for _, item := range result.Items {
		output.row(item.Denial.Method, safeExternalText(item.Denial.Path)+" · "+string(item.ContextID), styleWarning)
	}
	if len(items) > 0 {
		output.next("review permissions", "Inspect and decide the retained requests.")
	}
	return c.emitResult(ctx, output.bytes())
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
		"aws_wire_protocol": denial.AWSWireProtocol, "aws_service": denial.AWSService,
		"aws_protocol_version": denial.AWSProtocolVersion, "aws_target_namespace": denial.AWSTargetNamespace, "aws_operation": denial.AWSOperation,
		"kubernetes_kind": denial.KubernetesKind, "kubernetes_verb": denial.KubernetesVerb,
		"kubernetes_group": denial.KubernetesGroup, "kubernetes_version": denial.KubernetesVersion,
		"kubernetes_resource": denial.KubernetesResource, "kubernetes_namespace": denial.KubernetesNamespace,
		"kubernetes_name": denial.KubernetesName, "kubernetes_subresource": denial.KubernetesSubresource,
		"kubernetes_dry_run": denial.KubernetesDryRun, "kubernetes_non_resource_path": denial.KubernetesNonResourcePath,
		"git_service": denial.GitService, "git_repository": denial.GitRepository,
		"oci_action": denial.OCIAction, "oci_repository": denial.OCIRepository, "oci_object": denial.OCIObject,
		"reason": denial.Reason, "status_code": denial.StatusCode, "learnable": denial.Learnable,
		"destination_kind": denial.EffectiveDestinationKind(), "authority_lifetime": denial.EffectiveAuthorityLifetime(), "attachment_epoch_id": denial.AttachmentEpochID,
	}
}

func renderFinalClusterStatus(path string, status tobari.FinalClusterStatus, format successFormat) ([]byte, error) {
	return renderFinalClusterStatusWithColor(path, status, format, false)
}

func renderFinalClusterStatusWithColor(path string, status tobari.FinalClusterStatus, format successFormat, color bool) ([]byte, error) {
	if err := status.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_status_result", "final cluster status is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster", tobari.FinalClusterStatusSchemaVersion, newFinalClusterStatusPublicResult(status))
	}
	output := newHumanOutput(color)
	marker, token, title := "✓", styleSuccess, "Cluster ready"
	if status.Runtime != tobari.FinalClusterRuntimeRunning || status.Authority != tobari.FinalClusterAuthorityPresent {
		marker, token, title = "!", styleWarning, "Cluster needs attention"
	}
	output.heading(marker, title, token)
	output.row("Runtime", string(status.Runtime), humanStatusToken(string(status.Runtime)))
	output.row("Authority", string(status.Authority), humanStatusToken(string(status.Authority)))
	output.row("Receipt", string(status.Receipt), humanStatusToken(string(status.Receipt)))
	if status.Authority == tobari.FinalClusterAuthorityPresent {
		output.row("Collection", fmt.Sprintf("generation %d · %s", status.Generation, status.CollectionRevision), styleText)
		if status.AggregateRevision != nil && status.EvaluatorIdentity != nil && status.PolicyDataIdentity != nil {
			output.row("Aggregate", *status.AggregateRevision, styleText)
			output.row("Evaluator", status.EvaluatorIdentity.Version+"@"+string(status.EvaluatorIdentity.Digest), styleText)
			output.row("Policy data", string(status.PolicyDataIdentity.Digest), styleText)
		}
	}
	output.row("Resources", fmt.Sprintf("%d Templates · %d Contexts · %d Workspaces", status.TemplateCount, status.ContextCount, status.WorkspaceCount), styleText)
	if len(status.Components) > 0 {
		output.section("Components")
	}
	for _, component := range status.Components {
		health := ""
		if component.Health != "" {
			health = " · " + safeExternalText(component.Health)
		}
		output.row(finalClusterComponentLabel(component.Name), string(component.State)+health+" · identity "+safeExternalText(string(component.Identity))+" · topology "+safeExternalText(string(component.Topology)), humanStatusToken(string(component.State)))
	}
	return output.bytes(), nil
}

func renderFinalClusterUp(path string, result workspaceauthoritycmd.FinalClusterReconciliation, format successFormat) ([]byte, error) {
	return renderFinalClusterUpWithColor(path, result, format, false)
}

func renderFinalClusterUpWithColor(path string, result workspaceauthoritycmd.FinalClusterReconciliation, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_reconciliation_result", "final cluster activation result is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster_up", tobari.FinalClusterUpSchemaVersion, newFinalClusterUpPublicResult(result))
	}
	output := newHumanOutput(color)
	output.heading("✓", "Cluster activated", styleSuccess)
	output.row("Collection", fmt.Sprintf("generation %d · %s", result.Generation, result.CollectionRevision), styleText)
	output.row("Aggregate", result.AggregateRevision, styleText)
	output.row("Evaluator", result.EvaluatorIdentity.Version+"@"+string(result.EvaluatorIdentity.Digest), styleText)
	output.row("Policy data", string(result.PolicyDataIdentity.Digest), styleText)
	output.row("Contexts", fmt.Sprintf("%d · content %s", len(result.Contexts), result.ContentDigest), styleText)
	return output.bytes(), nil
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
	AggregateRevision  string                                      `json:"aggregate_revision"`
	EvaluatorIdentity  tobari.PolicyEvaluatorIdentity              `json:"evaluator_identity"`
	PolicyDataIdentity tobari.PolicyDataIdentity                   `json:"policy_data_identity"`
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

type finalClusterStatusPublicResult struct {
	Task               string                                    `json:"task"`
	Authority          tobari.FinalClusterAuthorityState         `json:"authority"`
	Generation         uint64                                    `json:"generation,omitempty"`
	CollectionRevision tobari.SemanticDigest                     `json:"collection_revision,omitempty"`
	AggregateRevision  *string                                   `json:"aggregate_revision"`
	EvaluatorIdentity  *tobari.PolicyEvaluatorIdentity           `json:"evaluator_identity"`
	PolicyDataIdentity *tobari.PolicyDataIdentity                `json:"policy_data_identity"`
	TemplateCount      int                                       `json:"template_count"`
	ContextCount       int                                       `json:"context_count"`
	WorkspaceCount     int                                       `json:"workspace_count"`
	Runtime            tobari.FinalClusterRuntimeState           `json:"runtime"`
	Receipt            tobari.FinalClusterReceiptState           `json:"receipt"`
	Contexts           []finalClusterContextReceiptPublicResult  `json:"contexts"`
	Components         []tobari.FinalClusterComponentObservation `json:"components"`
}

type finalClusterContextReceiptPublicResult struct {
	ContextID      tobari.ContextID                        `json:"context_id"`
	TemplatePolicy *tobari.TemplatePolicyActivationReceipt `json:"template_policy,omitempty"`
	PolicyMemory   *finalClusterPolicyMemoryPublicResult   `json:"policy_memory,omitempty"`
}

func newFinalClusterStatusPublicResult(status tobari.FinalClusterStatus) finalClusterStatusPublicResult {
	contexts := make([]finalClusterContextReceiptPublicResult, len(status.Contexts))
	for index, context := range status.Contexts {
		contexts[index] = finalClusterContextReceiptPublicResult{ContextID: context.ContextID, TemplatePolicy: context.TemplatePolicy}
		if context.PolicyMemory != nil {
			contexts[index].PolicyMemory = &finalClusterPolicyMemoryPublicResult{
				ContextID: context.PolicyMemory.ContextID,
				Revision:  context.PolicyMemory.Revision,
			}
		}
	}
	return finalClusterStatusPublicResult{
		Task: status.Task, Authority: status.Authority, Generation: status.Generation, CollectionRevision: status.CollectionRevision,
		AggregateRevision: status.AggregateRevision, EvaluatorIdentity: status.EvaluatorIdentity, PolicyDataIdentity: status.PolicyDataIdentity,
		TemplateCount: status.TemplateCount, ContextCount: status.ContextCount, WorkspaceCount: status.WorkspaceCount,
		Runtime: status.Runtime, Receipt: status.Receipt, Contexts: contexts, Components: status.Components,
	}
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
		AggregateRevision: result.AggregateRevision, EvaluatorIdentity: result.EvaluatorIdentity, PolicyDataIdentity: result.PolicyDataIdentity,
		Applied: result.Applied, Contexts: contexts,
	}
}

func renderFinalClusterDown(path string, result workspaceauthoritycmd.FinalClusterDownResult, format successFormat) ([]byte, error) {
	return renderFinalClusterDownWithColor(path, result, format, false)
}

func renderFinalClusterDownWithColor(path string, result workspaceauthoritycmd.FinalClusterDownResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_cluster_down_result", "final cluster retirement result is invalid", false, err)
	}
	if format == successFormatJSON {
		return finalClusterJSON(path, "cluster_down", tobari.FinalClusterDownSchemaVersion, finalClusterDownPublicResult{
			Task: result.Task, Stopped: result.Stopped, Purged: result.Purged, Generation: result.Generation,
			CollectionRevision: result.CollectionRevision, EnvelopeChanged: result.EnvelopeChanged,
		})
	}
	output := newHumanOutput(color)
	output.heading("✓", "Cluster stopped", styleSuccess)
	output.row("Volumes purged", humanBool(result.Purged), humanOutcomeBoolToken(result.Purged))
	output.row("Collection", fmt.Sprintf("generation %d · %s", result.Generation, result.CollectionRevision), styleText)
	output.row("Receipts cleared", humanBool(result.EnvelopeChanged), humanOutcomeBoolToken(result.EnvelopeChanged))
	return output.bytes(), nil
}

type finalClusterDownPublicResult struct {
	Task               string                `json:"task"`
	Stopped            bool                  `json:"stopped"`
	Purged             bool                  `json:"purged"`
	Generation         uint64                `json:"generation"`
	CollectionRevision tobari.SemanticDigest `json:"collection_revision"`
	EnvelopeChanged    bool                  `json:"envelope_changed"`
}

func finalClusterJSON(path, envelope string, schemaVersion int, value any) ([]byte, error) {
	encoded, err := marshalCommandJSON(path, map[string]any{"schema_version": schemaVersion, envelope: value})
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
