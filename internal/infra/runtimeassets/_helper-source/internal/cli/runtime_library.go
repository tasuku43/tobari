package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeListDocument struct {
	SchemaVersion int `json:"schema_version"`
	Runtimes      struct {
		Task  string                  `json:"task"`
		Items []tobari.RuntimeSummary `json:"items"`
	} `json:"runtimes"`
}

type runtimeReportDocument struct {
	SchemaVersion int                        `json:"schema_version"`
	Runtime       tobari.RuntimePublicReport `json:"runtime"`
}

type runtimeRestoreDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Restore       tobari.RuntimeRestoreResult `json:"runtime_restore"`
}

func runRuntimeList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	return runRuntimeListCommand(ctx, c, command, inputs)
}

func runRuntimeReview(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help review runtimes", "Correct the command arguments.")
	}
	if !runtimeReviewAvailable(ctx, c, format) {
		return runRuntimeListCommand(ctx, c, command, inputs)
	}
	deletion, found, err := c.runtime.ReviewDeleteRecovery(ctx)
	if err != nil {
		return c.fail(ctx, normalizeRuntimeReviewError(command.Path, err))
	}
	if found {
		confirmed, confirmErr := confirmRuntimeDeleteRecovery(ctx, c, deletion)
		if confirmErr != nil {
			return c.fail(ctx, normalizeRuntimeReviewError(command.Path, confirmErr))
		}
		if !confirmed {
			return c.fail(ctx, context.Canceled)
		}
		deleteCommand, registered := c.catalog.lookupRegistered("runtime delete")
		if !registered {
			return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Runtime delete recovery action is missing.", false))
		}
		actionCtx := withCommandPath(ctx, deleteCommand.Path)
		intent := operation.Intent{Command: deleteCommand.Path, Effect: deleteCommand.Effect, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: deletion.RuntimeRef}, Impact: deleteCommand.Agent.Mutation.Impact}
		result, deleteErr := c.runtime.Delete(actionCtx, intent, deletion.RuntimeRef)
		if deleteErr != nil {
			return c.fail(actionCtx, deleteErr)
		}
		output, renderErr := renderRuntimeDelete(deleteCommand.Path, result, successFormatText, humanStyleAllowed(actionCtx, c, c.Out))
		if renderErr != nil {
			classified, ok := renderErr.(*fault.Error)
			if !ok {
				classified = fault.Wrap(fault.KindContract, "output_encoding_failed", "Runtime delete result output could not be encoded", false, renderErr)
			}
			return c.fail(actionCtx, fault.WithClassification(classified, fault.PhasePresentation, fault.ChangeConfirmed))
		}
		return c.emitMutationResult(actionCtx, deleteCommand, output)
	}
	recovery, found, err := c.runtime.ReviewRecovery(ctx)
	if err != nil {
		return c.fail(ctx, normalizeRuntimeReviewError(command.Path, err))
	}
	if found {
		confirmed, confirmErr := confirmRuntimeBuildRecovery(ctx, c, recovery)
		if confirmErr != nil {
			return c.fail(ctx, normalizeRuntimeReviewError(command.Path, confirmErr))
		}
		if !confirmed {
			return c.fail(ctx, context.Canceled)
		}
		if recovery.RevisionRef != "" {
			restore, registered := c.catalog.lookupRegistered("runtime restore")
			if !registered {
				return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Runtime restore recovery action is missing.", false))
			}
			actionCtx := withCommandPath(ctx, restore.Path)
			intent := operation.Intent{Command: restore.Path, Effect: restore.Effect, Target: operation.TargetRef{Kind: tobari.RuntimeRevisionReferenceKind, ID: recovery.RevisionRef}, Impact: restore.Agent.Mutation.Impact}
			diagnostics := newRuntimeBuildOutput(c.Err, humanStyleAllowed(actionCtx, c, c.Err))
			result, recoverErr := c.runtime.RecoverRestore(actionCtx, intent, recovery, diagnostics)
			if recoverErr != nil {
				return c.fail(actionCtx, recoverErr)
			}
			diagnostics.Flush()
			output, renderErr := renderRuntimeRestore(restore.Path, result, successFormatText, humanStyleAllowed(actionCtx, c, c.Out))
			if renderErr != nil {
				return c.fail(actionCtx, renderErr)
			}
			return c.emitMutationResult(actionCtx, restore, output)
		}
		build, registered := c.catalog.lookupRegistered("runtime build")
		if !registered {
			return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Runtime Review recovery action is missing.", false))
		}
		actionCtx := withCommandPath(ctx, build.Path)
		intent := operation.Intent{Command: build.Path, Effect: build.Effect, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: recovery.RuntimeRef}, Impact: build.Agent.Mutation.Impact}
		result, recoverErr := c.runtime.Recover(actionCtx, intent, recovery)
		if recoverErr != nil {
			return c.fail(actionCtx, recoverErr)
		}
		output, renderErr := renderRuntimeReport(build.Path, result, successFormatText, humanStyleAllowed(actionCtx, c, c.Out))
		if renderErr != nil {
			return c.fail(actionCtx, renderErr)
		}
		return c.emitMutationResult(actionCtx, build, output)
	}
	runtimeRef, err := chooseRuntimeBuild(ctx, c)
	if err != nil {
		return c.fail(ctx, normalizeRuntimeReviewError(command.Path, err))
	}
	build, found := c.catalog.lookupRegistered("runtime build")
	if !found {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Runtime Review build action is missing.", false))
	}
	buildInputs := ParsedInputs{
		values:   map[string][]string{"--id": {runtimeRef}, "--format": {"text"}},
		provided: map[string]bool{"--id": true},
		defaults: map[string]bool{"--format": true},
	}
	return runRuntimeBuild(withCommandPath(ctx, build.Path), c, build, operation.Intent{Command: build.Path, Effect: build.Effect}, buildInputs)
}

func runRuntimeListCommand(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime list", "Correct the command arguments.")
	}
	result, err := c.runtime.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimeList(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runRuntimeShow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	return runRuntimeRead(ctx, c, command, inputs, false)
}

func runRuntimeHistory(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	return runRuntimeRead(ctx, c, command, inputs, true)
}

func runRuntimeRead(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs, history bool) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help "+command.Path, "Correct the command arguments.")
	}
	var result tobari.RuntimeReport
	if history {
		result, err = c.runtime.History(ctx, inputs.One("--name"))
	} else {
		result, err = c.runtime.Show(ctx, inputs.One("--name"))
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimeReport(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runRuntimeCreate(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime create", "Correct the command arguments.")
	}
	base := inputs.One("--copy-source-from")
	if !inputs.Provided("--copy-source-from") {
		base = tobari.StandardRuntimeName
		if runtimeReviewAvailable(ctx, c, format) {
			base, err = chooseRuntimeCreateBase(ctx, c, inputs.One("--name"))
			if err != nil {
				return c.fail(ctx, normalizeRuntimeReviewError(command.Path, err))
			}
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.runtime.Create(ctx, intent, inputs.One("--name"), base)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderRuntimeReport(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeBuild(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime build", "Correct the command arguments.")
	}
	runtimeRef := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: runtimeRef}
	intent.Impact = command.Agent.Mutation.Impact
	buildOutput := newRuntimeBuildOutput(c.Err, humanStyleAllowed(ctx, c, c.Err))
	result, err := c.runtime.Build(ctx, intent, runtimeRef, buildOutput)
	if err != nil {
		code := c.fail(ctx, err)
		if invocationErrorFormat(ctx) == errorFormatText {
			buildOutput.WriteFailureSummary()
		}
		return code
	}
	buildOutput.Flush()
	output, err := renderRuntimeReport(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeRestore(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime restore", "Correct the command arguments.")
	}
	revisionRef := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.RuntimeRevisionReferenceKind, ID: revisionRef}
	intent.Impact = command.Agent.Mutation.Impact
	diagnostics := newRuntimeBuildOutput(c.Err, humanStyleAllowed(ctx, c, c.Err))
	result, err := c.runtime.Restore(ctx, intent, revisionRef, diagnostics)
	if err != nil {
		return c.fail(ctx, err)
	}
	diagnostics.Flush()
	output, err := renderRuntimeRestore(command.Path, result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func renderRuntimeList(path string, result tobari.RuntimeListResult, format successFormat, color bool) ([]byte, error) {
	if err := result.ValidatePublic(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_list", "Runtime list is invalid", false, err)
	}
	if format == successFormatJSON {
		document := runtimeListDocument{SchemaVersion: 1}
		document.Runtimes.Task, document.Runtimes.Items = result.Task, result.Items
		output, err := marshalCommandJSON(path, document)
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	output.WriteString(applyStyleToken(color, styleAccent, "Runtimes"))
	output.WriteString("\n\n")
	for _, item := range result.Items {
		if item.Kind == tobari.RuntimeKindManaged && item.Ready && item.RevisionRef != tobari.RuntimeRevisionRef(item.ID, item.Revision) {
			return nil, fault.New(fault.KindContract, "invalid_runtime_list", "Managed Runtime summary has no exact head revision reference.", false)
		}
		state := "draft"
		if item.Ready {
			state = fmt.Sprintf("ready · %s@%d · %s", safeExternalText(item.Name), item.Head, item.Revision[:12])
		}
		fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleText, safeExternalText(item.Name)))
		writeContextCardValue(&output, color, "Reference", item.RuntimeRef, styleAccent)
		if item.Kind == tobari.RuntimeKindManaged && item.Ready {
			writeContextCardValue(&output, color, "Revision reference", item.RevisionRef, styleAccent)
		}
		writeContextCardValue(&output, color, "Status", state, humanStatusToken(map[bool]string{true: "ready", false: "draft"}[item.Ready]))
		writeContextCardValue(&output, color, "Kind", string(item.Kind), styleText)
		if item.Ready {
			writeContextCardValue(&output, color, "Availability", string(item.Availability.State), humanStatusToken(string(item.Availability.State)))
			writeContextCardValue(&output, color, "Last used", string(item.LastUsed.State), styleMuted)
			writeContextCardValue(&output, color, "Snapshot", string(item.Snapshot.State), styleText)
			if item.Storage != nil {
				writeContextCardValue(&output, color, "Storage", fmt.Sprintf("source %s logical · snapshot %s logical · image %s virtual · reclaimable unknown",
					optionalBytes(item.Storage.SourceLogicalBytes), optionalBytes(item.Storage.SnapshotLogicalBytes), optionalBytes(item.Storage.ImageVirtualBytes)), styleText)
			}
		}
		if item.SourcePath != "" {
			writeContextCardValue(&output, color, "Source", safeExternalText(item.SourcePath), styleText)
		}
		output.WriteString("\n")
	}
	return []byte(output.String()), nil
}

func renderRuntimeReport(path string, result tobari.RuntimeReport, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime report is invalid", false, err)
	}
	if result.Public == nil {
		return nil, fault.New(fault.KindContract, "invalid_runtime_report", "Runtime report has no semantic lifecycle projection.", false)
	}
	if err := result.Public.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime semantic lifecycle projection is invalid", false, err)
	}
	if result.Runtime.RuntimeRef != tobari.RuntimeRef(result.Runtime.ID) || result.Public.Runtime.RuntimeRef != result.Runtime.RuntimeRef {
		return nil, fault.New(fault.KindContract, "invalid_runtime_report", "Runtime report has no exact Runtime reference.", false)
	}
	for _, revision := range result.Runtime.Revisions {
		if revision.RuntimeRef != result.Runtime.RuntimeRef {
			return nil, fault.New(fault.KindContract, "invalid_runtime_report", "Runtime revision has no exact owning Runtime reference.", false)
		}
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(path, runtimeReportDocument{SchemaVersion: 1, Runtime: *result.Public})
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	manifest := result.Public.Runtime
	fmt.Fprintf(&output, "%s\n\n", applyStyleToken(color, styleAccent, "Runtime "+safeExternalText(manifest.Name)))
	writeContextCardValue(&output, color, "Reference", manifest.RuntimeRef, styleAccent)
	writeContextCardValue(&output, color, "Kind", string(manifest.Kind), styleText)
	if manifest.SourcePath != "" {
		writeContextCardValue(&output, color, "Source", safeExternalText(manifest.SourcePath), styleText)
		if result.Created {
			writeContextCardValue(&output, color, "Source rules", "no group/other permissions · 1,024 files · 256 directories · 32 MiB/file · 64 MiB total", styleText)
		}
	}
	if len(manifest.Revisions) == 0 {
		writeContextCardValue(&output, color, "Status", "draft · edit the current source", styleWarning)
		if path == "runtime create" {
			writeContextCardValue(&output, color, "Next", ProgramName+" runtime build --id "+manifest.RuntimeRef, styleAccent)
		}
	} else {
		for _, revision := range manifest.Revisions {
			value := fmt.Sprintf("%s@%d · %s · %s", safeExternalText(manifest.Name), revision.Ordinal, revision.SourceDigest[:12], revision.CreatedAt.Format("2006-01-02 15:04 UTC"))
			writeContextCardValue(&output, color, "Revision", value, styleText)
			if revision.RevisionRef != "" {
				writeContextCardValue(&output, color, "Revision reference", revision.RevisionRef, styleAccent)
			}
			writeContextCardValue(&output, color, "Availability", string(revision.Availability.State), humanStatusToken(string(revision.Availability.State)))
			writeContextCardValue(&output, color, "Last used", string(revision.LastUsed.State), styleMuted)
			writeContextCardValue(&output, color, "Snapshot", string(revision.Snapshot.State), styleText)
			if revision.Storage != nil {
				writeContextCardValue(&output, color, "Storage", fmt.Sprintf("source %s logical · snapshot %s logical · image %s virtual · reclaimable unknown",
					optionalBytes(revision.Storage.SourceLogicalBytes), optionalBytes(revision.Storage.SnapshotLogicalBytes), optionalBytes(revision.Storage.ImageVirtualBytes)), styleText)
			}
		}
	}
	if result.NoChange {
		writeContextCardValue(&output, color, "Build", "unchanged · no revision created", styleMuted)
	}
	if result.Built {
		writeContextCardValue(&output, color, "Build", "revision created · no Workspace Template changed", styleAccent)
	}
	if path == "runtime build" && (result.Built || result.NoChange) && len(manifest.Revisions) != 0 {
		writeContextCardValue(&output, color, "Next", ProgramName+" template list", styleAccent)
	}
	return []byte(output.String()), nil
}

func renderRuntimeRestore(path string, result tobari.RuntimeRestoreResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.WithClassification(
			fault.Wrap(fault.KindContract, "invalid_runtime_restore_result_confirmed", "Runtime restore result is invalid", false, err),
			fault.PhaseVerification, fault.ChangeConfirmed,
		)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(path, runtimeRestoreDocument{SchemaVersion: 1, Restore: result})
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	title := "Runtime revision restored"
	if result.State == tobari.RuntimeAlreadyAvailable {
		title = "Runtime revision already available"
	}
	fmt.Fprintf(&output, "%s\n\n", applyStyleToken(color, styleAccent, title))
	writeContextCardValue(&output, color, "Runtime", safeExternalText(result.Name), styleText)
	writeContextCardValue(&output, color, "Revision reference", result.RevisionRef, styleAccent)
	state := "restored · exact content digest confirmed"
	if result.State == tobari.RuntimeAlreadyAvailable {
		state = "already available · no durable state changed"
	}
	writeContextCardValue(&output, color, "State", state, styleAccent)
	writeContextCardValue(&output, color, "History", "unchanged", styleText)
	writeContextCardValue(&output, color, "Workspace Templates and Contexts", "unchanged", styleText)
	writeContextCardValue(&output, color, "Workspaces", "unchanged", styleText)
	return []byte(output.String()), nil
}

func runtimeListOutput() CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields:   []OutputField{{Name: "task", Type: OutputFieldTypeString, Description: "Declared Runtime list task identity."}, {Name: "items", Type: OutputFieldTypeArray, Description: "Complete local Runtime catalog.", SemanticScope: "This Tobari installation's Runtime catalog.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Runtime summary.", Fields: runtimeSummaryOutputFields()}}},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "runtimes", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func runtimeSummaryOutputFields() []OutputField {
	return []OutputField{{Name: "id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."}, {Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque stable Runtime reference.", ReferenceKind: tobari.RuntimeReferenceKind}, {Name: "revision_ref", Type: OutputFieldTypeString, Description: "Opaque exact managed head revision reference; omitted for built-in standard and drafts.", ReferenceKind: tobari.RuntimeRevisionReferenceKind, Optional: true}, {Name: "name", Type: OutputFieldTypeString, Description: "Unique local Runtime name."}, {Name: "kind", Type: OutputFieldTypeString, Description: "Built-in or managed source.", Enum: []string{"builtin", "managed"}}, {Name: "ready", Type: OutputFieldTypeBoolean, Description: "Whether at least one successful revision exists; independent from current material availability."}, {Name: "head", Type: OutputFieldTypeInteger, Description: "Latest successful human ordinal.", Optional: true}, {Name: "source_digest", Type: OutputFieldTypeString, Description: "Latest semantic SHA-256 source identity.", Optional: true}, {Name: "source_path", Type: OutputFieldTypeString, Description: "Managed editable source directory.", Optional: true},
		{Name: "availability", Type: OutputFieldTypeObject, Description: "Current head material availability; null for a draft.", Nullable: true, Fields: runtimeAvailabilityOutputFields()},
		{Name: "storage", Type: OutputFieldTypeObject, Description: "Current head bounded storage evidence; null for standard or a draft.", Nullable: true, Fields: runtimeRevisionStorageOutputFields()},
		{Name: "last_used", Type: OutputFieldTypeObject, Description: "Current head usage certainty; null for a draft.", Nullable: true, Fields: runtimeLastUsedOutputFields()},
		{Name: "snapshot", Type: OutputFieldTypeObject, Description: "Current head immutable snapshot state; null for a draft.", Nullable: true, Fields: runtimeSnapshotOutputFields()},
	}
}

func runtimeReportOutput() CommandOutput {
	return runtimeReportOutputWithRevisionReferences(true)
}

func runtimeCreateOutput() CommandOutput {
	return runtimeReportOutputWithRevisionReferences(false)
}

func runtimeReportOutputWithRevisionReferences(includeRevisionReferences bool) CommandOutput {
	revisionFields := []OutputField{{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Contiguous human revision ordinal."}, {Name: "source_digest", Type: OutputFieldTypeString, Description: "Semantic SHA-256 source identity."}, {Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque stable owning Runtime reference.", ReferenceKind: tobari.RuntimeReferenceKind}, {Name: "revision_ref", Type: OutputFieldTypeString, Description: "Opaque exact managed revision reference; omitted for built-in standard.", ReferenceKind: tobari.RuntimeRevisionReferenceKind, Optional: true}, {Name: "created_at", Type: OutputFieldTypeString, Description: "UTC revision creation time."},
		{Name: "availability", Type: OutputFieldTypeObject, Description: "Current local execution-material availability.", Fields: runtimeAvailabilityOutputFields()},
		{Name: "storage", Type: OutputFieldTypeObject, Description: "Bounded storage evidence; null for the built-in standard Runtime.", Nullable: true, Fields: runtimeRevisionStorageOutputFields()},
		{Name: "last_used", Type: OutputFieldTypeObject, Description: "Usage certainty without inferred history.", Fields: runtimeLastUsedOutputFields()},
		{Name: "snapshot", Type: OutputFieldTypeObject, Description: "Immutable source snapshot state without a private path.", Fields: runtimeSnapshotOutputFields()},
	}
	if !includeRevisionReferences {
		revisionFields = []OutputField{{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Contiguous human revision ordinal."}, {Name: "source_digest", Type: OutputFieldTypeString, Description: "Semantic SHA-256 source identity."}, {Name: "created_at", Type: OutputFieldTypeString, Description: "UTC revision creation time."},
			{Name: "availability", Type: OutputFieldTypeObject, Description: "Current local execution-material availability.", Fields: runtimeAvailabilityOutputFields()},
			{Name: "storage", Type: OutputFieldTypeObject, Description: "Bounded storage evidence.", Nullable: true, Fields: runtimeRevisionStorageOutputFields()},
			{Name: "last_used", Type: OutputFieldTypeObject, Description: "Usage certainty without inferred history.", Fields: runtimeLastUsedOutputFields()},
			{Name: "snapshot", Type: OutputFieldTypeObject, Description: "Immutable source snapshot state without a private path.", Fields: runtimeSnapshotOutputFields()},
		}
	}
	manifestFields := []OutputField{{Name: "schema_version", Type: OutputFieldTypeInteger, Description: "Runtime manifest schema version."}, {Name: "id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."}, {Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque stable Runtime reference.", ReferenceKind: tobari.RuntimeReferenceKind}, {Name: "name", Type: OutputFieldTypeString, Description: "Unique local Runtime name."}, {Name: "kind", Type: OutputFieldTypeString, Description: "Built-in or managed source.", Enum: []string{"builtin", "managed"}}, {Name: "source_path", Type: OutputFieldTypeString, Description: "Managed editable source directory; its root and children must have no group/other permissions and stay within the declared Runtime source limits.", Optional: true}, {Name: "revisions", Type: OutputFieldTypeArray, Description: "Ordered immutable successful revisions.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One successful Runtime revision.", Fields: revisionFields}}}
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields:   []OutputField{{Name: "task", Type: OutputFieldTypeString, Description: "Declared Runtime task identity."}, {Name: "runtime", Type: OutputFieldTypeObject, Description: "Complete Runtime authority record.", Fields: manifestFields}, {Name: "created", Type: OutputFieldTypeBoolean, Description: "Whether this invocation created the Runtime.", Optional: true}, {Name: "built", Type: OutputFieldTypeBoolean, Description: "Whether this invocation appended a revision.", Optional: true}, {Name: "no_change", Type: OutputFieldTypeBoolean, Description: "Whether source matched existing history.", Optional: true}},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "runtime", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func runtimeAvailabilityOutputFields() []OutputField {
	return []OutputField{{Name: "state", Type: OutputFieldTypeString, Description: "Current material state, independent from immutable history readiness.", Enum: []string{"available", "missing", "mismatched", "unknown", "pruned"}}}
}

func runtimeRevisionStorageOutputFields() []OutputField {
	return []OutputField{
		{Name: "source_logical_bytes", Type: OutputFieldTypeInteger, Description: "Exact logical bytes in the editable managed source.", Nullable: true},
		{Name: "snapshot_logical_bytes", Type: OutputFieldTypeInteger, Description: "Exact logical bytes in the immutable revision snapshot.", Nullable: true},
		{Name: "image_virtual_bytes", Type: OutputFieldTypeInteger, Description: "Non-additive Docker virtual-size evidence, or null when unavailable.", Nullable: true},
		{Name: "reclaimable_bytes", Type: OutputFieldTypeInteger, Description: "Authoritative reclaimable bytes; always null in V1.", Nullable: true},
	}
}

func runtimeLastUsedOutputFields() []OutputField {
	return []OutputField{{Name: "state", Type: OutputFieldTypeString, Description: "V1 usage certainty.", Enum: []string{"unknown"}}, {Name: "observed_at", Type: OutputFieldTypeString, Description: "Exact usage observation time; null because V1 has no usage receipt.", Nullable: true}}
}

func runtimeSnapshotOutputFields() []OutputField {
	return []OutputField{{Name: "state", Type: OutputFieldTypeString, Description: "Whether an exact retained snapshot applies.", Enum: []string{"retained", "not_applicable"}}}
}

func runtimeRestoreOutput() CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared Runtime restore task identity."},
			{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority ID."},
			{Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque stable owning Runtime reference.", ReferenceKind: tobari.RuntimeReferenceKind},
			{Name: "revision", Type: OutputFieldTypeString, Description: "Exact semantic revision digest."},
			{Name: "revision_ref", Type: OutputFieldTypeString, Description: "Opaque exact managed Runtime revision reference.", ReferenceKind: tobari.RuntimeRevisionReferenceKind},
			{Name: "name", Type: OutputFieldTypeString, Description: "Managed Runtime display name."},
			{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Human revision ordinal."},
			{Name: "state", Type: OutputFieldTypeString, Description: "Whether material was restored or already available.", Enum: []string{"restored", "already_available"}},
			{Name: "digest_match", Type: OutputFieldTypeBoolean, Description: "Whether exact restored content matches immutable recorded authority."},
			{Name: "artifact_disposition", Type: OutputFieldTypeString, Description: "Whether no staging artifact was created or the owned staging artifact was removed.", Enum: []string{"not_created", "removed"}},
			{Name: "revision_appended", Type: OutputFieldTypeBoolean, Description: "Always false; restore never appends history."},
			{Name: "manifest_changed", Type: OutputFieldTypeBoolean, Description: "Legacy-named field retained by the unchanged Runtime schema; always false because restore changes no Workspace Template, Context, or Workspace authority."},
			{Name: "workspace_changed", Type: OutputFieldTypeBoolean, Description: "Always false; restore never changes a Workspace."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "runtime_restore", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}
