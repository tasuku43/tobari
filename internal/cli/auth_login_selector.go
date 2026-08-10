package cli

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/app/authcmd"
)

type authLoginProviderSelector interface {
	Select(context.Context, string, []string, io.Reader, io.Writer) (string, error)
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
	ctx context.Context, contextName string, providers []string, in io.Reader, out io.Writer,
) (string, error) {
	options := make([]configurationWizardOption, 0, len(providers))
	for _, provider := range providers {
		options = append(options, authLoginProviderOption(provider))
	}
	index, err := s.wizard.choose(ctx, in, out, configurationWizardMenu{
		title:       "Tobari · Provider login",
		contextName: contextName,
		current:     "Choose a provider first. Its sole current Workspace client tool is selected automatically.",
		prompt:      "Choose a provider",
		options:     options,
	})
	if err != nil {
		return "", err
	}
	return options[index].value, nil
}

func authLoginProviderOption(provider string) configurationWizardOption {
	switch provider {
	case authcmd.BuiltinGitHubProviderID:
		return configurationWizardOption{
			label: "GitHub", description: "Tool: GitHub CLI (gh), selected automatically. Login: reviewed device flow.", value: provider,
		}
	case authcmd.BuiltinAWSProviderID:
		return configurationWizardOption{
			label: "AWS", description: "Tool: AWS CLI (aws), selected automatically. Login: IAM Identity Center by default.", value: provider,
		}
	case authcmd.BuiltinDatadogProviderID:
		return configurationWizardOption{
			label: "Datadog", description: "Tool: pup, selected automatically. Login: reviewed US1 OAuth flow.", value: provider,
		}
	default:
		return configurationWizardOption{label: safeExternalText(provider), value: provider}
	}
}
