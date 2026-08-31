package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/installationmigrationcmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func installationMigrationPlanSpec() CommandSpec {
	fields := []OutputField{
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque exact installation migration plan.", ReferenceKind: tobari.InstallationMigrationPlanReferenceKind},
		{Name: "source_digest", Type: OutputFieldTypeString, Description: "Exact authority.json byte digest."},
		{Name: "runtime_source_digest", Type: OutputFieldTypeString, Description: "Exact predecessor Runtime catalog byte digest."},
		{Name: "source_generation", Type: OutputFieldTypeInteger, Description: "Exact legacy typed authority generation."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Exact legacy typed authority revision."},
		{Name: "template_count", Type: OutputFieldTypeInteger, Description: "Templates to migrate."},
		{Name: "context_count", Type: OutputFieldTypeInteger, Description: "Contexts to migrate."},
		{Name: "policy_memory_count", Type: OutputFieldTypeInteger, Description: "Context-owned Policy Memory objects to migrate."},
		{Name: "workspace_count", Type: OutputFieldTypeInteger, Description: "Workspaces to migrate."},
		{Name: "target_generation", Type: OutputFieldTypeInteger, Description: "Resulting active generation."},
	}
	errors := readCommandErrors(installationmigrationcmd.TaskPlan, true,
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report build identity without repeating migration-plan encoding."),
		declaredCommandError(fault.KindRejected, "installation_migration_not_supported", false, "doctor", "Only the exact supported typed authority.json can be planned."),
		declaredCommandError(fault.KindRejected, "installation_migration_source_rejected", false, "doctor", "Repair unsafe ownership or filesystem state before planning."),
		declaredCommandError(fault.KindUnavailable, "installation_migration_failed", false, "doctor", "Inspect the installation without modifying it."),
		classifiedCommandError(fault.KindContract, "invalid_installation_migration_plan", false, fault.PhaseVerification, fault.ChangeUnknown, "doctor", "Repair the typed installation migration plan result."),
	)
	return CommandSpec{Path: installationmigrationcmd.TaskPlan, Summary: "Review the exact supported installation migration", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "installation.migration", Outcome: "Bind one exact typed authority.json to a no-widening concept-separated migration", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("installation_migration_plan", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"The canonical authority root contains only one supported typed authority.json."}, Errors: errors}, handler: runInstallationMigrationPlan}
}

func installationMigrationApplySpec() CommandSpec {
	fields := []OutputField{
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Exact consumed migration plan.", ReferenceKind: tobari.InstallationMigrationPlanReferenceKind},
		{Name: "active_generation", Type: OutputFieldTypeInteger, Description: "New coherent active authority generation."},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "New coherent active authority revision."},
		{Name: "changed", Type: OutputFieldTypeBoolean, Description: "Always true after a committed migration."},
	}
	errors := mutationCommandErrors(installationmigrationcmd.TaskApply, installationmigrationcmd.TaskPlan,
		classifiedCommandError(fault.KindContract, "output_encoding_failed", false, fault.PhasePresentation, fault.ChangeConfirmed, "version", "Report build identity without repeating the confirmed migration."),
		declaredCommandError(fault.KindInvalidInput, "invalid_installation_migration_plan_ref", false, installationmigrationcmd.TaskPlan, "Pass one opaque plan unchanged."),
		declaredCommandError(fault.KindRejected, "installation_migration_not_supported", false, "doctor", "Only the exact supported typed authority.json can migrate."),
		declaredCommandError(fault.KindRejected, "installation_migration_source_rejected", false, "doctor", "Repair unsafe source or destination state."),
		declaredCommandError(fault.KindRejected, "installation_migration_plan_stale", false, installationmigrationcmd.TaskPlan, "Review a fresh plan after any drift."),
		declaredCommandError(fault.KindUnavailable, "installation_migration_incomplete", false, "doctor", "Inspect exact migration recovery state."),
		classifiedCommandError(fault.KindUnavailable, "installation_migration_failed", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect exact installation state."),
		classifiedCommandError(fault.KindContract, "invalid_installation_migration_result", false, fault.PhaseVerification, fault.ChangeUnknown, installationmigrationcmd.TaskPlan, "Reconcile exact installation authority before another migration."),
	)
	return CommandSpec{Path: installationmigrationcmd.TaskApply, Summary: "Apply one exact reviewed installation migration", Args: "--plan <installation-migration-plan-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "installation.migration", Outcome: "Publish concept sources and one verified coherent generation, then retire authority.json", Inputs: []CommandInput{finalReferenceInput("--plan", "Opaque plan emitted by installation migration plan and consumed unchanged.", tobari.InstallationMigrationPlanReferenceKind), formatInput()}, Output: finalJSONOutput("installation_migration", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"The exact planned authority.json and destination filesystem remain unchanged."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.InstallationMigrationPlanReferenceKind, TargetInputs: []string{"--plan"}, TargetIDInput: "--plan", Impact: installationmigrationcmd.Impact()}}, handler: runInstallationMigrationApply}
}

type installationMigrationPlanProjection struct {
	PlanRef             string                `json:"plan_ref"`
	SourceDigest        tobari.SemanticDigest `json:"source_digest"`
	RuntimeSourceDigest tobari.SemanticDigest `json:"runtime_source_digest"`
	SourceGeneration    uint64                `json:"source_generation"`
	SourceRevision      tobari.SemanticDigest `json:"source_revision"`
	TemplateCount       int                   `json:"template_count"`
	ContextCount        int                   `json:"context_count"`
	PolicyMemoryCount   int                   `json:"policy_memory_count"`
	WorkspaceCount      int                   `json:"workspace_count"`
	TargetGeneration    uint64                `json:"target_generation"`
}

type installationMigrationResultProjection struct {
	PlanRef          string                `json:"plan_ref"`
	ActiveGeneration uint64                `json:"active_generation"`
	ActiveRevision   tobari.SemanticDigest `json:"active_revision"`
	Changed          bool                  `json:"changed"`
}

func installationMigrationPlanText(plan tobari.InstallationMigrationPlan, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("·", "Installation migration planned", styleMuted)
	output.section("Authority")
	output.row("Source", fmt.Sprintf("generation %d · %s", plan.SourceGeneration, plan.SourceRevision), styleText)
	output.row("Target", fmt.Sprintf("generation %d", plan.TargetGeneration), styleText)
	output.row("Resources", fmt.Sprintf("%d Templates · %d Contexts · %d Policy memories · %d Workspaces", plan.TemplateCount, plan.ContextCount, plan.PolicyMemoryCount, plan.WorkspaceCount), styleText)
	output.next(installationmigrationcmd.TaskApply+" --plan="+plan.PlanRef, "Apply this exact reviewed migration.")
	output.section("Details")
	output.row("Plan reference", plan.PlanRef, styleText)
	output.row("Source digest", string(plan.SourceDigest), styleText)
	output.row("Runtime source", string(plan.RuntimeSourceDigest), styleText)
	return output.bytes()
}

func installationMigrationResultText(result tobari.InstallationMigrationResult, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Installation migration applied", styleSuccess)
	output.section("Authority")
	output.row("Status", "✓ active · migration committed", styleSuccess)
	output.row("Generation", fmt.Sprintf("%d", result.ActiveGeneration), styleText)
	output.row("Revision", string(result.ActiveRevision), styleText)
	output.row("Changed", humanBool(result.Changed), humanOutcomeBoolToken(result.Changed))
	output.next("doctor", "Verify the migrated installation and inspect its final authority.")
	output.section("Details")
	output.row("Plan reference", result.PlanRef, styleText)
	return output.bytes()
}

func runInstallationMigrationPlan(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.installationMigration == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	plan, err := c.installationMigration.Plan(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	projection := installationMigrationPlanProjection{
		PlanRef: plan.PlanRef, SourceDigest: plan.SourceDigest, RuntimeSourceDigest: plan.RuntimeSourceDigest, SourceGeneration: plan.SourceGeneration,
		SourceRevision: plan.SourceRevision, TemplateCount: plan.TemplateCount, ContextCount: plan.ContextCount,
		PolicyMemoryCount: plan.PolicyMemoryCount, WorkspaceCount: plan.WorkspaceCount, TargetGeneration: plan.TargetGeneration,
	}
	output, err := finalAuthorityOutput(command.Path, "installation_migration_plan", projection, format, installationMigrationPlanText(plan, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, fault.Wrap(fault.KindContract, "output_encoding_failed", "installation migration plan could not be encoded", false, err))
	}
	return c.emitResult(ctx, output)
}

func runInstallationMigrationApply(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.installationMigration == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	planRef := inputs.One("--plan")
	intent.Target = operation.TargetRef{Kind: tobari.InstallationMigrationPlanReferenceKind, ID: planRef}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.installationMigration.Apply(ctx, intent, planRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	projection := installationMigrationResultProjection{PlanRef: result.PlanRef, ActiveGeneration: result.ActiveGeneration, ActiveRevision: result.ActiveRevision, Changed: result.Changed}
	output, err := finalAuthorityOutput(command.Path, "installation_migration", projection, format, installationMigrationResultText(result, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		classified := fault.WithClassification(fault.Wrap(fault.KindContract, "output_encoding_failed", "installation migration result could not be encoded", false, err), fault.PhasePresentation, fault.ChangeConfirmed)
		return c.fail(ctx, classified)
	}
	return c.emitMutationResult(ctx, command, output)
}
