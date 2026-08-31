package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeDeleteItemProjection struct {
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

type runtimeDeleteProjection struct {
	Task                        string                                   `json:"task"`
	RuntimeID                   string                                   `json:"runtime_id"`
	RuntimeRef                  string                                   `json:"runtime_ref"`
	Name                        string                                   `json:"name"`
	State                       tobari.RuntimeDeleteState                `json:"state"`
	SourceLogicalBytes          int64                                    `json:"source_logical_bytes"`
	SnapshotLogicalBytes        int64                                    `json:"snapshot_logical_bytes"`
	SourceDisposition           tobari.RuntimeDeleteAuthorityDisposition `json:"source_disposition"`
	SnapshotsDisposition        tobari.RuntimeDeleteAuthorityDisposition `json:"snapshots_disposition"`
	HistoryDisposition          tobari.RuntimeDeleteAuthorityDisposition `json:"history_disposition"`
	Items                       []runtimeDeleteItemProjection            `json:"items"`
	RemovedTagCount             int                                      `json:"removed_tag_count"`
	ReclaimedBytes              *int64                                   `json:"reclaimed_bytes"`
	ReceiptRevision             uint64                                   `json:"receipt_revision"`
	WorkspaceManifestsPreserved bool                                     `json:"workspace_manifests_preserved"`
	WorkspacesPreserved         bool                                     `json:"workspaces_preserved"`
	WorkspaceIDsPreserved       bool                                     `json:"workspace_ids_preserved"`
	WorkspaceHomesPreserved     bool                                     `json:"workspace_homes_preserved"`
	AppliedReceiptsPreserved    bool                                     `json:"applied_receipts_preserved"`
	ProjectRootsPreserved       bool                                     `json:"project_roots_preserved"`
	CredentialsPreserved        bool                                     `json:"credentials_preserved"`
	SharedResourcesPreserved    bool                                     `json:"shared_resources_preserved"`
}

type runtimeDeleteDocument struct {
	SchemaVersion int                     `json:"schema_version"`
	Delete        runtimeDeleteProjection `json:"runtime_delete"`
}

func runRuntimeDelete(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime delete", "Correct the command arguments.")
	}
	runtimeRef := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: runtimeRef}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.runtime.Delete(ctx, intent, runtimeRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimeDelete(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		classified, ok := err.(*fault.Error)
		if !ok {
			classified = fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime delete result output could not be encoded", false, err)
		}
		phase := fault.PhasePresentation
		if classified.Code == "invalid_runtime_delete_result_confirmed" {
			phase = fault.PhaseVerification
		}
		return c.fail(ctx, fault.WithClassification(classified, phase, fault.ChangeConfirmed))
	}
	return c.emitMutationResult(ctx, command, output)
}

func renderRuntimeDelete(path string, result tobari.RuntimeDeleteResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_delete_result_confirmed", "Runtime delete result is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(path, runtimeDeleteDocument{SchemaVersion: 1, Delete: runtimeDeleteProjectionFrom(result)})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime delete result output could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	output.WriteString(applyStyleToken(color, styleAccent, "Delete Runtime "+safeExternalText(result.Name)))
	output.WriteString("\n\n")
	writeContextCardValue(&output, color, "Reference", result.RuntimeRef, styleAccent)
	writeContextCardValue(&output, color, "State", string(result.State), humanStatusToken("ready"))
	writeContextCardValue(&output, color, "Editable source", fmt.Sprintf("removed · %d B logical", result.SourceLogicalBytes), styleWarning)
	writeContextCardValue(&output, color, "Immutable snapshots", fmt.Sprintf("removed · %d B logical", result.SnapshotLogicalBytes), styleWarning)
	writeContextCardValue(&output, color, "Revision history", "removed", styleWarning)
	for _, item := range result.Items {
		output.WriteString("\n")
		output.WriteString(applyStyleToken(color, styleText, runtimePruneDisplayName(item.Name, item.Ordinal, item.Kind)))
		output.WriteString("\n")
		writeContextCardValue(&output, color, "Authority", item.RuntimeID+" · "+item.Revision, styleText)
		writeContextCardValue(&output, color, "Image disposition", string(item.Disposition), styleAccent)
		writeContextCardValue(&output, color, "Removed exact tags", fmt.Sprintf("%d", item.RemovedTagCount), styleText)
		writeContextCardValue(&output, color, "Reclaimed bytes", "unknown", styleWarning)
	}
	output.WriteString("\n")
	writeContextCardValue(&output, color, "Removed exact tags", fmt.Sprintf("%d", result.RemovedTagCount), styleText)
	writeContextCardValue(&output, color, "Reclaimed bytes", "unknown", styleWarning)
	writeContextCardValue(&output, color, "Receipt revision", fmt.Sprintf("%d", result.ReceiptRevision), styleText)
	writeContextCardValue(&output, color, "Preserved", "Workspace Templates · Contexts · Workspaces · IDs · homes · applied receipts · Project roots · credentials · shared resources", styleAccent)
	return []byte(output.String()), nil
}

func runtimeDeleteProjectionFrom(result tobari.RuntimeDeleteResult) runtimeDeleteProjection {
	items := make([]runtimeDeleteItemProjection, len(result.Items))
	for index, item := range result.Items {
		items[index] = runtimeDeleteItemProjection{
			Kind: item.Kind, RuntimeID: item.RuntimeID, Revision: item.Revision, Name: item.Name,
			Ordinal: item.Ordinal, LastUsed: item.LastUsed, SourceLogicalBytes: item.SourceLogicalBytes,
			SnapshotLogicalBytes: item.SnapshotLogicalBytes, Disposition: item.Disposition,
			RemovedTagCount: item.RemovedTagCount, ImageVirtualBytes: item.ImageVirtualBytes, ReclaimedBytes: item.ReclaimedBytes,
		}
	}
	return runtimeDeleteProjection{
		Task: result.Task, RuntimeID: result.RuntimeID, RuntimeRef: result.RuntimeRef, Name: result.Name, State: result.State,
		SourceLogicalBytes: result.SourceLogicalBytes, SnapshotLogicalBytes: result.SnapshotLogicalBytes,
		SourceDisposition: result.SourceDisposition, SnapshotsDisposition: result.SnapshotsDisposition, HistoryDisposition: result.HistoryDisposition,
		Items: items, RemovedTagCount: result.RemovedTagCount, ReclaimedBytes: result.ReclaimedBytes, ReceiptRevision: result.ReceiptRevision,
		WorkspaceManifestsPreserved: result.WorkspaceManifestsPreserved, WorkspacesPreserved: result.WorkspacesPreserved,
		WorkspaceIDsPreserved: result.WorkspaceIDsPreserved, WorkspaceHomesPreserved: result.WorkspaceHomesPreserved,
		AppliedReceiptsPreserved: result.AppliedReceiptsPreserved, ProjectRootsPreserved: result.ProjectRootsPreserved,
		CredentialsPreserved: result.CredentialsPreserved, SharedResourcesPreserved: result.SharedResourcesPreserved,
	}
}

func runtimeDeleteOutput() CommandOutput {
	itemFields := []OutputField{
		{Name: "kind", Type: OutputFieldTypeString, Description: "Successful Runtime revision or exact journaled failed-build artifact.", Enum: []string{string(tobari.RuntimePruneCandidateRevision), string(tobari.RuntimePruneCandidateFailedBuild)}},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Semantic source revision digest."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Human Runtime display name."},
		{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Human successful revision ordinal, or zero for a failed build artifact."},
		{Name: "last_used", Type: OutputFieldTypeString, Description: "V1 usage certainty; no timestamp is inferred.", Enum: []string{string(tobari.RuntimeLastUsedUnknown)}},
		{Name: "source_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the deleted editable Runtime source."},
		{Name: "snapshot_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical bytes in the deleted immutable or staging source snapshot."},
		OutputField{Name: "disposition", Type: OutputFieldTypeString, Description: "Exact tag retirement disposition.", Enum: []string{string(tobari.RuntimePruneRemoved), string(tobari.RuntimePruneAlreadyAbsent), string(tobari.RuntimePrunePreservedShared)}},
		OutputField{Name: "removed_tag_count", Type: OutputFieldTypeInteger, Description: "Exact Tobari-owned tag removals for this item."},
		OutputField{Name: "image_virtual_bytes", Type: OutputFieldTypeInteger, Description: "Bounded Docker virtual-size evidence, or null when unavailable.", Nullable: true},
		OutputField{Name: "reclaimed_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimed bytes; always null in V1.", Nullable: true},
	}
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared whole-Runtime deletion task identity."},
			{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable deleted Runtime authority ID."},
			{Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Exact consumed Runtime reference retained by the idempotency receipt.", ReferenceKind: tobari.RuntimeReferenceKind},
			{Name: "name", Type: OutputFieldTypeString, Description: "Deleted Runtime display name."},
			{Name: "state", Type: OutputFieldTypeString, Description: "Durable whole-Runtime deletion state.", Enum: []string{string(tobari.RuntimeDeleted), string(tobari.RuntimeAlreadyDeleted)}},
			{Name: "source_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical editable-source bytes removed."},
			{Name: "snapshot_logical_bytes", Type: OutputFieldTypeInteger, Description: "Logical immutable and settled failed-build snapshot bytes removed."},
			{Name: "source_disposition", Type: OutputFieldTypeString, Description: "Editable source authority disposition.", Enum: []string{string(tobari.RuntimeDeleteAuthorityRemoved)}},
			{Name: "snapshots_disposition", Type: OutputFieldTypeString, Description: "Immutable snapshot authority disposition.", Enum: []string{string(tobari.RuntimeDeleteAuthorityRemoved)}},
			{Name: "history_disposition", Type: OutputFieldTypeString, Description: "Runtime revision-history authority disposition.", Enum: []string{string(tobari.RuntimeDeleteAuthorityRemoved)}},
			{Name: "items", Type: OutputFieldTypeArray, Description: "Complete ordered image-material outcomes for the deleted Runtime.", SemanticScope: "Every successful revision and settled failed-build material bound into the whole-Runtime deletion.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact material outcome.", Fields: itemFields}},
			{Name: "removed_tag_count", Type: OutputFieldTypeInteger, Description: "Total exact Tobari-owned tag removals."},
			{Name: "reclaimed_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimed bytes; always null in V1.", Nullable: true},
			{Name: "receipt_revision", Type: OutputFieldTypeInteger, Description: "Durable idempotency receipt revision."},
			{Name: "workspace_manifests_preserved", Type: OutputFieldTypeBoolean, Description: "Legacy-named field retained by the unchanged Runtime schema; always true because no Workspace Template, Context, or Workspace was deleted or rewritten."},
			{Name: "workspaces_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no Workspace was deleted or rewritten."},
			{Name: "workspace_ids_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no workspace_id was deleted or rewritten."},
			{Name: "workspace_homes_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no Workspace home was deleted or rewritten."},
			{Name: "applied_receipts_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no applied receipt was deleted or rewritten."},
			{Name: "project_roots_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no Project root was deleted or rewritten."},
			{Name: "credentials_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: no credential state was deleted or rewritten."},
			{Name: "shared_resources_preserved", Type: OutputFieldTypeBoolean, Description: "Always true: shared image content and non-Runtime resources were preserved."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "runtime_delete", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
	}
}
