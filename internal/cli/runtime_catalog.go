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
		clusterDenialsSpec(),
		clusterLogsSpec(),
		clusterDownSpec(),
		policyCandidatesSpec(),
		policyReviewSpec(),
		policyTailSpec(),
		policyRulesSpec(),
		policyAllowSpec(),
		policyDenySpec(),
		policyResetSpec(),
		policyCompactionsSpec(),
		policyCompactSpec(),
		contextListSpec(),
		contextShowSpec(),
		contextCreateSpec(),
		contextUseSpec(),
		runtimeInitSpec(),
		runtimeBuildSpec(),
		projectEnterSpec(),
		statusSpec(),
		listSpec(),
		deleteSpec(),
	}
}

func contextListSpec() CommandSpec {
	return CommandSpec{
		Path: "context list", Summary: "List named execution Contexts",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "List the complete local Context collection and identify the active Context",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "active", Type: OutputFieldTypeString, Description: "Name of the host-selected active Context."},
					{Name: "items", Type: OutputFieldTypeArray, Description: "Complete local Context collection with active state, image, agent profile, policy mode, and runtime status."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "contexts", JSONSchemaVersion: 2,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("context list", true,
				declaredCommandError(fault.KindInternal, "context_read_failed", false, "doctor", "Inspect the host Context stores."),
				declaredCommandError(fault.KindContract, "invalid_context_list", false, "doctor", "Repair the Context manifest collection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runContextList,
	}
}

func contextShowSpec() CommandSpec {
	return CommandSpec{
		Path: "context show", Summary: "Inspect one execution Context",
		Args: "[--name <name>] [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "Inspect the active Context or one named Context and its separated store references",
			Inputs: []CommandInput{
				{Name: "--name", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Named Context to inspect; omission selects the active Context.", AllowedValues: []string{}},
				formatInput(),
			},
			Output:        contextReportOutput(),
			Prerequisites: []string{},
			Errors: readCommandErrors("context show", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindInternal, "context_read_failed", false, "doctor", "Inspect the host Context stores."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context list", "Repair the Context manifest."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runContextShow,
	}
}

func contextCreateSpec() CommandSpec {
	return CommandSpec{
		Path: "context create", Summary: "Create a named execution Context",
		Args:   "--name <name> [--image <image>] [--mode guided|advanced] [--format text|json]",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "context.composition",
			Outcome:       "Create one named Context with separate owner-only policy and managed-credential stores",
			Inputs:        []CommandInput{contextNameInput(), contextImageInput(), contextModeInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The host Context directory is accessible."},
			FixedTarget:   fixedContextCatalogTarget(),
			Errors: mutationCommandErrors("context create", "context list",
				declaredCommandError(fault.KindInvalidInput, "invalid_context", false, "help context create", "Correct the Context name, image, or policy mode."),
				declaredCommandError(fault.KindRejected, "context_exists", false, "context show", "Inspect the existing Context or choose another name."),
				declaredCommandError(fault.KindRejected, "context_create_failed", false, "context list", "Inspect the partially initialized Context stores."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context list", "Reconcile the confirmed Context creation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextCatalogTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runContextCreate,
	}
}

func contextUseSpec() CommandSpec {
	return CommandSpec{
		Path: "context use", Summary: "Select and apply the active execution Context",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "context.composition",
			Outcome:       "Select one existing Context and synchronously apply it to a running shared cluster; leave stopped or unconfigured clusters for explicit cluster up",
			Inputs:        []CommandInput{contextNameInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The named Context already exists."},
			FixedTarget:   fixedActiveContextTarget(),
			Errors: mutationCommandErrors("context use", "context show",
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context or create it first."),
				declaredCommandError(fault.KindRejected, "context_use_failed", false, "cluster up", "Reconcile the shared cluster with the selected Context."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed Context selection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runContextUse,
	}
}

func runtimeInitSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime init", Summary: "Create a runtime recipe for the active Context",
		Args: "[--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "runtime.customization",
			Outcome:       "Create the active Context's owner-only Dockerfile template without changing its selected runtime image",
			Inputs:        []CommandInput{formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"An active Context is available on the trusted host."},
			FixedTarget:   fixedActiveContextRuntimeTarget(),
			Errors: mutationCommandErrors("runtime init", "context show",
				declaredCommandError(fault.KindRejected, "runtime_recipe_exists", false, "context show", "Inspect the existing active Context runtime recipe."),
				declaredCommandError(fault.KindRejected, "runtime_init_failed", false, "context show", "Inspect the active Context stores."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the Context runtime report."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextRuntimeTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runRuntimeInit,
	}
}

func runtimeBuildSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime build", Summary: "Build and select the active Context runtime",
		Args: "[--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "runtime.customization",
			Outcome:       "Build the active Context Dockerfile with observable Docker diagnostics, validate the Tobari runtime contract, and select the generated local image",
			Inputs:        []CommandInput{formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The active Context has a runtime/Dockerfile recipe.", "The trusted host Docker daemon and Buildx plugin are available."},
			FixedTarget:   fixedActiveContextRuntimeTarget(),
			Errors: mutationCommandErrors("runtime build", "context show",
				declaredCommandError(fault.KindInvalidInput, "runtime_recipe_missing", false, "runtime init", "Create the active Context runtime template first."),
				declaredCommandError(fault.KindRejected, "runtime_build_failed", false, "context show", "Inspect the unchanged selected runtime and recipe state."),
				declaredCommandError(fault.KindUnavailable, "image_not_found", false, "runtime build", "Build or make the official base image available to Docker."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "context show", "Correct the Dockerfile so the selected image preserves the Tobari runtime contract."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed runtime promotion."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextRuntimeTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runRuntimeBuild,
	}
}

func projectEnterSpec() CommandSpec {
	return CommandSpec{
		Path: "tobari", Summary: "Choose or create the current directory's Workspace and enter a reusable session",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Choose an ancestor Workspace or explicitly create one at the current directory, recover its runtime, and enter an interactive session; exit leaves the Workspace existing for reuse and delete removes it explicitly",
			Inputs:       []CommandInput{}, Output: noOutput(),
			Prerequisites: []string{
				"The current directory is an accessible project directory.",
				"The caller is attached to an interactive terminal.",
			},
			FixedTarget: fixedCurrentDirectoryTarget(),
			Errors:      projectEnterErrors(),
			Mutation: &MutationContract{
				TargetKind: tobari.CurrentDirectoryTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runProjectEnter,
	}
}

func statusSpec() CommandSpec {
	return CommandSpec{
		Path: "status", Summary: "Inspect the nearest current-directory Tobari",
		Args:   "[--format text|json]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Report whether a Tobari exists for the current directory and its recoverable runtime diagnostic",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "exists", Type: OutputFieldTypeBoolean, Description: "Whether logical Tobari state exists for the current directory."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Nearest canonical project root when one exists."},
					{Name: "id", Type: OutputFieldTypeString, Description: "Diagnostic stable logical ID when one exists."},
					{Name: "home", Type: OutputFieldTypeString, Description: "Diagnostic per-Tobari XDG home path when one exists."},
					{Name: "runtime", Type: OutputFieldTypeString, Description: "Recoverable runtime diagnostic; incomplete means the logical state record is missing and must be deleted before recreation."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "status", JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("status", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local CWD-owned state."),
				declaredCommandError(fault.KindInternal, "runtime_status_failed", false, "status", "Inspect the selected project's runtime."),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "help status", "Repair the CWD status contract."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runProjectStatus,
	}
}

func deleteSpec() CommandSpec {
	return CommandSpec{
		Path: "delete", Summary: "Delete the nearest current-directory Tobari when no session is attached",
		Args: "[--force]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Delete one nearest CWD-owned Tobari when detached; reject attached sessions unless --force overrides the guard",
			Inputs: []CommandInput{{
				Name: "--force", Source: InputSourceFlag, Required: false,
				ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
				Description: "Override the attached-session safety guard and delete anyway.", AllowedValues: []string{}, DefaultValue: stringPointer("false"),
			}},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Whether the selected logical Tobari was deleted."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Deleted canonical project root."},
					{Name: "id", Type: OutputFieldTypeString, Description: "Deleted stable logical ID."},
					{Name: "home", Type: OutputFieldTypeString, Description: "Deleted per-Tobari XDG home path."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The target is the nearest CWD-owned Tobari; without --force, no session is attached."},
			FixedTarget:   fixedCurrentDirectoryTarget(),
			Errors: mutationCommandErrors("delete", "status",
				declaredCommandError(fault.KindRejected, "project_session_attached", false, "delete", "Exit the attached session, then retry; use --force only when terminating it is intentional."),
				declaredCommandError(fault.KindNotFound, "project_not_found", false, "tobari", "Create a Tobari from the current project directory."),
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local CWD-owned state."),
				declaredCommandError(fault.KindInternal, "session_status_failed", false, "status", "Inspect the Workspace runtime before retrying deletion."),
				declaredCommandError(fault.KindUnavailable, "runtime_reconcile_failed", false, "status", "Retry deletion after inspecting remaining runtime state."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.CurrentDirectoryTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes,
				},
			},
		},
		handler: runProjectDelete,
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
				"The routine path uses the immutable Gateway image and official runtime base image.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster up", "cluster status",
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before startup."),
				declaredCommandError(fault.KindRejected, "legacy_named_state", false, "doctor", "Remove legacy named state with the older binary before continuing."),
				declaredCommandError(fault.KindRejected, "legacy_state", false, "doctor", "Remove schema-1 state with the older binary."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "cluster status", "Reconcile the confirmed startup."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway and OPA cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "cluster status", "Repair the runtime status contract."),
				declaredCommandError(fault.KindUnavailable, "cluster_start_failed", false, "cluster status", "Reconcile partial Docker state."),
				declaredCommandError(fault.KindUnavailable, "gateway_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the verified Gateway image."),
				declaredCommandError(fault.KindContract, "gateway_image_incompatible", false, "doctor", "Inspect the Gateway image API, digest, and architecture contract."),
				declaredCommandError(fault.KindUnavailable, "runtime_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the official runtime base image."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "context show", "Inspect the active Context runtime image contract."),
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
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
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
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway and OPA cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the status contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "cluster status", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runClusterStatus,
	}
}

func clusterDenialsSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster denials", Summary: "Read policy-denial evidence",
		Args:   "[--tail <lines>] [--format text|json]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Identify recent denied HTTP effects and the pending permission review command",
			Inputs: []CommandInput{
				denialTailInput(),
				formatInput(),
			},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical trusted-host XDG policy directory."},
					{Name: "window_lines", Type: OutputFieldTypeInteger, Description: "Maximum recent Gateway lines inspected."},
					{
						Name: "items", Type: OutputFieldTypeArray,
						Description: "Validated denials ordered oldest to newest with host-issued project principal, scheme-independent request authority (host and port), method, path, reason, status, and exact-rule learnability.",
					},
					{Name: "review_command", Type: OutputFieldTypeString, Description: "Exact command that opens the pending permission review queue."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "denials", JSONSchemaVersion: 2,
			},
			Prerequisites: []string{"The cluster has been created."},
			Errors: append(readCommandErrors("cluster denials", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_denial_request", false, "help cluster denials", "Select a valid bounded window."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster logs", "Inspect raw Gateway logs."),
				declaredCommandError(fault.KindContract, "invalid_denial_contract", false, "cluster logs", "Inspect raw Gateway logs and audit compatibility."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "cluster denials", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			), policyClusterReadinessErrors()...),
		},
		handler: runClusterDenials,
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

func policyCandidatesSpec() CommandSpec {
	return CommandSpec{
		Path: "policy candidates", Summary: "Discover exact rules from denials",
		Args:   "[--tail <lines>] [--format text|json]",
		Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Return unique pending exact host, port, method, and path proposals with opaque approval IDs",
			Inputs:       []CommandInput{denialTailInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "policy_candidates", JSONSchemaVersion: 3,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors:        policyCandidateReadErrors("policy candidates", true),
		},
		handler: runPolicyCandidates,
	}
}

func policyTailSpec() CommandSpec {
	return CommandSpec{
		Path: "policy tail", Summary: "Review pending exact policy rules",
		Args:   "[--tail <lines>]",
		Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Review the bounded pending policy queue with exact approval commands",
			Inputs:       []CommandInput{denialTailInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors:        policyCandidateReadErrors("policy tail", true),
		},
		handler: runPolicyTail,
	}
}

func policyReviewSpec() CommandSpec {
	return CommandSpec{
		Path: "policy review", Summary: "Review pending network permissions",
		Args: "[--tail <lines>] [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Review the bounded pending exact network-permission queue; an interactive terminal can explicitly allow or deny one exact permission",
			Inputs:       []CommandInput{reviewTailInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "policy_review", JSONSchemaVersion: 3,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors:        policyCandidateReadErrors("policy review", true),
			Interactive: &InteractiveWorkflowContract{
				ActionCommand:          "policy allow",
				ActionCommands:         []string{"policy allow", "policy deny"},
				SelectionReferenceKind: tobari.PolicyCandidateKind,
				SelectionOutputField:   "id",
				Confirmation:           "explicit_yes",
				NonInteractiveBehavior: "read_only",
			},
		},
		handler: runPolicyReview,
	}
}

func policyRulesSpec() CommandSpec {
	return CommandSpec{
		Path: "policy rules", Summary: "Inspect current learned policy decisions",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Inspect the complete current project-bound learned Allow and exact Deny decisions; on a TTY explicitly reset one decision",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyRuleOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "policy_rules", JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"The active Context has a validated policy store."},
			Errors:        policyRuleReadErrors("policy rules", true),
			Interactive: &InteractiveWorkflowContract{
				ActionCommand:          "policy reset",
				SelectionReferenceKind: tobari.PolicyRuleKind,
				SelectionOutputField:   "id",
				Confirmation:           "explicit_yes",
				NonInteractiveBehavior: "read_only",
			},
		},
		handler: runPolicyRules,
	}
}

func policyAllowSpec() CommandSpec {
	return CommandSpec{
		Path: "policy allow", Summary: "Allow one exact denied effect",
		Args: "--id <id>", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Test, record, and activate one exact retained host, port, method, and path permission",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates, policy review, or policy tail")},
			Output:       policyLearningChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates, policy review, or policy tail and remains in retained Gateway logs.",
			},
			Errors: policyMutationCommandErrors("policy allow", "policy candidates",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_candidate_id", false, "policy candidates", "Use a candidate ID unchanged."),
				declaredCommandError(fault.KindInvalidInput, "policy_candidate_not_found", false, "policy candidates", "Rediscover the current pending queue."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "policy candidates", "Rediscover after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete candidate policy."),
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before activation."),
				declaredCommandError(fault.KindInternal, "policy_write_failed", false, "policy candidates", "Inspect the unchanged or atomically updated policy data."),
				declaredCommandError(fault.KindUnavailable, "policy_learning_failed", false, "cluster status", "Reconcile OPA and current policy state."),
				declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Inspect retained denial compatibility."),
				declaredCommandError(fault.KindContract, "invalid_learned_policy", false, "doctor", "Repair the learned-rule contract."),
				declaredCommandError(fault.KindContract, "invalid_policy_learning_result", false, "cluster status", "Reconcile the confirmed policy mutation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.PolicyCandidateKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runPolicyAllow,
	}
}

func policyDenySpec() CommandSpec {
	return CommandSpec{
		Path: "policy deny", Summary: "Deny one exact denied effect",
		Args: "--id <id>", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Test, record, and activate one exact project-bound denial for a retained host, port, method, and path",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates, policy review, or policy tail")},
			Output:       policyDenyChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates, policy review, or policy tail and remains an actionable pending candidate.",
			},
			Errors: policyMutationCommandErrors("policy deny", "policy review",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_candidate_id", false, "policy review", "Use a candidate ID unchanged."),
				declaredCommandError(fault.KindInvalidInput, "policy_candidate_not_found", false, "policy review", "Rediscover the current pending queue."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "policy review", "Rediscover after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete candidate policy."),
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before activation."),
				declaredCommandError(fault.KindInternal, "policy_write_failed", false, "policy review", "Inspect the unchanged or atomically updated policy data."),
				declaredCommandError(fault.KindUnavailable, "policy_learning_failed", false, "cluster status", "Reconcile OPA and current policy state."),
				declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Inspect retained denial compatibility."),
				declaredCommandError(fault.KindContract, "invalid_policy_deny", false, "doctor", "Repair the exact-deny contract."),
				declaredCommandError(fault.KindContract, "invalid_policy_deny_result", false, "cluster status", "Reconcile the confirmed policy mutation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.PolicyCandidateKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runPolicyDeny,
	}
}

func policyResetSpec() CommandSpec {
	return CommandSpec{
		Path: "policy reset", Summary: "Return one learned decision to default deny",
		Args: "--id <id>", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Remove one current learned Allow or exact Deny and leave the matching effect at default deny",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyRuleKind, "policy rules")},
			Output:       policyRuleResetOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy rules and remains a current learned decision.",
			},
			Errors: policyMutationCommandErrors("policy reset", "policy rules",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_rule_id", false, "policy rules", "Use a rule ID unchanged."),
				declaredCommandError(fault.KindInvalidInput, "policy_rule_not_found", false, "policy rules", "Rediscover the current learned decisions."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "policy rules", "Rediscover after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete candidate policy."),
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before activation."),
				declaredCommandError(fault.KindInternal, "policy_write_failed", false, "policy rules", "Inspect the unchanged or atomically updated policy data."),
				declaredCommandError(fault.KindUnavailable, "policy_learning_failed", false, "cluster status", "Reconcile OPA and current policy state."),
				declaredCommandError(fault.KindContract, "invalid_policy_rule_reset_result", false, "cluster status", "Reconcile the confirmed policy mutation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.PolicyRuleKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runPolicyReset,
	}
}

func policyCompactionsSpec() CommandSpec {
	return CommandSpec{
		Path: "policy compactions", Summary: "Discover test-backed rule compactions",
		Args:   "[--format text|json]",
		Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Return current same-host, port, and method exact-rule groups eligible for bounded prefix compaction",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "id", Type: OutputFieldTypeString, Description: "Opaque current compaction reference.", ReferenceKind: tobari.PolicyCompactionKind},
					{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to every source rule."},
					{Name: "host", Type: OutputFieldTypeString, Description: "Exact request host."},
					{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact request port shared by every source rule."},
					{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
					{Name: "path_prefix", Type: OutputFieldTypeString, Description: "Proposed directory-bound path prefix."},
					{Name: "source_rule_count", Type: OutputFieldTypeInteger, Description: "Number of exact source rules replaced."},
					{Name: "examples", Type: OutputFieldTypeArray, Description: "Positive paths retained as policy tests."},
					{Name: "outside_canary", Type: OutputFieldTypeString, Description: "Adjacent path that must remain outside the proposal."},
					{Name: "compact_command", Type: OutputFieldTypeString, Description: "Exact reference-bound compaction command."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "policy_compactions", JSONSchemaVersion: 2,
			},
			Prerequisites: []string{"At least three exact learned rules share one sufficiently deep directory."},
			Errors: append(readCommandErrors("policy compactions", true,
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindContract, "invalid_compaction_contract", false, "doctor", "Repair the learned-rule contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "policy compactions", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			), policyClusterReadinessErrors()...),
		},
		handler: runPolicyCompactions,
	}
}

func policyCompactSpec() CommandSpec {
	return CommandSpec{
		Path: "policy compact", Summary: "Apply one test-backed compaction",
		Args: "--id <id>", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Replace one current bound exact host, port, and method rule set with its tested directory prefix",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCompactionKind, "policy compactions")},
			Output:       policyLearningChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy compactions and its exact source rules remain unchanged.",
			},
			Errors: policyMutationCommandErrors("policy compact", "policy compactions",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_compaction_id", false, "policy compactions", "Use a compaction ID unchanged."),
				declaredCommandError(fault.KindInvalidInput, "policy_compaction_not_found", false, "policy compactions", "Rediscover current compactions."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "policy compactions", "Rediscover after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete compacted policy."),
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before activation."),
				declaredCommandError(fault.KindInternal, "policy_write_failed", false, "policy compactions", "Inspect the unchanged or atomically updated policy data."),
				declaredCommandError(fault.KindUnavailable, "policy_learning_failed", false, "cluster status", "Reconcile OPA and current policy state."),
				declaredCommandError(fault.KindContract, "invalid_compaction_contract", false, "doctor", "Repair the learned-rule contract."),
				declaredCommandError(fault.KindContract, "invalid_policy_learning_result", false, "cluster status", "Reconcile the confirmed policy mutation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.PolicyCompactionKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{
					Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runPolicyCompact,
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
				declaredCommandError(fault.KindRejected, "legacy_named_state", false, "doctor", "Remove legacy named state with the older binary before continuing."),
				declaredCommandError(fault.KindRejected, "cluster_not_empty", false, "list", "Detach every listed Tobari."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway and OPA cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
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
					Description: "Locally available compatible OCI image or builtin; omission uses the active Context image.", AllowedValues: []string{},
				},
			},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
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
				declaredCommandError(fault.KindRejected, "invalid_image_config", false, "context show", "Inspect the active Context image configuration."),
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
		Path: "list", Summary: "List local Workspaces",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Return every configured Workspace root with diagnostic runtime state",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "root", Type: OutputFieldTypeString, Description: "Canonical Workspace root."},
					{Name: "runtime", Type: OutputFieldTypeString, Description: "Recoverable runtime diagnostic; incomplete means the logical state record is missing and must be deleted before recreation."},
					{Name: "id", Type: OutputFieldTypeString, Description: "Diagnostic stable Workspace ID; not a routine action input."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "tobari", JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("list", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Validate the current directory."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "runtime_status_failed", false, "status", "Inspect the selected project's runtime."),
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
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
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

func fixedCurrentDirectoryTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.CurrentDirectoryTargetKind, ID: tobari.CurrentDirectoryTargetID,
		Description: "The CWD-owned Workspace associated with this process's canonical current directory.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextCatalogTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ContextCatalogTargetKind, ID: tobari.ContextCatalogTargetID,
		Description: "This installation's host-owned collection of named Contexts.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedActiveContextTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID,
		Description: "This installation's host-owned active Context selection.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedActiveContextRuntimeTarget() *FixedTarget {
	return &FixedTarget{
		Kind:        tobari.ContextRuntimeTargetKind,
		ID:          tobari.ActiveContextRuntimeID,
		Description: "The active Context's host-owned runtime recipe and selected image.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func contextNameInput() CommandInput {
	return CommandInput{
		Name: "--name", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Portable Context name; it is a selection label, not a credential authority.",
		AllowedValues: []string{},
	}
}

func contextImageInput() CommandInput {
	return CommandInput{
		Name: "--image", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Built-in compatible Tobari image selector stored in the Context.",
		AllowedValues: []string{}, DefaultValue: stringPointer(tobari.BuiltinImageSelector),
	}
}

func contextModeInput() CommandInput {
	return CommandInput{
		Name: "--mode", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Policy development mode: guided exact permission review or advanced trusted-host Rego.",
		AllowedValues: []string{"guided", "advanced"}, DefaultValue: stringPointer("guided"),
	}
}

func contextReportOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "name", Type: OutputFieldTypeString, Description: "Named Context identifier."},
			{Name: "active", Type: OutputFieldTypeBoolean, Description: "Whether this Context is the host-selected active Context."},
			{Name: "agent_profile", Type: OutputFieldTypeString, Description: "Read-only shared agent profile reference."},
			{Name: "image", Type: OutputFieldTypeString, Description: "Default compatible Tobari image selector stored in the Context."},
			{Name: "policy_mode", Type: OutputFieldTypeString, Description: "Guided or advanced policy-development mode."},
			{Name: "stores", Type: OutputFieldTypeObject, Description: "Resolved policy, managed-credential, and runtime recipe paths; secret values are never included."},
			{Name: "runtime", Type: OutputFieldTypeObject, Description: "Selected runtime source, recipe status, source digest, and image digest."},
			{Name: "cluster", Type: OutputFieldTypeString, Description: "For context use, whether the running shared cluster was reconciled, was already aligned, or still needs explicit cluster up; otherwise not_applicable."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "context", JSONSchemaVersion: 2,
	}
}

func projectEnterErrors() []CommandError {
	errors := mutationCommandErrors("tobari", "status")
	filtered := errors[:0]
	for _, declared := range errors {
		if declared.Code != "mutation_output_write_failed" {
			filtered = append(filtered, declared)
		}
	}
	return append(filtered,
		declaredCommandError(fault.KindInvalidInput, "tty_required", false, "help tobari", "Run the root command from an interactive terminal."),
		declaredCommandError(fault.KindRejected, "already_inside", false, "help tobari", "Exit the current Tobari before entering another session."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_configured", false, "cluster up", "Create the shared cluster explicitly before entering a Tobari."),
		declaredCommandError(fault.KindUnavailable, "cluster_status_failed", false, "cluster status", "Inspect the shared cluster before entering a Tobari."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_ready", false, "cluster up", "Reconcile the shared cluster explicitly before entering a Tobari."),
		declaredCommandError(fault.KindRejected, "project_state_incomplete", false, "delete", "Review the exact delete command and confirm removal of the incomplete current-directory Tobari."),
		declaredCommandError(fault.KindInternal, "missing_workspace_selector", false, "doctor", "Configure the Tobari terminal selector."),
		declaredCommandError(fault.KindContract, "invalid_workspace_selection", false, "doctor", "Inspect local Workspace state."),
		declaredCommandError(fault.KindContract, "workspace_selection_invalid", false, "tobari", "Choose a current Workspace or explicitly create one again."),
		declaredCommandError(fault.KindRejected, "workspace_selection_stale", true, "tobari", "Refresh the Workspace choices and select again."),
		declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
		declaredCommandError(fault.KindUnavailable, "image_not_found", false, "runtime build", "Build or make the selected compatible runtime image available to Docker."),
		declaredCommandError(fault.KindUnavailable, "runtime_reconcile_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindInternal, "enter_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	)
}

func idInput() CommandInput {
	return CommandInput{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Opaque Tobari ID emitted by list; pass unchanged.", AllowedValues: []string{},
		ReferenceKind: tobari.ReferenceKind,
	}
}

func policyReferenceInput(kind, producer string) CommandInput {
	return CommandInput{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Opaque " + kind + " ID emitted by " + producer + "; pass unchanged.",
		AllowedValues: []string{}, ReferenceKind: kind,
	}
}

func policyRuleOutputFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque current learned policy-rule reference.", ReferenceKind: tobari.PolicyRuleKind},
		{Name: "decision", Type: OutputFieldTypeString, Description: "Current learned decision: allow or deny."},
		{Name: "match", Type: OutputFieldTypeString, Description: "Exact or prefix match mode."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to the decision."},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact decision host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact decision port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact path or safe directory prefix."},
		{Name: "examples", Type: OutputFieldTypeArray, Description: "Positive request paths retained by an Allow rule; empty for Deny."},
		{Name: "source_candidates", Type: OutputFieldTypeArray, Description: "Opaque denial candidates that support this decision."},
		{Name: "reset_command", Type: OutputFieldTypeString, Description: "Exact command that returns this decision to default deny."},
	}
}

func policyRuleResetOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical trusted-host XDG policy directory."},
			{Name: "target_id", Type: OutputFieldTypeString, Description: "Opaque policy-rule ID consumed unchanged."},
			{Name: "decision", Type: OutputFieldTypeString, Description: "Removed learned decision: allow or deny."},
			{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Whether the tested reset is active."},
			{Name: "next", Type: OutputFieldTypeString, Description: "Exact command to review the now-default-deny effect again."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func policyCandidateOutputFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque exact policy-candidate reference.", ReferenceKind: tobari.PolicyCandidateKind},
		{Name: "observed_at", Type: OutputFieldTypeString, Description: "Latest matching Gateway denial timestamp."},
		{Name: "observation_count", Type: OutputFieldTypeInteger, Description: "Matching retained denial observations; legacy candidates without this field mean one."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal for the denied request."},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact denied request host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact denied request port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact denied uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact denied HTTP path without query data."},
		{Name: "reason", Type: OutputFieldTypeString, Description: "Bounded secret-free denial reason."},
		{Name: "status_code", Type: OutputFieldTypeInteger, Description: "Gateway denial status."},
		{Name: "credential_profile", Type: OutputFieldTypeString, Description: "Requested bound credential profile or null."},
		{Name: "allow_command", Type: OutputFieldTypeString, Description: "Exact reference-bound approval command."},
		{Name: "deny_command", Type: OutputFieldTypeString, Description: "Exact reference-bound rejection command."},
	}
}

func policyDenyChangeOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical trusted-host XDG policy directory."},
			{Name: "target_id", Type: OutputFieldTypeString, Description: "Opaque candidate ID consumed unchanged."},
			{Name: "rule_id", Type: OutputFieldTypeString, Description: "Deterministic stored exact-deny identity."},
			{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to the denial."},
			{Name: "host", Type: OutputFieldTypeString, Description: "Stored exact host."},
			{Name: "port", Type: OutputFieldTypeInteger, Description: "Stored exact request port."},
			{Name: "method", Type: OutputFieldTypeString, Description: "Stored exact uppercase HTTP method."},
			{Name: "path", Type: OutputFieldTypeString, Description: "Stored exact path."},
			{Name: "source_rule_count", Type: OutputFieldTypeInteger, Description: "Number of source candidates represented by the result."},
			{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Whether the tested exact denial is active."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func policyLearningChangeOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical trusted-host XDG policy directory."},
			{Name: "target_id", Type: OutputFieldTypeString, Description: "Opaque target ID consumed unchanged."},
			{Name: "rule_id", Type: OutputFieldTypeString, Description: "Deterministic stored learned-rule identity."},
			{Name: "match", Type: OutputFieldTypeString, Description: "Stored exact or prefix match mode."},
			{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to the stored rule."},
			{Name: "host", Type: OutputFieldTypeString, Description: "Stored exact host."},
			{Name: "port", Type: OutputFieldTypeInteger, Description: "Stored exact request port."},
			{Name: "method", Type: OutputFieldTypeString, Description: "Stored exact uppercase HTTP method."},
			{Name: "path", Type: OutputFieldTypeString, Description: "Stored exact path or directory prefix."},
			{Name: "source_rule_count", Type: OutputFieldTypeInteger, Description: "Number of source rules represented by the result."},
			{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Whether the tested rule is active."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
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

func doctorFormatInput() CommandInput {
	return CommandInput{
		Name: "--format", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Select human text, tab-separated data, or schema-versioned JSON.",
		AllowedValues: []string{"text", "tsv", "json"}, DefaultValue: stringPointer("text"),
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

func denialTailInput() CommandInput {
	return CommandInput{
		Name: "--tail", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
		Description: "Maximum recent Gateway log lines inspected for denials.", AllowedValues: []string{},
		DefaultValue: stringPointer("200"), Minimum: int64Pointer(1), Maximum: int64Pointer(10_000),
	}
}

func reviewTailInput() CommandInput {
	return CommandInput{
		Name: "--tail", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
		Description: "Maximum retained Gateway log lines inspected for pending permissions.", AllowedValues: []string{},
		DefaultValue: stringPointer("10000"), Minimum: int64Pointer(1), Maximum: int64Pointer(10_000),
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
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether cluster state remains configured."},
			{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether shared components are running."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
	}
}

func logOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
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

func policyClusterReadinessErrors() []CommandError {
	return []CommandError{
		declaredCommandError(fault.KindUnavailable, "cluster_not_configured", false, "cluster up", "Create the shared cluster explicitly."),
		declaredCommandError(fault.KindUnavailable, "cluster_status_failed", false, "cluster status", "Inspect the shared cluster before using policy data."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_ready", false, "cluster up", "Reconcile the shared cluster explicitly."),
		declaredCommandError(fault.KindInternal, "context_read_failed", false, "context show", "Inspect the selected Context before using policy data."),
		declaredCommandError(fault.KindRejected, "context_mismatch", false, "cluster up", "Reconcile the shared cluster with the selected Context."),
	}
}

func policyCandidateReadErrors(path string, hasOutput bool) []CommandError {
	return append(readCommandErrors(path, hasOutput,
		declaredCommandError(fault.KindInvalidInput, "invalid_candidate_request", false, "help "+path, "Select a valid bounded denial window."),
		declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
		declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
		declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
		declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Inspect retained denial compatibility."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, path, "Repair JSON projection."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	), policyClusterReadinessErrors()...)
}

func policyRuleReadErrors(path string, hasOutput bool) []CommandError {
	return append(readCommandErrors(path, hasOutput,
		declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
		declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
		declaredCommandError(fault.KindContract, "invalid_policy_rule_report", false, "doctor", "Inspect the current policy-rule contract."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, path, "Repair JSON projection."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	), policyClusterReadinessErrors()...)
}

func policyMutationCommandErrors(path, recovery string, extra ...CommandError) []CommandError {
	return append(mutationCommandErrors(path, recovery, extra...), policyClusterReadinessErrors()...)
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
