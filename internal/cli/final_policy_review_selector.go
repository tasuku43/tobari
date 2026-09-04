package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalPolicyReviewRawKind uint8

const (
	finalPolicyReviewRawCancel finalPolicyReviewRawKind = iota
	finalPolicyReviewRawRefresh
	finalPolicyReviewRawApply
	finalPolicyReviewRawResume
)

type finalPolicyReviewRawResult struct {
	kind   finalPolicyReviewRawKind
	staged map[string]tobari.PolicyMemoryDecision
	lines  int
}

type finalPolicyReviewChoice struct {
	label    string
	decision tobari.PolicyMemoryDecision
	back     bool
}

func selectFinalPolicyReviewRaw(
	ctx context.Context,
	snapshot tobari.PolicyMemoryReviewSnapshot,
	in io.Reader,
	out io.Writer,
	style bool,
) (finalPolicyReviewRawResult, error) {
	selected := 0
	staged := map[string]tobari.PolicyMemoryDecision{}
	lines := 0
	for {
		if len(snapshot.Items) > 0 && selected >= len(snapshot.Items) {
			selected = len(snapshot.Items) - 1
		}
		var err error
		lines, err = renderFinalPolicyReviewList(out, snapshot, selected, staged, lines, style)
		if err != nil {
			return finalPolicyReviewRawResult{lines: lines}, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return finalPolicyReviewRawResult{lines: lines}, err
		}
		switch key.kind {
		case selectorKeyUp:
			if selected > 0 {
				selected--
			}
		case selectorKeyDown:
			if selected+1 < len(snapshot.Items) {
				selected++
			}
		case selectorKeyHome:
			selected = 0
		case selectorKeyEnd:
			if len(snapshot.Items) > 0 {
				selected = len(snapshot.Items) - 1
			}
		case selectorKeyNumber:
			if key.index < len(snapshot.Items) {
				selected = key.index
			}
		case selectorKeyEnter, selectorKeyOpen:
			if len(snapshot.Items) == 0 {
				continue
			}
			decision, back, detailLines, detailErr := selectFinalPolicyReviewDetail(ctx, snapshot.Items[selected], in, out, lines, style)
			lines = detailLines
			if detailErr != nil {
				return finalPolicyReviewRawResult{lines: lines}, detailErr
			}
			if !back {
				staged[snapshot.Items[selected].ID] = decision
			}
		case selectorKeyAllow:
			if len(snapshot.Items) > 0 && finalPolicyReviewDecisionAllowed(snapshot.Items[selected], tobari.PolicyMemoryAllow) {
				staged[snapshot.Items[selected].ID] = tobari.PolicyMemoryAllow
			}
		case selectorKeyDeny:
			if len(snapshot.Items) > 0 && finalPolicyReviewDecisionAllowed(snapshot.Items[selected], tobari.PolicyMemoryDeny) {
				staged[snapshot.Items[selected].ID] = tobari.PolicyMemoryDeny
			}
		case selectorKeyClear:
			if len(snapshot.Items) > 0 {
				delete(staged, snapshot.Items[selected].ID)
			}
		case selectorKeyApply:
			if len(staged) == 0 {
				continue
			}
			apply, finalLines, finalErr := selectFinalPolicyReviewConfirmation(ctx, snapshot, staged, in, out, lines, style)
			lines = finalLines
			if finalErr != nil {
				return finalPolicyReviewRawResult{lines: lines}, finalErr
			}
			if apply {
				return finalPolicyReviewRawResult{kind: finalPolicyReviewRawApply, staged: staged, lines: lines}, nil
			}
		case selectorKeyResume:
			if snapshot.ReviewedApplyRecovery == nil {
				continue
			}
			apply, finalLines, finalErr := selectFinalPolicyReviewRecoveryConfirmation(ctx, snapshot, in, out, lines, style)
			lines = finalLines
			if finalErr != nil {
				return finalPolicyReviewRawResult{lines: lines}, finalErr
			}
			if apply {
				return finalPolicyReviewRawResult{kind: finalPolicyReviewRawResume, lines: lines}, nil
			}
		case selectorKeyReset:
			return finalPolicyReviewRawResult{kind: finalPolicyReviewRawRefresh, lines: lines}, nil
		case selectorKeyCancel:
			return finalPolicyReviewRawResult{kind: finalPolicyReviewRawCancel, lines: lines}, nil
		}
	}
}

func renderFinalPolicyReviewList(
	out io.Writer,
	snapshot tobari.PolicyMemoryReviewSnapshot,
	selected int,
	staged map[string]tobari.PolicyMemoryDecision,
	previousLines int,
	style bool,
) (int, error) {
	lines := []string{
		applyStyleToken(style, styleAccent, "Tobari · Permission Inbox"),
		"",
		fmt.Sprintf("%d requests need review", len(snapshot.Items)),
		"",
	}
	if len(snapshot.Items) == 0 {
		lines = append(lines, applyStyleToken(style, styleMuted, "· Nothing needs review."))
	} else {
		for index, item := range snapshot.Items {
			marker := "  "
			if index == selected {
				marker = applyStyleToken(style, styleAccent, "> ")
			}
			state, token := "Pending", styleMuted
			if decision, found := staged[item.ID]; found {
				if decision == tobari.PolicyMemoryDeny {
					state, token = "Remember deny", styleWarning
				} else if item.Match == tobari.PolicyMatchPathTemplate {
					state, token = "Allow matching paths", styleSuccess
				} else {
					state, token = "Allow", styleSuccess
				}
			}
			lines = append(lines,
				marker+applyStyleToken(style, token, padFinalPolicyReviewState(state))+"  "+safeExternalText(item.Template)+" · "+safeExternalText(finalPolicyReviewContextRef(item)),
				"    "+safeExternalText(item.ProjectRoot),
				"    "+finalPolicyEffectSummary(item.Rule.PolicyProtocolIdentity, item.Rule.Method, item.Rule.Host, item.Rule.Port, item.Rule.Path),
			)
		}
	}
	if snapshot.ReviewedApplyRecovery != nil {
		lines = append(lines, "", applyStyleToken(style, styleWarning, "Interrupted reviewed Apply is ready to resume."))
	}
	lines = append(lines, "", applyStyleToken(style, styleMuted, "↑↓ move · enter review · a allow · d deny · x clear"))
	footer := "p review staged · r refresh · q cancel"
	if snapshot.ReviewedApplyRecovery != nil {
		footer = "u resume confirmed Apply · " + footer
	}
	lines = append(lines, applyStyleToken(style, styleMuted, footer))
	return renderSelectorScreen(out, lines, previousLines)
}

func selectFinalPolicyReviewRecoveryConfirmation(
	ctx context.Context,
	snapshot tobari.PolicyMemoryReviewSnapshot,
	in io.Reader,
	out io.Writer,
	previousLines int,
	style bool,
) (bool, int, error) {
	if snapshot.ReviewedApplyRecovery == nil {
		return false, previousLines, fmt.Errorf("Permission Inbox recovery authority is missing")
	}
	frame := []string{
		applyStyleToken(style, styleAccent, "Tobari · Resume Reviewed Apply"),
		"",
		fmt.Sprintf("%d already-confirmed decisions", len(snapshot.ReviewedApplyRecovery.DecisionSet.Decisions)),
		"This resumes the preserved exact Apply; it does not stage current candidates.",
		"",
		applyStyleToken(style, styleAccent, "> Resume confirmed Apply"),
		"  Back",
		"",
		applyStyleToken(style, styleMuted, "y resume · b back · q cancel"),
	}
	lines, err := renderSelectorScreen(out, frame, previousLines)
	if err != nil {
		return false, lines, err
	}
	for {
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return false, lines, err
		}
		switch key.kind {
		case selectorKeyConfirm, selectorKeyEnter, selectorKeyResume:
			return true, lines, nil
		case selectorKeyBack:
			return false, lines, nil
		case selectorKeyCancel:
			return false, lines, context.Canceled
		}
	}
}

func padFinalPolicyReviewState(value string) string {
	const width = 20
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func selectFinalPolicyReviewDetail(
	ctx context.Context,
	item tobari.PolicyMemoryReviewItem,
	in io.Reader,
	out io.Writer,
	previousLines int,
	style bool,
) (tobari.PolicyMemoryDecision, bool, int, error) {
	choices := finalPolicyReviewChoices(item)
	selected := 0
	lines := previousLines
	for {
		frame := []string{
			applyStyleToken(style, styleAccent, "Tobari · Review Permission"),
			"",
			"Context   " + safeExternalText(finalPolicyReviewContextRef(item)),
		}
		if item.Match == tobari.PolicyMatchExact {
			frame = append(frame, "Project   "+safeExternalText(item.ProjectRoot))
		}
		frame = append(frame,
			"Template  "+safeExternalText(item.Template),
			"Match     "+safeExternalText(string(item.Match)),
			"Request   "+finalPolicyEffectSummary(item.Rule.PolicyProtocolIdentity, item.Rule.Method, item.Rule.Host, item.Rule.Port, item.Rule.Path),
			"",
		)
		for index, choice := range choices {
			marker := "  "
			if index == selected {
				marker = applyStyleToken(style, styleAccent, "> ")
			}
			frame = append(frame, marker+choice.label)
		}
		frame = append(frame, "", applyStyleToken(style, styleMuted, "↑↓ move · enter choose · b back · q cancel"))
		var err error
		lines, err = renderSelectorScreen(out, frame, lines)
		if err != nil {
			return "", false, lines, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return "", false, lines, err
		}
		switch key.kind {
		case selectorKeyUp:
			if selected > 0 {
				selected--
			}
		case selectorKeyDown:
			if selected+1 < len(choices) {
				selected++
			}
		case selectorKeyAllow:
			if finalPolicyReviewDecisionAllowed(item, tobari.PolicyMemoryAllow) {
				return tobari.PolicyMemoryAllow, false, lines, nil
			}
		case selectorKeyDeny:
			if finalPolicyReviewDecisionAllowed(item, tobari.PolicyMemoryDeny) {
				return tobari.PolicyMemoryDeny, false, lines, nil
			}
		case selectorKeyBack:
			return "", true, lines, nil
		case selectorKeyCancel:
			return "", false, lines, context.Canceled
		case selectorKeyEnter:
			choice := choices[selected]
			return choice.decision, choice.back, lines, nil
		}
	}
}

func finalPolicyReviewContextRef(item tobari.PolicyMemoryReviewItem) string {
	reference, err := tobari.ContextRef(item.ContextID)
	if err != nil {
		return string(item.ContextID)
	}
	return reference
}

func finalPolicyReviewChoices(item tobari.PolicyMemoryReviewItem) []finalPolicyReviewChoice {
	choices := make([]finalPolicyReviewChoice, 0, 3)
	if finalPolicyReviewDecisionAllowed(item, tobari.PolicyMemoryAllow) {
		label := "Allow this exact request"
		if item.Match == tobari.PolicyMatchPathTemplate {
			label = "Allow matching paths"
		}
		choices = append(choices, finalPolicyReviewChoice{label: label, decision: tobari.PolicyMemoryAllow})
	}
	if finalPolicyReviewDecisionAllowed(item, tobari.PolicyMemoryDeny) {
		choices = append(choices, finalPolicyReviewChoice{label: "Remember deny", decision: tobari.PolicyMemoryDeny})
	}
	return append(choices, finalPolicyReviewChoice{label: "Back", back: true})
}

func finalPolicyReviewDecisionAllowed(item tobari.PolicyMemoryReviewItem, decision tobari.PolicyMemoryDecision) bool {
	if item.AttachmentCandidate != nil {
		return decision.Validate() == nil
	}
	_, err := item.ReviewedDecision(decision)
	return err == nil
}

func selectFinalPolicyReviewConfirmation(
	ctx context.Context,
	snapshot tobari.PolicyMemoryReviewSnapshot,
	staged map[string]tobari.PolicyMemoryDecision,
	in io.Reader,
	out io.Writer,
	previousLines int,
	style bool,
) (bool, int, error) {
	lines := previousLines
	for {
		frame := []string{
			applyStyleToken(style, styleAccent, "Tobari · Review Decisions"),
			"",
			fmt.Sprintf("%d staged decisions", len(staged)),
			"",
		}
		for _, item := range snapshot.Items {
			decision, found := staged[item.ID]
			if !found {
				continue
			}
			label, token := "Allow", styleSuccess
			if decision == tobari.PolicyMemoryDeny {
				label, token = "Remember deny", styleWarning
			} else if item.Match == tobari.PolicyMatchPathTemplate {
				label = "Allow matching paths"
			}
			frame = append(frame, applyStyleToken(style, token, label)+"  "+safeExternalText(item.Template))
			frame = append(frame, "  "+finalPolicyEffectSummary(item.Rule.PolicyProtocolIdentity, item.Rule.Method, item.Rule.Host, item.Rule.Port, item.Rule.Path))
		}
		frame = append(frame,
			"",
			applyStyleToken(style, styleMuted, "Staging has not changed the active policy."),
			"",
			applyStyleToken(style, styleAccent, "> Apply reviewed decisions"),
			"  Back",
			"  Discard and cancel",
			"",
			applyStyleToken(style, styleMuted, "y apply · b back · q discard and cancel"),
		)
		var err error
		lines, err = renderSelectorScreen(out, frame, lines)
		if err != nil {
			return false, lines, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return false, lines, err
		}
		switch key.kind {
		case selectorKeyConfirm, selectorKeyEnter, selectorKeyApply:
			return true, lines, nil
		case selectorKeyBack:
			return false, lines, nil
		case selectorKeyCancel:
			return false, lines, context.Canceled
		}
	}
}
