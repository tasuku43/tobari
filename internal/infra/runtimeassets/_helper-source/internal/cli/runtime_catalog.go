package cli

import (
	"fmt"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runtimeCommandSpecs() []CommandSpec {
	specs := []CommandSpec{
		finalClusterUpSpec(),
		finalClusterStatusSpec(),
	}
	specs = append(specs, researchRuntimeCommandSpecs()...)
	specs = append(specs,
		finalClusterDenialsSpec(),
		finalClusterLogsSpec(),
		finalClusterDownSpec(),
		finalPolicyCandidatesSpec(),
		finalPolicyReviewSpec(),
		finalPolicyApplyReviewedSpec(),
		finalPolicyRulesSpec(),
		finalPolicyAllowSpec(),
		finalPolicyDenySpec(),
		finalPolicyResetSpec(),
		finalTemplateListSpec(),
		finalTemplateShowSpec(),
		finalTemplateCreateSpec(),
		finalTemplateCopySpec(),
		finalTemplateDefaultSetSpec(),
		finalTemplateDeleteSpec(),
		finalConfigShellSpec(),
		finalConfigGitSpec(),
		finalConfigBootstrapAWSSpec(),
		finalConfigBootstrapEKSSpec(),
		finalTemplateRuntimeSetSpec(),
		finalContextListSpec(),
		finalContextShowSpec(),
		finalContextCreateSpec(),
		finalContextEnterSpec(),
		finalContextDeleteSpec(),
		finalWorkspaceListSpec(),
		finalWorkspaceStatusSpec(),
		finalWorkspaceDeleteSpec(),
		runtimeListSpec(),
		runtimeShowSpec(),
		runtimeCreateSpec(),
		runtimeHistorySpec(),
		runtimeReviewSpec(),
		runtimeBuildSpec(),
		runtimeRestoreSpec(),
		runtimeDeleteSpec(),
		runtimePruneDryRunSpec(),
		runtimePruneApplySpec(),
		finalDefaultPairEnterSpec(),
		finalDefaultPairStatusSpec(),
	)
	return append(specs, authCommandSpecs()...)
}

func configBootstrapAWSSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{
		Path: "config bootstrap aws", Summary: "Configure, refresh, or remove the AWS snapshot applied once to future Workspace homes",
		Args:   "[--profile <name>] [--refresh] [--remove] [--manifest <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "manifest.workspace-bootstrap",
			Outcome:      "Normalize one host AWS IAM Identity Center profile into a secret-free Workspace Manifest snapshot for future Workspaces, refresh its semantic revision, or remove the future recipe without rewriting existing Workspace homes",
			Inputs: []CommandInput{
				{Name: "--profile", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact host AWS shared-config profile to normalize now; conflicts with refresh and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
				{Name: "--refresh", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Re-read the profile named by the selected Workspace Manifest snapshot; conflicts with profile and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--remove"}},
				{Name: "--remove", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove the recipe for future Workspaces; existing Workspace homes retain their create-time bytes.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--refresh"}},
				executionContextInput(), formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Workspace Manifest exists and the fixed host ~/.aws/config path is a bounded regular non-symlink file not writable by group or other users.",
				"The selected profile uses one sso_session section and only the reviewed secret-free IAM Identity Center fields; credentials and ~/.aws/sso/cache are never read.",
				"When action flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text.",
			},
			FixedTarget: fixedContextBootstrapTarget(),
			Errors: mutationCommandErrors("config bootstrap aws", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config bootstrap aws", "Supply one action flag or run the wizard on interactive text streams."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config bootstrap aws", "Retry with one direct action flag or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_aws_bootstrap_change", false, "help config bootstrap aws", "Choose exactly one configure, refresh, or remove action."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindNotFound, "bootstrap_not_configured", false, "help config bootstrap aws", "Configure a profile before refreshing."),
				declaredCommandError(fault.KindRejected, "config_bootstrap_failed", false, "manifest show", "Inspect the current recipe and strict host AWS profile."),
				declaredCommandError(fault.KindRejected, "bootstrap_source_changed", true, "config bootstrap aws", "Review a fresh semantic diff before applying."),
				declaredCommandError(fault.KindRejected, "bootstrap_dependency", false, "config bootstrap kubernetes eks", "Remove the dependent EKS adapter first with --remove."),
				declaredCommandError(fault.KindRejected, "aws_bootstrap_source_rejected", false, "help config bootstrap aws", "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."),
				declaredCommandError(fault.KindContract, "invalid_bootstrap_preview", false, "manifest show", "Inspect the Workspace Manifest recipe before retrying."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the confirmed Workspace Manifest bootstrap change."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{TargetKind: tobari.ManifestBootstrapTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
		},
		handler: runConfigBootstrapAWS,
	}
}

func configBootstrapEKSSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{
		Path: "config bootstrap kubernetes eks", Summary: "Configure, refresh, or remove one reviewed EKS target for future Workspace homes",
		Args:   "[--kube-context <name>] [--refresh] [--remove] [--manifest <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "manifest.workspace-bootstrap",
			Outcome:      "Compose one host AWS CLI-generated EKS context with the Workspace Manifest AWS profile, refresh it, or remove only the EKS target without rewriting existing Workspace homes",
			Inputs: []CommandInput{
				{Name: "--kube-context", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact context name in fixed host ~/.kube/config; conflicts with refresh and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
				{Name: "--refresh", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Re-read the currently selected host kube context; conflicts with context selection and remove.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--remove"}},
				{Name: "--remove", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove only the EKS adapter for future Workspaces; preserve AWS and existing Workspace homes.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--refresh"}},
				executionContextInput(), formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Workspace Manifest already has an AWS IAM Identity Center bootstrap profile.",
				"Fixed host ~/.kube/config is a bounded safe regular file and the selected context resolves to an inline-CA commercial EKS endpoint with the reviewed aws eks get-token exec contract and matching AWS_PROFILE.",
				"No host credential, token cache, arbitrary exec, alternate kubeconfig path, or network authority is imported.",
			},
			FixedTarget: fixedContextBootstrapTarget(),
			Errors: mutationCommandErrors("config bootstrap kubernetes eks", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config bootstrap kubernetes eks", "Supply one action flag or use interactive text streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_eks_bootstrap_change", false, "help config bootstrap kubernetes eks", "Choose exactly one configure, refresh, or remove action."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindNotFound, "bootstrap_not_configured", false, "help config bootstrap kubernetes eks", "Configure AWS first, or select EKS before refresh/remove."),
				declaredCommandError(fault.KindRejected, "eks_bootstrap_source_rejected", false, "help config bootstrap kubernetes eks", "Use a strict AWS CLI-generated EKS context bound to the Workspace Manifest AWS profile."),
				declaredCommandError(fault.KindRejected, "config_bootstrap_failed", false, "manifest show", "Inspect the current recipe and selected kube context."),
				declaredCommandError(fault.KindRejected, "bootstrap_source_changed", true, "config bootstrap kubernetes eks", "Review a fresh semantic diff before applying."),
				declaredCommandError(fault.KindContract, "invalid_bootstrap_preview", false, "manifest show", "Inspect the Workspace Manifest recipe before retrying."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the confirmed Workspace Manifest bootstrap change."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{TargetKind: tobari.ManifestBootstrapTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
		},
		handler: runConfigBootstrapEKS,
	}
}

func configShellSpec() CommandSpec {
	return CommandSpec{
		Path: "config shell", Summary: "Configure Workspace Manifest shell session defaults directly or with one staged terminal Apply",
		Args:   "[--variable COLORTERM|NO_COLOR|PS1|TERM] [--source default|inherit|literal] [--value <value>] [--manifest <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "manifest.composition",
			Outcome:      "Configure one allowlisted shell session default through complete flags, or stage several rows and apply them atomically; later child sessions resolve it without rewriting Workspace home",
			Inputs: []CommandInput{
				{
					Name: "--variable", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Allowlisted shell environment variable configured for future interactive sessions.",
					AllowedValues: tobari.ManifestShellEnvironmentVariables(),
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
					Description:   "Exact Workspace Manifest-owned value of at most 4096 UTF-8 bytes; required only for literal and may be explicitly empty.",
					AllowedValues: []string{}, Requires: []string{"--variable", "--source"},
				},
				executionContextInput(),
				formatInput(),
			},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The selected Workspace Manifest exists on the trusted host; inherited values must be exported by the process that starts Tobari.", "When setting flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text."},
			FixedTarget:   fixedContextShellTarget(),
			Errors: mutationCommandErrors("config shell", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config shell", "Supply every setting flag or run the wizard with text success/error output on interactive stdin and stderr."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config shell", "Retry with complete setting flags or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_shell_environment", false, "help config shell", "Choose an allowlisted variable and a valid source/value combination."),
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "doctor", "Inspect the host Workspace Manifest stores before retrying the wizard."),
				declaredCommandError(fault.KindRejected, "config_shell_failed", false, "manifest show", "Inspect the Workspace Manifest shell environment before retrying."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the confirmed Workspace Manifest shell setting."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ManifestShellTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runConfigShell,
	}
}

func configGitSpec() CommandSpec {
	return CommandSpec{
		Path: "config git", Summary: "Configure one Workspace Manifest Git session fallback directly or from one staged terminal screen",
		Args:   "[--source default|inherit|literal] [--name <name>] [--email <email>] [--manifest <name>] [--format text|json]",
		Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "manifest.composition",
			Outcome:      "Choose no Git session fallback, inherited host user.name and user.email, or one fixed Workspace Manifest-owned identity; later Workspace entry resolves it without rewriting Workspace home",
			Inputs: []CommandInput{
				{
					Name: "--source", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "default removes the Workspace Manifest fallback; inherit projects host user.name and user.email at Workspace entry; literal uses --name and --email. Omit all setting flags to use the staged terminal editor.",
					AllowedValues: []string{"default", "inherit", "literal"},
				},
				{
					Name: "--name", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Exact non-empty Workspace Manifest-owned Git user.name of at most 4096 safe UTF-8 bytes; required with --email only for literal.",
					AllowedValues: []string{}, Requires: []string{"--source", "--email"},
				},
				{
					Name: "--email", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "Exact non-empty Workspace Manifest-owned Git user.email of at most 4096 safe UTF-8 bytes; required with --name only for literal.",
					AllowedValues: []string{}, Requires: []string{"--source", "--name"},
				},
				executionContextInput(),
				formatInput(),
			},
			Output: contextReportOutput(),
			Prerequisites: []string{
				"The selected Workspace Manifest exists on the trusted host; inherited identity is resolved from only host global Git configuration at Workspace entry.",
				"When setting flags are omitted, stdin and stderr are interactive terminals and both success and error formats are text.",
			},
			FixedTarget: fixedContextGitIdentityTarget(),
			Errors: mutationCommandErrors("config git", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "configuration_wizard_unavailable", false, "help config git", "Supply every setting flag or run the wizard with text success/error output on interactive stdin and stderr."),
				declaredCommandError(fault.KindInternal, "configuration_wizard_failed", false, "help config git", "Retry with complete setting flags or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_git_identity", false, "help config git", "Choose default, inherit, or a literal source with both name and email."),
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "doctor", "Inspect the host Workspace Manifest stores before retrying the wizard."),
				declaredCommandError(fault.KindRejected, "config_git_failed", false, "manifest show", "Inspect the Workspace Manifest Git identity before retrying."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the confirmed Workspace Manifest Git identity setting."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ManifestGitIdentityTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runConfigGit,
	}
}

func contextListSpec() CommandSpec {
	return CommandSpec{
		Path: "manifest list", Summary: "List Workspace Manifest definitions with effective Access and exact Runtime",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "manifest.composition",
			Outcome:      "List the complete local Workspace Manifest collection and identify the omission default",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "workspace_manifest_state", Type: OutputFieldTypeString, Description: "Whether persisted Workspace Manifest authority exists.", Enum: []string{"persisted", "absent"}},
					{Name: "default_manifest_id", Type: OutputFieldTypeString, Description: "Exact default Workspace Manifest identity, omitted for an empty catalog.", Optional: true},
					{Name: "default_manifest", Type: OutputFieldTypeString, Description: "Default Workspace Manifest name, omitted for an empty catalog.", Optional: true},
					{Name: "items", Type: OutputFieldTypeArray, Description: "Complete local Workspace Manifest collection with default-selection state, stable ID, image, agent profile, policy mode, source access, and runtime status.", SemanticScope: "All locally configured Workspace Manifests at one observation.", Items: &OutputField{
						Type: OutputFieldTypeObject, Description: "One configured Workspace Manifest.", Fields: []OutputField{
							{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable Workspace Manifest authority identity.", Nullable: true},
							{Name: "name", Type: OutputFieldTypeString, Description: "Human Workspace Manifest name."},
							{Name: "workspace_manifest_state", Type: OutputFieldTypeString, Description: "Persisted Workspace Manifest authority.", Enum: []string{"persisted"}},
							{Name: "default", Type: OutputFieldTypeBoolean, Description: "Whether this Workspace Manifest is the omission default."},
							workspaceManifestDesiredOutputField(),
							{Name: "agent_profile", Type: OutputFieldTypeString, Description: "Read-only agent profile reference."},
							{Name: "image", Type: OutputFieldTypeString, Description: "Selected compatible runtime image."},
							{Name: "policy_mode", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary policy-development mode.", Enum: []string{"guided", "advanced"}},
							{Name: "source_access", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary access for the direct project-source bind.", Enum: []string{"read-only", "read-write"}},
							{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Immutable revision of the Workspace Manifest-owned normalized policy snapshot."},
							{Name: "native_readiness", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary choice for native-client readiness participation.", Enum: []string{"enabled", "disabled"}},
							{Name: "method_policy", Type: OutputFieldTypeObject, Description: "Effective default and exact method decisions owned by the Workspace Manifest.", Fields: contextPolicyMethodPolicyOutput("Effective default and exact method decisions owned by the Workspace Manifest.").Fields},
							{Name: "runtime_status", Type: OutputFieldTypeString, Description: "Selected Runtime readiness when observed.", Optional: true, Enum: []string{"official", "ready"}},
							contextBootstrapOutputField(),
						},
					}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "workspace_manifests", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("manifest list", true,
				declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "doctor", "Inspect the host Workspace Manifest stores."),
				declaredCommandError(fault.KindContract, "invalid_manifest_list", false, "doctor", "Repair the Workspace Manifest manifest collection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runContextList,
	}
}

func contextShowSpec() CommandSpec {
	return CommandSpec{
		Path: "manifest show", Summary: "Inspect one Workspace definition's effective Access, tools, and defaults",
		Args: "[--name <name>] [--details] [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "manifest.composition",
			Outcome:      "Inspect one stable Workspace Manifest definition, including its immutable Boundary, exact mutable Runtime binding, session defaults, future-Workspace creation defaults, and separated store references",
			Inputs: []CommandInput{
				{Name: "--name", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Named Workspace Manifest to inspect; omission selects the default Workspace Manifest.", AllowedValues: []string{}, Completion: InputCompletionContextName},
				{Name: "--details", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Expand human text with complete Workspace Manifest diagnostics; JSON is already complete and remains unchanged.", AllowedValues: []string{}, DefaultValue: stringPointer("false")},
				formatInput(),
			},
			Output:        contextReportOutput(),
			Prerequisites: []string{},
			Errors: readCommandErrors("manifest show", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "doctor", "Inspect the host Workspace Manifest stores."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest list", "Repair the Workspace Manifest manifest."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runContextShow,
	}
}

func contextCreateSpec() CommandSpec {
	return CommandSpec{
		Path: "manifest create", Summary: "Create a named Workspace Manifest definition directly or by completing omitted settings",
		Args:   "[--copy-from <manifest-name>] [--name <name>] [--runtime <standard|name@ordinal>] [--mode guided|advanced] [--source-access read-only|read-write] [--native-readiness enabled|disabled] [--bootstrap-aws-profile <name>] [--bootstrap-eks-context <name>] [--format text|json]",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "manifest.composition",
			Outcome:       "Create one stable named Workspace definition with an immutable source/network Boundary, exact Runtime binding, narrow Workspace defaults, and separate owner-only policy and authentication state",
			Inputs:        []CommandInput{contextCreateBaseInput(), contextCreateNameInput(), contextCreateRuntimeInput(), contextModeInput(), contextSourceAccessInput(), contextNativeReadinessInput(), contextCreateAWSBootstrapInput(), contextCreateEKSBootstrapInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The host Workspace Manifest directory is accessible."},
			FixedTarget:   fixedContextCatalogTarget(),
			Errors: mutationCommandErrors("manifest create", "manifest list",
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_copy_source", false, "manifest list", "Choose one existing Workspace Manifest as the copy source."),
				declaredCommandError(fault.KindNotFound, "manifest_copy_source_not_found", false, "manifest list", "Choose one existing Workspace Manifest as the copy source."),
				declaredCommandError(fault.KindRejected, "manifest_copy_source_changed", true, "manifest list", "Review the copy source's current revision before creating."),
				declaredCommandError(fault.KindRejected, "manifest_collection_changed", true, "manifest list", "Inspect the Workspace Manifest collection before retrying Tobari."),
				declaredCommandError(fault.KindInvalidInput, "manifest_create_wizard_unavailable", false, "help manifest create", "Complete omitted settings interactively, supply --copy-from with --name, or supply the complete direct input group."),
				declaredCommandError(fault.KindInternal, "manifest_create_wizard_failed", false, "manifest create", "Retry the wizard or use the complete direct input group."),
				declaredCommandError(fault.KindRejected, "aws_bootstrap_source_rejected", false, "help config bootstrap aws", "Choose a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."),
				declaredCommandError(fault.KindRejected, "eks_bootstrap_source_rejected", false, "help config bootstrap kubernetes eks", "Choose a strict AWS CLI-generated EKS context bound to the selected AWS profile."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context", false, "help manifest create", "Correct the Workspace Manifest name, image, policy mode, or source access."),
				declaredCommandError(fault.KindRejected, "manifest_exists", false, "manifest list", "List existing Workspace Manifests before choosing another name."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_revision_not_ready", false, "review runtimes", "Choose an existing successful revision."),
				declaredCommandError(fault.KindRejected, "manifest_create_failed", false, "manifest list", "Inspect the partially initialized Workspace Manifest stores."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest list", "Reconcile the confirmed Workspace Manifest creation."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ManifestCatalogTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runContextCreate,
	}
}

func manifestDefaultSetSpec() CommandSpec {
	return CommandSpec{
		Path: "manifest default set", Summary: "Select the default Workspace Manifest",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  "manifest.composition",
			Outcome:       "Select one existing Workspace Manifest as the omission default without changing existing Workspaces or shared enforcement authority",
			Inputs:        []CommandInput{contextNameInput(), formatInput()},
			Output:        contextReportOutput(),
			Prerequisites: []string{"The named Workspace Manifest already exists."},
			FixedTarget:   fixedActiveContextTarget(),
			Errors: mutationCommandErrors("manifest default set", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest or create it first."),
				declaredCommandError(fault.KindRejected, "manifest_default_set_failed", false, "manifest show", "Inspect the default Workspace Manifest selection."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the confirmed Workspace Manifest selection."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ManifestTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runManifestDefaultSet,
	}
}

func contextDeleteSpec() CommandSpec {
	return CommandSpec{
		Path: "manifest delete", Summary: "Delete one unused non-default Workspace Manifest",
		Args: "--name <name> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "manifest.composition",
			Outcome:      "Delete one exact non-default Workspace Manifest only when no logical Workspace remains bound, preserving project files and shared runtime images",
			Inputs:       []CommandInput{contextNameInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "id", Type: OutputFieldTypeString, Description: "Stable authority ID of the deleted Workspace Manifest."},
					{Name: "name", Type: OutputFieldTypeString, Description: "Name of the deleted Workspace Manifest."},
					{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Confirmed deletion state."},
					{Name: "cluster", Type: OutputFieldTypeString, Description: "Whether the shared cluster requires reconciliation.", Enum: []string{"not_applicable", "requires_reconcile"}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "workspace_manifest_deletion", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
			},
			Prerequisites: []string{"The named Workspace Manifest is not the default and owns no logical Workspace."},
			FixedTarget:   fixedContextCatalogTarget(),
			Errors: mutationCommandErrors("manifest delete", "manifest list",
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindRejected, "manifest_is_default", false, "manifest default set", "Select another Workspace Manifest first."),
				declaredCommandError(fault.KindRejected, "manifest_is_protected", false, "manifest show", "Keep the foundational default Workspace Manifest."),
				declaredCommandError(fault.KindRejected, "manifest_has_workspaces", false, "list", "Delete every Workspace bound to the Workspace Manifest first."),
				declaredCommandError(fault.KindRejected, "manifest_delete_failed", false, "manifest list", "Inspect the Workspace Manifest collection before retrying."),
				declaredCommandError(fault.KindContract, "invalid_manifest_delete_result", false, "manifest list", "Reconcile the Workspace Manifest collection after deletion."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ManifestCatalogTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes},
			},
		},
		handler: runContextDelete,
	}
}

func runtimeListSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime list", Summary: "List reusable Runtimes and their ready head revisions",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID:  "runtime.lifecycle",
			Outcome:       "List the complete installation-wide Runtime catalog and each ready head revision",
			Inputs:        []CommandInput{formatInput()},
			Output:        runtimeListOutput(),
			Prerequisites: []string{},
			Errors: readCommandErrors("runtime list", true,
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating Runtime-list JSON encoding."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runRuntimeList,
	}
}

func contextRuntimeSetSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{Path: "manifest runtime set", Summary: "Replace one Workspace Manifest Runtime binding with a ready revision", Args: "[--runtime <standard|name@ordinal>] [--manifest <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{CapabilityID: "manifest.composition", Outcome: "Explicitly replace one Workspace Manifest Runtime binding; bound Workspaces adopt it on next entry without changing Workspace Manifest identity or existing Workspace homes",
			Inputs: []CommandInput{{Name: "--runtime", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Exact ready Runtime selection as standard or name@ordinal; omission opens terminal Review in text mode.", AllowedValues: []string{}, Completion: InputCompletionReadyRuntimeReference}, executionContextInput(), formatInput()},
			Output: contextReportOutput(), Prerequisites: []string{"The selected Runtime revision already exists and is ready."}, FixedTarget: fixedContextRuntimeBindingTarget(),
			Errors: mutationCommandErrors("manifest runtime set", "manifest show",
				declaredCommandError(fault.KindInvalidInput, "runtime_review_unavailable", false, "help manifest runtime set", "Supply --runtime or use interactive text streams."),
				declaredCommandError(fault.KindInternal, "runtime_review_failed", false, "help manifest runtime set", "Retry with --runtime or repair the interactive terminal streams."),
				declaredCommandError(fault.KindInvalidInput, "invalid_manifest_name", false, "manifest list", "Choose a valid Workspace Manifest name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_selection", false, "review runtimes", "Choose standard or one ready name@ordinal revision."),
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "doctor", "Inspect the host Workspace Manifest stores."),
				declaredCommandError(fault.KindContract, "invalid_manifest_list", false, "doctor", "Inspect the host Workspace Manifest stores."),
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindRejected, "runtime_revision_not_ready", false, "review runtimes", "Choose an existing successful revision."),
				declaredCommandError(fault.KindRejected, "manifest_runtime_set_failed", false, "manifest show", "Inspect the unchanged Workspace Manifest Runtime binding."),
				declaredCommandError(fault.KindContract, "invalid_manifest_report", false, "manifest show", "Reconcile the Workspace Manifest Runtime binding."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime.")),
			Mutation: &MutationContract{TargetKind: tobari.ManifestRuntimeBindingTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}},
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
	return CommandSpec{Path: path, Summary: summary, Args: "--name <name> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{CapabilityID: "runtime.lifecycle", Outcome: outcome,
			Inputs: []CommandInput{{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Unique local Runtime name.", AllowedValues: []string{}, Completion: InputCompletionRuntimeName}, formatInput()},
			Output: runtimeReportOutput(), Prerequisites: []string{}, Errors: readCommandErrors(path, true,
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_name", false, "runtime list", "Choose a Runtime from the local catalog."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing Runtime."),
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating Runtime JSON encoding."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			)}, handler: handler}
}

func runtimeCreateSpec() CommandSpec {
	minimum := int64(1)
	return CommandSpec{Path: "runtime create", Summary: "Create a reusable Runtime by copying current editable source once", Args: "[--copy-source-from <runtime-name>] --name <name> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{CapabilityID: "runtime.lifecycle", Outcome: "Create one standalone managed Runtime source tree from the built-in standard starter or another managed Runtime's current editable source; its root and future children must have no group/other permissions; do not build, retain inheritance, or change a Workspace Template",
			Inputs: []CommandInput{
				{Name: "--copy-source-from", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Copy current editable source once from standard or an existing managed Runtime; the new Runtime receives a fresh ID and empty history.", AllowedValues: []string{}, DefaultValue: stringPointer(tobari.StandardRuntimeName), Completion: InputCompletionRuntimeName},
				{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Unique local Runtime name.", AllowedValues: []string{}}, formatInput()},
			Output: runtimeCreateOutput(), Prerequisites: []string{"A managed copy source must remain an owner-only bounded editable tree throughout the copy; immutable name@ordinal revisions are not copy sources."}, FixedTarget: fixedRuntimeCatalogTarget(),
			Errors: mutationCommandErrors("runtime create", "runtime list",
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_name", false, "runtime list", "Choose a valid unique Runtime name."),
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_copy_source", false, "runtime list", "Choose standard or an existing managed Runtime name, not a name@ordinal revision."),
				declaredCommandError(fault.KindNotFound, "runtime_copy_source_not_found", false, "runtime list", "Choose standard or an existing managed Runtime name."),
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store before retrying interactive copy-source selection."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store before retrying interactive copy-source selection."),
				declaredCommandError(fault.KindInternal, "runtime_review_failed", false, "help runtime create", "Retry with --copy-source-from or repair the interactive terminal streams."),
				declaredCommandError(fault.KindRejected, "runtime_source_invalid", false, "review runtimes", "Inspect the unchanged source and Runtime catalog."),
				declaredCommandError(fault.KindRejected, "runtime_exists", false, "review runtimes", "Inspect the existing Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_create_failed", false, "runtime list", "Inspect the local Runtime catalog."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "runtime list", "Reconcile the Runtime catalog."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime.")),
			Mutation: &MutationContract{TargetKind: tobari.RuntimeCatalogTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}},
		handler: runRuntimeCreate}
}

func runtimeReviewSpec() CommandSpec {
	return CommandSpec{
		Path: "review runtimes", Summary: "Review managed Runtimes or recover one interrupted lifecycle action",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Review interrupted Runtime build, restore, or whole-delete authority first, or the complete Runtime catalog, then cross into the separate exact-reference mutation only after trusted-terminal confirmation",
			Inputs:       []CommandInput{formatInput()}, Output: runtimeListOutput(),
			Prerequisites: []string{"Interactive build selection and journal recovery require trusted terminal input and error output; redirected and JSON use remains read-only.", "An interrupted journal is reviewed before any new Runtime build selection."},
			Errors: readCommandErrors("review runtimes", true,
				declaredCommandError(fault.KindNotFound, "managed_runtime_not_found", false, "help runtime create", "Create a managed Runtime source tree first."),
				declaredCommandError(fault.KindRejected, "runtime_selection_changed", false, "review runtimes", "Restart from a fresh Runtime catalog."),
				declaredCommandError(fault.KindRejected, "runtime_recovery_observation_unknown", false, "review runtimes", "Retry the trusted-host read-only review."),
				classifiedCommandError(fault.KindInvalidInput, "invalid_runtime_recovery", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Restart from current recovery authority."),
				declaredCommandError(fault.KindRejected, "runtime_recovery_failed", false, "review runtimes", "Re-observe the retained journal before another mutation."),
				classifiedCommandError(fault.KindNotFound, "runtime_not_found", false, fault.PhasePrecondition, fault.ChangeNone, "runtime list", "Discover the current managed Runtime catalog."),
				classifiedCommandError(fault.KindNotFound, "runtime_revision_not_found", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Discover the current retained Runtime revisions."),
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				classifiedCommandError(fault.KindRejected, "runtime_lifecycle_active", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the retained Runtime lifecycle journal."),
				classifiedCommandError(fault.KindRejected, "runtime_revision_unrestorable", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Review the retained immutable revision authority."),
				classifiedCommandError(fault.KindInternal, "runtime_restore_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "review runtimes", "Resume the exact retained restore authority."),
				classifiedCommandError(fault.KindInternal, "runtime_restore_outcome_unknown", false, fault.PhaseMutation, fault.ChangeUnknown, "review runtimes", "Observe the retained Runtime lifecycle journal before another mutation."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_restore_result_partial", false, fault.PhaseVerification, fault.ChangePartial, "review runtimes", "Reconcile the retained Runtime revision and current image availability."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_restore_result_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the retained Runtime revision and current image availability."),
				classifiedCommandError(fault.KindRejected, "runtime_delete_protected", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the Runtime and its current Template or Workspace protections."),
				classifiedCommandError(fault.KindInternal, "runtime_delete_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "review runtimes", "Resume the exact retained Runtime deletion authority."),
				classifiedCommandError(fault.KindInternal, "runtime_delete_outcome_unknown", false, fault.PhaseMutation, fault.ChangeUnknown, "review runtimes", "Observe the retained Runtime lifecycle journal before another mutation."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_delete_result_partial", false, fault.PhaseVerification, fault.ChangePartial, "review runtimes", "Reconcile the retained Runtime deletion receipt and lifecycle state."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_delete_result_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the retained Runtime deletion receipt and lifecycle state."),
				classifiedCommandError(fault.KindContract, "output_encoding_failed", false, fault.PhasePresentation, fault.ChangeConfirmed, "version", "Report the exact build identity without repeating confirmed-delete JSON encoding."),
				classifiedCommandError(fault.KindInternal, "missing_runtime_delete", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the Runtime delete application boundary."),
				declaredCommandError(fault.KindContract, "runtime_recovery_contract_invalid", false, "review runtimes", "Reconcile the current Runtime catalog."),
				classifiedCommandError(fault.KindInternal, "runtime_build_observation_unknown", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the confirmed Runtime revision and current material availability."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_report_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the confirmed Runtime revision and current material availability."),
				declaredCommandError(fault.KindInternal, "runtime_review_failed", false, "help review runtimes", "Retry on a trusted terminal or use redirected/JSON read-only discovery."),
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Interactive: &InteractiveWorkflowContract{
				ActionCommand: "runtime build", SelectionReferenceKind: tobari.RuntimeReferenceKind,
				SelectionOutputField: "items[].runtime_ref", Confirmation: "explicit_yes", NonInteractiveBehavior: "read_only",
			},
		},
		handler: runRuntimeReview,
	}
}

func runtimeBuildSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime build", Summary: "Build an immutable revision of a reusable Runtime",
		Args: "--id <runtime-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Snapshot a managed source with no group/other permissions and at most 1,024 files, 256 directories, 32 MiB per file, and 64 MiB total; build and validate it; append one immutable revision without changing any Workspace Template",
			Inputs: []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
				Description: "Opaque Runtime reference emitted by runtime list, show, history, create, or review runtimes; it is consumed unchanged.", AllowedValues: []string{}, ReferenceKind: tobari.RuntimeReferenceKind}, formatInput()},
			Output: runtimeReportOutput(),
			Prerequisites: []string{
				"The referenced managed Runtime exists and its source root and directories have no group/other permissions.",
				"The source contains at most 1,024 owner-only regular files, 256 directories, 32 MiB per file, and 64 MiB total; it contains a regular Dockerfile and no links or special files.",
				"The trusted host Docker daemon and Buildx plugin are available.",
			},
			Errors: mutationCommandErrors("runtime build", "review runtimes",
				declaredCommandError(fault.KindNotFound, "managed_runtime_not_found", false, "help runtime create", "Create a managed Runtime source tree first."),
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_ref", false, "runtime list", "Use one Runtime reference unchanged."),
				declaredCommandError(fault.KindNotFound, "runtime_not_found", false, "runtime list", "Choose an existing managed Runtime."),
				declaredCommandError(fault.KindRejected, "runtime_reference_unresolved", false, "runtime list", "Discover the current Runtime catalog."),
				declaredCommandError(fault.KindRejected, "runtime_not_managed", false, "runtime list", "Choose a managed Runtime."),
				declaredCommandError(fault.KindInternal, "runtime_read_failed", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindContract, "invalid_runtime_list", false, "doctor", "Inspect the host Runtime store."),
				declaredCommandError(fault.KindRejected, "runtime_source_invalid", false, "review runtimes", "Inspect the unchanged Runtime source path and history."),
				declaredCommandError(fault.KindRejected, "runtime_build_failed", false, "review runtimes", "Inspect the unchanged Runtime history and source path."),
				declaredCommandError(fault.KindRejected, "runtime_recovery_observation_unknown", false, "review runtimes", "Retry the trusted-host read-only review."),
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_recovery", false, "review runtimes", "Restart from current recovery authority."),
				declaredCommandError(fault.KindRejected, "runtime_recovery_failed", false, "review runtimes", "Re-observe the retained journal before another mutation."),
				declaredCommandError(fault.KindContract, "runtime_recovery_contract_invalid", false, "review runtimes", "Reconcile the current Runtime catalog."),
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				classifiedCommandError(fault.KindInternal, "runtime_build_observation_unknown", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the confirmed Runtime revision and current material availability."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_report_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the confirmed Runtime revision and current material availability."),
				declaredCommandError(fault.KindUnavailable, "image_not_found", false, "review runtimes", "Review the Runtime source and current material availability before another build."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "review runtimes", "Correct the source so the image preserves the Tobari runtime contract."),
				declaredCommandError(fault.KindContract, "invalid_runtime_report", false, "review runtimes", "Reconcile the confirmed Runtime build."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.RuntimeReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
			},
		},
		handler: runRuntimeBuild,
	}
}

func runtimeRestoreSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime restore", Summary: "Restore one exact retained Runtime revision image",
		Args: "--id <revision-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Rebuild one exact managed Runtime revision from its retained immutable source and publish it only when the rebuilt content digest matches, without appending history or changing any Workspace Template, Context, or Workspace",
			Inputs: []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
				Description: "Opaque managed Runtime revision reference emitted by runtime list, show, history, build, or review runtimes; it is consumed unchanged.", AllowedValues: []string{}, ReferenceKind: tobari.RuntimeRevisionReferenceKind}, formatInput()},
			Output: runtimeRestoreOutput(),
			Prerequisites: []string{
				"The referenced managed Runtime and retained immutable source revision exist and pass bounded owner-only source validation.",
				"The trusted host Docker daemon and Buildx plugin are available; mutable base refresh is disabled for exact restore.",
			},
			Errors: mutationCommandErrors("runtime restore", "review runtimes",
				classifiedCommandError(fault.KindInvalidInput, "invalid_runtime_revision_ref", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Use one managed Runtime revision reference unchanged."),
				classifiedCommandError(fault.KindNotFound, "runtime_not_found", false, fault.PhasePrecondition, fault.ChangeNone, "runtime list", "Discover the current managed Runtime catalog."),
				classifiedCommandError(fault.KindNotFound, "runtime_revision_not_found", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Discover the current retained Runtime revisions."),
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				classifiedCommandError(fault.KindRejected, "runtime_lifecycle_active", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the retained Runtime lifecycle journal."),
				classifiedCommandError(fault.KindRejected, "runtime_revision_unrestorable", false, fault.PhaseVerification, fault.ChangeNone, "review runtimes", "Review the retained immutable revision authority."),
				classifiedCommandError(fault.KindInternal, "runtime_restore_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "review runtimes", "Resume the exact retained restore authority."),
				classifiedCommandError(fault.KindInternal, "runtime_restore_outcome_unknown", false, fault.PhaseMutation, fault.ChangeUnknown, "review runtimes", "Observe the retained Runtime lifecycle journal before another mutation."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_restore_result_partial", false, fault.PhaseVerification, fault.ChangePartial, "review runtimes", "Reconcile the retained Runtime revision and current image availability."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_restore_result_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the retained Runtime revision and current image availability."),
				classifiedCommandError(fault.KindInternal, "missing_runtime_restore", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the Tobari Runtime restore adapter."),
				classifiedCommandError(fault.KindInternal, "missing_runtime", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.RuntimeRevisionReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: runtimecmd.RestoreImpact(),
			},
		},
		handler: runRuntimeRestore,
	}
}

func runtimeDeleteSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime delete", Summary: "Delete one complete unused managed Runtime",
		Args: "--id <runtime-ref> --confirm=delete [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Delete one exact unused managed Runtime as a whole, including editable source, immutable snapshots, revision history, and exact owned image tags, while preserving every Workspace Template, Context, Workspace, ID, home, applied receipt, Project root, credential, and shared resource",
			Inputs: []CommandInput{
				{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque managed Runtime reference emitted by Runtime discovery and consumed unchanged.", AllowedValues: []string{}, ReferenceKind: tobari.RuntimeReferenceKind},
				{Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Confirm irreversible whole-Runtime source, snapshot, history, and owned-tag deletion without cascading to Workspace authority.", AllowedValues: []string{"delete"}},
				formatInput(),
			},
			Output: runtimeDeleteOutput(),
			Prerequisites: []string{
				"The target is one managed Runtime; built-in standard is never a deletion target.",
				"Current and retained Workspace Template revisions, every Context desired binding, and every Workspace applied, pending, and observed Runtime binding are completely observed and do not protect the target.",
				"No Workspace or external container uses target material, migration and ownership evidence is verified, and exact source, snapshot, journal, and Docker observations remain complete.",
			},
			Errors: mutationCommandErrors("runtime delete", "review runtimes",
				classifiedCommandError(fault.KindInvalidInput, "invalid_runtime_ref", false, fault.PhasePrecondition, fault.ChangeNone, "runtime list", "Use one managed Runtime reference unchanged."),
				classifiedCommandError(fault.KindNotFound, "runtime_not_found", false, fault.PhasePrecondition, fault.ChangeNone, "runtime list", "Discover the current managed Runtime catalog."),
				classifiedCommandError(fault.KindRejected, "runtime_delete_protected", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the Runtime and its current Template or Workspace protections."),
				classifiedCommandError(fault.KindRejected, "runtime_lifecycle_active", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the retained Runtime lifecycle journal."),
				classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
				classifiedCommandError(fault.KindInternal, "runtime_delete_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "review runtimes", "Resume the exact retained Runtime deletion authority."),
				classifiedCommandError(fault.KindInternal, "runtime_delete_outcome_unknown", false, fault.PhaseMutation, fault.ChangeUnknown, "review runtimes", "Observe the retained Runtime lifecycle journal before another mutation."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_delete_result_partial", false, fault.PhaseVerification, fault.ChangePartial, "review runtimes", "Reconcile the retained Runtime deletion receipt and lifecycle state."),
				classifiedCommandError(fault.KindContract, "invalid_runtime_delete_result_confirmed", false, fault.PhaseVerification, fault.ChangeConfirmed, "review runtimes", "Reconcile the retained Runtime deletion receipt and lifecycle state."),
				classifiedCommandError(fault.KindContract, "output_encoding_failed", false, fault.PhasePresentation, fault.ChangeConfirmed, "version", "Report the exact build identity without repeating confirmed-delete JSON encoding."),
				classifiedCommandError(fault.KindInternal, "missing_runtime_delete", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the Runtime delete application boundary."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.RuntimeReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id",
				Impact: runtimecmd.DeleteImpact(),
			},
		},
		handler: runRuntimeDelete,
	}
}

func runtimePruneDryRunSpec() CommandSpec {
	return CommandSpec{
		Path: "runtime prune dry-run", Summary: "Review exact unused Runtime image material without changing state",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Completely review exact unused managed Runtime revision and settled failed-build image material, every protection and blocker, preserved source/snapshot bytes, and bounded Docker evidence; produce one opaque plan without changing state",
			Inputs:       []CommandInput{formatInput()},
			Output:       runtimePrunePlanOutput(),
			Prerequisites: []string{
				"The owner-only Runtime catalog, build/retirement journals, Workspace Template current and retained revisions, Context desired bindings, Workspace applied/pending/observed state, and immutable source snapshots are complete and valid.",
				"Bounded Docker image and exact candidate-container observation can finish; no state directory, lock, journal, timestamp, or Docker resource is created or changed.",
			},
			Errors: readCommandErrors("runtime prune dry-run", true,
				declaredCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, "doctor", "Inspect the host Runtime lifecycle state."),
				declaredCommandError(fault.KindContract, "invalid_runtime_prune_plan", false, "doctor", "Inspect the complete Runtime lifecycle inventory."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating prune-plan JSON encoding."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
		},
		handler: runRuntimePruneDryRun,
	}
}

func runtimePruneApplySpec() CommandSpec {
	return CommandSpec{
		Path: "runtime prune apply", Summary: "Apply one exact reviewed Runtime prune plan",
		Args: "--plan <runtime-prune-plan-ref> --confirm=prune [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "runtime.lifecycle",
			Outcome:      "Apply one unchanged reviewed Runtime prune plan by removing only exact Tobari-owned unused image tags while preserving Runtime source, immutable snapshots, revision history, Workspace Templates, Contexts, Workspaces, homes, IDs, and shared content",
			Inputs: []CommandInput{
				{Name: "--plan", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque Runtime prune plan reference emitted by runtime prune dry-run and consumed unchanged.", AllowedValues: []string{}, ReferenceKind: tobari.RuntimePrunePlanReferenceKind},
				{Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Confirm exact unused image-tag retirement without deleting source, snapshots, history, Workspace Templates, Contexts, or Workspaces.", AllowedValues: []string{"prune"}},
				formatInput(),
			},
			Output: runtimePruneResultOutput(),
			Prerequisites: []string{
				"The exact plan still recomputes unchanged under the installation lifecycle and Runtime-store locks, or its matching durable journal/receipt authorizes idempotent resume.",
				"Every candidate remains unreferenced and unused with complete ownership, immutable source, journal, and bounded Docker evidence; any unknown or migration-unverified state blocks the whole plan.",
			},
			Errors: mutationCommandErrors("runtime prune apply", "runtime prune dry-run",
				declaredCommandError(fault.KindInvalidInput, "invalid_runtime_prune_plan_ref", false, "runtime prune dry-run", "Create and use one fresh Runtime prune plan reference unchanged."),
				declaredCommandError(fault.KindRejected, "runtime_prune_plan_stale", false, "runtime prune dry-run", "Review a fresh exact Runtime prune plan."),
				declaredCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, "doctor", "Inspect the host Runtime lifecycle state."),
				declaredCommandError(fault.KindInternal, "runtime_prune_interrupted", false, "runtime prune dry-run", "Observe the retained journal or current lifecycle state before another mutation."),
				declaredCommandError(fault.KindContract, "invalid_runtime_retirement_result", false, "runtime prune dry-run", "Reconcile the current Runtime lifecycle state."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating confirmed-prune JSON encoding."),
				declaredCommandError(fault.KindInternal, "missing_runtime_prune", false, "doctor", "Configure the Runtime prune application boundary."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.RuntimePrunePlanReferenceKind, TargetInputs: []string{"--plan"}, TargetIDInput: "--plan",
				Impact: runtimecmd.PruneImpact(),
			},
		},
		handler: runRuntimePruneApply,
	}
}

func projectEnterSpec() CommandSpec {
	return CommandSpec{
		Path: WorkspaceEntryCommandPath, Summary: "Prepare the current directory's Workspace and enter Bash or an exact command",
		Args:   "[--manifest <name>] [-- <command>...]",
		Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "On first use, review recommended Workspace Manifest settings or Customize; then prepare shared services, choose or create the current directory's Workspace, reconcile Runtime, and enter Bash or one exact foreground command before returning to the host",
			Inputs: []CommandInput{lifecycleContextInput(), {
				Name: "command", Source: InputSourceArgument, Required: false,
				ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
				Description:   "Exact child argv after --; the first value is the executable and later values are passed unchanged, including duplicates, dash-prefixed values, and explicit empty arguments.",
				AllowedValues: []string{}, PositionalOnly: true,
			}}, Output: noOutput(),
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
		Path: "status", Summary: "Inspect the current directory's Workspace state and next action",
		Args:   "[--manifest <name>] [--format text|json]",
		Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Report the selected Workspace Manifest, whether logical Workspace state exists for the current directory, its desired next entry, last successfully applied entry, adoption state, and read-only runtime observation",
			Inputs:       []CommandInput{lifecycleContextInput(), formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "workspace_manifest_state", Type: OutputFieldTypeString, Description: "Whether persisted Workspace Manifest authority exists.", Enum: []string{"persisted", "absent"}},
					{Name: "exists", Type: OutputFieldTypeBoolean, Description: "Whether a Workspace exists for the current directory and selected Workspace Manifest."},
					{Name: "project_root", Type: OutputFieldTypeString, Description: "Nearest canonical project root when one exists."},
					{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Diagnostic stable Workspace identity when one exists."},
					{Name: "workspace_home", Type: OutputFieldTypeString, Description: "Diagnostic Workspace XDG home path when one exists."},
					{Name: "runtime", Type: OutputFieldTypeString, Description: "Recoverable runtime diagnostic; incomplete means the logical state record is missing and must be deleted before recreation."},
					{Name: "attachment", Type: OutputFieldTypeString, Description: "Transient session observation: attached or detached when the Workspace exists, and not_applicable when it does not."},
					{Name: "bootstrap", Type: OutputFieldTypeObject, Description: "One-time future-Workspace configuration snapshot relationship; never credential state.", Fields: []OutputField{
						{Name: "state", Type: OutputFieldTypeString, Description: "Whether no recipe exists, this Workspace never received it, its applied revision is current, or it retains an older create-time revision.", Enum: []string{"not_configured", "not_applied", "current", "older"}},
						{Name: "applied_revision", Type: OutputFieldTypeString, Description: "Semantic revision projected when this Workspace was created, or an empty string when none was applied."},
						{Name: "current_revision", Type: OutputFieldTypeString, Description: "Current Workspace Manifest recipe revision, or an empty string when the recipe was removed."},
					}},
					{Name: "workspace_manifest", Type: OutputFieldTypeString, Description: "Selected invocation Workspace Manifest display name, including when no Manifest is persisted."},
					{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Selected stable Workspace Manifest authority identity, or null before one is persisted.", Nullable: true},
					{Name: "adoption", Type: OutputFieldTypeString, Description: "Relationship between Current and Next entry identities, or null when no Workspace exists.", Enum: []string{"never_applied", "current", "pending"}, Nullable: true},
					workspaceAppliedEntryOutputField("current", "Last successfully reconciled entry, or null before the first successful entry or when no Workspace exists.", true),
					workspaceDesiredEntryOutputField("next", "Exact desired entry derived from the selected Workspace Manifest, or null when no persisted Manifest exists.", true),
					workspaceReconciliationFailureOutputField(),
					{Name: "next_argv", Type: OutputFieldTypeArray, Description: "Exact argv that re-enters the persisted Workspace Manifest-bound lifecycle target, or uses omission-based selection when no Manifest is persisted.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One exact argv token."}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "status", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("status", true,
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "template list", "Choose existing final Template and Context authority."),
				declaredCommandError(fault.KindContract, "invalid_manifest_binding", false, "context list", "Inspect final Context authority before selecting a Workspace."),
				declaredCommandError(fault.KindContract, "manifest_binding_stale", false, "doctor", "Inspect Workspace Manifest and Workspace state."),
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
		Path: "delete", Summary: "Delete the nearest current-directory Workspace when no session is attached",
		Args: "[--manifest <name>] [--force]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "tobari.lifecycle",
			Outcome:      "Delete one nearest Workspace Manifest-bound CWD Workspace, its exact owned runtime resources, persistent home, and tool-owned authentication state when detached; reject attached sessions unless --force overrides only that guard, while preserving the mounted project root",
			Inputs: []CommandInput{lifecycleContextInput(), {
				Name: "--force", Source: InputSourceFlag, Required: false,
				ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
				Description: "Override only the attached-session safety guard and terminate that session while deleting the disclosed Workspace Manifest-bound Workspace, persistent home, and tool-owned authentication state.", AllowedValues: []string{}, DefaultValue: stringPointer("false"),
			}},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Whether the selected Workspace was deleted."},
					{Name: "project_root", Type: OutputFieldTypeString, Description: "Deleted Workspace's preserved canonical project root."},
					{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Deleted stable Workspace identity."},
					{Name: "workspace_home", Type: OutputFieldTypeString, Description: "Deleted Workspace XDG home path."},
					{Name: "workspace_manifest", Type: OutputFieldTypeString, Description: "Workspace Manifest display name bound to the deleted Workspace."},
					{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable Workspace Manifest authority identity bound to the deleted Workspace."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The target is the nearest Workspace in the explicit or default Workspace Manifest; its mounted project root is preserved.", "Without --force, no session is attached; --force terminates any attached session while deleting the persistent home and tool-owned authentication state."},
			FixedTarget:   fixedCurrentDirectoryTarget(),
			Errors: mutationCommandErrors("delete", "status",
				declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "manifest list", "Choose an existing Workspace Manifest."),
				declaredCommandError(fault.KindContract, "invalid_manifest_binding", false, "manifest list", "Inspect the Workspace Manifest catalog before selecting a Workspace."),
				declaredCommandError(fault.KindContract, "manifest_binding_stale", false, "delete", "Review the newly selected target before retrying force deletion."),
				declaredCommandError(fault.KindRejected, "project_session_attached", false, "delete", "Exit the attached session, then retry; use --force only when terminating it is intentional."),
				declaredCommandError(fault.KindNotFound, "project_not_found", false, WorkspaceEntryCommandPath, "Create a Workspace from the current project directory."),
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
					fault.NextAction{Command: "cluster status", Reason: "Inspect shared-cluster state before choosing an explicit reconciliation action."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "cluster status", "Repair the runtime status contract."),
				declaredCommandError(fault.KindUnavailable, "cluster_start_failed", false, "cluster status", "Reconcile partial Docker state."),
				declaredCommandError(fault.KindUnavailable, "network_guard_failed", false, "doctor", "Inspect Docker Engine network-namespace and nftables support."),
				declaredCommandError(fault.KindUnavailable, "gateway_image_unavailable", true, "doctor", "Inspect the local Docker image store before retrying Gateway validation."),
				declaredCommandError(fault.KindUnavailable, "gateway_image_build_failed", false, "doctor", "Inspect Docker BuildKit and the pinned embedded Gateway build inputs."),
				declaredCommandError(fault.KindContract, "runtime_image_api_mismatch", false, "doctor", "Inspect the executable resolver channel and selected immutable component API authorities."),
				declaredCommandError(fault.KindContract, "gateway_image_incompatible", false, "doctor", "Inspect the Gateway image API, source identity, and architecture contract."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the verified Auth Broker image."),
				declaredCommandError(fault.KindContract, "auth_broker_image_incompatible", false, "doctor", "Inspect the Auth Broker image API, digest, entrypoint, user, and architecture contract."),
				declaredCommandError(fault.KindUnavailable, "credential_companion_unavailable", true, "cluster status", "Inspect shared authentication-service state before reconciliation."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "cluster status", "Inspect shared-cluster state before another broker reconciliation."),
				declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "cluster status", "Inspect partial shared-cluster state before another reconcile."),
				declaredCommandError(fault.KindContract, "auth_broker_unlock_failed", false, "doctor", "Inspect Auth Broker and root-key provider state."),
				declaredCommandError(fault.KindUnavailable, "root_key_unavailable", false, "doctor", "Inspect the host root-key provider."),
				declaredCommandError(fault.KindRejected, "root_key_missing_with_vault", false, "doctor", "Restore the original root key or explicitly remove local authentication state."),
				declaredCommandError(fault.KindRejected, "root_key_unsafe", false, "doctor", "Repair unsafe root-key or Auth Broker state paths."),
				declaredCommandError(fault.KindUnavailable, "keychain_denied", false, "doctor", "Inspect trusted-host root-key readiness before cluster reconciliation."),
				declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect Workspace Manifest vault integrity without printing its contents."),
				declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported Workspace Manifest vault."),
				declaredCommandError(fault.KindRejected, "invalid_provider_manifest", false, "doctor", "Repair the owner-controlled provider manifest collection."),
				declaredCommandError(fault.KindRejected, "ambiguous_provider_http_binding", false, "doctor", "Remove the overlapping exact provider HTTP binding."),
				declaredCommandError(fault.KindUnavailable, "runtime_image_unavailable", true, "doctor", "Inspect Docker registry access before retrying the official runtime base image."),
				declaredCommandError(fault.KindRejected, "incompatible_image", false, "template show", "Inspect the relevant Workspace Template Runtime binding."),
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
	spec.Agent.Errors = append(spec.Agent.Errors, workspaceStartReadinessErrors()...)
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
			Outcome:      "Observe shared enforcement health, aggregate Workspace Manifest policy revision, registry integrity, attached count, and recent errors",
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
					fault.NextAction{Command: "cluster status", Reason: "Inspect shared-cluster state before choosing an explicit reconciliation action."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the status contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating cluster JSON encoding."),
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
		{Name: "workspace_count", Type: OutputFieldTypeInteger, Description: "Number of configured Workspaces."},
		{Name: "manifest_count", Type: OutputFieldTypeInteger, Description: "Number of Workspace Manifest policies loaded in the aggregate projection."},
		{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Content-addressed aggregate policy revision, or null when unconfigured.", Nullable: true},
		{Name: "policy_projection", Type: OutputFieldTypeString, Description: "All-Workspace Manifest policy projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
		{Name: "principal_registry", Type: OutputFieldTypeString, Description: "Principal registry integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
		{Name: "gateway_projection", Type: OutputFieldTypeString, Description: "Gateway routing projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
	}
	componentNames := []string{"gateway", "opa"}
	componentDescription := "Exact Gateway and OPA observations."
	componentScope := "The two shared standard services when configured; empty when unconfigured."
	if buildIdentityHasBroker() {
		fields = append(fields,
			OutputField{Name: "auth_provider_projection", Type: OutputFieldTypeString, Description: "Research Auth Broker provider projection integrity observation.", Enum: []string{"valid", "invalid", "unavailable"}},
			OutputField{Name: "auth_broker_state", Type: OutputFieldTypeString, Description: "Observed ready, locked, or unavailable research Auth Broker state.", Enum: []string{"ready", "locked", "unavailable"}},
			OutputField{Name: "credential_companion_state", Type: OutputFieldTypeString, Description: "Observed research trusted-host credential companion state.", Enum: []string{"ready", "prepared", "absent", "unavailable"}},
			OutputField{Name: "root_key_backend", Type: OutputFieldTypeString, Description: "Selected research host root-key backend or unavailable state.", Enum: []string{"macos_keychain", "xdg_file", "unavailable"}},
		)
		componentNames = []string{"auth-broker", "gateway", "opa"}
		componentDescription = "Exact Auth Broker, Gateway, and OPA observations."
		componentScope = "The three shared research services when configured; empty when unconfigured."
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
					{Name: "unparsed_lines", Type: OutputFieldTypeInteger, Description: "Denial-shaped Gateway lines that could not safely enter the typed projection."},
					{
						Name: "items", Type: OutputFieldTypeArray,
						Description:   "Validated denials ordered oldest to newest with host-issued project principal, scheme-independent request authority (host and port), method, path, protocol, optional exact GraphQL operation/root or MCP method/tool coordinate, reason, status, and exact-rule learnability.",
						SemanticScope: "Valid denials found in the requested bounded Gateway log-line window.",
						Items:         &OutputField{Type: OutputFieldTypeObject, Description: "One validated denial observation.", Fields: policyDenialOutputFields()},
					},
					{Name: "review_command", Type: OutputFieldTypeString, Description: "Exact command that opens the pending permission review queue."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "denials", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 3,
			},
			Prerequisites: []string{"The cluster has been created."},
			Errors: append(readCommandErrors("cluster denials", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_denial_request", false, "help cluster denials", "Select a valid bounded window."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster logs", "Inspect raw Gateway logs."),
				declaredCommandError(fault.KindContract, "invalid_denial_contract", false, "cluster logs", "Inspect raw Gateway logs and audit compatibility."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating denial JSON encoding."),
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
		Path: "review permissions", Summary: "Review pending network permissions",
		Args: "[--tail <lines>] [--format text|json] [--watch] [--notify auto|osc9|bel|off]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "policy.learning",
			Outcome:      "Review pending exact HTTP or GraphQL effects and typed HTTP single-segment path-template proposals; a raw terminal can stage exact decisions from the list, inspect template scope, apply one Workspace Manifest's reviewed set, and optionally watch bounded snapshots",
			Inputs:       []CommandInput{reviewTailInput(), formatInput(), policyReviewWatchInput(), policyReviewNotifyInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields:   policyCandidateOutputFields(),
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow,
				JSONEnvelope: "policy_review", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{"The cluster has retained Gateway denial evidence."},
			Errors: append(policyCandidateReadErrors("review permissions", true),
				declaredCommandError(fault.KindInvalidInput, "policy_review_watch_requires_tty", false, "help review permissions", "Run watch with text output in an interactive raw terminal."),
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
			Outcome:      "Revalidate and activate one bounded typed set of exact or single-segment-template Allows and exact Denies for one Workspace Manifest staged by interactive review permissions",
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
							{Name: "decision", Type: OutputFieldTypeString, Description: "Applied Allow or Deny decision.", Enum: tobari.PolicyDecisionValues()},
							{Name: "match", Type: OutputFieldTypeString, Description: "Exact or single-segment path-template match.", Enum: tobari.PolicyMatchValues()},
							{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable Workspace Manifest identity."},
							{Name: "workspace_manifest", Type: OutputFieldTypeString, Description: "Exact Workspace Manifest display name."},
							{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity carried by the host-issued principal."},
							{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe canonical project root."},
							{Name: "host", Type: OutputFieldTypeString, Description: "Exact request host."},
							{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact request port."},
							{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
							{Name: "path", Type: OutputFieldTypeString, Description: "Exact path without query data."},
							{Name: "source_candidates", Type: OutputFieldTypeArray, Description: "Sorted exact candidate evidence bound to the stored rule.", Items: &OutputField{Type: OutputFieldTypeString, Description: "Opaque exact policy-candidate identity."}},
							{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: tobari.PolicyProtocolValues()},
							{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
							{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL operation type; empty for HTTP."},
							{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact GraphQL root field; empty for HTTP."},
							{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
							{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
							{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Observed AWS Query or JSON wire protocol; empty outside AWS."},
							{Name: "aws_service", Type: OutputFieldTypeString, Description: "Exact SigV4 signing service; empty outside AWS."},
							{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Exact observed AWS wire operation token; empty outside AWS."},
							{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Exact Kubernetes API verb; empty outside Kubernetes."},
							{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Exact Kubernetes resource coordinate; empty outside Kubernetes."},
							{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Exact Kubernetes dry-run mode; empty outside Kubernetes."},
							{Name: "git_service", Type: OutputFieldTypeString, Description: "Exact Git Smart HTTP service; empty outside Git."},
							{Name: "git_repository", Type: OutputFieldTypeString, Description: "Exact Git repository path; empty outside Git."},
							{Name: "oci_action", Type: OutputFieldTypeString, Description: "Exact OCI Distribution action; empty outside OCI."},
							{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Exact OCI repository; empty outside OCI."},
							{Name: "oci_object", Type: OutputFieldTypeString, Description: "Exact OCI object coordinate; empty outside OCI."},
						},
					}},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"An interactive review permissions session has a bounded non-empty staged decision set."},
			FixedTarget: &FixedTarget{
				Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID,
				Description: "The one CLI-owned installation policy decision set.", Scope: FixedTargetScopeToolLocal,
			},
			Errors: policyMutationCommandErrors("review permissions", "review permissions",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_review_session", false, "review permissions", "Stage decisions through an interactive Permission Inbox."),
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_review_set", false, "review permissions", "Review a bounded non-empty set of exact candidates."),
				declaredCommandError(fault.KindRejected, "policy_review_changed", false, "review permissions", "Review the current pending queue again."),
				declaredCommandError(fault.KindRejected, "policy_review_scope_mixed", false, "review permissions", "Apply or discard the staged Workspace Manifest decisions before reviewing another Workspace Manifest."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "review permissions", "Review again after the concurrent policy change."),
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
			Prerequisites: []string{"Every configured Workspace Manifest has a validated policy source and the shared aggregate is current."},
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
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates or review permissions")},
			Output:       policyLearningChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates or review permissions and remains in retained Gateway logs.",
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
			Inputs:       []CommandInput{policyReferenceInput(tobari.PolicyCandidateKind, "policy candidates or review permissions")},
			Output:       policyDenyChangeOutput(),
			Prerequisites: []string{
				"The ID was emitted by policy candidates or review permissions and remains an actionable pending candidate.",
			},
			Errors: policyMutationCommandErrors("policy deny", "review permissions",
				declaredCommandError(fault.KindInvalidInput, "invalid_policy_candidate_id", false, "review permissions", "Use a candidate ID unchanged."),
				declaredCommandError(fault.KindInvalidInput, "policy_candidate_not_found", false, "review permissions", "Rediscover the current pending queue."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
				declaredCommandError(fault.KindRejected, "policy_data_changed", false, "review permissions", "Rediscover after the concurrent policy change."),
				declaredCommandError(fault.KindRejected, "policy_preflight_failed", false, "doctor", "Correct the complete candidate policy."),
				declaredCommandError(fault.KindRejected, "policy_test_failed", false, "doctor", "Correct the policy or ensure its XDG directory is accessible to the Docker Engine before activation."),
				declaredCommandError(fault.KindInternal, "policy_write_failed", false, "review permissions", "Inspect the unchanged or atomically updated policy data."),
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
			Outcome:      clusterDownOutcome(),
			Inputs:       []CommandInput{purgeInput("Also remove exact shared CA and active policy-bundle volumes.")},
			Output:       textClusterStatusOutput(),
			Prerequisites: []string{
				"Every logical Workspace has been deleted.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster down", "cluster status",
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindRejected, "cluster_not_empty", false, "workspace list", "Delete every listed final Workspace explicitly."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster status", Reason: "Inspect shared-cluster state before choosing an explicit reconciliation action."}),
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
			Outcome:      "Return every configured Workspace with its Manifest binding, desired next entry, last successfully applied entry, adoption state, and read-only runtime observation",
			Inputs:       []CommandInput{formatInput()},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical project root mounted by the Workspace."},
					{Name: "runtime", Type: OutputFieldTypeString, Description: "Recoverable runtime diagnostic; incomplete means the logical state record is missing and must be deleted before recreation."},
					{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Diagnostic stable Workspace ID; not a routine action input."},
					{Name: "workspace_home", Type: OutputFieldTypeString, Description: "Workspace-owned persistent home path."},
					{Name: "workspace_manifest", Type: OutputFieldTypeString, Description: "Workspace Manifest display name permanently bound to the Workspace."},
					{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable Workspace Manifest authority identity permanently bound to the Workspace."},
					{Name: "adoption", Type: OutputFieldTypeString, Description: "Relationship between Current and Next entry identities.", Enum: []string{"never_applied", "current", "pending"}},
					workspaceAppliedEntryOutputField("current", "Last successfully reconciled entry, or null before the first successful entry.", true),
					workspaceDesiredEntryOutputField("next", "Exact desired entry derived from the bound Workspace Manifest.", false),
					workspaceReconciliationFailureOutputField(),
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "workspaces", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 2,
			},
			Prerequisites: []string{},
			Errors: readCommandErrors("list", true,
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Validate the current directory."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "runtime_status_failed", false, "status", "Inspect the selected project's runtime."),
				declaredCommandError(fault.KindContract, "invalid_list_contract", false, "doctor", "Repair list semantics."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating list JSON encoding."),
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
		Kind: tobari.ManifestCatalogTargetKind, ID: tobari.ManifestCatalogTargetID,
		Description: "This installation's host-owned collection of named Workspace Manifests.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedRuntimeCatalogTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.RuntimeCatalogTargetKind, ID: tobari.RuntimeCatalogTargetID, Description: "This installation's host-owned reusable Runtime catalog.", Scope: FixedTargetScopeToolLocal}
}

func fixedContextRuntimeBindingTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.ManifestRuntimeBindingTargetKind, ID: tobari.ManifestRuntimeBindingTargetID, Description: "One explicit or default Workspace Manifest's exact mutable Runtime revision binding, adopted by bound Workspaces on next entry.", Scope: FixedTargetScopeToolLocal}
}

func fixedActiveContextTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ManifestTargetKind, ID: tobari.DefaultManifestSelectionTargetID,
		Description: "This installation's host-owned Workspace Manifest omission default.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextShellTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID,
		Description: "This installation's Workspace Manifest-owned allowlisted shell session defaults.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextGitIdentityTarget() *FixedTarget {
	return &FixedTarget{
		Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID,
		Description: "This installation's Workspace Manifest-owned narrow Git identity session defaults.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func fixedContextBootstrapTarget() *FixedTarget {
	return &FixedTarget{Kind: tobari.ManifestBootstrapTargetKind, ID: tobari.ManifestBootstrapTargetID, Description: "This installation's Workspace Manifest-owned secret-free creation default applied only to future Workspace homes.", Scope: FixedTargetScopeToolLocal}
}

func fixedActiveContextRuntimeTarget() *FixedTarget {
	return &FixedTarget{
		Kind:        tobari.ManifestRuntimeTargetKind,
		ID:          tobari.ActiveContextRuntimeID,
		Description: "The default Workspace Manifest's host-owned runtime recipe and selected image.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func contextNameInput() CommandInput {
	return CommandInput{
		Name: "--name", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Portable Workspace Manifest name; it is a selection label, not a credential authority.",
		AllowedValues: []string{}, Completion: InputCompletionContextName,
	}
}

func contextCreateNameInput() CommandInput {
	input := contextNameInput()
	input.Required = false
	input.Completion = InputCompletionNone
	input.Description = "Portable Workspace Manifest name; on interactive text streams a supplied name prefills and skips the Name stage while omitted settings remain reviewed."
	return input
}

func contextCreateBaseInput() CommandInput {
	minimum := int64(1)
	return CommandInput{
		Name: "--copy-from", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum,
		Description:   "Existing Workspace Manifest used only to initialize a standalone creation draft; no lineage or authority relationship is persisted.",
		AllowedValues: []string{}, Completion: InputCompletionContextName,
	}
}

func contextCreateRuntimeInput() CommandInput {
	minimum := int64(1)
	return CommandInput{Name: "--runtime", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: &minimum, Description: "Initial exact mutable Runtime binding as standard or name@ordinal; interactive partial creation reviews omission, while complete direct creation requires an explicit value.", AllowedValues: []string{}, DefaultValue: stringPointer(tobari.StandardRuntimeName), Completion: InputCompletionReadyRuntimeReference}
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
		Name: "--manifest", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Workspace Manifest display name for this invocation; omission uses the default Workspace Manifest without changing it.", AllowedValues: []string{}, Completion: InputCompletionContextName,
	}
}

func lifecycleContextInput() CommandInput {
	input := executionContextInput()
	minimumLength := int64(1)
	input.MinimumLength = &minimumLength
	input.Description = fmt.Sprintf("Non-empty Workspace Manifest display name for this invocation; both `%s --manifest toolbox status` and `%s status --manifest toolbox` are accepted, omission uses the default Workspace Manifest without changing it, and duplicate placement is rejected.", ProgramName, ProgramName)
	return input
}

func contextImageInput() CommandInput {
	return CommandInput{
		Name: "--image", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Built-in compatible Tobari image selector stored in the Workspace Manifest.",
		AllowedValues: []string{}, DefaultValue: stringPointer(tobari.BuiltinImageSelector),
	}
}

func contextModeInput() CommandInput {
	return CommandInput{
		Name: "--mode", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Creation-time immutable Boundary mode: guided exact permission review or advanced trusted-host Rego; required in the complete direct input group.",
		AllowedValues: []string{"guided", "advanced"}, DefaultValue: stringPointer("guided"),
	}
}

func contextSourceAccessInput() CommandInput {
	return CommandInput{
		Name: "--source-access", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Creation-time immutable Boundary authority for the one direct project source bind; interactive partial creation reviews omission and Workspace home plus tmpfs remain writable.",
		AllowedValues: []string{"read-only", "read-write"}, DefaultValue: stringPointer("read-write"),
	}
}

func contextNativeReadinessInput() CommandInput {
	return CommandInput{Name: "--native-readiness", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Creation-time immutable Boundary choice that admits the trusted binary's current native-client readiness overlay; required with --mode and still bounded by Workspace Manifest policy ceilings.", AllowedValues: []string{"enabled", "disabled"}, DefaultValue: stringPointer("enabled")}
}

func contextReportOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Declared Workspace Manifest task identity for this report."},
			{Name: "workspace_manifest_state", Type: OutputFieldTypeString, Description: "Persisted Workspace Manifest authority.", Enum: []string{"persisted"}},
			{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable host-issued Workspace Manifest identity.", Nullable: true},
			{Name: "name", Type: OutputFieldTypeString, Description: "Named Workspace Manifest identifier."},
			{Name: "default", Type: OutputFieldTypeBoolean, Description: "Whether this Workspace Manifest is the default for omitted Manifest input."},
			workspaceManifestDesiredOutputField(),
			{Name: "agent_profile", Type: OutputFieldTypeString, Description: "Read-only shared agent profile reference."},
			{Name: "image", Type: OutputFieldTypeString, Description: "Default compatible Tobari image selector stored in the Workspace Manifest."},
			{Name: "policy_mode", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary policy-development mode.", Enum: []string{"guided", "advanced"}},
			{Name: "source_access", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary access for the direct project-source bind; this does not describe Workspace home or tmpfs.", Enum: []string{"read-only", "read-write"}},
			{Name: "policy_revision", Type: OutputFieldTypeString, Description: "SHA-256 revision of the immutable Workspace Manifest-owned normalized policy snapshot; empty only for a synthetic default."},
			{Name: "native_readiness", Type: OutputFieldTypeString, Description: "Creation-time immutable Boundary choice for native-client readiness participation; the system policy ceiling still bounds its effects.", Enum: []string{"enabled", "disabled"}},
			{Name: "method_policy", Type: OutputFieldTypeObject, Description: "Effective default and exact HTTP method decisions owned by the Workspace Manifest.", Fields: contextPolicyMethodPolicyOutput("Effective default and exact HTTP method decisions owned by the Workspace Manifest.").Fields},
			{Name: "shell_environment", Type: OutputFieldTypeArray, Description: "Complete allowlisted shell session-default inventory, resolved for later child sessions without rewriting Workspace home; literal carries its exact value.", SemanticScope: "The fixed four-variable Workspace Manifest shell presentation inventory.", Items: &OutputField{
				Type: OutputFieldTypeObject, Description: "One allowlisted shell variable policy.", Fields: []OutputField{
					{Name: "variable", Type: OutputFieldTypeString, Description: "Allowlisted variable name.", Enum: []string{"COLORTERM", "NO_COLOR", "PS1", "TERM"}},
					{Name: "source", Type: OutputFieldTypeString, Description: "Value source.", Enum: []string{"default", "inherit", "literal"}},
					{Name: "value", Type: OutputFieldTypeString, Description: "Exact literal value, including explicit empty, only for literal source.", Optional: true},
				},
			}},
			{Name: "git_identity", Type: OutputFieldTypeObject, Description: "Atomic Git session-default policy resolved on later Workspace entry without rewriting Workspace home; literal carries exact name/email.", Fields: []OutputField{
				{Name: "source", Type: OutputFieldTypeString, Description: "Identity source.", Enum: []string{"default", "inherit", "literal"}},
				{Name: "name", Type: OutputFieldTypeString, Description: "Literal Git user name, or null.", Nullable: true},
				{Name: "email", Type: OutputFieldTypeString, Description: "Literal Git user email, or null.", Nullable: true},
			}},
			contextBootstrapOutputField(),
			{Name: "stores", Type: OutputFieldTypeObject, Description: "Resolved paths, or null for a synthetic default; secret values are never included.", Nullable: true, Fields: []OutputField{
				{Name: "policy_directory", Type: OutputFieldTypeString, Description: "Canonical Workspace Manifest policy directory."},
			}},
			{Name: "runtime", Type: OutputFieldTypeObject, Description: "Exact explicitly mutable built-in or managed Runtime revision binding; bound Workspaces adopt replacements on next entry with identity and home preserved.", Fields: []OutputField{
				{Name: "kind", Type: OutputFieldTypeString, Description: "Built-in or managed Runtime source kind.", Enum: []string{"official", "managed"}},
				{Name: "status", Type: OutputFieldTypeString, Description: "Selected Runtime readiness.", Enum: []string{"official", "ready"}},
				{Name: "image", Type: OutputFieldTypeString, Description: "Execution image material selected by this binding."},
				{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority identity."},
				{Name: "name", Type: OutputFieldTypeString, Description: "Runtime name."},
				{Name: "revision", Type: OutputFieldTypeString, Description: "Exact semantic Runtime revision."},
				{Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Human Runtime revision ordinal."},
			}},
			{Name: "cluster", Type: OutputFieldTypeString, Description: "How this task relates to cluster activation.", Enum: []string{"not_applicable", "not_configured", "not_running", "already_ready", "reconciled", "default_updated", "requires_reconcile"}},
			contextAuthenticationOutputField(),
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "workspace_manifest", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
	}
}

func workspaceManifestDesiredOutputField() OutputField {
	return OutputField{Name: "desired", Type: OutputFieldTypeObject, Description: "Exact immutable desired revision identities.", Fields: []OutputField{
		{Name: "manifest_generation", Type: OutputFieldTypeInteger, Description: "Monotonic correlation generation."},
		{Name: "manifest_revision", Type: OutputFieldTypeString, Description: "Complete semantic Workspace Manifest digest."},
		{Name: "boundary_revision", Type: OutputFieldTypeString, Description: "Immutable Boundary digest."},
		{Name: "cluster_projection_revision", Type: OutputFieldTypeString, Description: "Cluster projection activation digest."},
		{Name: "entry_revision", Type: OutputFieldTypeString, Description: "Next Workspace entry configuration digest."},
		{Name: "session_defaults_revision", Type: OutputFieldTypeString, Description: "Later-session defaults digest."},
		{Name: "creation_defaults_revision", Type: OutputFieldTypeString, Description: "New-Workspace creation defaults digest."},
	}}
}

func workspaceDesiredEntryOutputField(name, description string, nullable bool) OutputField {
	return OutputField{Name: name, Type: OutputFieldTypeObject, Description: description, Nullable: nullable, Fields: []OutputField{
		{Name: "manifest_generation", Type: OutputFieldTypeInteger, Description: "Monotonic Workspace Manifest generation used for correlation."},
		{Name: "manifest_revision", Type: OutputFieldTypeString, Description: "Complete semantic Workspace Manifest digest."},
		{Name: "entry_revision", Type: OutputFieldTypeString, Description: "Exact Workspace-entry configuration digest."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority identity, including the built-in standard identity."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Exact semantic Runtime revision digest."},
	}}
}

func workspaceAppliedEntryOutputField(name, description string, nullable bool) OutputField {
	return OutputField{Name: name, Type: OutputFieldTypeObject, Description: description, Nullable: nullable, Fields: []OutputField{
		{Name: "manifest_generation", Type: OutputFieldTypeInteger, Description: "Workspace Manifest generation confirmed by the last successful entry."},
		{Name: "manifest_revision", Type: OutputFieldTypeString, Description: "Complete Workspace Manifest digest confirmed by the last successful entry."},
		{Name: "entry_revision", Type: OutputFieldTypeString, Description: "Workspace-entry configuration digest confirmed by the last successful entry."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Stable Runtime authority identity consumed by the successful entry."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Exact semantic Runtime revision consumed by the successful entry."},
		{Name: "resolved_spec_revision", Type: OutputFieldTypeString, Description: "Digest of the fully resolved entry specification that was applied."},
		{Name: "reconciled_at", Type: OutputFieldTypeString, Description: "UTC timestamp of the last successful reconciliation; it is not Runtime last-used evidence."},
	}}
}

func workspaceReconciliationFailureOutputField() OutputField {
	return OutputField{Name: "last_reconciliation_failure", Type: OutputFieldTypeObject, Description: "Bounded latest failed or unknown entry attempt; null does not imply an entry was attempted.", Nullable: true, Fields: []OutputField{
		{Name: "attempted_generation", Type: OutputFieldTypeInteger, Description: "Workspace Manifest generation attempted."},
		{Name: "attempted_manifest_revision", Type: OutputFieldTypeString, Description: "Workspace Manifest revision attempted."},
		{Name: "attempted_entry_revision", Type: OutputFieldTypeString, Description: "Workspace-entry revision attempted."},
		{Name: "phase", Type: OutputFieldTypeString, Description: "Closed reconciliation phase that failed."},
		{Name: "code", Type: OutputFieldTypeString, Description: "Stable secret-free failure code."},
		{Name: "change_state", Type: OutputFieldTypeString, Description: "Whether mutation was known absent, partial, or unknown.", Enum: []string{"none", "partial", "unknown"}},
		{Name: "occurred_at", Type: OutputFieldTypeString, Description: "UTC timestamp of the failed attempt."},
	}}
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
	errors := mutationCommandErrors(WorkspaceEntryCommandPath, "status")
	filtered := errors[:0]
	for _, declared := range errors {
		if declared.Code != "mutation_output_write_failed" {
			filtered = append(filtered, declared)
		}
	}
	result := append(filtered,
		workspaceStartReadinessErrors()...,
	)
	result = append(result,
		declaredCommandError(fault.KindInternal, "first_use_review_failed", false, WorkspaceEntryCommandPath, "Retry in an interactive terminal."),
		declaredCommandError(fault.KindContract, "invalid_first_use_draft", false, "help "+WorkspaceEntryCommandPath, "Inspect the root first-use contract."),
		declaredCommandError(fault.KindNotFound, "manifest_not_found", false, "template list", "Choose existing final Template and Context authority."),
		declaredCommandError(fault.KindContract, "invalid_manifest_binding", false, "context list", "Inspect final Context authority before selecting a Workspace."),
		declaredCommandError(fault.KindContract, "manifest_binding_stale", false, "doctor", "Inspect Workspace Manifest and Workspace state."),
		declaredCommandError(fault.KindInvalidInput, "tty_required", false, "help "+WorkspaceEntryCommandPath, "Run the root command from an interactive terminal."),
		declaredCommandError(fault.KindRejected, "already_inside", false, "help "+WorkspaceEntryCommandPath, "Exit the current Workspace session before entering another."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_configured", false, "cluster up", "Create the shared cluster explicitly before entering a Workspace."),
		declaredCommandError(fault.KindUnavailable, "cluster_status_failed", false, "cluster status", "Inspect the shared cluster before entering a Workspace."),
		declaredCommandError(fault.KindUnavailable, "cluster_not_ready", false, "cluster up", "Reconcile the shared cluster explicitly before entering a Workspace."),
		declaredCommandError(fault.KindRejected, "cluster_projection_stale", false, "cluster up", "Load the complete Workspace Manifest catalog into the shared cluster before entering a Workspace."),
		declaredCommandError(fault.KindRejected, "runtime_build_required", false, "review runtimes", "Review the staged Runtime and select its exact build action before entering a Workspace."),
		declaredCommandError(fault.KindRejected, "runtime_recipe_invalid", false, "template show", "Inspect and correct the invalid custom Runtime binding before entry."),
		declaredCommandError(fault.KindInternal, "runtime_choice_failed", false, WorkspaceEntryCommandPath, "Resume from the persisted Workspace Manifest and ready cluster."),
		declaredCommandError(fault.KindRejected, "project_state_incomplete", false, "workspace list", "Discover and review the exact final Workspace reference before deletion."),
		declaredCommandError(fault.KindInternal, "missing_workspace_selector", false, "doctor", "Configure the Tobari terminal selector."),
		declaredCommandError(fault.KindContract, "invalid_workspace_selection", false, "doctor", "Inspect local Workspace state."),
		declaredCommandError(fault.KindContract, "workspace_selection_invalid", false, WorkspaceEntryCommandPath, "Choose a current Workspace or explicitly create one again."),
		declaredCommandError(fault.KindRejected, "workspace_selection_stale", true, WorkspaceEntryCommandPath, "Refresh the Workspace choices and select again."),
		declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Inspect the current directory and host access."),
		declaredCommandError(fault.KindUnavailable, "image_not_found", false, "review runtimes", "Review the selected Runtime and current image availability before recovery."),
		declaredCommandError(fault.KindUnavailable, "git_identity_resolution_failed", false, "template show", "Inspect the selected Template Git identity without changing Workspace state."),
		declaredCommandError(fault.KindUnavailable, "runtime_reconcile_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindUnavailable, "network_guard_failed", false, "doctor", "Inspect Docker Engine network-namespace and nftables support."),
		declaredCommandError(fault.KindInternal, "enter_failed", false, "status", "Inspect the selected project's runtime."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "status", "Inspect Workspace and shared-cluster state before authentication reconciliation."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "status", "Inspect Workspace state before another Auth Broker request."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_locked", false, "status", "Inspect Workspace state before unlocking the Auth Broker."),
		declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect the Workspace Manifest vault integrity without printing its contents."),
		declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported Workspace Manifest vault."),
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

func workspaceStartReadinessErrors() []CommandError {
	return []CommandError{
		classifiedCommandError(fault.KindUnavailable, "docker_cli_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness before starting a Workspace."),
		classifiedCommandError(fault.KindUnavailable, "docker_engine_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness before starting a Workspace."),
		classifiedCommandError(fault.KindUnsupported, "docker_engine_incompatible", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness before starting a Workspace."),
		classifiedCommandError(fault.KindUnavailable, "docker_manifest_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness before starting a Workspace."),
		classifiedCommandError(fault.KindUnavailable, "docker_compose_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness before starting a Workspace."),
		classifiedCommandError(fault.KindContract, "invalid_readiness_profile", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the generic Docker readiness contract."),
		classifiedCommandError(fault.KindContract, "invalid_readiness_observation", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the generic Docker readiness observation contract."),
	}
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
		{Name: "decision", Type: OutputFieldTypeString, Description: "Current learned decision: allow or deny.", Enum: tobari.PolicyDecisionValues()},
		{Name: "match", Type: OutputFieldTypeString, Description: "Exact or single-segment path-template match.", Enum: tobari.PolicyMatchValues()},
		{Name: "workspace_manifest_id", Type: OutputFieldTypeString, Description: "Stable Workspace Manifest authority bound to the decision."},
		{Name: "workspace_manifest", Type: OutputFieldTypeString, Description: "Human-readable Workspace Manifest name."},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity bound to the decision."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe diagnostic canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact decision scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact decision host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact decision port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact path."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: tobari.PolicyProtocolValues()},
		{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL query or mutation type; empty for HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact canonical GraphQL root field; empty for HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Observed AWS Query or JSON wire protocol; empty outside AWS."},
		{Name: "aws_service", Type: OutputFieldTypeString, Description: "Exact SigV4 signing service; empty outside AWS."},
		{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Exact observed AWS wire operation token; empty outside AWS."},
		{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Exact Kubernetes API verb; empty outside Kubernetes."},
		{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Exact Kubernetes resource coordinate; empty outside Kubernetes."},
		{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Exact Kubernetes dry-run mode; empty outside Kubernetes."},
		{Name: "git_service", Type: OutputFieldTypeString, Description: "Exact Git Smart HTTP service; empty outside Git."},
		{Name: "git_repository", Type: OutputFieldTypeString, Description: "Exact Git repository path; empty outside Git."},
		{Name: "oci_action", Type: OutputFieldTypeString, Description: "Exact OCI Distribution action; empty outside OCI."},
		{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Exact OCI repository; empty outside OCI."},
		{Name: "oci_object", Type: OutputFieldTypeString, Description: "Exact OCI object coordinate; empty outside OCI."},
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
			{Name: "decision", Type: OutputFieldTypeString, Description: "Removed learned decision: allow or deny.", Enum: tobari.PolicyDecisionValues()},
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
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable final Context authority established by Gateway network identity."},
		{Name: "context", Type: OutputFieldTypeString, Description: "Human-readable final Context presentation."},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity for the denied request."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe diagnostic canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact denied request scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact denied request host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact denied request port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact denied uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact denied HTTP path without query data."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: tobari.PolicyProtocolValues()},
		{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL query or mutation type; empty for HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact canonical GraphQL root field; empty for HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Observed AWS Query or JSON wire protocol; empty outside AWS."},
		{Name: "aws_service", Type: OutputFieldTypeString, Description: "Exact SigV4 signing service; empty outside AWS."},
		{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Exact observed AWS wire operation token; empty outside AWS."},
		{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Exact Kubernetes API verb; empty outside Kubernetes."},
		{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Exact Kubernetes resource coordinate; empty outside Kubernetes."},
		{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Exact Kubernetes dry-run mode; empty outside Kubernetes."},
		{Name: "git_service", Type: OutputFieldTypeString, Description: "Exact Git Smart HTTP service; empty outside Git."},
		{Name: "git_repository", Type: OutputFieldTypeString, Description: "Exact Git repository path; empty outside Git."},
		{Name: "oci_action", Type: OutputFieldTypeString, Description: "Exact OCI Distribution action; empty outside OCI."},
		{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Exact OCI repository; empty outside OCI."},
		{Name: "oci_object", Type: OutputFieldTypeString, Description: "Exact OCI object coordinate; empty outside OCI."},
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
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable final Context authority established by Gateway network identity."},
		{Name: "context", Type: OutputFieldTypeString, Description: "Human-readable final Context presentation."},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity carried by the host-issued principal."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Safe canonical project root."},
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact denied scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact denied host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact denied port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact denied uppercase HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact redacted denial path without query data."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective policy protocol.", Enum: tobari.PolicyProtocolValues()},
		{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL operation type, or an empty string for ordinary HTTP."},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact GraphQL root field, or an empty string for ordinary HTTP."},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method; empty outside MCP."},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact MCP tool name for tools/call; empty otherwise."},
		{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Observed AWS Query or JSON wire protocol; empty outside AWS."},
		{Name: "aws_service", Type: OutputFieldTypeString, Description: "Exact SigV4 signing service; empty outside AWS."},
		{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Exact observed AWS wire operation token; empty outside AWS."},
		{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Exact Kubernetes API verb; empty outside Kubernetes."},
		{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Exact Kubernetes resource coordinate; empty outside Kubernetes."},
		{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Exact Kubernetes dry-run mode; empty outside Kubernetes."},
		{Name: "git_service", Type: OutputFieldTypeString, Description: "Exact Git Smart HTTP service; empty outside Git."},
		{Name: "git_repository", Type: OutputFieldTypeString, Description: "Exact Git repository path; empty outside Git."},
		{Name: "oci_action", Type: OutputFieldTypeString, Description: "Exact OCI Distribution action; empty outside OCI."},
		{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Exact OCI repository; empty outside OCI."},
		{Name: "oci_object", Type: OutputFieldTypeString, Description: "Exact OCI object coordinate; empty outside OCI."},
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
			{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity bound to the denial."},
			{Name: "host", Type: OutputFieldTypeString, Description: "Stored exact host."},
			{Name: "port", Type: OutputFieldTypeInteger, Description: "Stored exact request port."},
			{Name: "method", Type: OutputFieldTypeString, Description: "Stored exact uppercase HTTP method."},
			{Name: "path", Type: OutputFieldTypeString, Description: "Stored exact path."},
			{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective stored policy protocol.", Enum: tobari.PolicyProtocolValues()},
			{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
			{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Stored GraphQL query or mutation type; empty for HTTP."},
			{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Stored canonical GraphQL root field; empty for HTTP."},
			{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Stored exact MCP JSON-RPC method; empty outside MCP."},
			{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Stored exact MCP tool name for tools/call; empty otherwise."},
			{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Stored AWS Query or JSON wire protocol; empty outside AWS."},
			{Name: "aws_service", Type: OutputFieldTypeString, Description: "Stored exact SigV4 signing service; empty outside AWS."},
			{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Stored exact AWS wire operation token; empty outside AWS."},
			{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes API verb; empty outside Kubernetes."},
			{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes resource coordinate; empty outside Kubernetes."},
			{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes dry-run mode; empty outside Kubernetes."},
			{Name: "git_service", Type: OutputFieldTypeString, Description: "Stored exact Git Smart HTTP service; empty outside Git."},
			{Name: "git_repository", Type: OutputFieldTypeString, Description: "Stored exact Git repository path; empty outside Git."},
			{Name: "oci_action", Type: OutputFieldTypeString, Description: "Stored exact OCI Distribution action; empty outside OCI."},
			{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Stored exact OCI repository; empty outside OCI."},
			{Name: "oci_object", Type: OutputFieldTypeString, Description: "Stored exact OCI object coordinate; empty outside OCI."},
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
			{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity bound to the stored rule."},
			{Name: "host", Type: OutputFieldTypeString, Description: "Stored exact host."},
			{Name: "port", Type: OutputFieldTypeInteger, Description: "Stored exact request port."},
			{Name: "method", Type: OutputFieldTypeString, Description: "Stored exact uppercase HTTP method."},
			{Name: "path", Type: OutputFieldTypeString, Description: "Stored exact path."},
			{Name: "protocol", Type: OutputFieldTypeString, Description: "Effective stored policy protocol.", Enum: tobari.PolicyProtocolValues()},
			{Name: "state_change", Type: OutputFieldTypeString, Description: "Conservative protocol-derived state-change potential; review evidence, never independent authority.", Enum: tobari.PolicyStateChangeValues()},
			{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Stored GraphQL query or mutation type; empty for HTTP."},
			{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Stored canonical GraphQL root field; empty for HTTP."},
			{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Stored exact MCP JSON-RPC method; empty outside MCP."},
			{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Stored exact MCP tool name for tools/call; empty otherwise."},
			{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "Stored AWS Query or JSON wire protocol; empty outside AWS."},
			{Name: "aws_service", Type: OutputFieldTypeString, Description: "Stored exact SigV4 signing service; empty outside AWS."},
			{Name: "aws_operation", Type: OutputFieldTypeString, Description: "Stored exact AWS wire operation token; empty outside AWS."},
			{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes API verb; empty outside Kubernetes."},
			{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes resource coordinate; empty outside Kubernetes."},
			{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Stored exact Kubernetes dry-run mode; empty outside Kubernetes."},
			{Name: "git_service", Type: OutputFieldTypeString, Description: "Stored exact Git Smart HTTP service; empty outside Git."},
			{Name: "git_repository", Type: OutputFieldTypeString, Description: "Stored exact Git repository path; empty outside Git."},
			{Name: "oci_action", Type: OutputFieldTypeString, Description: "Stored exact OCI Distribution action; empty outside OCI."},
			{Name: "oci_repository", Type: OutputFieldTypeString, Description: "Stored exact OCI repository; empty outside OCI."},
			{Name: "oci_object", Type: OutputFieldTypeString, Description: "Stored exact OCI object coordinate; empty outside OCI."},
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
		{Name: "manifest_count", Type: OutputFieldTypeInteger, Description: "Number of Workspace Manifest policies in the shared enforcement projection."},
		{Name: "policy_revision", Type: OutputFieldTypeString, Description: "Content-addressed aggregate policy revision."},
		{Name: "policy_projection", Type: OutputFieldTypeString, Description: "All-Workspace Manifest policy projection integrity observation."},
		{Name: "principal_registry", Type: OutputFieldTypeString, Description: "Principal registry integrity observation."},
		{Name: "gateway_projection", Type: OutputFieldTypeString, Description: "Gateway routing projection integrity observation."},
	}
	if buildIdentityHasBroker() {
		fields = append(fields,
			OutputField{Name: "auth_provider_projection", Type: OutputFieldTypeString, Description: "Research Auth Broker provider projection integrity observation."},
			OutputField{Name: "auth_broker_state", Type: OutputFieldTypeString, Description: "Observed research Auth Broker state."},
			OutputField{Name: "credential_companion_state", Type: OutputFieldTypeString, Description: "Observed research trusted-host credential companion state."},
			OutputField{Name: "root_key_backend", Type: OutputFieldTypeString, Description: "Selected research host root-key backend."},
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
		declaredCommandError(fault.KindInternal, "manifest_read_failed", false, "context list", "Inspect final Context authority before using policy data."),
		declaredCommandError(fault.KindRejected, "manifest_mismatch", false, "cluster up", "Reconcile the shared cluster's Context policy projection."),
	}
}

func policyCandidateReadErrors(path string, hasOutput bool) []CommandError {
	return append(readCommandErrors(path, hasOutput,
		declaredCommandError(fault.KindInvalidInput, "invalid_candidate_request", false, "help "+path, "Select a valid bounded denial window."),
		declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
		declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
		declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
		declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Inspect retained denial compatibility."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating JSON encoding."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	), policyClusterReadinessErrors()...)
}

func policyRuleReadErrors(path string, hasOutput bool) []CommandError {
	return append(readCommandErrors(path, hasOutput,
		declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
		declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair the owner-only XDG policy data."),
		declaredCommandError(fault.KindContract, "invalid_policy_rule_report", false, "doctor", "Inspect the current policy-rule contract."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity for diagnosis."),
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
