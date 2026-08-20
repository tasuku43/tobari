package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runtimeCommandSpecs() []CommandSpec {
	specs := []CommandSpec{
		clusterUpSpec(),
		clusterStatusSpec(),
	}
	specs = append(specs, experimentalRuntimeCommandSpecs()...)
	specs = append(specs,
		clusterDenialsSpec(),
		clusterLogsSpec(),
		clusterDownSpec(),
		policyCandidatesSpec(),
		policyReviewSpec(),
		policyApplyReviewedSpec(),
		policyRulesSpec(),
		policyAllowSpec(),
		policyDenySpec(),
		policyResetSpec(),
		contextListSpec(),
		contextShowSpec(),
		configShellSpec(),
		configGitSpec(),
		configBootstrapAWSSpec(),
		configBootstrapEKSSpec(),
		contextCreateSpec(),
		contextDeleteSpec(),
		contextUseSpec(),
		contextRuntimeSetSpec(),
		runtimeListSpec(),
		runtimeShowSpec(),
		runtimeCreateSpec(),
		runtimeHistorySpec(),
		runtimeBuildSpec(),
		projectEnterSpec(),
		statusSpec(),
		listSpec(),
		deleteSpec(),
	)
	return append(specs, authCommandSpecs()...)
}

func configBootstrapAWSSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{
		Path: "config bootstrap aws", Summary: "Configure, refresh, or remove the AWS snapshot applied once to future Workspace homes",
		Args:   "[--profile <name>] [--refresh] [--remove] [--context <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "context.workspace-bootstrap",
			Outcome:      "Normalize one host AWS IAM Identity Center profile into a secret-free Context snapshot for future Workspaces, refresh its semantic revision, or remove the future recipe without rewriting existing Workspace homes",
			Inputs: []CommandInput{
				{Name: "--profile", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact host AWS shared-config profile to normalize now; conflicts with refresh and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
				{Name: "--refresh", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Re-read the profile named by the current Context snapshot; conflicts with profile and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--remove"}},
				{Name: "--remove", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove the recipe for future Workspaces; existing Workspace homes retain their create-time bytes.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--refresh"}},
				executionContextInput(), formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Context exists and the fixed host ~/.aws/config path is a bounded regular non-symlink file not writable by group or other users.",
				"The selected profile uses one sso_session section and only the reviewed secret-free IAM Identity Center fields; credentials and ~/.aws/sso/cache are never read.",
				"When action flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text.",
			},
			FixedTarget: fixedContextBootstrapTarget(),
			Errors: mutationCommandErrors("config bootstrap aws", "context show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config bootstrap aws", "Supply one action flag or run the wizard on interactive text streams."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config bootstrap aws", "Retry with one direct action flag or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_aws_bootstrap_change", false, "help config bootstrap aws", "Choose exactly one configure, refresh, or remove action."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindNotFound, "bootstrap_not_configured", false, "help config bootstrap aws", "Configure a profile before refreshing."),
				declaredCommandError(fault.KindRejected, "config_bootstrap_failed", false, "context show", "Inspect the current recipe and strict host AWS profile."),
				declaredCommandError(fault.KindRejected, "bootstrap_source_changed", true, "config bootstrap aws", "Review a fresh semantic diff before applying."),
				declaredCommandError(fault.KindRejected, "bootstrap_dependency", false, "config bootstrap kubernetes eks", "Remove the dependent EKS adapter first with --remove."),
				declaredCommandError(fault.KindRejected, "aws_bootstrap_source_rejected", false, "help config bootstrap aws", "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."),
				declaredCommandError(fault.KindContract, "invalid_bootstrap_preview", false, "context show", "Inspect the Context recipe before retrying."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed Context bootstrap change."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{TargetKind: tobari.ContextBootstrapTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
		},
		handler: runConfigBootstrapAWS,
	}
}

func configBootstrapEKSSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{
		Path: "config bootstrap kubernetes eks", Summary: "Configure, refresh, or remove one reviewed EKS target for future Workspace homes",
		Args:   "[--kube-context <name>] [--refresh] [--remove] [--context <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "context.workspace-bootstrap",
			Outcome:      "Compose one host AWS CLI-generated EKS context with the Context AWS profile, refresh it, or remove only the EKS target without rewriting existing Workspace homes",
			Inputs: []CommandInput{
				{Name: "--kube-context", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact context name in fixed host ~/.kube/config; conflicts with refresh and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
				{Name: "--refresh", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Re-read the currently selected host kube context; conflicts with context selection and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--remove"}},
				{Name: "--remove", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove only the EKS adapter for future Workspaces; preserve AWS and existing Workspace homes.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--refresh"}},
				executionContextInput(), formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Context already has an AWS IAM Identity Center bootstrap profile.",
				"Fixed host ~/.kube/config is a bounded safe regular file and the selected context resolves to an inline-CA commercial EKS endpoint with the reviewed aws eks get-token exec contract and matching AWS_PROFILE.",
				"No host credential, token cache, arbitrary exec, alternate kubeconfig path, or network authority is imported.",
			},
			FixedTarget: fixedContextBootstrapTarget(),
			Errors: mutationCommandErrors("config bootstrap kubernetes eks", "context show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config bootstrap kubernetes eks", "Supply one action flag or use interactive text streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_eks_bootstrap_change", false, "help config bootstrap kubernetes eks", "Choose exactly one configure, refresh, or remove action."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindNotFound, "bootstrap_not_configured", false, "help config bootstrap kubernetes eks", "Configure AWS first, or select EKS before refresh/remove."),
				declaredCommandError(fault.KindRejected, "eks_bootstrap_source_rejected", false, "help config bootstrap kubernetes eks", "Use a strict AWS CLI-generated EKS context bound to the Context AWS profile."),
				declaredCommandError(fault.KindRejected, "config_bootstrap_failed", false, "context show", "Inspect the current recipe and selected kube context."),
				declaredCommandError(fault.KindRejected, "bootstrap_source_changed", true, "config bootstrap kubernetes eks", "Review a fresh semantic diff before applying."),
				declaredCommandError(fault.KindContract, "invalid_bootstrap_preview", false, "context show", "Inspect the Context recipe before retrying."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed Context bootstrap change."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{TargetKind: tobari.ContextBootstrapTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
		},
		handler: runConfigBootstrapEKS,
	}
}

func configShellSpec() CommandSpec {
	return CommandSpec{
		Path: "config shell", Summary: "Configure Context shell presentation directly or with one staged terminal Apply",
		Args:   "[--variable COLORTERM|NO_COLOR|PS1|TERM] [--source default|inherit|literal] [--value <value>] [--context <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "Configure one allowlisted shell variable through complete flags, or stage several rows from the complete terminal inventory and apply them atomically",
			Inputs: []CommandInput{
				{
					Name: "--variable", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Allowlisted shell environment variable configured for future interactive sessions.",
					AllowedValues: tobari.ContextShellEnvironmentVariables(),
					Requires:      []string{"--source"},
				},
				{
					Name: "--source", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "default removes the override; inherit reads an exported host value at entry; literal uses --value. Omit all setting flags to use the staged terminal editor.",
					AllowedValues: []string{"default", "inherit", "literal"},
					Requires:      []string{"--variable"},
				},
				{
					Name: "--value", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Exact Context-owned value of at most 4096 UTF-8 bytes; required only for literal and may be explicitly empty.",
					AllowedValues: []string{}, Requires: []string{"--variable", "--source"},
				},
				executionContextInput(),
				formatInput(),
			},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The selected Context exists on the trusted host; inherited values must be exported by the process that starts Tobari.", "When setting flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text."},
			FixedTarget:   fixedContextShellTarget(),
			Errors: mutationCommandErrors("config shell", "context show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config shell", "Supply every setting flag or run the wizard with text success/error output on interactive stdin and stderr."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config shell", "Retry with complete setting flags or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_shell_environment", false, "help config shell", "Choose an allowlisted variable and a valid source/value combination."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindInternal, "context_read_failed", false, "doctor", "Inspect the host Context stores before retrying the wizard."),
				declaredCommandError(fault.KindRejected, "config_shell_failed", false, "context show", "Inspect the Context shell environment before retrying."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed Context shell setting."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextShellTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runConfigShell,
	}
}

func configGitSpec() CommandSpec {
	return CommandSpec{
		Path: "config git", Summary: "Configure one Context Git identity directly or from one staged terminal screen",
		Args:   "[--source default|inherit|literal] [--name <name>] [--email <email>] [--context <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "Choose no Context fallback, inherited host user.name and user.email, or one fixed Context-owned Git identity through complete flags or one staged terminal screen",
			Inputs: []CommandInput{
				{
					Name: "--source", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "default removes the Context fallback; inherit projects host user.name and user.email at Workspace entry; literal uses --name and --email. Omit all setting flags to use the staged terminal editor.",
					AllowedValues: []string{"default", "inherit", "literal"},
				},
				{
					Name: "--name", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Exact non-empty Context-owned Git user.name of at most 4096 safe UTF-8 bytes; required with --email only for literal.",
					AllowedValues: []string{}, Requires: []string{"--source", "--email"},
				},
				{
					Name: "--email", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Exact non-empty Context-owned Git user.email of at most 4096 safe UTF-8 bytes; required with --name only for literal.",
					AllowedValues: []string{}, Requires: []string{"--source", "--name"},
				},
				executionContextInput(),
				formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Context exists on the trusted host; inherited identity is resolved from only host global Git configuration at Workspace entry.",
				"When setting flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text.",
			},
			FixedTarget: fixedContextGitIdentityTarget(),
			Errors: mutationCommandErrors("config git", "context show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config git", "Supply every setting flag or run the wizard with text success/error output on interactive stdin and stderr."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config git", "Retry with complete setting flags or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_git_identity", false, "help config git", "Choose default, inherit, or a literal source with both name and email."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindInternal, "context_read_failed", false, "doctor", "Inspect the host Context stores before retrying the wizard."),
				declaredCommandError(fault.KindRejected, "config_git_failed", false, "context show", "Inspect the Context Git identity before retrying."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the confirmed Context Git identity setting."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextGitIdentityTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runConfigGit,
	}
}

func contextListSpec() CommandSpec {
	return CommandSpec{
		Path: "context list", Summary: "List named execution Contexts",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "List the complete local Context collection and identify the current omission default",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "active", Type: OutputFieldTypeString, Description: "Name of the host-selected current Context used when Context is omitted."},
					{Name: "context_state", Type: OutputFieldTypeString, Description: "Whether the selected/default Context is persisted authority or a display-only synthetic default.", Enum: []string{"persisted", "synthetic_default"}},
					{Name: "items", Type: OutputFieldTypeArray, Description: "Complete local Context collection with current-default state, stable ID, image, agent profile, policy mode, source access, and runtime status.", SemanticScope: "All locally configured Contexts at one observation.", Items: &OutputField{
						Type: OutputFieldTypeObject, Description: "One configured Context.", Fields: []OutputField{
							{Name: "id", Type: OutputFieldTypeString, Description: "Stable Context authority identity, or null for the display-only synthetic default.", Nullable: true},
							{Name: "name", Type: OutputFieldTypeString, Description: "Human Context name."},
							{Name: "context_state", Type: OutputFieldTypeString, Description: "Persisted authority or a display-only synthetic default.", Enum: []string{"persisted", "synthetic_default"}},
							{Name: "active", Type: OutputFieldTypeBoolean, Description: "Whether this Context is the current default."},
							{Name: "agent_profile", Type: OutputFieldTypeString, Description: "Read-only agent profile reference."},
							{Name: "image", Type: OutputFieldTypeString, Description: "Selected compatible runtime image."},
							{Name: "policy_mode", Type: OutputFieldTypeString, Description: "Policy development mode.", Enum: []string{"guided", "advanced"}},
							{Name: "source_access", Type: OutputFieldTypeString, Description: "Direct project-source bind access.", Enum: []string{"read-only", "read-write"}},
							{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Immutable revision of the Context-owned normalized policy snapshot."},
							{Name: "native_readiness", Type: OutputFieldTypeString, Description: "Immutable native-client readiness selection.", Enum: []string{"enabled", "disabled"}},
							{Name: "method_policy", Type: OutputFieldTypeObject, Description: "Effective default and exact method decisions owned by the Context.", Fields: contextPolicyMethodPolicyOutput("Effective default and exact method decisions owned by the Context.").Fields},
							{Name: "runtime_status", Type: OutputFieldTypeString, Description: "Selected Runtime readiness when observed.", Optional: true, Enum: []string{"official", "ready"}},
							contextBootstrapOutputField(),
						},
					}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "contexts", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
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
		Args: "[--name <name>] [--details] [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "Inspect the current Context or one named Context and its separated store references",
			Inputs: []CommandInput{
				{Name: "--name", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Named Context to inspect; omission selects the current/default Context.", AllowedValues: []string{}},
				{Name: "--details", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Expand human text with complete Context diagnostics; JSON is already complete and remains unchanged.", AllowedValues: []string{}, DefaultValue: stringPointer("false")},
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
		Path: "context create", Summary: "Create a named execution Context directly or with the terminal wizard",
		Args:   "[--name <name>] [--runtime <standard|name@ordinal>] [--mode guided|advanced] [--source-access read-only|read-write] [--native-readiness enabled|disabled] [--bootstrap-aws-profile <name>] [--bootstrap-eks-context <name>] [--format text|json]",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "context.composition",
			Outcome:       "Create one named Context with separate owner-only policy and brokered-authentication state",
			Inputs:        []CommandInput{contextCreateNameInput(), contextCreateRuntimeInput(), contextModeInput(), contextSourceAccessInput(), contextNativeReadinessInput(), contextCreateAWSBootstrapInput(), contextCreateEKSBootstrapInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The host Context directory is accessible."},
			FixedTarget:   fixedContextCatalogTarget(),
			Errors: mutationCommandErrors("context create", "context list",
				declaredCommandError(fault.KindInvalidInput, "context_create_wizard_unavailable", false, "help context create", "Run the argument-free wizard on interactive text streams or supply --name for direct mode."),
				declaredCommandError(fault.KindInternal, "context_create_wizard_failed", false, "context create", "Retry the wizard or use direct mode with --name."),
				declaredCommandError(fault.KindRejected, "aws_bootstrap_source_rejected", false, "help config bootstrap aws", "Choose a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."),
				declaredCommandError(fault.KindRejected, "eks_bootstrap_source_rejected", false, "help config bootstrap kubernetes eks", "Choose a strict AWS CLI-generated EKS context bound to the selected AWS profile."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context", false, "help context create", "Correct the Context name, image, policy mode, or source access."),
				declaredCommandError(fault.KindRejected, "context_exists", false, "context list", "List existing Contexts before choosing another name."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_revision_not_ready", false, "runtime history", "Choose an existing successful revision."),
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
		Path: "context use", Summary: "Select the current default Context",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "context.composition",
			Outcome:       "Select one existing Context as the omission default without changing existing Tobari or shared enforcement authority",
			Inputs:        []CommandInput{contextNameInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The named Context already exists."},
			FixedTarget:   fixedActiveContextTarget(),
			Errors: mutationCommandErrors("context use", "context show",
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context or create it first."),
				declaredCommandError(fault.KindRejected, "context_use_failed", false, "context show", "Inspect the current/default Context marker."),
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

func contextDeleteSpec() CommandSpec {
	return CommandSpec{
		Path: "context delete", Summary: "Delete one unused non-current execution Context",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "context.composition",
			Outcome:      "Delete one exact non-current Context only when no logical Workspace remains bound, preserving project files and shared runtime images",
			Inputs:       []CommandInput{contextNameInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "id", Type: OutputFieldTypeString, Description: "Stable authority ID of the deleted Context."},
					{Name: "name", Type: OutputFieldTypeString, Description: "Name of the deleted Context."},
					{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Confirmed deletion state."},
					{Name: "cluster", Type: OutputFieldTypeString, Description: "Whether the shared cluster requires reconciliation.", Enum: []string{"not_applicable", "requires_reconcile"}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "context_deletion", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"The named Context is not current and owns no logical Workspace."},
			FixedTarget:   fixedContextCatalogTarget(),
			Errors: mutationCommandErrors("context delete", "context list",
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindRejected, "context_is_current", false, "context use", "Select another Context first."),
				declaredCommandError(fault.KindRejected, "context_is_protected", false, "context show", "Keep the foundational default Context."),
				declaredCommandError(fault.KindRejected, "context_has_workspaces", false, "list", "Delete every Workspace bound to the Context first."),
				declaredCommandError(fault.KindRejected, "context_delete_failed", false, "context list", "Inspect the Context collection before retrying."),
				declaredCommandError(fault.KindContract, "invalid_context_delete_result", false, "context list", "Reconcile the Context collection after deletion."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ContextCatalogTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes},
			},
		},
		handler: runContextDelete,
	}
}

func runtimeListSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime list", Summary: "List reusable Runtimes and their ready head revisions",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID:  "runtime.customization",
			Outcome:       "List the complete installation-wide Runtime catalog and each ready head revision",
			Inputs:        []CommandInput{formatInput()},
			Output:        runtimeListOutput(),
			Prerequisites: []string{},
			Errors: readCommandErrors("runtime list", true,
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "runtime list", "Repair the Runtime list JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runRuntimeList,
	}
}

func contextRuntimeSetSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{Path: "context runtime set", Summary: "Pin a Context to one ready Runtime revision", Args: "--runtime <standard|name@ordinal> [--context <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{CapabilityID: "context.composition", Outcome: "Explicitly upgrade or roll back one Context Runtime binding without rebuilding or changing existing Workspace homes",
			Inputs: []CommandInput{{Name: "--runtime", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact ready Runtime selection as standard or name@ordinal.", AllowedValues: []string{}}, executionContextInput(), formatInput()},
			Output: contextReportOutput(), Prerequisites: []string{"The selected Runtime revision already exists and is ready."}, FixedTarget: fixedContextRuntimeBindingTarget(),
			Errors: mutationCommandErrors("context runtime set", "context show",
				declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose a valid Context name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_selection", false, "runtime history", "Choose standard or one ready name@ordinal revision."),
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_revision_not_ready", false, "runtime history", "Choose an existing successful revision."),
				declaredCommandError(fault.KindRejected, "context_runtime_set_failed", false, "context show", "Inspect the unchanged Context Runtime binding."),
				declaredCommandError(fault.KindContract, "invalid_context_report", false, "context show", "Reconcile the Context Runtime binding."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime.")),
			Mutation: &MutationContract{TargetKind: tobari.ContextRuntimeBindingTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}},
		handler: runContextRuntimeSet}
}

func runtimeShowSpec() CommandSpec {
	return runtimeReadSpec("runtime show", "Inspect one reusable Runtime", "Inspect one Runtime's source and complete successful revision inventory", tobari.TaskRuntimeShow, runRuntimeShow)
}

func runtimeHistorySpec() CommandSpec {
	return runtimeReadSpec("runtime history", "Show one Runtime's immutable revision history", "Inspect one Runtime's ordered immutable successful revisions", tobari.TaskRuntimeHistory, runRuntimeHistory)
}

func runtimeReadSpec(path, summary, outcome, _ string, handler commandHandler) CommandSpec {
	minimum := int64(1)
	return CommandSpec{Path: path, Summary: summary, Args: "--name <name> [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{CapabilityID: "runtime.customization", Outcome: outcome,
			Inputs: []CommandInput{{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Unique local Runtime name.", AllowedValues: []string{}}, formatInput()},
			Output: runtimeReportOutput(), Prerequisites: []string{}, Errors: readCommandErrors(path, true,
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_name", false, "runtime list", "Choose a Runtime from the local catalog."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, path, "Repair the Runtime JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			)}, handler: handler}
}

func runtimeCreateSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{Path: "runtime create", Summary: "Create a reusable Runtime source tree", Args: "--name <name> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{CapabilityID: "runtime.customization", Outcome: "Create one owner-only managed Runtime source tree without building or changing a Context",
			Inputs: []CommandInput{{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Unique local Runtime name.", AllowedValues: []string{}}, formatInput()},
			Output: runtimeReportOutput(), Prerequisites: []string{}, FixedTarget: fixedRuntimeCatalogTarget(),
			Errors: mutationCommandErrors("runtime create", "runtime list",
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_name", false, "runtime list", "Choose a valid unique Runtime name."),
				declaredCommandError(fault.KindRejected, "runtime_exists", false, "runtime show", "Inspect the existing Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_create_failed", false, "runtime list", "Inspect the local Runtime catalog."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "runtime list", "Reconcile the Runtime catalog."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime.")),
			Mutation: &MutationContract{TargetKind: tobari.RuntimeCatalogTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}},
		handler: runRuntimeCreate}
}

func runtimeBuildSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{
		Path: "runtime build", Summary: "Build an immutable revision of a reusable Runtime",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "runtime.customization",
			Outcome:       "Snapshot the complete managed source tree, build and validate it, and append one immutable semantic revision without changing any Context",
			Inputs:        []CommandInput{{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Unique local managed Runtime name.", AllowedValues: []string{}}, formatInput()},
			Output:        runtimeReportOutput(),
			Prerequisites: []string{"The named managed Runtime exists and its bounded owner-only source tree contains Dockerfile.", "The trusted host Docker daemon and Buildx plugin are available."},
			FixedTarget:   fixedRuntimeCatalogTarget(),
			Errors: mutationCommandErrors("runtime build", "runtime show",
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_name", false, "runtime list", "Choose a valid managed Runtime name."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing managed Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_build_failed", false, "runtime show", "Inspect the unchanged Runtime history and source path."),
				declaredCommandError(fault.KindUnavailable, "image_not_found", false, "runtime build", "Build or make the official base image available to Docker."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "runtime show", "Correct the source so the image preserves the Tobari runtime contract."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "runtime show", "Reconcile the confirmed Runtime build."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.RuntimeCatalogTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runRuntimeBuild,
	}
}

func projectEnterSpec() CommandSpec {
	return CommandSpec{
		Path: "tobari", Summary: "Set up, choose, or create the current directory's Workspace and enter a reusable session",
		Args:   "[--context <name>]",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "On interactive first use, review a Context and prepare shared services; then choose or create the current directory's Workspace, reconcile its selected runtime, and enter a reusable session",
			Inputs:       []CommandInput{lifecycleContextInput()}, Output: noOutput(),
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
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runProjectEnter,
	}
}

func statusSpec() CommandSpec {
	return CommandSpec{
		Path: "status", Summary: "Inspect the nearest current-directory Tobari",
		Args:   "[--context <name>] [--format text|json]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Report the bound Context, whether logical Workspace state exists for the current directory, its recoverable runtime diagnostic, attached or detached session observation, and create-time bootstrap revision relationship",
			Inputs:       []CommandInput{lifecycleContextInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "context_state", Type: OutputFieldTypeString, Description: "Whether the selected/default Context is persisted authority or a display-only synthetic default.", Enum: []string{"persisted", "synthetic_default"}},
					{Name: "exists", Type: OutputFieldTypeBoolean, Description: "Whether logical Tobari state exists for the current directory."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Nearest canonical project root when one exists."},
					{Name: "id", Type: OutputFieldTypeString, Description: "Diagnostic stable logical ID when one exists."},
					{Name: "home", Type: OutputFieldTypeString, Description: "Diagnostic per-Tobari XDG home path when one exists."},
					{Name: "runtime", Type: OutputFieldTypeString, Description: "Recoverable runtime diagnostic; incomplete means the logical state record is missing and must be deleted before recreation."},
					{Name: "attachment", Type: OutputFieldTypeString, Description: "Transient session observation: attached or detached when the Workspace exists, and not_applicable when it does not."},
					{Name: "bootstrap", Type: OutputFieldTypeObject, Description: "One-time future-Workspace configuration snapshot relationship; never credential state.", Fields: []OutputField{
						{Name: "state", Type: OutputFieldTypeString, Description: "Whether no recipe exists, this Workspace never received it, its applied revision is current, or it retains an older create-time revision.", Enum: []string{"not_configured", "not_applied", "current", "older"}},
						{Name: "applied_revision", Type: OutputFieldTypeString, Description: "Semantic revision projected when this Workspace was created, or an empty string when none was applied."},
						{Name: "current_revision", Type: OutputFieldTypeString, Description: "Current Context recipe revision, or an empty string when the recipe was removed."},
					}},
					{Name: "context", Type: OutputFieldTypeString, Description: "Selected invocation Context display name, including when no Workspace exists."},
					{Name: "context_id", Type: OutputFieldTypeString, Description: "Selected stable Context authority identity, or null before a Context is persisted.", Nullable: true},
					{Name: "next_argv", Type: OutputFieldTypeArray, Description: "Exact argv that re-enters the persisted Context-bound lifecycle target, or uses omission-based selection when the Context is only a synthetic default.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One exact argv token."}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "status", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("status", true,
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindContract, "invalid_context_binding", false, "context list", "Inspect the Context catalog before selecting a Workspace."),
				declaredCommandError(fault.KindContract, "context_binding_stale", false, "doctor", "Inspect Context and Workspace state."),
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local CWD-owned state."),
				declaredCommandError(fault.KindInternal, "runtime_status_failed", false, "status", "Inspect the selected project's runtime."),
				declaredCommandError(fault.KindInternal, "session_status_failed", false, "status", "Inspect the selected Workspace runtime again."),
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
		Args: "[--context <name>] [--force]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Delete one nearest Context-bound CWD Workspace, its exact owned runtime resources, persistent home, and tool-owned authentication state when detached; reject attached sessions unless --force overrides only that guard, while preserving the mounted project root",
			Inputs: []CommandInput{lifecycleContextInput(), {
				Name: "--force", Source: InputSourceFlag, Required: false,
				ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
				Description: "Override only the attached-session safety guard and terminate that session while deleting the disclosed Context-bound Workspace, persistent home, and tool-owned authentication state.", AllowedValues: []string{}, DefaultValue: stringPointer("false"),
			}},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Whether the selected logical Tobari was deleted."},
					{Name: "root", Type: OutputFieldTypeString, Description: "Deleted canonical project root."},
					{Name: "id", Type: OutputFieldTypeString, Description: "Deleted stable logical ID."},
					{Name: "home", Type: OutputFieldTypeString, Description: "Deleted per-Tobari XDG home path."},
					{Name: "context", Type: OutputFieldTypeString, Description: "Context display name bound to the deleted Tobari."},
					{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority identity bound to the deleted Tobari."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The target is the nearest Workspace in the explicit or current Context; its mounted project root is preserved.", "Without --force, no session is attached; --force terminates any attached session while deleting the persistent home and tool-owned authentication state."},
			FixedTarget:   fixedCurrentDirectoryTarget(),
			Errors: mutationCommandErrors("delete", "status",
				declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
				declaredCommandError(fault.KindContract, "invalid_context_binding", false, "context list", "Inspect the Context catalog before selecting a Workspace."),
				declaredCommandError(fault.KindContract, "context_binding_stale", false, "delete", "Review the newly selected target before retrying force deletion."),
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
	spec := CommandSpec{
		Path: "cluster up", Summary: "Start the shared Gateway, OPA, and Auth Broker",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Start one healthy shared enforcement cluster without mounting a work root",
			Inputs:       []CommandInput{},
			Output:       textClusterStatusOutput(),
			Prerequisites: []string{
				"Docker Engine and Docker Compose v2 are available.",
				"The routine path builds or reuses pinned local Gateway and runtime images from embedded source.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster up", "cluster status",
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before startup."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "cluster status", "Reconcile the confirmed startup."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "cluster status", "Repair the runtime status contract."),
				declaredCommandError(fault.KindUnavailable, "cluster_start_failed", false, "cluster status", "Reconcile partial Docker state."),
				declaredCommandError(fault.KindUnavailable, "network_guard_failed", false, "doctor", "Inspect Docker Engine network-namespace and nftables support."),
				declaredCommandError(fault.KindUnavailable, "gateway_image_unavailable", true, "doctor", "Inspect the local Docker image store before retrying Gateway validation."),
				declaredCommandError(fault.KindUnavailable, "gateway_image_build_failed", false, "doctor", "Inspect Docker BuildKit and the pinned embedded Gateway build inputs."),
				declaredCommandError(fault.KindContract, "runtime_image_api_mismatch", false, "doctor", "Inspect the executable resolver channel and selected immutable component API authorities."),
				declaredCommandError(fault.KindContract, "gateway_image_incompatible", false, "doctor", "Inspect the Gateway image API, source identity, and architecture contract."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the verified Auth Broker image."),
				declaredCommandError(fault.KindContract, "auth_broker_image_incompatible", false, "doctor", "Inspect the Auth Broker image API, digest, entrypoint, user, and architecture contract."),
				declaredCommandError(fault.KindUnavailable, "credential_companion_unavailable", true, "cluster up", "Reconcile the shared cluster and its Auth Broker companion session."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "cluster up", "Reconcile the shared cluster and retry the bounded broker control path."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "cluster status", "Inspect partial shared-cluster state before another reconcile."),
				declaredCommandError(fault.KindContract, "auth_broker_unlock_failed", false, "doctor", "Inspect Auth Broker and root-key provider state."),
				declaredCommandError(fault.KindUnavailable, "root_key_unavailable", false, "doctor", "Inspect the host root-key provider."),
				declaredCommandError(fault.KindRejected, "root_key_missing_with_vault", false, "doctor", "Restore the original root key or explicitly remove local authentication state."),
				declaredCommandError(fault.KindRejected, "root_key_unsafe", false, "doctor", "Repair unsafe root-key or Auth Broker state paths."),
				declaredCommandError(fault.KindUnavailable, "keychain_denied", false, "cluster up", "Allow trusted-host Keychain access and retry cluster reconciliation."),
				declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect Context vault integrity without printing its contents."),
				declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported Context vault."),
				declaredCommandError(fault.KindRejected, "invalid_provider_manifest", false, "doctor", "Repair the owner-controlled provider manifest collection."),
				declaredCommandError(fault.KindRejected, "ambiguous_provider_http_binding", false, "doctor", "Remove the overlapping exact provider HTTP binding."),
				declaredCommandError(fault.KindUnavailable, "runtime_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the official runtime base image."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "context show", "Inspect the relevant Context runtime image contract."),
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
	if !buildIdentityHasBroker() {
		spec.Summary = "Start the shared Gateway and OPA"
		spec.Agent.Prerequisites[1] = "The routine path uses one immutable Gateway image plus the official runtime base image."
		spec.Agent.Errors = standardClusterErrors(spec.Agent.Errors)
	}
	return spec
}

func clusterStatusSpec() CommandSpec {
	summary := "Inspect the shared Gateway and OPA"
	if buildIdentityHasBroker() {
		summary = "Inspect the shared Gateway, OPA, and Auth Broker"
	}
	return CommandSpec{
		Path: "cluster status", Summary: summary,
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Observe shared enforcement health, aggregate Context policy revision, registry integrity, attached count, and recent errors",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   clusterStatusOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "cluster", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("cluster status", true,
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "doctor", "Inspect Docker and cluster state."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the status contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "cluster status", "Repair JSON projection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runClusterStatus,
	}
}

func clusterStatusOutputFields() []OutputField {
	fields := []OutputField{
		{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether schema-1 aggregate cluster state exists."},
		{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether the compiled shared services are healthy."},
		{Name: "policy", Type: OutputFieldTypeString, Description: "Canonical host XDG policy directory, or null when unconfigured.", Nullable: true},
		{Name: "tobari_count", Type: OutputFieldTypeInteger, Description: "Number of attached Tobari."},
		{Name: "context_count", Type: OutputFieldTypeInteger, Description: "Number of Context policies loaded in the aggregate projection."},
		{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Content-addressed aggregate policy revision, or null when unconfigured.", Nullable: true},
		{Name: "policy_projection", Type: OutputFieldTypeString, Description: "All-Context policy projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
		{Name: "principal_registry", Type: OutputFieldTypeString, Description: "Principal registry integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
		{Name: "gateway_projection", Type: OutputFieldTypeString, Description: "Gateway routing projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
	}
	componentNames := []string{"gateway", "opa"}
	componentDescription := "Exact Gateway and OPA observations."
	componentScope := "The two shared standard services when configured; empty when unconfigured."
	if buildIdentityHasBroker() {
		fields = append(fields,
			OutputField{Name: "auth_provider_projection", Type: OutputFieldTypeString, Description: "Experimental Auth Broker provider projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
			OutputField{Name: "auth_broker_state", Type: OutputFieldTypeString, Description: "Observed ready, locked, or unavailable experimental Auth Broker state.", Enum: []string{"ready", "locked", "unavailable"}},
			OutputField{Name: "credential_companion_state", Type: OutputFieldTypeString, Description: "Observed experimental trusted-host credential companion state.", Enum: []string{"ready", "prepared", "absent", "unavailable"}},
			OutputField{Name: "root_key_backend", Type: OutputFieldTypeString, Description: "Selected experimental host root-key backend or unavailable state.", Enum: []string{"macos_keychain", "xdg_file", "unavailable"}},
		)
		componentNames = []string{"auth-broker", "gateway", "opa"}
		componentDescription = "Exact Auth Broker, Gateway, and OPA observations."
		componentScope = "The three shared experimental services when configured; empty when unconfigured."
	}
	fields = append(fields,
		OutputField{Name: "components", Type: OutputFieldTypeArray, Description: componentDescription, SemanticScope: componentScope, Items: &OutputField{
			Type: OutputFieldTypeObject, Description: "One shared service observation.", Fields: []OutputField{
				{Name: "name", Type: OutputFieldTypeString, Description: "Stable shared component name.", Enum: componentNames},
				{Name: "state", Type: OutputFieldTypeString, Description: "Observed container state.", Enum: []string{"absent", "created", "running", "paused", "restarting", "removing", "exited", "dead"}},
				{Name: "health", Type: OutputFieldTypeString, Description: "Observed healthcheck state.", Enum: []string{"none", "starting", "healthy", "unhealthy"}},
			},
		}},
		OutputField{Name: "recent_error", Type: OutputFieldTypeString, Description: "Bounded recent runtime error, or null.", Nullable: true},
	)
	return fields
}

func clusterDenialsSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster denials", Summary: "Read policy-denial evidence",
		Args:   "[--tail <lines>] [--format text|json]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Identify recent denied HTTP effects, including exact GraphQL operation/root coordinates when classified, and the pending permission review command",
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
						Description:   "Validated denials ordered oldest to newest with host-issued project principal, scheme-independent request authority (host and port), method, path, protocol, optional exact GraphQL operation/root or MCP method/tool coordinate, reason, status, and exact-rule learnability.",
						SemanticScope: "Valid denials found in the requested bounded Gateway log-line window.",
						Items:         &OutputField{Type: OutputFieldTypeObject, Description: "One validated denial observation.", Fields: policyDenialOutputFields()},
					},
					{Name: "review_command", Type: OutputFieldTypeString, Description: "Exact command that opens the pending permission review queue."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "denials", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
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
	summary := "Read Gateway and OPA logs"
	args := "[--component gateway|opa|all] [--tail <lines>]"
	componentDescription := "Select Gateway, OPA, or every shared component."
	components := []string{"gateway", "opa", "all"}
	if buildIdentityHasBroker() {
		summary = "Read Auth Broker, Gateway, and OPA logs"
		args = "[--component auth-broker|gateway|opa|all] [--tail <lines>]"
		componentDescription = "Select Auth Broker, Gateway, OPA, or every shared component."
		components = []string{"auth-broker", "gateway", "opa", "all"}
	}
	return CommandSpec{
		Path: "cluster logs", Summary: summary,
		Args:   args,
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "cluster.logs",
			Outcome:      "Inspect bounded redacted shared logs including policy-denial evidence",
			Inputs: []CommandInput{
				{
					Name: "--component", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: componentDescription, AllowedValues: components,
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
			Outcome:      "Return unique pending exact host, port, method, path, and optional GraphQL operation/root proposals with opaque approval IDs",
			Inputs:       []CommandInput{denialTailInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "policy_candidates", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors:        policyCandidateReadErrors("policy candidates", true),
		},
		handler: runPolicyCandidates,
	}
}

func policyReviewSpec() CommandSpec {
	return CommandSpec{
		Path: "policy review", Summary: "Review pending network permissions",
		Args: "[--tail <lines>] [--format text|json] [--watch] [--notify auto|osc9|bel|off]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Review pending exact HTTP or GraphQL effects and typed HTTP single-segment path-template proposals; a raw terminal can stage exact decisions from the list, inspect template scope, apply one Context's reviewed set, and optionally watch bounded snapshots",
			Inputs:       []CommandInput{reviewTailInput(), formatInput(), policyReviewWatchInput(), policyReviewNotifyInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "policy_review", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors: append(policyCandidateReadErrors("policy review", true),
				declaredCommandError(fault.KindInvalidInput, "policy_review_watch_requires_tty", false, "help policy review", "Run watch with text output in an interactive raw terminal."),
			),
			Interactive: &InteractiveWorkflowContract{
				ActionCommand:          "policy apply-reviewed",
				SelectionReferenceKind: tobari.PolicyCandidateKind,
				SelectionOutputField:   "id",
				Confirmation:           "explicit_yes",
				NonInteractiveBehavior: "read_only",
			},
		},
		handler: runPolicyReview,
	}
}

func policyApplyReviewedSpec() CommandSpec {
	return CommandSpec{
		Path: "policy apply-reviewed", Summary: "Apply decisions staged by Permission Inbox",
		Args: "", Effect: operation.EffectWrite, Role: RoleAct,
		Visibility: CommandVisibilityInternal,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Revalidate and activate one bounded typed set of exact or single-segment-template Allows and exact Denies for one Context staged by interactive policy review",
			Inputs:       []CommandInput{},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
				TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed reviewed-set task identity."},
					{Name: "policy", Type: OutputFieldTypeString, Description: "Host policy source associated with the confirmed aggregate."},
					{Name: "allow_count", Type: OutputFieldTypeInteger, Description: "Number of exact or path-template Allow rules applied."},
					{Name: "deny_count", Type: OutputFieldTypeInteger, Description: "Number of exact Denies applied."},
					{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Always true after exact revision confirmation."},
					{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact confirmed active aggregate revision."},
					{Name: "decisions", Type: OutputFieldTypeArray, Description: "Ordered stored rules repeated in the confirmed receipt.", Items: &OutputField{
						Type: OutputFieldTypeObject, Description: "One freshly revalidated applied decision.", Fields: []OutputField{
							{Name: "rule_id", Type: OutputFieldTypeString, Description: "Opaque stored policy-rule identity."},
							{Name: "review_item_id", Type: OutputFieldTypeString, Description: "Unchanged opaque exact-candidate or path-template proposal identity."},
							{Name: "decision", Type: OutputFieldTypeString, Description: "Applied Allow or Deny decision.", Enum: []string{tobari.PolicyDecisionAllow, tobari.PolicyDecisionDeny}},
							{Name: "match", Type: OutputFieldTypeString, Description: "Exact or single-segment path-template match.", Enum: []string{tobari.PolicyMatchExact, tobari.PolicyMatchPathTemplate}},
							{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context identity."},
							{Name: "context", Type: OutputFieldTypeString, Description: "Exact Context display name."},
							{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal."},
							{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe canonical project root."},
							{Name: "host", Type: OutputFieldTypeString, Description: "Exact request host."},
							{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact request port."},
							{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
							{Name: "path", Type: OutputFieldTypeString, Description: "Exact path without query data."},
							{Name: "source_candidates", Type: OutputFieldTypeArray, Description: "Sorted exact candidate evidence bound to the stored rule.", Items: &OutputField{Type: OutputFieldTypeString, Description: "Opaque exact policy-candidate identity."}},
							{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: []string{tobari.PolicyProtocolHTTP, tobari.PolicyProtocolGraphQL, tobari.PolicyProtocolMCP}},
							{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL operation type; empty for HTTP."},
							{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact GraphQL root field; empty for HTTP."},
							{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
							{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
						},
					}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"An interactive policy review session has a bounded non-empty staged decision set."},
			FixedTarget: &FixedTarget{
				Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID,
				Description: "The one CLI-owned installation policy decision set.", Scope: FixedTargetScopeToolLocal,
			},
			Errors: policyMutationCommandErrors("policy review", "policy review",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_review_session", false, "policy review", "Stage decisions through an interactive Permission Inbox."),
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_review_set", false, "policy review", "Review a bounded non-empty set of exact candidates."),
				declaredCommandError(fault.KindInvalidInput, "empty_policy_review_set", false, "policy review", "Stage at least one offered decision."),
				declaredCommandError(fault.KindRejected, "policy_review_changed", false, "policy review", "Review the current pending queue again."),
				declaredCommandError(fault.KindRejected, "policy_review_scope_mixed", false, "policy review", "Apply or discard the current Context decisions before reviewing another Context."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "policy review", "Review again after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete candidate policy."),
				declaredCommandError(fault.KindUnavailable, "policy_learning_failed", false, "cluster status", "Reconcile OPA and current policy state."),
				declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Inspect retained denial compatibility."),
				declaredCommandError(fault.KindContract, "invalid_policy_review_result", false, "cluster status", "Reconcile the confirmed reviewed set."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.PolicyDecisionSetKind, TargetInputs: []string{},
				Impact: operation.Impact{
					Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
				},
			},
		},
		handler: runPolicyApplyReviewed,
	}
}

func policyRulesSpec() CommandSpec {
	return CommandSpec{
		Path: "policy rules", Summary: "Inspect current learned policy decisions",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Inspect complete current project-bound learned Allow and exact Deny decisions, including optional GraphQL operation/root coordinates; on a TTY explicitly reset one decision",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyRuleOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "policy_rules", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"Every configured Context has a validated policy source and the shared aggregate is current."},
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
			Outcome:      "Test, record, and activate one exact retained host, port, method, path, and optional GraphQL operation/root permission",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates or policy review")},
			Output:       policyLearningChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates or policy review and remains in retained Gateway logs.",
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
			Outcome:      "Test, record, and activate one exact project-bound denial for a retained host, port, method, path, and optional GraphQL operation/root coordinate",
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates or policy review")},
			Output:       policyDenyChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates or policy review and remains an actionable pending candidate.",
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

func clusterDownSpec() CommandSpec {
	spec := CommandSpec{
		Path: "cluster down", Summary: "Remove the empty shared cluster",
		Args: "[--purge]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Remove shared containers and networks after every logical Workspace is deleted. With --purge, also remove shared CA volumes and the active policy-bundle volume. Preserve encrypted Context vaults and the installation root key in both modes.",
			Inputs:       []CommandInput{purgeInput("Also remove exact shared CA and active policy-bundle volumes.")},
			Output:       textClusterStatusOutput(),
			Prerequisites: []string{
				"Every logical Workspace has been deleted.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster down", "cluster status",
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "cluster_not_empty", false, "list", "Delete every listed logical Workspace from its project directory."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster."},
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
	if !buildIdentityHasBroker() {
		spec.Agent.Outcome = "Remove shared containers and networks after every logical Workspace is deleted. With --purge, also remove shared CA volumes and the active policy-bundle volume."
		spec.Agent.Errors = standardClusterErrors(spec.Agent.Errors)
	}
	return spec
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
					{Name: "context", Type: OutputFieldTypeString, Description: "Context display name permanently bound to the Workspace."},
					{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority identity permanently bound to the Workspace."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "tobari", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
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

func fixedRuntimeCatalogTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.RuntimeCatalogTargetKind, ID: tobari.RuntimeCatalogTargetID, Description: "This installation's host-owned reusable Runtime catalog.", Scope: FixedTargetScopeToolLocal}
}

func fixedContextRuntimeBindingTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.ContextRuntimeBindingTargetKind, ID: tobari.ContextRuntimeBindingTargetID, Description: "One explicit or current Context's exact Runtime revision binding.", Scope: FixedTargetScopeToolLocal}
}

func fixedActiveContextTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID,
		Description: "This installation's host-owned current Context omission default.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextShellTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID,
		Description: "This installation's Context-owned allowlisted shell environment configuration.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextGitIdentityTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID,
		Description: "This installation's Context-owned narrow Git identity configuration.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextBootstrapTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.ContextBootstrapTargetKind, ID: tobari.ContextBootstrapTargetID, Description: "This installation's Context-owned secret-free create-only Workspace bootstrap recipe.", Scope: FixedTargetScopeToolLocal}
}

func fixedActiveContextRuntimeTarget() *FixedTarget {
	return &FixedTarget{
		Kind:        tobari.ContextRuntimeTargetKind,
		ID:          tobari.ActiveContextRuntimeID,
		Description: "The current Context's host-owned runtime recipe and selected image.",
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

func contextCreateNameInput() CommandInput {
	input := contextNameInput()
	input.Required = false
	input.Description = "Portable Context name; omission together with every other input opens the terminal wizard. Any explicit input selects direct mode and requires --name."
	return input
}

func contextCreateRuntimeInput() CommandInput {
	minimum := int64(1)
	return CommandInput{Name: "--runtime", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Ready Runtime revision as standard or name@ordinal; omission defaults to standard.", AllowedValues: []string{}, DefaultValue: stringPointer(tobari.StandardRuntimeName)}
}

func contextCreateAWSBootstrapInput() CommandInput {
	minimum := int64(1)
	return CommandInput{Name: "--bootstrap-aws-profile", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Host AWS IAM Identity Center profile normalized into a secret-free create-only snapshot; omission imports nothing.", AllowedValues: []string{}}
}

func contextCreateEKSBootstrapInput() CommandInput {
	minimum := int64(1)
	return CommandInput{Name: "--bootstrap-eks-context", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Host AWS CLI-generated EKS context composed with --bootstrap-aws-profile for create-only projection.", AllowedValues: []string{}, Requires: []string{"--bootstrap-aws-profile"}}
}

func executionContextInput() CommandInput {
	return CommandInput{
		Name: "--context", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Context display name for this invocation; omission uses the current Context without changing it.", AllowedValues: []string{},
	}
}

func lifecycleContextInput() CommandInput {
	input := executionContextInput()
	minimumLength := int64(1)
	input.MinimumLength = &minimumLength
	input.Description = "Non-empty Context display name for this invocation; both `tobari --context toolbox status` and `tobari status --context toolbox` are accepted, omission uses the current Context without changing it, and duplicate placement is rejected."
	return input
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

func contextSourceAccessInput() CommandInput {
	return CommandInput{
		Name: "--source-access", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Write authority for the one direct project source bind; Workspace home and tmpfs remain writable.",
		AllowedValues: []string{"read-only", "read-write"}, DefaultValue: stringPointer("read-write"),
	}
}

func contextNativeReadinessInput() CommandInput {
	return CommandInput{Name: "--native-readiness", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Trusted binary native-client readiness overlay; the Context system policy ceiling remains terminal.", AllowedValues: []string{"enabled", "disabled"}, DefaultValue: stringPointer("enabled")}
}

func contextReportOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared Context task identity for this report."},
			{Name: "context_state", Type: OutputFieldTypeString, Description: "Persisted authority or a display-only synthetic default.", Enum: []string{"persisted", "synthetic_default"}},
			{Name: "id", Type: OutputFieldTypeString, Description: "Stable host-issued Context identity, or null before authority is persisted.", Nullable: true},
			{Name: "name", Type: OutputFieldTypeString, Description: "Named Context identifier."},
			{Name: "active", Type: OutputFieldTypeBoolean, Description: "Whether this Context is the current/default selection for omitted Context input."},
			{Name: "agent_profile", Type: OutputFieldTypeString, Description: "Read-only shared agent profile reference."},
			{Name: "image", Type: OutputFieldTypeString, Description: "Default compatible Tobari image selector stored in the Context."},
			{Name: "policy_mode", Type: OutputFieldTypeString, Description: "Guided or advanced policy-development mode.", Enum: []string{"guided", "advanced"}},
			{Name: "source_access", Type: OutputFieldTypeString, Description: "Direct project-source bind access; this does not describe Workspace home or tmpfs.", Enum: []string{"read-only", "read-write"}},
			{Name: "policy_revision", Type: OutputFieldTypeString, Description: "SHA-256 revision of the immutable Context-owned normalized policy snapshot; empty only for a synthetic default."},
			{Name: "native_readiness", Type: OutputFieldTypeString, Description: "Immutable native-client readiness capability selection; the system policy ceiling still bounds its effects.", Enum: []string{"enabled", "disabled"}},
			{Name: "method_policy", Type: OutputFieldTypeObject, Description: "Effective default and exact HTTP method decisions owned by the Context.", Fields: contextPolicyMethodPolicyOutput("Effective default and exact HTTP method decisions owned by the Context.").Fields},
			{Name: "shell_environment", Type: OutputFieldTypeArray, Description: "Complete allowlisted shell variable inventory with default, inherited, or literal source and an exact value only for literal.", SemanticScope: "The fixed four-variable Context shell presentation inventory.", Items: &OutputField{
				Type: OutputFieldTypeObject, Description: "One allowlisted shell variable policy.", Fields: []OutputField{
					{Name: "variable", Type: OutputFieldTypeString, Description: "Allowlisted variable name.", Enum: []string{"COLORTERM", "NO_COLOR", "PS1", "TERM"}},
					{Name: "source", Type: OutputFieldTypeString, Description: "Value source.", Enum: []string{"default", "inherit", "literal"}},
					{Name: "value", Type: OutputFieldTypeString, Description: "Exact literal value, including explicit empty, only for literal source.", Optional: true},
				},
			}},
			{Name: "git_identity", Type: OutputFieldTypeObject, Description: "Atomic Git identity policy with default, inherited, or literal source and exact name/email only for literal.", Fields: []OutputField{
				{Name: "source", Type: OutputFieldTypeString, Description: "Identity source.", Enum: []string{"default", "inherit", "literal"}},
				{Name: "name", Type: OutputFieldTypeString, Description: "Literal Git user name, or null.", Nullable: true},
				{Name: "email", Type: OutputFieldTypeString, Description: "Literal Git user email, or null.", Nullable: true},
			}},
			contextBootstrapOutputField(),
			{Name: "stores", Type: OutputFieldTypeObject, Description: "Resolved paths, or null for a synthetic default; secret values are never included.", Nullable: true, Fields: []OutputField{
				{Name: "policy_directory", Type: OutputFieldTypeString, Description: "Canonical Context policy directory."},
			}},
			{Name: "runtime", Type: OutputFieldTypeObject, Description: "Exact built-in or managed Runtime revision binding.", Fields: []OutputField{
				{Name: "kind", Type: OutputFieldTypeString, Description: "Built-in or managed Runtime source kind.", Enum: []string{"official", "managed"}},
				{Name: "status", Type: OutputFieldTypeString, Description: "Selected Runtime readiness.", Enum: []string{"official", "ready"}},
				{Name: "image", Type: OutputFieldTypeString, Description: "Execution image material selected by this binding."},
				{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority identity."},
				{Name: "name", Type: OutputFieldTypeString, Description: "Runtime name."},
				{Name: "revision", Type: OutputFieldTypeString, Description: "Exact semantic Runtime revision."},
				{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Human Runtime revision ordinal."},
			}},
			{Name: "cluster", Type: OutputFieldTypeString, Description: "How this task relates to cluster activation.", Enum: []string{"not_applicable", "not_configured", "not_running", "already_ready", "reconciled", "default_updated", "requires_reconcile"}},
			{Name: "authentication", Type: OutputFieldTypeObject, Description: "Workspace-native authentication mode or experimental Broker status without credential values.", Fields: []OutputField{
				{Name: "mode", Type: OutputFieldTypeString, Description: "Authentication ownership mode compiled into this executable.", Enum: []string{"native_workspace", "broker", "not_applicable"}},
				{Name: "broker_state", Type: OutputFieldTypeString, Description: "Experimental Auth Broker observation.", Enum: []string{"not_applicable", "ready", "locked", "unavailable"}, Optional: true},
				{Name: "declared_bindings", Type: OutputFieldTypeString, Description: "Experimental authentication route for installed declared provider bindings.", Enum: []string{"broker_required"}, Optional: true},
				{Name: "undeclared_bindings", Type: OutputFieldTypeString, Description: "Experimental route for bindings absent from the provider projection.", Enum: []string{"workspace_owned_compatibility"}, Optional: true},
				{Name: "providers", Type: OutputFieldTypeArray, Description: "Installed provider states, or null when this mutation did not observe authentication.", Nullable: true, SemanticScope: "Every installed provider for the selected Context when authentication was observed.", Items: &OutputField{
					Type: OutputFieldTypeObject, Description: "One installed provider observation.", Fields: []OutputField{
						{Name: "provider", Type: OutputFieldTypeString, Description: "Installed provider ID."},
						{Name: "state", Type: OutputFieldTypeString, Description: "Provider credential state.", Enum: []string{"configured", "not_configured", "unavailable"}},
						{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free account label, or null.", Nullable: true},
						{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Secret-free credential revision, or null.", Nullable: true},
					},
				}},
			}},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "context", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
	}
}

func contextBootstrapOutputField() OutputField {
	return OutputField{Name: "bootstrap", Type: OutputFieldTypeObject, Description: "Secret-free create-only recipe for future Workspace homes.", Fields: []OutputField{
		{Name: "state", Type: OutputFieldTypeString, Description: "Whether a future-Workspace recipe is configured.", Enum: []string{"not_configured", "configured"}},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Monotonic semantic-change generation; zero when unconfigured."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Semantic SHA-256 revision; empty when unconfigured."},
		{Name: "adapters", Type: OutputFieldTypeArray, Description: "Closed adapter inventory in dependency order.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One reviewed adapter ID."}},
		{Name: "aws_profile", Type: OutputFieldTypeString, Description: "Host profile name selected as the snapshot source; empty when unconfigured."},
		{Name: "kubernetes_eks_context", Type: OutputFieldTypeString, Description: "Selected host EKS context name; empty when the EKS adapter is absent."},
	}}
}

func projectEnterErrors() []CommandError {
	errors := mutationCommandErrors("tobari", "status")
	filtered := errors[:0]
	for _, declared := range errors {
		if declared.Code != "mutation_output_write_failed" {
			filtered = append(filtered, declared)
		}
	}
	result := append(filtered,
		declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context."),
		declaredCommandError(fault.KindContract, "invalid_context_binding", false, "context list", "Inspect the Context catalog before selecting a Workspace."),
		declaredCommandError(fault.KindContract, "context_binding_stale", false, "doctor", "Inspect Context and Workspace state."),
		declaredCommandError(fault.KindInvalidInput, "tty_required", false, "help tobari", "Run the root command from an interactive terminal."),
		declaredCommandError(fault.KindRejected, "already_inside", false, "help tobari", "Exit the current Tobari before entering another session."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_configured", false, "cluster up", "Create the shared cluster explicitly before entering a Tobari."),
		declaredCommandError(fault.KindUnavailable, "cluster_status_failed", false, "cluster status", "Inspect the shared cluster before entering a Tobari."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_ready", false, "cluster up", "Reconcile the shared cluster explicitly before entering a Tobari."),
		declaredCommandError(fault.KindRejected, "cluster_projection_stale", false, "cluster up", "Load the complete Context catalog into the shared cluster before entering a Tobari."),
		declaredCommandError(fault.KindRejected, "runtime_build_required", false, "runtime build", "Build and select the staged custom runtime before entering a Workspace."),
		declaredCommandError(fault.KindRejected, "runtime_recipe_invalid", false, "context show", "Inspect and correct the invalid custom runtime recipe before entry."),
		declaredCommandError(fault.KindInternal, "runtime_choice_failed", false, "tobari", "Resume from the persisted Context and ready cluster."),
		declaredCommandError(fault.KindRejected, "project_state_incomplete", false, "delete", "Review the exact delete command and confirm removal of the incomplete current-directory Tobari."),
		declaredCommandError(fault.KindInternal, "missing_workspace_selector", false, "doctor", "Configure the Tobari terminal selector."),
		declaredCommandError(fault.KindContract, "invalid_workspace_selection", false, "doctor", "Inspect local Workspace state."),
		declaredCommandError(fault.KindContract, "workspace_selection_invalid", false, "tobari", "Choose a current Workspace or explicitly create one again."),
		declaredCommandError(fault.KindRejected, "workspace_selection_stale", true, "tobari", "Refresh the Workspace choices and select again."),
		declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
		declaredCommandError(fault.KindUnavailable, "image_not_found", false, "runtime build", "Build or make the selected compatible runtime image available to Docker."),
		declaredCommandError(fault.KindUnavailable, "git_identity_resolution_failed", false, "context show", "Inspect the selected Context Git identity without changing Workspace state."),
		declaredCommandError(fault.KindUnavailable, "runtime_reconcile_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindUnavailable, "network_guard_failed", false, "doctor", "Inspect Docker Engine network-namespace and nftables support."),
		declaredCommandError(fault.KindInternal, "enter_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "cluster up", "Start or reconcile the shared cluster before projecting Context authentication."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "cluster up", "Reconcile the shared cluster before another Auth Broker request."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_locked", false, "cluster up", "Reconcile the shared cluster and unlock the Auth Broker."),
		declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect the Context vault integrity without printing its contents."),
		declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported Context vault."),
		declaredCommandError(fault.KindRejected, "invalid_provider_manifest", false, "doctor", "Repair the owner-controlled provider manifest collection."),
		declaredCommandError(fault.KindRejected, "ambiguous_provider_http_binding", false, "doctor", "Remove the overlapping exact provider HTTP binding."),
		declaredCommandError(fault.KindContract, "invalid_auth_handle_result", false, "doctor", "Inspect Broker and provider projection consistency."),
		declaredCommandError(fault.KindRejected, "auth_projection_file_exists", false, "doctor", "Inspect the provider file path and preserve the non-Tobari-owned Workspace file."),
		declaredCommandError(fault.KindRejected, "auth_projection_file_modified", false, "doctor", "Inspect the modified Tobari-owned Workspace authentication file."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	)
	if !buildIdentityHasBroker() {
		return withoutBrokerErrors(result)
	}
	return result
}

func withoutBrokerErrors(errors []CommandError) []CommandError {
	brokerErrors := map[string]struct{}{
		"auth_broker_image_unavailable": {}, "auth_broker_image_incompatible": {},
		"credential_companion_unavailable": {}, "auth_broker_unavailable": {},
		"auth_broker_request_failed": {}, "auth_broker_unlock_failed": {}, "auth_broker_locked": {},
		"root_key_unavailable": {}, "root_key_missing_with_vault": {}, "root_key_unsafe": {},
		"keychain_denied": {}, "auth_vault_invalid": {}, "auth_vault_version_unsupported": {},
		"invalid_provider_manifest": {}, "ambiguous_provider_http_binding": {},
		"invalid_auth_handle_result": {}, "auth_projection_file_exists": {},
		"auth_projection_file_modified": {},
	}
	filtered := errors[:0]
	for _, declared := range errors {
		if _, remove := brokerErrors[declared.Code]; !remove {
			filtered = append(filtered, declared)
		}
	}
	return filtered
}

func standardClusterErrors(errors []CommandError) []CommandError {
	errors = withoutBrokerErrors(errors)
	for index := range errors {
		if errors[index].Code != "cluster_reconcile_interrupted" {
			continue
		}
		for actionIndex := range errors[index].NextActions {
			if errors[index].NextActions[actionIndex].Command == "cluster up" {
				errors[index].NextActions[actionIndex].Reason = "Reconcile the shared Gateway and OPA cluster."
			}
		}
	}
	return errors
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
		{Name: "decision", Type: OutputFieldTypeString, Description: "Current learned decision: allow or deny.", Enum: []string{"allow", "deny"}},
		{Name: "match", Type: OutputFieldTypeString, Description: "Exact match mode.", Enum: []string{"exact"}},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority bound to the decision."},
		{Name: "context", Type: OutputFieldTypeString, Description: "Human-readable Context name."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to the decision."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe diagnostic canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact decision scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact decision host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact decision port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact path."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: []string{"http", "graphql", "mcp"}},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL query or mutation type; empty for HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact canonical GraphQL root field; empty for HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "examples", Type: OutputFieldTypeArray, Description: "Positive request paths retained by an Allow rule; empty for Deny.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One exact positive request path."}},
		{Name: "source_candidates", Type: OutputFieldTypeArray, Description: "Opaque denial candidates that support this decision.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One unchanged policy-candidate reference."}},
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
		{Name: "observation_count", Type: OutputFieldTypeInteger, Description: "Matching retained denial observations."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority established by Gateway network identity."},
		{Name: "context", Type: OutputFieldTypeString, Description: "Human-readable Context name."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal for the denied request."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe diagnostic canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact denied request scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact denied request host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact denied request port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact denied uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact denied HTTP path without query data."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: []string{"http", "graphql", "mcp"}},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL query or mutation type; empty for HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact canonical GraphQL root field; empty for HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "reason", Type: OutputFieldTypeString, Description: "Bounded secret-free denial reason."},
		{Name: "status_code", Type: OutputFieldTypeInteger, Description: "Gateway denial status."},
		{Name: "destination_kind", Type: OutputFieldTypeString, Description: "External or attachment Host Loopback destination.", Enum: []string{"external", "host_loopback"}},
		{Name: "authority_lifetime", Type: OutputFieldTypeString, Description: "Persistent or attachment-scoped authority lifetime.", Enum: []string{"persistent", "attachment"}},
		{Name: "attachment_epoch_id", Type: OutputFieldTypeString, Description: "Active attachment epoch for Host Loopback, otherwise empty."},
		{Name: "allow_command", Type: OutputFieldTypeString, Description: "Exact reference-bound approval command for a persistent external candidate; empty for Host Loopback, which requires interactive review."},
		{Name: "deny_command", Type: OutputFieldTypeString, Description: "Exact reference-bound rejection command for a persistent external candidate; empty for Host Loopback, which requires interactive review."},
	}
}

func policyDenialOutputFields() []OutputField {
	return []OutputField{
		{Name: "timestamp", Type: OutputFieldTypeString, Description: "Validated denial timestamp."},
		{Name: "request_id", Type: OutputFieldTypeString, Description: "Secret-free request identity."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority established by Gateway network identity."},
		{Name: "context", Type: OutputFieldTypeString, Description: "Human-readable Context name."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact denied scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact denied host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact denied port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact denied uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact redacted denial path without query data."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: []string{"http", "graphql", "mcp"}},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL operation type, or an empty string for ordinary HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact GraphQL root field, or an empty string for ordinary HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "reason", Type: OutputFieldTypeString, Description: "Bounded secret-free denial reason."},
		{Name: "status_code", Type: OutputFieldTypeInteger, Description: "Gateway denial status."},
		{Name: "learnable", Type: OutputFieldTypeBoolean, Description: "Whether one exact learned rule can close this denial."},
		{Name: "destination_kind", Type: OutputFieldTypeString, Description: "External or attachment Host Loopback destination.", Enum: []string{"external", "host_loopback"}},
		{Name: "authority_lifetime", Type: OutputFieldTypeString, Description: "Persistent or attachment-scoped authority lifetime.", Enum: []string{"persistent", "attachment"}},
		{Name: "attachment_epoch_id", Type: OutputFieldTypeString, Description: "Active attachment epoch for Host Loopback, otherwise empty."},
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
			{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective stored policy protocol: http, graphql, or mcp."},
			{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Stored GraphQL query or mutation type; empty for HTTP."},
			{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Stored canonical GraphQL root field; empty for HTTP."},
			{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Stored exact MCP JSON-RPC method; empty outside MCP."},
			{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Stored exact MCP tool name for tools/call; empty otherwise."},
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
			{Name: "match", Type: OutputFieldTypeString, Description: "Stored exact match mode.", Enum: []string{"exact"}},
			{Name: "project_id", Type: OutputFieldTypeString, Description: "Host-issued project principal bound to the stored rule."},
			{Name: "host", Type: OutputFieldTypeString, Description: "Stored exact host."},
			{Name: "port", Type: OutputFieldTypeInteger, Description: "Stored exact request port."},
			{Name: "method", Type: OutputFieldTypeString, Description: "Stored exact uppercase HTTP method."},
			{Name: "path", Type: OutputFieldTypeString, Description: "Stored exact path."},
			{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective stored policy protocol: http, graphql, or mcp."},
			{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Stored GraphQL query or mutation type; empty for HTTP."},
			{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Stored canonical GraphQL root field; empty for HTTP."},
			{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Stored exact MCP JSON-RPC method; empty outside MCP."},
			{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Stored exact MCP tool name for tools/call; empty otherwise."},
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

func policyReviewWatchInput() CommandInput {
	return CommandInput{
		Name: "--watch", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
		Description:   "Keep an interactive raw-terminal Permission Inbox open and refresh bounded denial snapshots automatically.",
		AllowedValues: []string{}, DefaultValue: stringPointer("false"),
	}
}

func policyReviewNotifyInput() CommandInput {
	return CommandInput{
		Name: "--notify", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Choose the terminal-emulator attention cue for newly arrived review items while watching.",
		AllowedValues: []string{"auto", "osc9", "bel", "off"}, DefaultValue: stringPointer("auto"),
		Requires: []string{"--watch"},
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
	fields := []OutputField{
		{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether cluster state remains configured."},
		{Name: "running", Type: OutputFieldTypeBoolean, Description: "Whether shared components are running."},
		{Name: "context_count", Type: OutputFieldTypeInteger, Description: "Number of Context policies in the shared enforcement projection."},
		{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Content-addressed aggregate policy revision."},
		{Name: "policy_projection", Type: OutputFieldTypeString, Description: "All-Context policy projection integrity observation."},
		{Name: "principal_registry", Type: OutputFieldTypeString, Description: "Principal registry integrity observation."},
		{Name: "gateway_projection", Type: OutputFieldTypeString, Description: "Gateway routing projection integrity observation."},
	}
	if buildIdentityHasBroker() {
		fields = append(fields,
			OutputField{Name: "auth_provider_projection", Type: OutputFieldTypeString, Description: "Experimental Auth Broker provider projection integrity observation."},
			OutputField{Name: "auth_broker_state", Type: OutputFieldTypeString, Description: "Observed experimental Auth Broker state."},
			OutputField{Name: "credential_companion_state", Type: OutputFieldTypeString, Description: "Observed experimental trusted-host credential companion state."},
			OutputField{Name: "root_key_backend", Type: OutputFieldTypeString, Description: "Selected experimental host root-key backend."},
		)
	}
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields:   fields,
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
		declaredCommandError(fault.KindRejected, "context_mismatch", false, "cluster up", "Reconcile the shared cluster's all-Context projection."),
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
