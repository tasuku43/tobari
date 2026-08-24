package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type recommendedTemplateCustomization struct {
	sourceAccess    tobari.ManifestSourceAccess
	methodPolicy    tobari.ManifestMethodPolicy
	nativeReadiness tobari.ManifestNativeReadiness
	runtimeRef      string
	runtimeName     string
	runtimeOrdinal  int
}

func (c *CLI) customizeRecommendedTemplateBody(ctx context.Context, draft tobari.RecommendedFirstUseDraft) (tobari.WorkspaceTemplateBody, error) {
	if c == nil || c.runtime == nil {
		return tobari.WorkspaceTemplateBody{}, fault.New(fault.KindInternal, "missing_runtime", "Runtime authority is unavailable", false)
	}
	catalog, err := c.runtime.ListAuthority(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	ready := make([]tobari.RuntimeSummary, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		if item.Ready {
			ready = append(ready, item)
		}
	}
	if len(ready) == 0 {
		return tobari.WorkspaceTemplateBody{}, fault.WithClassification(fault.New(
			fault.KindRejected, "runtime_not_ready", "No immutable Runtime revision is ready for this Template.", false,
			fault.NextAction{Command: "review runtimes", Reason: "Review or build one Runtime before customizing first use."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	selection := recommendedTemplateCustomization{
		sourceAccess: draft.Access.SourceAccess, methodPolicy: draft.Access.MethodPolicy.Clone(),
		nativeReadiness: draft.NativeReadiness,
		runtimeRef:      ready[0].RuntimeRef, runtimeName: ready[0].Name, runtimeOrdinal: ready[0].Head,
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: !c.noColor}
	for {
		selected, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Customize Workspace setup", current: string(selection.sourceAccess), prompt: "Project files",
			options: []configurationWizardOption{
				{label: "Read-write", description: "Workspace tools can change Project files directly.", value: string(tobari.ManifestSourceAccessReadWrite)},
				{label: "Read-only", description: "Workspace tools can inspect but not change Project files.", value: string(tobari.ManifestSourceAccessReadOnly)},
			},
		})
		if chooseErr != nil {
			return tobari.WorkspaceTemplateBody{}, chooseErr
		}
		selection.sourceAccess = tobari.ManifestSourceAccess([]string{string(tobari.ManifestSourceAccessReadWrite), string(tobari.ManifestSourceAccessReadOnly)}[selected])

		selected, chooseErr = chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Customize Workspace setup", current: string(selection.methodPolicy.Default), prompt: "Other network requests",
			options: []configurationWizardOption{
				{label: "Exact review", description: "Ask for an exact decision when no reviewed rule applies.", value: string(tobari.ManifestMethodExactReview)},
				{label: "Deny", description: "Deny requests that have no reviewed rule.", value: string(tobari.ManifestMethodDeny)},
			},
		})
		if chooseErr != nil {
			return tobari.WorkspaceTemplateBody{}, chooseErr
		}
		selection.methodPolicy.Default = []tobari.ManifestMethodDecision{tobari.ManifestMethodExactReview, tobari.ManifestMethodDeny}[selected]

		selected, chooseErr = chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Customize Workspace setup", current: string(selection.nativeReadiness), prompt: "Native client sign-in",
			options: []configurationWizardOption{
				{label: "Enabled", description: "Native clients can keep their own credentials in this Workspace.", value: string(tobari.ManifestNativeReadinessEnabled)},
				{label: "Disabled", description: "Do not prepare native client sign-in paths.", value: string(tobari.ManifestNativeReadinessDisabled)},
			},
		})
		if chooseErr != nil {
			return tobari.WorkspaceTemplateBody{}, chooseErr
		}
		selection.nativeReadiness = []tobari.ManifestNativeReadiness{tobari.ManifestNativeReadinessEnabled, tobari.ManifestNativeReadinessDisabled}[selected]

		runtimeOptions := make([]configurationWizardOption, len(ready))
		for index, item := range ready {
			runtimeOptions[index] = configurationWizardOption{
				label:       fmt.Sprintf("%s@%d", safeExternalText(item.Name), item.Head),
				description: "Immutable Runtime revision ready for selection.", value: item.RuntimeRef,
			}
		}
		selected, chooseErr = chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Customize Workspace setup", current: fmt.Sprintf("%s@%d", selection.runtimeName, selection.runtimeOrdinal),
			prompt: "Runtime", options: runtimeOptions,
		})
		if chooseErr != nil {
			return tobari.WorkspaceTemplateBody{}, chooseErr
		}
		selection.runtimeRef, selection.runtimeName, selection.runtimeOrdinal = ready[selected].RuntimeRef, ready[selected].Name, ready[selected].Head

		review, reviewErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Review Workspace setup",
			information: []string{
				"  " + safeExternalText(draft.ProjectRoot), "",
				"Project files", "  " + string(selection.sourceAccess), "",
				"Other network requests", "  " + string(selection.methodPolicy.Default), "",
				"Native client sign-in", "  " + string(selection.nativeReadiness), "",
				"Runtime", fmt.Sprintf("  %s@%d", safeExternalText(selection.runtimeName), selection.runtimeOrdinal),
			},
			prompt: "Action", options: []configurationWizardOption{
				{label: "Use this setup", value: "use"},
				{label: "Edit again", value: "edit"},
				{label: "Cancel", value: "cancel"},
			},
		})
		if reviewErr != nil {
			return tobari.WorkspaceTemplateBody{}, reviewErr
		}
		if review == 1 {
			continue
		}
		if review == 2 {
			return tobari.WorkspaceTemplateBody{}, context.Canceled
		}
		break
	}

	body, err := c.reviewedStandardTemplateBody(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	binding, err := c.runtime.BindingByReference(ctx, selection.runtimeRef, selection.runtimeOrdinal)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	body.Boundary.SourceAccess = selection.sourceAccess
	body.Boundary.MethodPolicy = selection.methodPolicy.Clone()
	body.Policy.NativeReadiness = selection.nativeReadiness
	body.EntryDefaults.Runtime = binding
	if err := body.Validate(); err != nil {
		return tobari.WorkspaceTemplateBody{}, fault.WithClassification(fault.Wrap(
			fault.KindContract, "invalid_template_body", "The customized Workspace Template is invalid.", false, err,
			fault.NextAction{Command: "help template create", Reason: "Inspect the supported Template body contract."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	return body, nil
}
