package authbroker

import (
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
)

const (
	BuiltinAnthropicProviderID = "anthropic"
	BuiltinAWSProviderID       = "aws"
	BuiltinChatworkProviderID  = "chatwork"
	BuiltinDatadogProviderID   = "datadog"
	BuiltinGitHubProviderID    = "github"
	BuiltinOpenAIProviderID    = "openai"
)

var knownBuiltinProviderIDs = [...]string{
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
var knownReviewedLoginProviders = [...]reviewedLoginProvider{
	{id: BuiltinGitHubProviderID, helper: "github-gh"},
	{id: BuiltinAWSProviderID, helper: "aws-sso"},
	{id: BuiltinDatadogProviderID, helper: "pup-oauth"},
	{id: BuiltinOpenAIProviderID, helper: "codex-chatgpt-oauth"},
	{id: BuiltinAnthropicProviderID, helper: "claude-native-oauth"},
}

// BuiltinProviderIDs returns the complete implementation vocabulary. It is a
// validation and reservation boundary, not the active product surface.
func BuiltinProviderIDs() []string {
	result := make([]string, len(knownBuiltinProviderIDs))
	copy(result, knownBuiltinProviderIDs[:])
	return result
}

// ActiveBuiltinProviderIDs returns the built-ins exposed by this immutable
// capability profile. AWS is experimental and cannot be activated at runtime.
func ActiveBuiltinProviderIDs() []string {
	result := make([]string, 0, len(knownBuiltinProviderIDs))
	for _, providerID := range knownBuiltinProviderIDs {
		if providerID == BuiltinAWSProviderID && !capabilityprofile.Compiled().IncludesExperimental() {
			continue
		}
		result = append(result, providerID)
	}
	return result
}

// ReviewedLoginProviderIDs returns the active closed provider union in public
// selector order. Each call returns a new slice.
func ReviewedLoginProviderIDs() []string {
	result := make([]string, 0, len(knownReviewedLoginProviders))
	for _, provider := range knownReviewedLoginProviders {
		if provider.id == BuiltinAWSProviderID && !capabilityprofile.Compiled().IncludesExperimental() {
			continue
		}
		result = append(result, provider.id)
	}
	return result
}

// KnownReviewedLoginProviderIDs returns the complete reviewed implementation
// union used to validate embedded manifests and cross-language fixtures.
func KnownReviewedLoginProviderIDs() []string {
	result := make([]string, len(knownReviewedLoginProviders))
	for index, provider := range knownReviewedLoginProviders {
		result[index] = provider.id
	}
	return result
}

// ReviewedLoginProviderHelper returns the exact reviewed manifest helper for
// one compiled host-login provider.
func ReviewedLoginProviderHelper(providerID string) (string, bool) {
	if providerID == BuiltinAWSProviderID && !capabilityprofile.Compiled().IncludesExperimental() {
		return "", false
	}
	return KnownReviewedLoginProviderHelper(providerID)
}

// KnownReviewedLoginProviderHelper returns the reviewed helper for embedded
// implementation validation independent of the active product profile.
func KnownReviewedLoginProviderHelper(providerID string) (string, bool) {
	for _, provider := range knownReviewedLoginProviders {
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
	expected := make(map[string]struct{}, len(knownBuiltinProviderIDs))
	for _, providerID := range knownBuiltinProviderIDs {
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

		helper, supportsLogin := KnownReviewedLoginProviderHelper(provider.ID)
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
	for _, providerID := range knownBuiltinProviderIDs {
		if _, found := seen[providerID]; !found {
			return fmt.Errorf("built-in provider collection is missing registered provider %q", providerID)
		}
	}
	return nil
}
