package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimePruneCandidateDocument struct {
	Kind                 tobari.RuntimePruneCandidateKind `json:"kind"`
	RuntimeID            string                           `json:"runtime_id"`
	Revision             string                           `json:"revision"`
	Name                 string                           `json:"name"`
	Ordinal              int                              `json:"ordinal"`
	LastUsed             tobari.RuntimeLastUsedState      `json:"last_used"`
	SourceLogicalBytes   int64                            `json:"source_logical_bytes"`
	SnapshotLogicalBytes int64                            `json:"snapshot_logical_bytes"`
	ImageVirtualBytes    *int64                           `json:"image_virtual_bytes"`
	ReclaimableBytes     *int64                           `json:"reclaimable_bytes"`
}

type runtimePruneItemDocument struct {
	Kind                 tobari.RuntimePruneCandidateKind `json:"kind"`
	RuntimeID            string                           `json:"runtime_id"`
	Revision             string                           `json:"revision"`
	Name                 string                           `json:"name"`
	Ordinal              int                              `json:"ordinal"`
	LastUsed             tobari.RuntimeLastUsedState      `json:"last_used"`
	SourceLogicalBytes   int64                            `json:"source_logical_bytes"`
	SnapshotLogicalBytes int64                            `json:"snapshot_logical_bytes"`
	Disposition          tobari.RuntimePruneDisposition   `json:"disposition"`
	RemovedTagCount      int                              `json:"removed_tag_count"`
	ImageVirtualBytes    *int64                           `json:"image_virtual_bytes"`
	ReclaimedBytes       *int64                           `json:"reclaimed_bytes"`
}

type runtimePrunePlanProjection struct {
	Task       string                             `json:"task"`
	PlanRef    string                             `json:"plan_ref"`
	ObservedAt string                             `json:"observed_at"`
	Empty      bool                               `json:"empty"`
	Applicable bool                               `json:"applicable"`
	Candidates []runtimePruneCandidateDocument    `json:"candidates"`
	Protected  []tobari.RuntimeProtection         `json:"protected"`
	Blockers   []runtimePruneBlockerDocument      `json:"blockers"`
	Storage    []tobari.RuntimeStorageObservation `json:"storage"`
}

type runtimePruneBlockerDocument struct {
	RuntimeID string                              `json:"runtime_id"`
	Revision  *string                             `json:"revision,omitempty"`
	Reason    tobari.RuntimeMaterialBlockerReason `json:"reason"`
}

type runtimePruneResultProjection struct {
	Task               string                         `json:"task"`
	PlanRef            string                         `json:"plan_ref"`
	State              tobari.RuntimePruneResultState `json:"state"`
	Items              []runtimePruneItemDocument     `json:"items"`
	RemovedTagCount    int                            `json:"removed_tag_count"`
	ReclaimedBytes     *int64                         `json:"reclaimed_bytes"`
	ReceiptRevision    uint64                         `json:"receipt_revision"`
	SourcePreserved    bool                           `json:"source_preserved"`
	SnapshotsPreserved bool                           `json:"snapshots_preserved"`
	HistoryPreserved   bool                           `json:"history_preserved"`
}

type runtimePrunePlanJSONDocument struct {
	SchemaVersion int                        `json:"schema_version"`
	Plan          runtimePrunePlanProjection `json:"runtime_prune_plan"`
}

type runtimePruneResultJSONDocument struct {
	SchemaVersion int                          `json:"schema_version"`
	Result        runtimePruneResultProjection `json:"runtime_prune_result"`
}

func runRuntimePruneDryRun(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime prune dry-run", "Correct the command arguments.")
	}
	plan, err := c.runtime.PlanPrune(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimePrunePlan(command.Path, plan, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runRuntimePruneApply(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime prune apply", "Correct the command arguments.")
	}
	planRef := inputs.One("--plan")
	intent.Target = operation.TargetRef{Kind: tobari.RuntimePrunePlanReferenceKind, ID: planRef}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.runtime.ApplyPrune(ctx, intent, planRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimePruneResult(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		classified, ok := err.(*fault.Error)
		if !ok {
			classified = fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime prune result output could not be encoded", false, err)
		}
		return c.fail(ctx, fault.WithClassification(classified, fault.PhasePresentation, fault.ChangeConfirmed))
	}
	return c.emitMutationResult(ctx, command, output)
}

func runtimePrunePlanProjectionFrom(plan tobari.RuntimePrunePlan) runtimePrunePlanProjection {
	candidates := make([]runtimePruneCandidateDocument, len(plan.Candidates))
	for index, candidate := range plan.Candidates {
		candidates[index] = runtimePruneCandidateDocument{
			Kind: candidate.Kind, RuntimeID: candidate.RuntimeID, Revision: candidate.Revision,
			Name: candidate.Name, Ordinal: candidate.Ordinal, LastUsed: candidate.LastUsed,
			SourceLogicalBytes: candidate.SourceLogicalBytes, SnapshotLogicalBytes: candidate.SnapshotLogicalBytes,
			ImageVirtualBytes: candidate.ImageVirtualBytes, ReclaimableBytes: candidate.ReclaimableBytes,
		}
	}
	protected := make([]tobari.RuntimeProtection, len(plan.Protected))
	copy(protected, plan.Protected)
	blockers := make([]runtimePruneBlockerDocument, len(plan.Blockers))
	for index, blocker := range plan.Blockers {
		blockers[index] = runtimePruneBlockerDocument{RuntimeID: blocker.RuntimeID, Reason: blocker.Reason}
		if blocker.Revision != "" {
			revision := blocker.Revision
			blockers[index].Revision = &revision
		}
	}
	storage := make([]tobari.RuntimeStorageObservation, len(plan.Storage))
	copy(storage, plan.Storage)
	return runtimePrunePlanProjection{
		Task: plan.Task, PlanRef: plan.PlanRef, ObservedAt: plan.ObservedAt.Format("2006-01-02T15:04:05.000000000Z"),
		Empty: plan.Empty, Applicable: plan.Applicable, Candidates: candidates,
		Protected: protected, Blockers: blockers, Storage: storage,
	}
}

func runtimePruneResultProjectionFrom(result tobari.RuntimePruneResult) runtimePruneResultProjection {
	items := make([]runtimePruneItemDocument, len(result.Items))
	for index, item := range result.Items {
		items[index] = runtimePruneItemDocument{
			Kind: item.Kind, RuntimeID: item.RuntimeID, Revision: item.Revision,
			Name: item.Name, Ordinal: item.Ordinal, LastUsed: item.LastUsed,
			SourceLogicalBytes: item.SourceLogicalBytes, SnapshotLogicalBytes: item.SnapshotLogicalBytes,
			Disposition: item.Disposition, RemovedTagCount: item.RemovedTagCount,
			ImageVirtualBytes: item.ImageVirtualBytes, ReclaimedBytes: item.ReclaimedBytes,
		}
	}
	return runtimePruneResultProjection{
		Task: result.Task, PlanRef: result.PlanRef, State: result.State, Items: items,
		RemovedTagCount: result.RemovedTagCount, ReclaimedBytes: result.ReclaimedBytes,
		ReceiptRevision: result.ReceiptRevision, SourcePreserved: result.SourcePreserved,
		SnapshotsPreserved: result.SnapshotsPreserved, HistoryPreserved: result.HistoryPreserved,
	}
}

func renderRuntimePrunePlan(path string, plan tobari.RuntimePrunePlan, format successFormat, color bool) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_prune_plan", "Runtime prune plan is invalid", false, err)
	}
	projection := runtimePrunePlanProjectionFrom(plan)
	if format == successFormatJSON {
		output, err := marshalCommandJSON(path, runtimePrunePlanJSONDocument{SchemaVersion: 2, Plan: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime prune plan output could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}

	var output strings.Builder
	output.WriteString(applyStyleToken(color, styleAccent, "Runtime prune plan"))
	output.WriteString("\n\n")
	writeContextCardValue(&output, color, "Plan reference", plan.PlanRef, styleAccent)
	writeContextCardValue(&output, color, "Observed", plan.ObservedAt.Format("2006-01-02 15:04:05 UTC"), styleText)
	writeContextCardValue(&output, color, "Applicable", humanBool(plan.Applicable), humanStatusToken(map[bool]string{true: "ready", false: "blocked"}[plan.Applicable]))
	writeContextCardValue(&output, color, "Candidates", fmt.Sprintf("%d", len(plan.Candidates)), styleText)
	writeContextCardValue(&output, color, "Protected", fmt.Sprintf("%d", len(plan.Protected)), styleText)
	writeContextCardValue(&output, color, "Blockers", fmt.Sprintf("%d", len(plan.Blockers)), styleText)

	for _, candidate := range plan.Candidates {
		output.WriteString("\n")
		output.WriteString(applyStyleToken(color, styleText, runtimePruneDisplayName(candidate.Name, candidate.Ordinal, candidate.Kind)))
		output.WriteString("\n")
		writeContextCardValue(&output, color, "Authority", candidate.RuntimeID+" · "+candidate.Revision, styleText)
		writeContextCardValue(&output, color, "Kind", string(candidate.Kind), styleText)
		writeContextCardValue(&output, color, "Last used", string(candidate.LastUsed), styleWarning)
		writeContextCardValue(&output, color, "Source", fmt.Sprintf("%d B logical · preserved", candidate.SourceLogicalBytes), styleText)
		writeContextCardValue(&output, color, "Snapshot", fmt.Sprintf("%d B logical · preserved", candidate.SnapshotLogicalBytes), styleText)
		writeContextCardValue(&output, color, "Image virtual bytes", optionalBytes(candidate.ImageVirtualBytes), styleText)
		writeContextCardValue(&output, color, "Reclaimable bytes", "unknown", styleWarning)
	}
	for _, protection := range plan.Protected {
		output.WriteString("\n")
		output.WriteString(applyStyleToken(color, styleWarning, "Protected Runtime revision"))
		output.WriteString("\n")
		writeContextCardValue(&output, color, "Authority", protection.RuntimeID+" · "+protection.RuntimeRevision, styleText)
		writeContextCardValue(&output, color, "Reason", string(protection.Reason), styleWarning)
		writeContextCardValue(&output, color, "Template", string(protection.WorkspaceTemplateID)+" · "+string(protection.TemplateRevision), styleText)
		if protection.ContextID != "" {
			writeContextCardValue(&output, color, "Context", string(protection.ContextID), styleText)
		}
		if protection.WorkspaceID != "" {
			writeContextCardValue(&output, color, "Workspace", string(protection.WorkspaceID), styleText)
		}
	}
	for _, blocker := range plan.Blockers {
		output.WriteString("\n")
		output.WriteString(applyStyleToken(color, styleWarning, "Prune blocker"))
		output.WriteString("\n")
		authority := blocker.RuntimeID
		if blocker.Revision != "" {
			authority += " · " + blocker.Revision
		}
		writeContextCardValue(&output, color, "Authority", authority, styleText)
		writeContextCardValue(&output, color, "Reason", string(blocker.Reason), styleWarning)
	}
	output.WriteString("\n")
	switch {
	case !plan.Applicable:
		writeContextCardValue(&output, color, "Apply", "blocked · resolve every reported blocker, then create a fresh plan", styleWarning)
	case plan.Empty:
		writeContextCardValue(&output, color, "Apply", "no eligible image material", styleMuted)
	default:
		writeContextCardValue(&output, color, "Apply", ProgramName+" runtime prune apply --plan "+plan.PlanRef+" --confirm=prune", styleAccent)
	}
	return []byte(output.String()), nil
}

func renderRuntimePruneResult(path string, result tobari.RuntimePruneResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_retirement_result", "Runtime prune result is invalid", false, err)
	}
	projection := runtimePruneResultProjectionFrom(result)
	if format == successFormatJSON {
		output, err := marshalCommandJSON(path, runtimePruneResultJSONDocument{SchemaVersion: 2, Result: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime prune result output could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}

	var output strings.Builder
	output.WriteString(applyStyleToken(color, styleAccent, "Runtime prune "+string(result.State)))
	output.WriteString("\n\n")
	writeContextCardValue(&output, color, "Plan reference", result.PlanRef, styleAccent)
	writeContextCardValue(&output, color, "State", string(result.State), humanStatusToken("ready"))
	for _, item := range result.Items {
		output.WriteString("\n")
		output.WriteString(applyStyleToken(color, styleText, runtimePruneDisplayName(item.Name, item.Ordinal, item.Kind)))
		output.WriteString("\n")
		writeContextCardValue(&output, color, "Authority", item.RuntimeID+" · "+item.Revision, styleText)
		writeContextCardValue(&output, color, "Disposition", string(item.Disposition), styleAccent)
		writeContextCardValue(&output, color, "Removed exact tags", fmt.Sprintf("%d", item.RemovedTagCount), styleText)
		writeContextCardValue(&output, color, "Last used", string(item.LastUsed), styleWarning)
		writeContextCardValue(&output, color, "Source", fmt.Sprintf("%d B logical · preserved", item.SourceLogicalBytes), styleText)
		writeContextCardValue(&output, color, "Snapshot", fmt.Sprintf("%d B logical · preserved", item.SnapshotLogicalBytes), styleText)
		writeContextCardValue(&output, color, "Image virtual bytes", optionalBytes(item.ImageVirtualBytes), styleText)
		writeContextCardValue(&output, color, "Reclaimed bytes", "unknown", styleWarning)
	}
	output.WriteString("\n")
	writeContextCardValue(&output, color, "Removed exact tags", fmt.Sprintf("%d", result.RemovedTagCount), styleText)
	writeContextCardValue(&output, color, "Reclaimed bytes", "unknown", styleWarning)
	receipt := "none"
	if result.ReceiptRevision != 0 {
		receipt = fmt.Sprintf("%d", result.ReceiptRevision)
	}
	writeContextCardValue(&output, color, "Receipt revision", receipt, styleText)
	writeContextCardValue(&output, color, "Preserved", "Runtime source · immutable snapshots · revision history", styleAccent)
	return []byte(output.String()), nil
}

func runtimePruneDisplayName(name string, ordinal int, kind tobari.RuntimePruneCandidateKind) string {
	if kind == tobari.RuntimePruneCandidateFailedBuild {
		return safeExternalText(name) + " · failed build"
	}
	return fmt.Sprintf("%s@%d", safeExternalText(name), ordinal)
}

func optionalBytes(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d B virtual", *value)
}

func runtimePruneCandidateOutputFields() []OutputField {
	return []OutputField{
		{Name: "kind", Type: OutputFieldTypeString, Description: "Successful Runtime revision or exact journaled failed-build artifact.", Enum: []string{string(tobari.RuntimePruneCandidateRevision), string(tobari.RuntimePruneCandidateFailedBuild)}},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Semantic source revision digest."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Human Runtime display name."},
		{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Human successful revision ordinal, or zero for a failed build artifact."},
		{Name: "last_used", Type: OutputFieldTypeString, Description: "V1 usage certainty; no timestamp is inferred.", Enum: []string{string(tobari.RuntimeLastUsedUnknown)}},
		{Name: "source_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the preserved editable Runtime source."},
		{Name: "snapshot_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the preserved immutable or staging source snapshot."},
		{Name: "image_virtual_bytes", Type: OutputFieldTypeInteger, Description: "Bounded Docker virtual-size evidence, or null when unavailable.", Nullable: true},
		{Name: "reclaimable_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimable bytes; always null in V1.", Nullable: true},
	}
}

func runtimePruneProtectionOutputFields() []OutputField {
	return []OutputField{
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Protected stable Runtime authority ID."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Protected semantic Runtime revision digest."},
		{Name: "reason", Type: OutputFieldTypeString, Description: "Exact authority edge preventing retirement.", Enum: []string{
			string(tobari.RuntimeProtectedByTemplateCurrent), string(tobari.RuntimeProtectedByTemplateRetained),
			string(tobari.RuntimeProtectedByContextDesired),
			string(tobari.RuntimeProtectedByWorkspaceApplied), string(tobari.RuntimeProtectedByWorkspacePending),
			string(tobari.RuntimeProtectedByWorkspaceObserved),
		}},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Owning Workspace Template stable ID."},
		{Name: "workspace_template_revision", Type: OutputFieldTypeString, Description: "Exact immutable Workspace Template revision providing the edge."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Owning Context ID for Workspace-derived evidence.", Optional: true},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Owning Workspace ID for Workspace-derived evidence.", Optional: true},
	}
}

func runtimePruneBlockerOutputFields() []OutputField {
	return []OutputField{
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Blocked stable Runtime authority ID."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Blocked semantic Runtime revision digest; absent only for whole-Runtime active lifecycle activity.", Optional: true},
		{Name: "reason", Type: OutputFieldTypeString, Description: "Exact material or observation blocker.", Enum: []string{
			string(tobari.RuntimeBlockedByWorkspaceContainer), string(tobari.RuntimeBlockedByExternalContainer),
			string(tobari.RuntimeBlockedByImageMissing), string(tobari.RuntimeBlockedByImageTagMissing),
			string(tobari.RuntimeBlockedByImageTagShared), string(tobari.RuntimeBlockedByImageMismatched),
			string(tobari.RuntimeBlockedByStagingMissing), string(tobari.RuntimeBlockedByStagingTagMissing),
			string(tobari.RuntimeBlockedByStagingTagShared), string(tobari.RuntimeBlockedByObservationUnknown),
			string(tobari.RuntimeBlockedByMigrationUnverified), string(tobari.RuntimeBlockedByImagePruned),
			string(tobari.RuntimeBlockedByActiveBuild), string(tobari.RuntimeBlockedByActiveRetirement),
		}},
	}
}

func runtimePruneStorageOutputFields() []OutputField {
	return []OutputField{
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Human Runtime display name."},
		{Name: "source_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the editable source tree."},
		{Name: "snapshots", Type: OutputFieldTypeArray, Description: "Complete validated immutable and staging source snapshot storage.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One validated source snapshot.", Fields: []OutputField{
			{Name: "kind", Type: OutputFieldTypeString, Description: "Successful Runtime revision or failed-build staging snapshot.", Enum: []string{string(tobari.RuntimePruneCandidateRevision), string(tobari.RuntimePruneCandidateFailedBuild)}},
			{Name: "revision", Type: OutputFieldTypeString, Description: "Semantic revision digest."},
			{Name: "semantic_fingerprint", Type: OutputFieldTypeString, Description: "Read-only bounded rehash of the source tree; equal to revision when valid."},
			{Name: "logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the snapshot tree."},
		}}},
	}
}

func runtimePrunePlanOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared Runtime prune planning task identity."},
			{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque exact Runtime prune plan reference.", ReferenceKind: tobari.RuntimePrunePlanReferenceKind},
			{Name: "observed_at", Type: OutputFieldTypeString, Description: "UTC time of the complete bounded lifecycle observation."},
			{Name: "empty", Type: OutputFieldTypeBoolean, Description: "Whether no eligible candidates exist."},
			{Name: "applicable", Type: OutputFieldTypeBoolean, Description: "Whether every observation is complete and the exact plan can be applied."},
			{Name: "candidates", Type: OutputFieldTypeArray, Description: "Complete ordered exact image-tag retirement candidates.", SemanticScope: "Every eligible managed Runtime revision or settled failed-build artifact in the installation observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact prune candidate.", Fields: runtimePruneCandidateOutputFields()}},
			{Name: "protected", Type: OutputFieldTypeArray, Description: "Complete current Runtime protection graph.", SemanticScope: "Every current or retained Workspace Template, desired Context binding, and applied, pending, or observed Workspace protection edge.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact protection edge.", Fields: runtimePruneProtectionOutputFields()}},
			{Name: "blockers", Type: OutputFieldTypeArray, Description: "Complete material, container-use, migration, observation, and lifecycle blockers.", SemanticScope: "Every fail-closed blocker in the same bounded lifecycle observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact blocker.", Fields: runtimePruneBlockerOutputFields()}},
			{Name: "storage", Type: OutputFieldTypeArray, Description: "Complete validated Runtime source and snapshot logical-byte evidence.", SemanticScope: "Every managed Runtime source and retained source snapshot in the installation observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Runtime storage observation.", Fields: runtimePruneStorageOutputFields()}},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
		JSONEnvelope: "runtime_prune_plan", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
	}
}

func runtimePruneResultOutput() CommandOutput {
	itemFields := append(runtimePruneCandidateOutputFields()[:8],
		OutputField{Name: "disposition", Type: OutputFieldTypeString, Description: "Exact tag retirement disposition.", Enum: []string{string(tobari.RuntimePruneRemoved), string(tobari.RuntimePruneAlreadyAbsent), string(tobari.RuntimePrunePreservedShared)}},
		OutputField{Name: "removed_tag_count", Type: OutputFieldTypeInteger, Description: "Exact Tobari-owned tag removals for this item."},
		OutputField{Name: "image_virtual_bytes", Type: OutputFieldTypeInteger, Description: "Bounded Docker virtual-size evidence, or null when unavailable.", Nullable: true},
		OutputField{Name: "reclaimed_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimed bytes; always null in V1.", Nullable: true},
	)
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared Runtime prune apply task identity."},
			{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Exact consumed Runtime prune plan reference.", ReferenceKind: tobari.RuntimePrunePlanReferenceKind},
			{Name: "state", Type: OutputFieldTypeString, Description: "Durable plan application state.", Enum: []string{string(tobari.RuntimePruneApplied), string(tobari.RuntimePruneAlreadyApplied), string(tobari.RuntimePruneEmpty)}},
			{Name: "items", Type: OutputFieldTypeArray, Description: "Complete ordered result for every exact plan candidate.", SemanticScope: "Every candidate bound into the consumed Runtime prune plan.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact prune item outcome.", Fields: itemFields}},
			{Name: "removed_tag_count", Type: OutputFieldTypeInteger, Description: "Total exact Tobari-owned tag removals."},
			{Name: "reclaimed_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimed bytes; always null in V1.", Nullable: true},
			{Name: "receipt_revision", Type: OutputFieldTypeInteger, Description: "Durable idempotency receipt revision, or zero for an empty plan."},
			{Name: "source_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: editable Runtime source was preserved."},
			{Name: "snapshots_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: immutable and staging source snapshots outside settled failed-build cleanup were preserved."},
			{Name: "history_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: Runtime revision authority and history were preserved."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "runtime_prune_result", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
	}
}
