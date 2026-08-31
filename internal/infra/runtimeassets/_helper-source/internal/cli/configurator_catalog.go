package cli

import (
	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func configureSpec() CommandSpec {
	return CommandSpec{
		Path: "configure", Summary: "Create or update this Project's configuration with an isolated agent",
		Args: "[--agent codex|claude]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "configuration.authoring",
			Outcome:       "Prepare the exact current Context Runtime, author a host-frozen submission with one selected agent, review it, and Apply the Project configuration",
			Inputs:        []CommandInput{{Name: "--agent", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Pinned agent client; omission opens the interactive chooser.", AllowedValues: []string{"codex", "claude"}}},
			Output:        noOutput(),
			Prerequisites: []string{"An interactive terminal and Docker are available; bootstrap prepares the standard Runtime and an existing Context prepares its exact Runtime revision."},
			FixedTarget:   &FixedTarget{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID, Description: "The current Project's configuration authority and non-authoritative managed working Home.", Scope: FixedTargetScopeToolLocal},
			Errors: configureErrors(
				classifiedCommandError(fault.KindRejected, "configurator_interactive_required", false, fault.PhasePrecondition, fault.ChangeNone, "help configure", "Run from an interactive terminal."),
				classifiedCommandError(fault.KindRejected, "configuration_runtime_review_pending", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the confirmed Runtime revision before resuming configure."),
				classifiedCommandError(fault.KindCanceled, "configuration_material_retained", false, fault.PhaseMutation, fault.ChangeConfirmed, "status", "Reconcile current authority before resuming the retained Project configuration material."),
				classifiedCommandError(fault.KindUnavailable, "configuration_cleanup_incomplete", false, fault.PhaseMutation, fault.ChangePartial, "status", "Reconcile current Project authority and Docker health before resuming configuration."),
				declaredCommandError(fault.KindCanceled, "configuration_canceled", false, "help configure", "Review the isolated authoring boundary and select an agent when ready."),
				classifiedCommandError(fault.KindInternal, "configurator_boundary_output_failed", false, fault.PhasePrecondition, fault.ChangeNone, "help configure", "Retry with a writable interactive terminal before preparing Runtime material."),
				declaredCommandError(fault.KindInternal, "missing_configurator", false, "doctor", "Inspect Configurator composition."),
			),
			Mutation: &MutationContract{TargetKind: tobari.ProjectConfigurationTargetKind, TargetInputs: []string{}, Impact: configuratorcmd.Impact()},
		},
		handler: runConfigure,
	}
}

func configureErrors(extra ...CommandError) []CommandError {
	errors := mutationCommandErrors("configure", "help configure", extra...)
	result := make([]CommandError, 0, len(errors))
	for _, item := range errors {
		if item.Code == "mutation_output_write_failed" {
			continue
		}
		if item.Code == "operation_canceled" {
			item.NextActions = []fault.NextAction{{Command: "help configure", Reason: "Review the isolated authoring boundary before retrying."}}
		}
		result = append(result, item)
	}
	return result
}
