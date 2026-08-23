package cli

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeChoice uint8

const (
	runtimeChoiceStandard runtimeChoice = iota
	runtimeChoiceCustomize
)

type runtimeChoiceWizard interface {
	Choose(context.Context, tobari.ManifestReport, io.Reader, io.Writer) (runtimeChoice, error)
}

type terminalRuntimeChoiceWizard struct {
	chooser *terminalContextConfigurationWizard
}

func newRuntimeChoiceWizardWithStyle(style bool) *terminalRuntimeChoiceWizard {
	return &terminalRuntimeChoiceWizard{chooser: newContextConfigurationWizardWithStyle(style)}
}

func (w *terminalRuntimeChoiceWizard) Choose(
	ctx context.Context, report tobari.ManifestReport, in io.Reader, out io.Writer,
) (runtimeChoice, error) {
	chooser := w.chooser
	if chooser == nil {
		chooser = &terminalContextConfigurationWizard{}
	}
	selected, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title:       "Tobari · Runtime",
		contextName: report.Name,
		current:     "standard Tobari runtime selected",
		information: []string{"The standard runtime is ready to create the first Workspace."},
		prompt:      "Continue",
		options: []configurationWizardOption{
			{label: "Enter with the standard runtime", description: "Create the first Workspace and enter it now."},
			{label: "Customize before creating the first Workspace", description: "Create a Dockerfile recipe, then build it explicitly."},
		},
	})
	if err != nil {
		return runtimeChoiceStandard, err
	}
	if selected == 1 {
		return runtimeChoiceCustomize, nil
	}
	return runtimeChoiceStandard, nil
}
