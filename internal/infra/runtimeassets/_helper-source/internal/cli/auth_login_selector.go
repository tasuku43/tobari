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
		current:     "Choose a provider first. A configured provider will rotate its Context grant after successful login.",
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
	option := configurationWizardOption{label: safeExternalText(provider), value: provider}
	if provider == authcmd.BuiltinGitHubProviderID {
		option = configurationWizardOption{
			label: "GitHub", description: "Use the reviewed GitHub CLI (gh) device flow.", value: provider,
		}
	}
	if status.State == authbroker.ProviderCredentialConfigured {
		option.label += " (configured)"
		option.description += " Selecting it rotates the Context grant and revokes previous Workspace handles after login succeeds."
	}
	return option
}
