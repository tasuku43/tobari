package authbroker

import "fmt"

const (
	BuiltinAnthropicProviderID = "anthropic"
	BuiltinAWSProviderID       = "aws"
	BuiltinChatworkProviderID  = "chatwork"
	BuiltinDatadogProviderID   = "datadog"
	BuiltinGitHubProviderID    = "github"
	BuiltinOpenAIProviderID    = "openai"
)

var builtinProviderIDs = [...]string{
	BuiltinAnthropicProviderID,
	BuiltinAWSProviderID,
	BuiltinChatworkProviderID,
	BuiltinDatadogProviderID,
	BuiltinGitHubProviderID,
	BuiltinOpenAIProviderID,
}

type reviewedLoginProvider struct {
	id     string
	helper string
}

// reviewedLoginProviders is the closed, presentation-ordered host-login
// vocabulary. Chatwork is a reviewed built-in provider, but deliberately uses
// protected stdin import and therefore does not belong to this registry.
var reviewedLoginProviders = [...]reviewedLoginProvider{
	{id: BuiltinGitHubProviderID, helper: "github-gh"},
	{id: BuiltinAWSProviderID, helper: "aws-sso"},
	{id: BuiltinDatadogProviderID, helper: "pup-oauth"},
	{id: BuiltinOpenAIProviderID, helper: "codex-chatgpt-oauth"},
	{id: BuiltinAnthropicProviderID, helper: "claude-native-oauth"},
}

// BuiltinProviderIDs returns the complete closed built-in vocabulary in
// deterministic ID order. Each call returns a new slice so callers cannot
// mutate the domain-owned registry.
func BuiltinProviderIDs() []string {
	result := make([]string, len(builtinProviderIDs))
	copy(result, builtinProviderIDs[:])
	return result
}

// ReviewedLoginProviderIDs returns the closed provider union in the public
// login-selector order. Each call returns a new slice.
func ReviewedLoginProviderIDs() []string {
	result := make([]string, len(reviewedLoginProviders))
	for index, provider := range reviewedLoginProviders {
		result[index] = provider.id
	}
	return result
}

// ReviewedLoginProviderHelper returns the exact reviewed manifest helper for
// one compiled host-login provider.
func ReviewedLoginProviderHelper(providerID string) (string, bool) {
	for _, provider := range reviewedLoginProviders {
		if provider.id == providerID {
			return provider.helper, true
		}
	}
	return "", false
}

// SupportsReviewedLoginProvider reports membership in the closed compiled
// host-login provider union. It does not consult owner manifests or runtime
// registration.
func SupportsReviewedLoginProvider(providerID string) bool {
	_, found := ReviewedLoginProviderHelper(providerID)
	return found
}

// ValidateBuiltinProviderCollection binds the embedded manifest collection to
// the complete domain vocabulary and the reviewed host-login helper plans.
// Owner manifests do not cross this boundary.
func ValidateBuiltinProviderCollection(providers []Provider) error {
	expected := make(map[string]struct{}, len(builtinProviderIDs))
	for _, providerID := range builtinProviderIDs {
		expected[providerID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if _, known := expected[provider.ID]; !known {
			return fmt.Errorf("built-in provider collection contains unregistered provider %q", provider.ID)
		}
		if _, duplicate := seen[provider.ID]; duplicate {
			return fmt.Errorf("built-in provider collection duplicates provider %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}

		helper, supportsLogin := ReviewedLoginProviderHelper(provider.ID)
		if supportsLogin {
			if provider.Acquisition != (Acquisition{Mode: AcquisitionBuiltinHelper, Helper: helper}) {
				return fmt.Errorf("built-in provider %q does not declare its reviewed login helper %q", provider.ID, helper)
			}
			continue
		}
		if provider.Acquisition.Mode == AcquisitionBuiltinHelper {
			return fmt.Errorf("built-in provider %q declares a helper outside the reviewed login registry", provider.ID)
		}
	}
	for _, providerID := range builtinProviderIDs {
		if _, found := seen[providerID]; !found {
			return fmt.Errorf("built-in provider collection is missing registered provider %q", providerID)
		}
	}
	return nil
}
