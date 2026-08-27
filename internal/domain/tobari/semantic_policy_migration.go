package tobari

import "fmt"

func validateV1ConvertibleBoundary(boundary WorkspaceTemplateBoundary) error {
	if boundary.DestinationCeiling.Mode != "public_https" || len(boundary.DestinationCeiling.Authorities) != 0 {
		return fmt.Errorf("alpha policy uses an exact destination ceiling; edit the V1 source manually")
	}
	if boundary.MethodPolicy.Default != ManifestMethodExactReview {
		return fmt.Errorf("alpha policy uses a method default that V1 cannot represent")
	}
	for _, override := range boundary.MethodPolicy.Overrides {
		if override.Decision != ManifestMethodDeny {
			return fmt.Errorf("alpha policy uses a method override that V1 cannot represent")
		}
	}
	return nil
}

// CompileWorkspaceTemplateBodyV1 returns the exact canonical body published by
// the V1 file-backed Template compiler. Built-in Templates use this before
// draft creation so the reviewed generation-1 body and the body reconstructed
// from template.yaml + policy.yaml have one revision identity.
func CompileWorkspaceTemplateBodyV1(body WorkspaceTemplateBody) (WorkspaceTemplateBody, error) {
	if err := body.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	if err := validateV1ConvertibleBoundary(body.Boundary); err != nil {
		return WorkspaceTemplateBody{}, err
	}

	modules := WorkspaceTemplateSemanticModules{}
	if body.Policy.SemanticModules == nil {
		converted, err := migrateAlphaPolicyBody(body.Policy)
		if err != nil {
			return WorkspaceTemplateBody{}, err
		}
		modules = converted
	} else {
		modules = body.Policy.SemanticModules.Normalize()
	}
	denied := deniedMethodsFromPolicy(body.Boundary.MethodPolicy)
	overrides := make([]ManifestMethodOverride, 0, len(denied))
	for _, method := range denied {
		overrides = append(overrides, ManifestMethodOverride{Method: method, Decision: ManifestMethodDeny})
	}
	result := body.Clone()
	result.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	result.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: overrides}
	result.Policy = WorkspaceTemplatePolicyBody{
		AgentProfile: body.Policy.AgentProfile, NativeReadiness: body.Policy.NativeReadiness,
		SemanticModules: &modules,
		BaselineGrants:  []ManifestPolicyExactRule{}, BaselineTemplates: []ManifestPolicyPathTemplateRule{},
		MCPBaselineGrants: []ManifestPolicyMCPRule{}, BaselineDenies: []ManifestPolicyExactRule{},
		GraphQLEndpoints: []ManifestPolicyExactRule{}, MCPEndpoints: []ManifestPolicyExactRule{},
	}
	if err := result.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	return result, nil
}

// migrateAlphaPolicyBody performs the lossless policy-language portion of the
// explicit alpha-to-V1 source migration. It never activates the result.
func migrateAlphaPolicyBody(policy WorkspaceTemplatePolicyBody) (WorkspaceTemplateSemanticModules, error) {
	if policy.SemanticModules != nil {
		return WorkspaceTemplateSemanticModules{}, fmt.Errorf("alpha policy contains final semantic_modules; edit the V1 source manually")
	}
	// Alpha exact denies applied before protocol refinement and therefore also
	// denied classified traffic with the same transport coordinates. V1 generic
	// HTTP denies intentionally exclude classified traffic, so translating an
	// alpha deny into that module would silently widen authority.
	if len(policy.BaselineDenies) != 0 {
		return WorkspaceTemplateSemanticModules{}, fmt.Errorf("alpha exact Deny cannot migrate losslessly; edit the V1 source manually")
	}
	modules := EmptyWorkspaceTemplateSemanticModules()
	authority := func(scheme, host string, port int) SemanticRuleAuthority {
		return SemanticRuleAuthority{Scheme: scheme, Host: host, Port: port}
	}
	endpoint := func(rule ManifestPolicyExactRule) SemanticHTTPEndpoint {
		return SemanticHTTPEndpoint{SemanticRuleAuthority: authority(rule.Scheme, rule.Host, rule.Port), Path: rule.Path}
	}
	for _, rule := range policy.BaselineGrants {
		switch rule.Protocol {
		case "":
			modules.Protocols.HTTP.Generic.Allow.Rules = append(modules.Protocols.HTTP.Generic.Allow.Rules, SemanticHTTPRule{
				SemanticRuleAuthority: authority(rule.Scheme, rule.Host, rule.Port), Method: rule.Method, Path: rule.Path,
			})
		case PolicyProtocolGraphQL:
			modules.Protocols.HTTP.GraphQL.Allow.Rules = append(modules.Protocols.HTTP.GraphQL.Allow.Rules, SemanticGraphQLRule{
				SemanticRuleAuthority: authority(rule.Scheme, rule.Host, rule.Port), Path: rule.Path,
				OperationType: rule.GraphQLOperationType, RootField: rule.GraphQLRootField,
			})
		default:
			return WorkspaceTemplateSemanticModules{}, fmt.Errorf("alpha grant protocol cannot migrate to V1")
		}
	}
	for _, rule := range policy.BaselineTemplates {
		modules.Protocols.HTTP.Generic.Allow.Rules = append(modules.Protocols.HTTP.Generic.Allow.Rules, SemanticHTTPRule{
			SemanticRuleAuthority: authority(rule.Scheme, rule.Host, rule.Port), Method: rule.Method, Path: rule.Path,
		})
	}
	for _, rule := range policy.MCPBaselineGrants {
		modules.Protocols.HTTP.MCP.Allow.Rules = append(modules.Protocols.HTTP.MCP.Allow.Rules, SemanticMCPRule{
			SemanticRuleAuthority: authority(rule.Scheme, rule.Host, rule.Port), Path: rule.Path,
			Method: rule.MCPMethod, ToolName: rule.MCPToolName,
		})
	}
	for _, rule := range policy.GraphQLEndpoints {
		modules.Protocols.HTTP.GraphQL.Endpoints = append(modules.Protocols.HTTP.GraphQL.Endpoints, endpoint(rule))
	}
	for _, rule := range policy.MCPEndpoints {
		modules.Protocols.HTTP.MCP.Endpoints = append(modules.Protocols.HTTP.MCP.Endpoints, endpoint(rule))
	}
	if err := modules.Validate(nil); err != nil {
		return WorkspaceTemplateSemanticModules{}, fmt.Errorf("alpha policy cannot migrate to V1: %w", err)
	}
	return modules.Normalize(), nil
}
