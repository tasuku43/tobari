package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

func runtimeCommandSpecs() []CommandSpec {
	return []CommandSpec{
		{
			Path: "up", Summary: "Create or reconcile the single Tobari Realm",
			Args: "--root <path>", Effect: operation.EffectCreate, Role: RoleAct,
			Agent: AgentContract{
				CapabilityID: "realm.lifecycle",
				Outcome:      "Start one healthy Docker-isolated Realm for an existing host root",
				Inputs: []CommandInput{
					{
						Name: "--root", Source: InputSourceFlag, Required: true,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Existing host directory mounted read-write at /workspace.", AllowedValues: []string{},
					},
				},
				Output: textStatusOutput(),
				Prerequisites: []string{
					"Docker Engine and Docker Compose v2 are available.",
					"The selected root is shared with the Docker Engine VM when applicable.",
				},
				FixedTarget: fixedRealmTarget(),
				Errors: mutationCommandErrors("up",
					declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Validate the intended root."),
					declaredCommandError(fault.KindInvalidInput, "root_conflict", false, "status", "Inspect the existing Realm root."),
					declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the OPA policy before startup."),
					declaredCommandError(fault.KindInternal, "status_failed", false, "status", "Reconcile the confirmed startup through status."),
					declaredCommandError(fault.KindContract, "invalid_status_contract", false, "status", "Repair the runtime status contract."),
					declaredCommandError(fault.KindUnavailable, "realm_start_failed", false, "status", "Reconcile partial Docker state."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
				Mutation: &MutationContract{
					TargetKind: realm.TargetKind, TargetInputs: []string{},
					Impact: operation.Impact{
						Cardinality:  operation.CardinalityMany,
						Notification: operation.DeclarationNo,
						AccessChange: operation.DeclarationNo,
						Destructive:  operation.DeclarationNo,
					},
				},
			},
			handler: runRealmUp,
		},
		{
			Path: "status", Summary: "Inspect the single Tobari Realm",
			Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
			Agent: AgentContract{
				CapabilityID: "realm.lifecycle",
				Outcome:      "Observe the configured Realm, component health, root, proxy, policy, and recent error state",
				Inputs: []CommandInput{
					{
						Name: "--format", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select human text or schema-versioned JSON.", AllowedValues: []string{"text", "json"},
						DefaultValue: stringPointer("text"),
					},
				},
				Output: CommandOutput{
					Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
					Fields: []OutputField{
						{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether a Realm state file exists."},
						{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether all exact Realm components are running and healthy."},
						{Name: "root", Type: OutputFieldTypeString, Description: "Canonical host root or empty when unconfigured."},
						{Name: "proxy", Type: OutputFieldTypeString, Description: "Realm-internal explicit proxy endpoint or empty when unconfigured."},
						{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical host policy directory or empty when unconfigured."},
						{Name: "components", Type: OutputFieldTypeArray, Description: "Exact gateway, OPA, and Realm state observations."},
						{Name: "recent_error", Type: OutputFieldTypeString, Description: "Bounded recent runtime error summary or empty when none is known."},
					},
					Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
					JSONEnvelope: "status", JSONSchemaVersion: 1,
				},
				Prerequisites: []string{},
				Errors: readCommandErrors("status", true,
					declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect the local state file."),
					declaredCommandError(fault.KindInternal, "status_failed", false, "doctor", "Inspect Docker Engine and Realm state."),
					declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the runtime status contract."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "status", "Repair the status JSON projection."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
			},
			handler: runRealmStatus,
		},
		{
			Path: "shell", Summary: "Open an interactive shell in the running Realm",
			Effect: operation.EffectRead, Role: RoleAct,
			Agent: AgentContract{
				CapabilityID:  "realm.execute",
				Outcome:       "Enter the one running Realm at the host-relative current directory when it is inside the root",
				Inputs:        []CommandInput{},
				Output:        noOutput(),
				Prerequisites: []string{"The Tobari Realm is running.", "An interactive terminal is attached."},
				FixedTarget:   fixedRealmTarget(),
				Errors: readCommandErrors("shell", false,
					declaredCommandError(fault.KindInvalidInput, "invalid_exec_request", false, "help shell", "Repair the shell request."),
					declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect the local state file."),
					declaredCommandError(fault.KindUnavailable, "realm_not_running", false, "up", "Start the Realm."),
					declaredCommandError(fault.KindInternal, "exec_failed", false, "doctor", "Inspect Docker and Realm process execution."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
			},
			handler: runRealmShell,
		},
		{
			Path: "exec", Summary: "Run one exact argv inside the running Realm",
			Args: "[--cwd <path>] <command>", Effect: operation.EffectRead, Role: RoleAct,
			Agent: AgentContract{
				CapabilityID: "realm.execute",
				Outcome:      "Run an arbitrary command in the shared Realm and preserve its process exit status",
				Inputs: []CommandInput{
					{
						Name: "--cwd", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Existing host directory inside the configured root.", AllowedValues: []string{},
					},
					{
						Name: "command", Source: InputSourceArgument, Required: true,
						ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
						Description: "Exact command argv after the positional-only marker; values are not interpreted.", AllowedValues: []string{},
					},
				},
				Output:        noOutput(),
				Prerequisites: []string{"The Tobari Realm is running."},
				FixedTarget:   fixedRealmTarget(),
				Errors: readCommandErrors("exec", false,
					declaredCommandError(fault.KindInvalidInput, "invalid_exec_request", false, "help exec", "Pass one command after the positional-only marker."),
					declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect the local state file."),
					declaredCommandError(fault.KindUnavailable, "realm_not_running", false, "up", "Start the Realm."),
					declaredCommandError(fault.KindInternal, "exec_failed", false, "doctor", "Inspect Docker and Realm process execution."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
			},
			handler: runRealmExec,
		},
		{
			Path: "logs", Summary: "Read a bounded window of Tobari component logs",
			Args:   "[--component gateway|opa|realm|all] [--tail <lines>]",
			Effect: operation.EffectRead, Role: RoleUtility,
			Agent: AgentContract{
				CapabilityID: "realm.logs",
				Outcome:      "Inspect redacted Gateway, OPA, or Realm logs without reading Docker resources by guesswork",
				Inputs: []CommandInput{
					{
						Name: "--component", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select one exact component or all components.", AllowedValues: []string{"gateway", "opa", "realm", "all"},
						DefaultValue: stringPointer("all"),
					},
					{
						Name: "--tail", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
						Description: "Maximum lines read from each selected component.", AllowedValues: []string{},
						DefaultValue: stringPointer("200"), Minimum: int64Pointer(1), Maximum: int64Pointer(10_000),
					},
				},
				Output: CommandOutput{
					Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
					Fields: []OutputField{
						{Name: "line", Type: OutputFieldTypeString, Description: "One component log line with unsafe structural runes visibly escaped."},
					},
					Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				},
				Prerequisites: []string{"The selected component has been created."},
				Errors: readCommandErrors("logs", true,
					declaredCommandError(fault.KindInvalidInput, "invalid_log_request", false, "help logs", "Select a supported component and line bound."),
					declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect the local state file."),
					declaredCommandError(fault.KindUnavailable, "realm_not_running", false, "up", "Start the Realm."),
					declaredCommandError(fault.KindInternal, "logs_failed", false, "doctor", "Inspect Docker and component state."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
			},
			handler: runRealmLogs,
		},
		{
			Path: "down", Summary: "Remove the single Realm's owned transient resources",
			Args: "[--purge]", Effect: operation.EffectWrite, Role: RoleAct,
			Agent: AgentContract{
				CapabilityID: "realm.lifecycle",
				Outcome:      "Stop and remove owned containers and networks, preserving persistent volumes unless purge is explicit",
				Inputs: []CommandInput{
					{
						Name: "--purge", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
						Description: "Also remove the exact Realm home and Gateway CA volumes.", AllowedValues: []string{},
						DefaultValue: stringPointer("false"),
					},
				},
				Output:        textStatusOutput(),
				Prerequisites: []string{"Docker Engine is available when owned resources exist."},
				FixedTarget:   fixedRealmTarget(),
				Errors: mutationCommandErrors("down",
					declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect the local state file."),
					declaredCommandError(fault.KindUnavailable, "realm_stop_failed", false, "status", "Reconcile remaining Docker state."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
				Mutation: &MutationContract{
					TargetKind: realm.TargetKind, TargetInputs: []string{},
					Impact: operation.Impact{
						Cardinality:  operation.CardinalityMany,
						Notification: operation.DeclarationNo,
						AccessChange: operation.DeclarationNo,
						Destructive:  operation.DeclarationYes,
					},
				},
			},
			handler: runRealmDown,
		},
	}
}

func fixedRealmTarget() *FixedTarget {
	return &FixedTarget{
		Kind: realm.TargetKind, ID: realm.TargetID,
		Description: "This Tobari installation's one shared local Realm.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func noOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatNone}, DefaultFormat: OutputFormatNone,
		Fields: []OutputField{}, Delivery: OutputDeliveryComplete,
		CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func textStatusOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
		Fields: []OutputField{
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether Realm state remains configured."},
			{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether all Realm components are running."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func readCommandErrors(path string, hasOutput bool, extra ...CommandError) []CommandError {
	errors := []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help "+path, "Correct the command arguments."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
	}
	errors = append(errors, extra...)
	if hasOutput {
		errors = append(errors, declaredCommandError(fault.KindInternal, "output_write_failed", true, path, "Retry with a writable output stream."))
	}
	return errors
}

func mutationCommandErrors(path string, extra ...CommandError) []CommandError {
	errors := []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help "+path, "Correct the command arguments."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
	}
	errors = append(errors, extra...)
	errors = append(errors,
		declaredCommandError(fault.KindContract, "invalid_mutation_contract", false, "status", "Repair the mutation declaration and reconcile state."),
		declaredCommandError(fault.KindContract, "missing_mutation_action", false, "status", "Configure the mutation action and reconcile state."),
		declaredCommandError(fault.KindRejected, "missing_mutation_policy", false, "status", "Configure the project mutation policy."),
		declaredCommandError(fault.KindRejected, "mutation_rejected", false, "status", "Review the fixed Realm ownership policy."),
		declaredCommandError(fault.KindContract, "unclassified_mutation_outcome", false, "status", "Reconcile Realm state before another mutation."),
		declaredCommandError(fault.KindInternal, "mutation_output_write_failed", false, "status", "Reconcile the confirmed mutation without repeating it."),
	)
	return errors
}
