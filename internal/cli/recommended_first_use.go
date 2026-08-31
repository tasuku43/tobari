package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type recommendedFirstUseAction uint8

const (
	recommendedFirstUseStart recommendedFirstUseAction = iota
	recommendedFirstUseCustomize
	recommendedFirstUseCancel
)

type recommendedFirstUseReviewer interface {
	Review(context.Context, tobari.RecommendedFirstUseDraft, io.Reader, io.Writer) (recommendedFirstUseAction, error)
}

type terminalRecommendedFirstUseReviewer struct {
	chooser *terminalContextConfigurationWizard
}

func newRecommendedFirstUseReviewerWithStyle(style bool) *terminalRecommendedFirstUseReviewer {
	return &terminalRecommendedFirstUseReviewer{chooser: newContextConfigurationWizardWithStyle(style)}
}

func (r *terminalRecommendedFirstUseReviewer) Review(
	ctx context.Context, draft tobari.RecommendedFirstUseDraft, in io.Reader, out io.Writer,
) (recommendedFirstUseAction, error) {
	if err := draft.Validate(); err != nil {
		return recommendedFirstUseCancel, err
	}
	selected, err := r.chooser.choose(ctx, in, out, recommendedFirstUseMenu(draft))
	if err != nil {
		return recommendedFirstUseCancel, err
	}
	switch selected {
	case 0:
		return recommendedFirstUseStart, nil
	case 1:
		return recommendedFirstUseCustomize, nil
	case 2:
		return recommendedFirstUseCancel, nil
	default:
		return recommendedFirstUseCancel, fmt.Errorf("recommended first-use action is invalid")
	}
}

func recommendedFirstUseMenu(draft tobari.RecommendedFirstUseDraft) configurationWizardMenu {
	session := recommendedFirstUseSession(draft)
	return configurationWizardMenu{
		title:            "Tobari · New Workspace",
		informationLines: recommendedFirstUseSummary(draft.ProjectRoot, draft.RuntimeSelection, session),
		prompt:           "Action",
		options: []configurationWizardOption{
			{label: "Create and enter Workspace", value: "start"},
			{label: "Customize", value: "customize"},
			{label: "Cancel", value: "cancel"},
		},
	}
}

func recommendedFirstUseSession(draft tobari.RecommendedFirstUseDraft) string {
	if draft.Session.Kind == tobari.RecommendedFirstUseSessionDirect {
		return "Run " + draft.Session.Executable + " directly"
	}
	return "Open Bash"
}

func recommendedFirstUseSummaryRow(label, value string, token styleToken) configurationWizardLine {
	return configurationWizardLine{
		{value: fmt.Sprintf("  %-20s", label), token: styleMuted},
		{value: value, token: token},
	}
}

func recommendedFirstUseSummary(projectRoot, runtimeSelection, session string) []configurationWizardLine {
	return []configurationWizardLine{
		{{value: "", token: styleText}},
		{{value: "Project", token: styleText}},
		{{value: "  " + safeExternalText(projectRoot), token: styleText}},
		{{value: "", token: styleText}},
		{{value: "Boundary", token: styleText}},
		recommendedFirstUseSummaryRow("Project files", "read-write", styleText),
		recommendedFirstUseSummaryRow("Reviewed traffic", "allowed", styleSuccess),
		recommendedFirstUseSummaryRow("New public traffic", "review when needed", styleWarning),
		recommendedFirstUseSummaryRow("Local/private", "denied", styleDanger),
		recommendedFirstUseSummaryRow("Host loopback", "explicit review only", styleWarning),
		{{value: "", token: styleText}},
		{{value: "Environment", token: styleText}},
		recommendedFirstUseSummaryRow("Runtime", safeExternalText(runtimeSelection), styleText),
		recommendedFirstUseSummaryRow("Host configuration", "not imported", styleText),
		recommendedFirstUseSummaryRow("Session", safeExternalText(session), styleText),
	}
}

func recommendedFirstUseSeed(draft tobari.RecommendedFirstUseDraft) contextCreateWizardSeed {
	composition := draft.Composition()
	return contextCreateWizardSeed{Selection: contextCreateSelection{
		Name: draft.WorkspaceManifestName, RuntimeSelection: composition.RuntimeSelection,
		SourceAccess: draft.Access.SourceAccess, NativeReadiness: composition.NativeReadiness,
		MethodPolicy: draft.Access.MethodPolicy.Clone(),
	}}
}
