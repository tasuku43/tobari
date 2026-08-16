package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func policyPresetNameInput(description string) CommandInput {
	minimum := int64(1)
	return CommandInput{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: description, MinimumLength: &minimum, AllowedValues: []string{}}
}

func policyPresetExactRuleOutput(description string) *OutputField {
	return &OutputField{Type: OutputFieldTypeObject, Description: description, Fields: []OutputField{
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact HTTP scheme.", Enum: []string{"http", "https"}},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact canonical public host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact TCP port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact HTTP method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact raw request path."},
	}}
}

func policyPresetTemplateRuleOutput() *OutputField {
	field := policyPresetExactRuleOutput("One bounded direct-child identifier template.")
	field.Fields = append(field.Fields, OutputField{Name: "segments", Type: OutputFieldTypeArray, Description: "Exact normalized segments with one terminal {id}.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One literal or {id} segment."}})
	return field
}

func policyPresetMCPRuleOutput() *OutputField {
	field := policyPresetExactRuleOutput("One exact MCP semantic baseline grant.")
	field.Fields = append(field.Fields,
		OutputField{Name: "mcp_method", Type: OutputFieldTypeString, Description: "Exact MCP JSON-RPC method."},
		OutputField{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "Exact tool name only for tools/call.", Optional: true},
	)
	return field
}

func policyPresetGraphQLRuleOutput() *OutputField {
	field := policyPresetExactRuleOutput("One exact GraphQL semantic baseline grant.")
	field.Fields = append(field.Fields,
		OutputField{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "Exact GraphQL operation type.", Enum: []string{"query", "mutation"}},
		OutputField{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "Exact canonical GraphQL root field."},
	)
	return field
}

func policyPresetOutput(collection bool) CommandOutput {
	fields := []OutputField{{Name: "task", Type: OutputFieldTypeString, Description: "Declared policy-preset task identity."}}
	if collection {
		fields = append(fields, OutputField{
			Name: "items", Type: OutputFieldTypeArray, Description: "Complete installed policy-preset collection.",
			SemanticScope: "All installed built-in and custom policy presets at one observation.",
			Items: &OutputField{Type: OutputFieldTypeObject, Description: "One policy preset.", Fields: []OutputField{
				{Name: "origin", Type: OutputFieldTypeString, Description: "Exact policy-preset selector."},
				{Name: "revision", Type: OutputFieldTypeString, Description: "SHA-256 revision of normalized bytes."},
				{Name: "guardrail", Type: OutputFieldTypeString, Description: "Terminal guardrail kind.", Enum: []string{"offline", "reviewed_exact", "get_only_reviewed"}},
				{Name: "immediate_grant_count", Type: OutputFieldTypeInteger, Description: "Number of Context-wide exact, template, and semantic baseline grants."},
				{Name: "destination_ceiling", Type: OutputFieldTypeString, Description: "Destination ceiling mode.", Enum: []string{"public_https", "exact"}}, {Name: "destination_count", Type: OutputFieldTypeInteger, Description: "Exact destination count."}, {Name: "method_ceiling", Type: OutputFieldTypeString, Description: "Method ceiling mode.", Enum: []string{"all", "exact"}}, {Name: "method_count", Type: OutputFieldTypeInteger, Description: "Exact method count."},
			}},
		})
	} else {
		fields = append(fields,
			OutputField{Name: "origin", Type: OutputFieldTypeString, Description: "Exact policy-preset selector."},
			OutputField{Name: "revision", Type: OutputFieldTypeString, Description: "SHA-256 revision of normalized bytes."},
			OutputField{Name: "source_path", Type: OutputFieldTypeString, Description: "Owner-only custom source path when applicable.", Optional: true}, OutputField{Name: "scope", Type: OutputFieldTypeString, Description: "Task-owned preset authority scope."}, OutputField{Name: "limitations", Type: OutputFieldTypeArray, Description: "Explicit limitations and non-claims.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One limitation."}},
			OutputField{Name: "preset", Type: OutputFieldTypeObject, Description: "Complete normalized non-executable schema-V1 preset.", Fields: []OutputField{
				{Name: "schema_version", Type: OutputFieldTypeInteger, Description: "Preset schema version."},
				{Name: "name", Type: OutputFieldTypeString, Description: "Canonical preset name."},
				{Name: "guardrail", Type: OutputFieldTypeString, Description: "Terminal guardrail kind.", Enum: []string{"offline", "reviewed_exact", "get_only_reviewed"}},
				{Name: "destination_ceiling", Type: OutputFieldTypeObject, Description: "Explicit public-HTTPS or exact destination ceiling.", Fields: []OutputField{{Name: "mode", Type: OutputFieldTypeString, Description: "Destination ceiling mode.", Enum: []string{"public_https", "exact"}}, {Name: "authorities", Type: OutputFieldTypeArray, Description: "Exact destination ceiling entries.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact authority.", Fields: []OutputField{
					{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact HTTP scheme.", Enum: []string{"http", "https"}}, {Name: "host", Type: OutputFieldTypeString, Description: "Exact canonical public host."}, {Name: "port", Type: OutputFieldTypeInteger, Description: "Exact TCP port."},
				}}}}},
				{Name: "method_ceiling", Type: OutputFieldTypeObject, Description: "Explicit all-eligible or exact method ceiling.", Fields: []OutputField{{Name: "mode", Type: OutputFieldTypeString, Description: "Method ceiling mode.", Enum: []string{"all", "exact"}}, {Name: "methods", Type: OutputFieldTypeArray, Description: "Explicit method ceiling.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One exact HTTP method."}}}},
				{Name: "baseline_grants", Type: OutputFieldTypeArray, Description: "Exact Context-wide grants.", Items: policyPresetExactRuleOutput("One exact grant.")},
				{Name: "baseline_templates", Type: OutputFieldTypeArray, Description: "Bounded Context-wide path-template grants.", Items: policyPresetTemplateRuleOutput()},
				{Name: "graphql_baseline_grants", Type: OutputFieldTypeArray, Description: "Exact Context-wide GraphQL semantic grants.", Items: policyPresetGraphQLRuleOutput()},
				{Name: "mcp_baseline_grants", Type: OutputFieldTypeArray, Description: "Exact Context-wide MCP semantic grants.", Items: policyPresetMCPRuleOutput()},
				{Name: "baseline_denies", Type: OutputFieldTypeArray, Description: "Exact terminal baseline denials.", Items: policyPresetExactRuleOutput("One exact denial.")},
				{Name: "graphql_endpoints", Type: OutputFieldTypeArray, Description: "Exact GraphQL classification endpoints.", Items: policyPresetExactRuleOutput("One exact endpoint.")},
				{Name: "mcp_endpoints", Type: OutputFieldTypeArray, Description: "Exact MCP classification endpoints.", Items: policyPresetExactRuleOutput("One exact endpoint.")},
			}},
		)
	}
	coverage := CollectionCoverageNotApplicable
	if collection {
		coverage = CollectionCoverageExhaustive
	}
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: coverage, JSONEnvelope: "policy_presets", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func policyPresetReadErrors(path string) []CommandError {
	return readCommandErrors(path, true,
		declaredCommandError(fault.KindInvalidInput, "invalid_policy_preset", false, "help "+path, "Choose an exact built-in or custom selector."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	)
}

func policyPresetListSpec() CommandSpec {
	return CommandSpec{Path: "policy preset list", Summary: "List installed policy presets", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{CapabilityID: "policy.presets", Outcome: "List the exhaustive installed built-in and custom policy preset catalog with immutable revisions and guardrails", Inputs: []CommandInput{formatInput()}, Output: policyPresetOutput(true), Prerequisites: []string{}, Errors: policyPresetReadErrors("policy preset list")}, handler: runPolicyPresetList}
}

func policyPresetShowSpec() CommandSpec {
	return CommandSpec{Path: "policy preset show", Summary: "Inspect one policy preset", Args: "--name <preset> [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{CapabilityID: "policy.presets", Outcome: "Inspect one complete normalized policy preset without activating it", Inputs: []CommandInput{policyPresetNameInput("Exact builtin/name or custom/name selector."), formatInput()}, Output: policyPresetOutput(false), Prerequisites: []string{}, Errors: policyPresetReadErrors("policy preset show")}, handler: runPolicyPresetShow}
}

func policyPresetValidateSpec() CommandSpec {
	return CommandSpec{Path: "policy preset validate", Summary: "Validate one custom policy preset", Args: "--name <preset> [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{CapabilityID: "policy.presets", Outcome: "Strictly validate, normalize, and digest one custom preset source without changing Context or active policy", Inputs: []CommandInput{policyPresetNameInput("Exact custom/name selector."), formatInput()}, Output: policyPresetOutput(false), Prerequisites: []string{}, Errors: policyPresetReadErrors("policy preset validate")}, handler: runPolicyPresetValidate}
}

func policyPresetInitSpec() CommandSpec {
	return CommandSpec{Path: "policy preset init", Summary: "Create a deny-all custom policy preset template", Args: "--name <name> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "policy.presets", Outcome: "Create one owner-only strict custom policy preset template without overwriting", Inputs: []CommandInput{policyPresetNameInput("Portable custom preset name without the custom/ prefix."), formatInput()}, Output: policyPresetOutput(false),
		Prerequisites: []string{"The owner-only local custom policy-preset catalog is accessible."},
		FixedTarget:   &FixedTarget{Kind: tobari.PolicyPresetCatalogTargetKind, ID: tobari.PolicyPresetCatalogTargetID, Description: "This installation's owner-only custom policy-preset catalog.", Scope: FixedTargetScopeToolLocal},
		Errors:        mutationCommandErrors("policy preset init", "policy preset list", declaredCommandError(fault.KindInvalidInput, "invalid_policy_preset", false, "help policy preset init", "Choose a portable custom preset name."), declaredCommandError(fault.KindRejected, "policy_preset_exists", false, "policy preset list", "Choose another custom preset name.")),
		Mutation:      &MutationContract{TargetKind: tobari.PolicyPresetCatalogTargetKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}},
	}, handler: runPolicyPresetInit}
}
