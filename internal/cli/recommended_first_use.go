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
	session := "Open Bash"
	if draft.Session.Kind == tobari.RecommendedFirstUseSessionDirect {
		session = "Run " + draft.Session.Executable + " directly"
	}
	return configurationWizardMenu{
		title: "Tobari will create an isolated Workspace for:",
		information: []string{
			"  " + draft.ProjectRoot, "",
			"Project files", "  Read-write · changes are made directly", "",
			"Network", "  Claude Code and Codex routine traffic   allowed",
			"  Other requests                          exact review",
			"  Private and unsafe destinations         denied", "",
			"Tools", "  " + draft.RuntimeSelection, "",
			"Host configuration", "  Not imported", "",
			"Session", "  " + session,
		},
		prompt: "Action",
		options: []configurationWizardOption{
			{label: "Start Workspace", value: "start"},
			{label: "Customize", value: "customize"},
			{label: "Cancel", value: "cancel"},
		},
	}
}

func recommendedFirstUseSeed(draft tobari.RecommendedFirstUseDraft) contextCreateWizardSeed {
	composition := draft.Composition()
	return contextCreateWizardSeed{Selection: contextCreateSelection{
		Name: draft.ContextName, RuntimeSelection: composition.RuntimeSelection,
		SourceAccess: draft.Access.SourceAccess, NativeReadiness: composition.NativeReadiness,
		MethodPolicy: draft.Access.MethodPolicy.Clone(),
	}}
}
