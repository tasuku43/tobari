package tobari

// These aliases keep the unpublished predecessor migration adapter buildable
// while the Domain Model V1 cutover and its migration are reviewed as separate
// commits. The migration commit removes them with the predecessor decoder
// update; they are not public CLI or persisted-schema compatibility.
type (
	ContextBootstrapSnapshot        = ManifestBootstrapSnapshot
	ContextGitIdentitySetting       = ManifestGitIdentitySetting
	ContextManifest                 = WorkspaceManifest
	ContextMethodPolicy             = ManifestMethodPolicy
	ContextNativeReadiness          = ManifestNativeReadiness
	ContextPolicy                   = ManifestPolicy
	ContextPolicyDestinationCeiling = ManifestPolicyDestinationCeiling
	ContextPolicyExactRule          = ManifestPolicyExactRule
	ContextPolicyMCPRule            = ManifestPolicyMCPRule
	ContextPolicyMode               = ManifestPolicyMode
	ContextPolicyPathTemplateRule   = ManifestPolicyPathTemplateRule
	ContextRuntimeRecipe            = ManifestRuntimeRecipe
	ContextShellEnvironmentSetting  = ManifestShellEnvironmentSetting
	ContextSourceAccess             = ManifestSourceAccess
	RuntimeSourceBase               = RuntimeCopySource
)
