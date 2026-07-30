package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runtimeCommandSpecs() []CommandSpec {
	return []CommandSpec{
		clusterUpSpec(),
		clusterStatusSpec(),
		clusterLogsSpec(),
		clusterDownSpec(),
		attachSpec(),
		listSpec(),
		shellSpec(),
		execSpec(),
		logsSpec(),
		detachSpec(),
	}
}

func clusterUpSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster up", Summary: "Start shared Gateway and OPA",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Start one healthy shared enforcement cluster without mounting a work root",
			Inputs:       []CommandInput{},
			Output:       textClusterStatusOutput(),
			Prerequisites: []string{
				"Docker Engine and Docker Compose v2 are available.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster up", "cluster status",
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the OPA policy before startup."),
				declaredCommandError(fault.KindRejected, "asset_conflict", false, "list", "Detach Tobari using the old asset before upgrade."),
				declaredCommandError(fault.KindRejected, "legacy_state", false, "doctor", "Remove schema-1 state with the older binary."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "cluster status", "Reconcile the confirmed startup."),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "cluster status", "Repair the runtime status contract."),
				declaredCommandError(fault.KindUnavailable, "cluster_start_failed", false, "cluster status", "Reconcile partial Docker state."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ClusterTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runClusterUp,
	}
}

func clusterStatusSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster status", Summary: "Inspect shared Gateway and OPA",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Observe cluster health, proxy, XDG policy path, attached count, and recent errors",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
				Fields: []OutputField{
					{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether schema-2 cluster state exists."},
					{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether Gateway and OPA are healthy."},
					{Name: "proxy", Type: OutputFieldTypeString, Description: "Tobari-internal explicit proxy endpoint."},
					{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical host XDG policy directory."},
					{Name: "tobari_count", Type: OutputFieldTypeInteger, Description: "Number of attached Tobari."},
					{Name: "components", Type: OutputFieldTypeArray, Description: "Exact Gateway and OPA observations."},
					{Name: "recent_error", Type: OutputFieldTypeString, Description: "Bounded recent runtime error."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "cluster", JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("cluster status", true,
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "doctor", "Inspect Docker and cluster state."),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the status contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "cluster status", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runClusterStatus,
	}
}

func clusterLogsSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster logs", Summary: "Read Gateway and OPA logs",
		Args:   "[--component gateway|opa|all] [--tail <lines>]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "cluster.logs",
			Outcome:      "Inspect bounded redacted shared logs including policy-denial evidence",
			Inputs: []CommandInput{
				{
					Name: "--component", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Select Gateway, OPA, or both.", AllowedValues: []string{"gateway", "opa", "all"},
					DefaultValue: stringPointer("all"),
				},
				tailInput(),
			},
			Output:        logOutput(),
			Prerequisites: []string{"The cluster has been created."},
			Errors: readCommandErrors("cluster logs", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_log_request", false, "help cluster logs", "Select a valid component and bound."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster up", "Start the cluster."),
				declaredCommandError(fault.KindInternal, "logs_failed", false, "cluster status", "Inspect shared components."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runClusterLogs,
	}
}

func clusterDownSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster down", Summary: "Remove the empty shared cluster",
		Args: "[--purge]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Remove shared containers and networks after every Tobari is detached",
			Inputs:       []CommandInput{purgeInput("Also remove exact shared Gateway CA volumes.")},
			Output:       textClusterStatusOutput(),
			Prerequisites: []string{
				"Every Tobari has been detached.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster down", "cluster status",
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "cluster_not_empty", false, "list", "Detach every listed Tobari."),
				declaredCommandError(fault.KindUnavailable, "cluster_stop_failed", false, "cluster status", "Reconcile shared resources."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ClusterTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes,
				},
			},
		},
		handler: runClusterDown,
	}
}

func attachSpec() CommandSpec {
	return CommandSpec{
		Path: "attach", Summary: "Attach one named Tobari to a root",
		Args: "--name <name> --root <path> [--image <image>]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Create one named isolated container with a dedicated network and persistent home",
			Inputs: []CommandInput{
				{
					Name: "--name", Source: InputSourceFlag, Required: true,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Unique portable display name matching [a-z][a-z0-9-]{0,62}.", AllowedValues: []string{},
				},
				{
					Name: "--root", Source: InputSourceFlag, Required: true,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Existing host directory mounted read-write at /workspace.", AllowedValues: []string{},
				},
				{
					Name: "--image", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Locally available compatible OCI image or builtin; omission uses XDG default_image, then builtin.", AllowedValues: []string{},
				},
			},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
				Fields: []OutputField{
					{Name: "name", Type: OutputFieldTypeString, Description: "Attached display name."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Canonical attached host root."},
					{Name: "image", Type: OutputFieldTypeString, Description: "Selected image selector."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The shared cluster is running.", "The root is shared with the Docker Engine VM when applicable."},
			FixedTarget:   fixedClusterTarget(),
			Errors: mutationCommandErrors("attach", "list",
				declaredCommandError(fault.KindInvalidInput, "invalid_name", false, "help attach", "Choose a portable unique name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Validate the intended root."),
				declaredCommandError(fault.KindInvalidInput, "invalid_image", false, "help attach", "Choose builtin or a portable OCI image reference."),
				declaredCommandError(fault.KindRejected, "invalid_image_config", false, "doctor", "Correct the XDG default image configuration."),
				declaredCommandError(fault.KindInvalidInput, "name_conflict", false, "list", "Choose another name or detach the existing Tobari."),
				declaredCommandError(fault.KindInvalidInput, "root_conflict", false, "list", "Use the existing Tobari or detach it."),
				declaredCommandError(fault.KindInvalidInput, "image_conflict", false, "list", "Use the existing image or detach the Tobari."),
				declaredCommandError(fault.KindUnavailable, "image_not_found", false, "help attach", "Build or pull the selected image explicitly."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "help attach", "Extend the documented Tobari runtime base."),
				declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster up", "Start the shared cluster."),
				declaredCommandError(fault.KindUnavailable, "attach_failed", false, "list", "Reconcile partial Docker state."),
				declaredCommandError(fault.KindContract, "invalid_attach_contract", false, "list", "Inspect confirmed local state."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ClusterTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runAttach,
	}
}

func listSpec() CommandSpec {
	return CommandSpec{
		Path: "list", Summary: "List named Tobari and action IDs",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Return every configured Tobari and an opaque ID for exact later actions",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
				Fields: []OutputField{
					{Name: "id", Type: OutputFieldTypeString, Description: "Opaque action reference.", ReferenceKind: tobari.ReferenceKind},
					{Name: "name", Type: OutputFieldTypeString, Description: "Human-readable Tobari name."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Canonical host root."},
					{Name: "image", Type: OutputFieldTypeString, Description: "Selected image selector."},
					{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether the exact container is healthy."},
					{Name: "container", Type: OutputFieldTypeString, Description: "Exact owned container name."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "tobari", JSONSchemaVersion: 2,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("list", true,
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "list_failed", false, "cluster status", "Inspect Docker state."),
				declaredCommandError(fault.KindContract, "invalid_list_contract", false, "doctor", "Repair list semantics."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "list", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runList,
	}
}

func shellSpec() CommandSpec {
	return CommandSpec{
		Path: "shell", Summary: "Open a shell in one Tobari",
		Args: "--id <id>", Effect: operation.EffectRead, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "tobari.execute",
			Outcome:       "Enter one exact referenced Tobari at the host-relative current directory",
			Inputs:        []CommandInput{idInput()},
			Output:        noOutput(),
			Prerequisites: []string{"The selected Tobari is running.", "An interactive terminal is attached."},
			Errors:        execErrors("shell"),
		},
		handler: runTobariShell,
	}
}

func execSpec() CommandSpec {
	return CommandSpec{
		Path: "exec", Summary: "Run exact argv in one Tobari",
		Args: "--id <id> [--cwd <path>] <command>", Effect: operation.EffectRead, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.execute",
			Outcome:      "Run an arbitrary command in one exact Tobari and preserve its exit status",
			Inputs: []CommandInput{
				idInput(),
				{
					Name: "--cwd", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Existing host directory inside the selected root.", AllowedValues: []string{},
				},
				{
					Name: "command", Source: InputSourceArgument, Required: true,
					ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
					Description: "Exact argv after the positional-only marker; values are not interpreted.", AllowedValues: []string{},
				},
			},
			Output: noOutput(), Prerequisites: []string{"The selected Tobari is running."},
			Errors: execErrors("exec"),
		},
		handler: runTobariExec,
	}
}

func logsSpec() CommandSpec {
	return CommandSpec{
		Path: "logs", Summary: "Read logs from one Tobari",
		Args: "--id <id> [--tail <lines>]", Effect: operation.EffectRead, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "tobari.logs",
			Outcome:       "Read a bounded log window from one exact referenced Tobari",
			Inputs:        []CommandInput{idInput(), tailInput()},
			Output:        logOutput(),
			Prerequisites: []string{"The selected Tobari container has been created."},
			Errors: readCommandErrors("logs", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_tobari_id", false, "list", "Use an ID from list unchanged."),
				declaredCommandError(fault.KindInvalidInput, "tobari_not_found", false, "list", "Select a configured Tobari."),
				declaredCommandError(fault.KindInvalidInput, "invalid_log_request", false, "help logs", "Select a valid line bound."),
				declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster up", "Start the cluster."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "logs_failed", false, "list", "Inspect the selected Tobari."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runTobariLogs,
	}
}

func detachSpec() CommandSpec {
	return CommandSpec{
		Path: "detach", Summary: "Detach one exact Tobari",
		Args: "--id <id> [--purge]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Remove one exact container and network while preserving its home unless purge is explicit",
			Inputs:       []CommandInput{idInput(), purgeInput("Also remove the selected Tobari home volume.")},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
				Fields:   []OutputField{{Name: "detached", Type: OutputFieldTypeBoolean, Description: "Whether the exact target is detached."}},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The ID was emitted by list."},
			Errors: mutationCommandErrors("detach", "list",
				declaredCommandError(fault.KindInvalidInput, "invalid_tobari_id", false, "list", "Use an ID from list unchanged."),
				declaredCommandError(fault.KindInvalidInput, "tobari_not_found", false, "list", "Select a configured Tobari."),
				declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster up", "Start the cluster."),
				declaredCommandError(fault.KindUnavailable, "detach_failed", false, "list", "Reconcile remaining Docker state."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.TargetKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes,
				},
			},
		},
		handler: runDetach,
	}
}

func fixedClusterTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID,
		Description: "This installation's one shared local enforcement cluster.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func idInput() CommandInput {
	return CommandInput{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Opaque Tobari ID emitted by list; pass unchanged.", AllowedValues: []string{},
		ReferenceKind: tobari.ReferenceKind,
	}
}

func formatInput() CommandInput {
	return CommandInput{
		Name: "--format", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Select human text or schema-versioned JSON.", AllowedValues: []string{"text", "json"},
		DefaultValue: stringPointer("text"),
	}
}

func tailInput() CommandInput {
	return CommandInput{
		Name: "--tail", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
		Description: "Maximum lines read from each selected component.", AllowedValues: []string{},
		DefaultValue: stringPointer("200"), Minimum: int64Pointer(1), Maximum: int64Pointer(10_000),
	}
}

func purgeInput(description string) CommandInput {
	return CommandInput{
		Name: "--purge", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
		Description: description, AllowedValues: []string{}, DefaultValue: stringPointer("false"),
	}
}

func noOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatNone}, DefaultFormat: OutputFormatNone,
		Fields: []OutputField{}, Delivery: OutputDeliveryComplete,
		CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func textClusterStatusOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
		Fields: []OutputField{
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether cluster state remains configured."},
			{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether shared components are running."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func logOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
		Fields:   []OutputField{{Name: "line", Type: OutputFieldTypeString, Description: "One visibly escaped component log line."}},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
	}
}

func execErrors(path string) []CommandError {
	return readCommandErrors(path, false,
		declaredCommandError(fault.KindInvalidInput, "invalid_tobari_id", false, "list", "Use an ID from list unchanged."),
		declaredCommandError(fault.KindInvalidInput, "tobari_not_found", false, "list", "Select a configured Tobari."),
		declaredCommandError(fault.KindInvalidInput, "invalid_exec_request", false, "help "+path, "Pass a valid command."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster up", "Start the cluster."),
		declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
		declaredCommandError(fault.KindInternal, "exec_failed", false, "list", "Inspect the selected Tobari."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	)
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

func mutationCommandErrors(path, recovery string, extra ...CommandError) []CommandError {
	errors := []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help "+path, "Correct the command arguments."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
	}
	errors = append(errors, extra...)
	errors = append(errors,
		declaredCommandError(fault.KindContract, "invalid_mutation_contract", false, recovery, "Repair the mutation declaration and reconcile state."),
		declaredCommandError(fault.KindContract, "missing_mutation_action", false, recovery, "Configure the mutation action and reconcile state."),
		declaredCommandError(fault.KindRejected, "missing_mutation_policy", false, recovery, "Configure the project mutation policy."),
		declaredCommandError(fault.KindRejected, "mutation_rejected", false, recovery, "Review exact Tobari ownership."),
		declaredCommandError(fault.KindContract, "unclassified_mutation_outcome", false, recovery, "Reconcile state before another mutation."),
		declaredCommandError(fault.KindInternal, "mutation_output_write_failed", false, recovery, "Reconcile the confirmed mutation without repeating it."),
	)
	return errors
}
