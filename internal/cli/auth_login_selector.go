package cli

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

type authLoginProviderSelector interface {
	Select(context.Context, string, []authbroker.ProviderStatus, io.Reader, io.Writer) (string, error)
}

type terminalAuthLoginProviderSelector struct {
	wizard *terminalContextConfigurationWizard
}

func newAuthLoginProviderSelector() *terminalAuthLoginProviderSelector {
	return newAuthLoginProviderSelectorWithStyle(true)
}

func newAuthLoginProviderSelectorWithStyle(enabled bool) *terminalAuthLoginProviderSelector {
	return &terminalAuthLoginProviderSelector{
		wizard: newContextConfigurationWizardWithStyle(enabled),
	}
}

func (s *terminalAuthLoginProviderSelector) Select(
	ctx context.Context, contextName string, providers []authbroker.ProviderStatus, in io.Reader, out io.Writer,
) (string, error) {
	options := make([]configurationWizardOption, 0, len(providers))
	for _, provider := range providers {
		options = append(options, authLoginProviderOption(provider))
	}
	index, err := s.wizard.choose(ctx, in, out, configurationWizardMenu{
		title:       "Tobari · Provider login",
		contextName: contextName,
		current:     "Choose a provider first. Configured rows will rotate the grant and revoke its previous Workspace handles after successful login.",
		prompt:      "Choose a provider",
		options:     options,
	})
	if err != nil {
		return "", err
	}
	return options[index].value, nil
}

func authLoginProviderOption(status authbroker.ProviderStatus) configurationWizardOption {
	provider := status.Provider
	var option configurationWizardOption
	switch provider {
	case authcmd.BuiltinGitHubProviderID:
		option = configurationWizardOption{
			label: "GitHub", description: "Tool: GitHub CLI (gh), selected automatically. Login: reviewed device flow.", value: provider,
		}
	case authcmd.BuiltinAWSProviderID:
		option = configurationWizardOption{
			label: "AWS", description: "Tool: AWS CLI (aws), selected automatically. Login: IAM Identity Center by default.", value: provider,
		}
	case authcmd.BuiltinDatadogProviderID:
		option = configurationWizardOption{
			label: "Datadog", description: "Tool: pup, selected automatically. Login: reviewed US1 OAuth flow.", value: provider,
		}
	default:
		option = configurationWizardOption{label: safeExternalText(provider), value: provider}
	}
	if status.State == authbroker.ProviderCredentialConfigured {
		option.label += " (configured)"
		option.description += " Selecting it rotates the Context grant and revokes previous Workspace handles after login succeeds."
	}
	return option
}
