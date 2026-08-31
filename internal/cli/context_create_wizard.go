package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const maxContextCreateNameBytes = 128

const (
	contextCreateMethodLabelWidth    = 25
	contextCreateMethodDecisionWidth = 15
)

var contextCreateHTTPMethods = []string{
	"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE",
}

type contextCreateSelection struct {
	CopyFrom            *tobari.ManifestCopySnapshot
	Name                string
	RuntimeSelection    string
	SourceAccess        tobari.ManifestSourceAccess
	NativeReadiness     tobari.ManifestNativeReadiness
	MethodPolicy        tobari.ManifestMethodPolicy
	AWSBootstrapProfile string
	EKSBootstrapContext string
	Bootstrap           *tobari.ManifestBootstrapSnapshot
}

type contextCreateWizard interface {
	Compose(context.Context, io.Reader, io.Writer) (contextCreateSelection, error)
}

type seededContextCreateWizard interface {
	ComposeSeeded(context.Context, io.Reader, io.Writer, contextCreateWizardSeed) (contextCreateSelection, error)
}

type contextCreateWizardSeed struct {
	Selection        contextCreateSelection
	NameProvided     bool
	FilesystemFilled bool
	NetworkFilled    bool
	RuntimeProvided  bool
	BootstrapFilled  bool
}

type terminalContextCreateWizard struct {
	mode      terminal.Mode
	style     bool
	bootstrap contextCreateBootstrapDiscovery
	runtimes  []tobari.RuntimeSummary
	bases     []tobari.ManifestSummary
	baseRead  interface {
		CopySnapshot(context.Context, string) (tobari.ManifestCopySnapshot, error)
	}
}

type contextCreateBootstrapDiscovery interface {
	DiscoverAWSBootstraps(context.Context) (tobari.ManifestAWSBootstrapDiscovery, error)
	DiscoverEKSBootstraps(context.Context, tobari.ManifestBootstrapSnapshot) (tobari.ManifestEKSBootstrapDiscovery, error)
	PrepareAWSBootstrap(context.Context, string) (tobari.ManifestBootstrapSnapshot, error)
	PrepareEKSBootstrap(context.Context, tobari.ManifestBootstrapSnapshot, string) (tobari.ManifestBootstrapSnapshot, error)
}

type contextCreateRawStep uint8

const (
	contextCreateStepName contextCreateRawStep = iota
	contextCreateStepFilesystem
	contextCreateStepNetwork
	contextCreateStepRuntime
	contextCreateStepBootstrap
	contextCreateStepReview
)

type contextCreateRawNavigation uint8

const (
	contextCreateNavigateNext contextCreateRawNavigation = iota
	contextCreateNavigateBack
	contextCreateNavigateCancel
)

type contextCreateRawDraft struct {
	base             *tobari.ManifestCopySnapshot
	name             string
	sourceIndex      int
	methodSelected   int
	methodDefault    tobari.ManifestMethodDecision
	methodOverrides  map[string]tobari.ManifestMethodDecision
	bootstrap        *tobari.ManifestBootstrapSnapshot
	runtimeSelection string
	nativeReadiness  tobari.ManifestNativeReadiness
	reviewAction     int
	reviewTop        int
	editSection      int
}

func newContextCreateWizardWithStyle(style bool) *terminalContextCreateWizard {
	return &terminalContextCreateWizard{mode: terminal.New(), style: style}
}

func (w *terminalContextCreateWizard) Compose(
	ctx context.Context, in io.Reader, out io.Writer,
) (contextCreateSelection, error) {
	return w.ComposeSeeded(ctx, in, out, contextCreateWizardSeed{})
}

func (w *terminalContextCreateWizard) ComposeSeeded(
	ctx context.Context, in io.Reader, out io.Writer, seed contextCreateWizardSeed,
) (contextCreateSelection, error) {
	if w != nil && w.mode != nil {
		restore, rawErr := w.mode.Enter(in)
		if rawErr == nil {
			selection, composeErr := w.composeRaw(ctx, in, out, seed)
			finishErr := finishSelectorScreen(out, 0)
			restoreErr := restore()
			if composeErr != nil {
				return contextCreateSelection{}, errors.Join(composeErr, finishErr, restoreErr)
			}
			if finishErr != nil {
				return contextCreateSelection{}, finishErr
			}
			if restoreErr != nil {
				return contextCreateSelection{}, restoreErr
			}
			return selection, nil
		}
	}
	return w.composeLine(ctx, in, out, seed)
}

func (w *terminalContextCreateWizard) composeLine(
	ctx context.Context, in io.Reader, out io.Writer, seed contextCreateWizardSeed,
) (contextCreateSelection, error) {
	draft := contextCreateDraftFromSeed(seed)
	if !seed.NameProvided {
		name, err := readContextCreateNameWithDefault(ctx, in, out, draft.name)
		if err != nil {
			return contextCreateSelection{}, err
		}
		draft.name = name
	}
	if !seed.FilesystemFilled {
		if err := editContextCreateFilesystemLine(ctx, in, out, w.style, &draft); err != nil {
			return contextCreateSelection{}, err
		}
	}
	if !seed.NetworkFilled {
		if err := editContextCreateNetworkLine(ctx, in, out, w.style, &draft); err != nil {
			return contextCreateSelection{}, err
		}
	}
	if !seed.RuntimeProvided {
		if err := w.editContextCreateRuntimeLine(ctx, in, out, &draft); err != nil {
			return contextCreateSelection{}, err
		}
	}
	if !seed.BootstrapFilled {
		if err := w.reviewContextCreateBootstrapLine(ctx, in, out, &draft); err != nil {
			return contextCreateSelection{}, err
		}
	}
	for {
		selection, err := contextCreateSelectionFromDraft(draft)
		if err != nil {
			return contextCreateSelection{}, err
		}
		action, err := reviewContextCreateLine(ctx, in, out, selection, w.style)
		if err != nil {
			return contextCreateSelection{}, err
		}
		switch action {
		case "create":
			changed, refreshed, err := w.revalidateBootstrap(ctx, selection.Bootstrap)
			if err != nil {
				if _, writeErr := fmt.Fprintln(out, "Host bootstrap configuration could not be revalidated. Workspace Manifest has not been created. Edit Workspace bootstrap or cancel."); writeErr != nil {
					return contextCreateSelection{}, writeErr
				}
				continue
			}
			if changed {
				draft.bootstrap = refreshed
				if _, err := fmt.Fprintln(out, "Host bootstrap configuration changed during review. Review the refreshed settings before creating."); err != nil {
					return contextCreateSelection{}, err
				}
				continue
			}
			return selection, nil
		case "edit":
			if err := w.editContextCreateSettingsLine(ctx, in, out, &draft); err != nil {
				return contextCreateSelection{}, err
			}
		case "cancel":
			return contextCreateSelection{}, context.Canceled
		}
	}
}

func (w *terminalContextCreateWizard) composeRaw(
	ctx context.Context, in io.Reader, out io.Writer, seed contextCreateWizardSeed,
) (contextCreateSelection, error) {
	draft := contextCreateDraftFromSeed(seed)
	step := firstContextCreateStep(seed)
	lineCount := 0
	for {
		var navigation contextCreateRawNavigation
		var err error
		switch step {
		case contextCreateStepName:
			draft.name, navigation, err = editContextCreateTextRaw(
				ctx, in, out, &lineCount, w.style, step, "Workspace Manifest name", draft.name,
				maxContextCreateNameBytes, "Must match [a-z][a-z0-9-]{0,62}.",
				func(value string) error { return tobari.ValidateName(strings.TrimSpace(value)) }, false,
			)
			draft.name = strings.TrimSpace(draft.name)
		case contextCreateStepFilesystem:
			draft.sourceIndex, navigation, err = editContextCreateChoiceRaw(
				ctx, in, out, &lineCount, w.style, step, draft.name,
				[]string{"Workspace home and tmpfs stay read-write in both choices."},
				"Project source access", []configurationWizardOption{
					{label: "Read-write", description: "Allow direct project-source changes.", value: string(tobari.ManifestSourceAccessReadWrite)},
					{label: "Read-only", description: "Prevent direct project-source changes.", value: string(tobari.ManifestSourceAccessReadOnly)},
				}, draft.sourceIndex,
			)
		case contextCreateStepNetwork:
			navigation, err = reviewContextCreateNetworkRaw(ctx, in, out, &lineCount, w.style, &draft)
		case contextCreateStepRuntime:
			navigation, err = w.editContextCreateRuntimeRaw(ctx, in, out, &lineCount, step, &draft)
		case contextCreateStepBootstrap:
			navigation, err = w.reviewContextCreateBootstrapRaw(ctx, in, out, &lineCount, step, &draft)
		case contextCreateStepReview:
			navigation, err = w.reviewContextCreateRaw(ctx, in, out, &lineCount, &draft)
		default:
			return contextCreateSelection{}, fmt.Errorf("Workspace Manifest creation wizard step is invalid")
		}
		if err != nil {
			return contextCreateSelection{}, err
		}
		if navigation == contextCreateNavigateCancel {
			return contextCreateSelection{}, context.Canceled
		}
		if navigation == contextCreateNavigateBack {
			if previous, ok := previousContextCreateStep(step, seed); ok {
				step = previous
			}
			continue
		}
		if step == contextCreateStepReview {
			break
		}
		step = nextContextCreateStep(step, seed)
	}

	return contextCreateSelectionFromDraft(draft)
}

func contextCreateDraftFromSeed(seed contextCreateWizardSeed) contextCreateRawDraft {
	draft := contextCreateRawDraft{
		base:             seed.Selection.CopyFrom,
		name:             seed.Selection.Name,
		sourceIndex:      0,
		runtimeSelection: seed.Selection.RuntimeSelection,
		nativeReadiness:  seed.Selection.NativeReadiness,
		methodDefault:    seed.Selection.MethodPolicy.Default,
		methodOverrides:  make(map[string]tobari.ManifestMethodDecision),
	}
	if draft.runtimeSelection == "" {
		draft.runtimeSelection = tobari.StandardRuntimeName
	}
	if draft.nativeReadiness == "" {
		draft.nativeReadiness = tobari.ManifestNativeReadinessEnabled
	}
	if draft.methodDefault == "" {
		draft.methodDefault = tobari.ManifestMethodExactReview
	}
	for _, override := range seed.Selection.MethodPolicy.Overrides {
		draft.methodOverrides[override.Method] = override.Decision
	}
	if seed.Selection.SourceAccess == tobari.ManifestSourceAccessReadOnly {
		draft.sourceIndex = 1
	}
	if seed.Selection.Bootstrap != nil {
		copy := seed.Selection.Bootstrap.Clone()
		draft.bootstrap = &copy
	}
	return draft
}

func resetContextCreateDraftBase(draft *contextCreateRawDraft, base *tobari.ManifestCopySnapshot) {
	name := draft.name
	reviewAction, reviewTop, editSection := draft.reviewAction, draft.reviewTop, draft.editSection
	seed := contextCreateWizardSeed{}
	if base != nil {
		copy := base.Clone()
		seed.Selection = contextCreateSelection{
			CopyFrom: &copy, RuntimeSelection: copy.RuntimeSelection,
			SourceAccess: copy.SourceAccess, NativeReadiness: copy.NativeReadiness,
			MethodPolicy: copy.MethodPolicy.Clone(),
		}
		if copy.Bootstrap != nil {
			bootstrap := copy.Bootstrap.Clone()
			seed.Selection.Bootstrap = &bootstrap
		}
	}
	replacement := contextCreateDraftFromSeed(seed)
	replacement.name = name
	replacement.reviewAction, replacement.reviewTop, replacement.editSection = reviewAction, reviewTop, editSection
	*draft = replacement
}

func firstContextCreateStep(seed contextCreateWizardSeed) contextCreateRawStep {
	for step := contextCreateStepName; step <= contextCreateStepReview; step++ {
		if contextCreateStepNeedsInput(step, seed) {
			return step
		}
	}
	return contextCreateStepReview
}

func nextContextCreateStep(current contextCreateRawStep, seed contextCreateWizardSeed) contextCreateRawStep {
	for step := current + 1; step <= contextCreateStepReview; step++ {
		if contextCreateStepNeedsInput(step, seed) {
			return step
		}
	}
	return contextCreateStepReview
}

func previousContextCreateStep(current contextCreateRawStep, seed contextCreateWizardSeed) (contextCreateRawStep, bool) {
	for candidate := current; candidate > contextCreateStepName; {
		candidate--
		if contextCreateStepNeedsInput(candidate, seed) {
			return candidate, true
		}
	}
	return current, false
}

func contextCreateStepNeedsInput(step contextCreateRawStep, seed contextCreateWizardSeed) bool {
	switch step {
	case contextCreateStepName:
		return !seed.NameProvided
	case contextCreateStepFilesystem:
		return !seed.FilesystemFilled
	case contextCreateStepNetwork:
		return !seed.NetworkFilled
	case contextCreateStepRuntime:
		return !seed.RuntimeProvided
	case contextCreateStepBootstrap:
		return !seed.BootstrapFilled
	case contextCreateStepReview:
		return true
	default:
		return false
	}
}

func editContextCreateTextRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	style bool,
	step contextCreateRawStep,
	label string,
	initial string,
	maxBytes int,
	helper string,
	validate func(string) error,
	backAllowed bool,
) (string, contextCreateRawNavigation, error) {
	value := initial
	message := ""
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			return "", contextCreateNavigateCancel, err
		}
		if needsRender {
			lines := []string{
				selectorTitle(style, "Tobari · Create Workspace Manifest"),
				selectorDetail(style, "Step", contextCreateStepLabel(step), styleText),
				"",
				applyStyleToken(style, styleText, label+":"),
				applyStyleToken(style, styleAccent, "> "+safeExternalText(value)+"▌"),
				selectorHelp(style, helper),
			}
			if message != "" {
				lines = append(lines, "", applyStyleToken(style, styleWarning, message))
			}
			controls := "Enter continue   Esc cancel"
			if backAllowed {
				controls = "Enter continue   Esc back   Ctrl-C cancel"
			}
			lines = append(lines, "", selectorHelp(style, controls))
			var err error
			*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
			if err != nil {
				return "", contextCreateNavigateCancel, err
			}
			needsRender = false
		}

		octet, err := readSelectorByte(ctx, in)
		if errors.Is(err, errSelectorTimeout) {
			continue
		}
		if errors.Is(err, errSelectorEOF) {
			return "", contextCreateNavigateCancel, context.Canceled
		}
		if err != nil {
			return "", contextCreateNavigateCancel, err
		}
		switch octet {
		case '\r', '\n':
			if validationErr := validate(value); validationErr != nil {
				message = validationErr.Error()
				needsRender = true
				continue
			}
			return value, contextCreateNavigateNext, nil
		case 3, 4:
			return "", contextCreateNavigateCancel, nil
		case 8, 127:
			if value != "" {
				_, size := utf8.DecodeLastRuneInString(value)
				value = value[:len(value)-size]
			}
			message = ""
			needsRender = true
		case 27:
			navigation, handled, escapeErr := readContextCreateTextEscape(ctx, in, backAllowed)
			if escapeErr != nil {
				return "", contextCreateNavigateCancel, escapeErr
			}
			if handled {
				return value, navigation, nil
			}
			message = "Use Backspace to edit the end of this value."
			needsRender = true
		default:
			if octet < 32 {
				message = "Control characters are not accepted."
				needsRender = true
				continue
			}
			if len(value) >= maxBytes {
				message = fmt.Sprintf("Input is limited to %d bytes.", maxBytes)
				needsRender = true
				continue
			}
			value += string(octet)
			message = ""
			needsRender = true
		}
	}
}

func readContextCreateTextEscape(
	ctx context.Context, in io.Reader, backAllowed bool,
) (contextCreateRawNavigation, bool, error) {
	next, err := readSelectorByte(ctx, in)
	if errors.Is(err, errSelectorTimeout) || errors.Is(err, errSelectorEOF) {
		if backAllowed {
			return contextCreateNavigateBack, true, nil
		}
		return contextCreateNavigateCancel, true, nil
	}
	if err != nil {
		return contextCreateNavigateCancel, false, err
	}
	if next != '[' && next != 'O' {
		if backAllowed {
			return contextCreateNavigateBack, true, nil
		}
		return contextCreateNavigateCancel, true, nil
	}
	if _, err := readSelectorByte(ctx, in); err != nil && !errors.Is(err, errSelectorTimeout) && !errors.Is(err, errSelectorEOF) {
		return contextCreateNavigateCancel, false, err
	}
	return contextCreateNavigateNext, false, nil
}

func editContextCreateChoiceRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	style bool,
	step contextCreateRawStep,
	name string,
	information []string,
	prompt string,
	options []configurationWizardOption,
	initial int,
) (int, contextCreateRawNavigation, error) {
	selected := initial
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			return selected, contextCreateNavigateCancel, err
		}
		lines := []string{
			selectorTitle(style, "Tobari · Create Workspace Manifest"),
			selectorDetail(style, "Step", contextCreateStepLabel(step), styleText),
			selectorDetail(style, "Workspace Manifest", safeExternalText(name), styleText),
		}
		for _, detail := range information {
			lines = append(lines, selectorHelp(style, detail))
		}
		lines = append(lines, "", applyStyleToken(style, styleText, prompt+":"))
		for index, option := range options {
			marker := "  "
			labelToken := styleText
			if index == selected {
				marker = "> "
				labelToken = styleAccent
			}
			line := marker + applyStyleToken(style, labelToken, option.label)
			if option.description != "" {
				line += " — " + applyStyleToken(style, styleMuted, option.description)
			}
			lines = append(lines, line)
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "↑/↓ move   Enter continue   b back   q cancel"))
		var err error
		*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
		if err != nil {
			return selected, contextCreateNavigateCancel, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return selected, contextCreateNavigateCancel, err
		}
		switch key.kind {
		case selectorKeyUp:
			selected = (selected - 1 + len(options)) % len(options)
			message = ""
		case selectorKeyDown:
			selected = (selected + 1) % len(options)
			message = ""
		case selectorKeyHome:
			selected = 0
		case selectorKeyEnd:
			selected = len(options) - 1
		case selectorKeyNumber:
			if key.index >= 0 && key.index < len(options) {
				selected = key.index
				return selected, contextCreateNavigateNext, nil
			}
			message = "That option does not exist."
		case selectorKeyEnter:
			return selected, contextCreateNavigateNext, nil
		case selectorKeyBack:
			return selected, contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return selected, contextCreateNavigateCancel, nil
		default:
			message = "Use ↑/↓ to move, Enter to continue, b to go back, or q to cancel."
		}
	}
}

func editContextCreateMethodPolicyRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	style bool,
	draft *contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	message := ""
	rows := append([]string{"Other methods (default)"}, contextCreateHTTPMethods...)
	for {
		if err := ctx.Err(); err != nil {
			return contextCreateNavigateCancel, err
		}
		sourceAccess := []tobari.ManifestSourceAccess{
			tobari.ManifestSourceAccessReadWrite,
			tobari.ManifestSourceAccessReadOnly,
		}[draft.sourceIndex]
		lines := []string{
			selectorTitle(style, "Tobari · Create Workspace Manifest"),
			selectorDetail(style, "Step", contextCreateStepLabel(contextCreateStepNetwork), styleText),
			selectorDetail(style, "Workspace Manifest", safeExternalText(draft.name), styleText),
			selectorDetail(style, "Filesystem", "source "+string(sourceAccess)+" · home read-write · tmpfs read-write", styleText),
			selectorHelp(style, "Every method resolves to allow, exact review, or deny."),
			"",
			applyStyleToken(style, styleMuted, "  METHOD                    POLICY          SOURCE"),
		}
		for index, row := range rows {
			marker := "  "
			labelToken := styleText
			if index == draft.methodSelected {
				marker = "> "
				labelToken = styleAccent
			}
			decision := draft.methodDefault
			source := "default"
			if index > 0 {
				source = "inherited"
				if value, ok := draft.methodOverrides[row]; ok {
					decision = value
					source = "override"
				}
			}
			lines = append(lines, contextCreateMethodPolicyRow(style, marker, row, labelToken, decision, source))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "↑/↓ move   a allow   e exact review   d deny   i inherit"), selectorHelp(style, "r reset defaults   Enter done   b back   q cancel"))
		var err error
		*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		switch key.kind {
		case selectorKeyUp:
			draft.methodSelected = (draft.methodSelected - 1 + len(rows)) % len(rows)
		case selectorKeyDown:
			draft.methodSelected = (draft.methodSelected + 1) % len(rows)
		case selectorKeyHome:
			draft.methodSelected = 0
		case selectorKeyEnd:
			draft.methodSelected = len(rows) - 1
		case selectorKeyAllow, selectorKeyExact, selectorKeyDeny:
			decision := tobari.ManifestMethodExactReview
			if key.kind == selectorKeyAllow {
				decision = tobari.ManifestMethodAllow
			} else if key.kind == selectorKeyDeny {
				decision = tobari.ManifestMethodDeny
			}
			if draft.methodSelected == 0 {
				draft.methodDefault = decision
			} else {
				draft.methodOverrides[rows[draft.methodSelected]] = decision
			}
			message = rows[draft.methodSelected] + " staged as " + displayMethodDecision(decision) + "."
		case selectorKeyInherit:
			if draft.methodSelected == 0 {
				message = "Other methods owns the default and cannot inherit."
			} else {
				delete(draft.methodOverrides, rows[draft.methodSelected])
				message = rows[draft.methodSelected] + " now inherits the default."
			}
		case selectorKeyReset:
			draft.methodDefault = tobari.ManifestMethodExactReview
			draft.methodOverrides = make(map[string]tobari.ManifestMethodDecision)
			message = "Method policies reset to the reviewed defaults."
		case selectorKeyEnter, selectorKeyApply:
			return contextCreateNavigateNext, nil
		case selectorKeyBack:
			return contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return contextCreateNavigateCancel, nil
		default:
			message = "Use a/e/d to set, i to inherit, r to reset, Enter to finish, b to go back, or q to cancel."
		}
	}
}

func reviewContextCreateNetworkRaw(
	ctx context.Context, in io.Reader, out io.Writer, lineCount *int, style bool, draft *contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	selected := 0
	message := ""
	for {
		policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		lines := []string{
			selectorTitle(style, "Tobari · Create Workspace Manifest"),
			selectorDetail(style, "Step", contextCreateStepLabel(contextCreateStepNetwork), styleText),
			selectorDetail(style, "Workspace Manifest", safeExternalText(draft.name), styleText),
			"", applyStyleToken(style, styleText, "Agent traffic"),
			"  Claude Code and Codex routine traffic  " + contextCreateRoutineTrafficPolicy(style, draft.nativeReadiness),
			"", applyStyleToken(style, styleText, "Other destinations"),
			applyStyleToken(style, styleMuted, "  METHOD                    POLICY"),
		}
		lines = append(lines, contextCreateEffectiveMethodLines(style, policy, false)...)
		lines = append(lines, "", applyStyleToken(style, styleText, "Network ceiling"),
			"  Private and unsafe destinations       "+applyStyleToken(style, styleWarning, "deny"), "")
		options := []string{"Continue with these settings", "Customize method policies"}
		for index, option := range options {
			marker := "  "
			token := styleText
			if index == selected {
				marker, token = "> ", styleAccent
			}
			lines = append(lines, marker+applyStyleToken(style, token, option))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "↑/↓ move   Enter select   b back   q cancel"))
		*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		switch key.kind {
		case selectorKeyUp, selectorKeyDown:
			selected = 1 - selected
		case selectorKeyEnter:
			if selected == 0 {
				return contextCreateNavigateNext, nil
			}
			return editContextCreateMethodPolicyRaw(ctx, in, out, lineCount, style, draft)
		case selectorKeyBack:
			return contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return contextCreateNavigateCancel, nil
		default:
			message = "Choose the effective settings or open method customization."
		}
	}
}

func contextCreateEffectiveMethodLines(style bool, policy tobari.ManifestMethodPolicy, includeSource bool) []string {
	rows := make([]string, 0, len(contextCreateHTTPMethods)+1)
	for _, method := range contextCreateHTTPMethods {
		decision := policy.Decision(method)
		line := fmt.Sprintf("  %-25s %s", method, applyStyleToken(style, methodDecisionStyle(decision), displayMethodDecision(decision)))
		if includeSource {
			source := "inherited"
			for _, override := range policy.Overrides {
				if override.Method == method {
					source = "override"
					break
				}
			}
			line += "  " + source
		}
		rows = append(rows, line)
	}
	rows = append(rows, fmt.Sprintf("  %-25s %s", "Other", applyStyleToken(style, methodDecisionStyle(policy.Default), displayMethodDecision(policy.Default))))
	return rows
}

// contextCreateMethodPolicyRow fixes the visible column layout before applying
// ANSI styles. Formatting a styled value directly makes the escape sequences
// count toward %-25s/%-15s and shifts the columns only for highlighted rows.
func contextCreateMethodPolicyRow(
	style bool, marker, label string, labelToken styleToken, decision tobari.ManifestMethodDecision, source string,
) string {
	labelCell := fmt.Sprintf("%-*s", contextCreateMethodLabelWidth, label)
	decisionCell := fmt.Sprintf("%-*s", contextCreateMethodDecisionWidth, displayMethodDecision(decision))
	return marker +
		applyStyleToken(style, labelToken, labelCell) + " " +
		applyStyleToken(style, methodDecisionStyle(decision), decisionCell) + " " + source
}

func reviewContextCreateLine(
	ctx context.Context, in io.Reader, out io.Writer, selection contextCreateSelection, style bool,
) (string, error) {
	if _, err := fmt.Fprintln(out, strings.Join(contextCreateReviewLines(style, selection), "\n")); err != nil {
		return "", err
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Workspace Manifest · Review & Create", contextName: selection.Name, current: "draft",
		prompt: "Action", options: []configurationWizardOption{
			{label: "Create Workspace Manifest", description: "Create this exact reviewed boundary.", value: "create"},
			{label: "Edit settings", description: "Change one section, then return here.", value: "edit"},
			{label: "Cancel", description: "Leave the Workspace Manifest collection unchanged.", value: "cancel"},
		},
	})
	if err != nil {
		return "", err
	}
	return []string{"create", "edit", "cancel"}[index], nil
}

func (w *terminalContextCreateWizard) reviewContextCreateRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	draft *contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			return contextCreateNavigateCancel, err
		}
		selection, err := contextCreateSelectionFromDraft(*draft)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		details := contextCreateReviewLines(w.style, selection)
		const reviewPageSize = 18
		maxTop := len(details) - reviewPageSize
		if maxTop < 0 {
			maxTop = 0
		}
		if draft.reviewTop > maxTop {
			draft.reviewTop = maxTop
		}
		end := draft.reviewTop + reviewPageSize
		if end > len(details) {
			end = len(details)
		}
		lines := append([]string{selectorTitle(w.style, "Tobari · Create Workspace Manifest"), selectorDetail(w.style, "Step", contextCreateStepLabel(contextCreateStepReview), styleText), ""}, details[draft.reviewTop:end]...)
		if len(details) > reviewPageSize {
			lines = append(lines, "", selectorHelp(w.style, fmt.Sprintf("Details %d-%d of %d · PgUp/PgDn scroll", draft.reviewTop+1, end, len(details))))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(w.style, styleWarning, message))
		}
		lines = append(lines, "")
		actions := []string{"Create Workspace Manifest", "Edit settings", "Cancel"}
		for index, action := range actions {
			marker, token := "  ", styleText
			if index == draft.reviewAction {
				marker, token = "> ", styleAccent
			}
			lines = append(lines, marker+applyStyleToken(w.style, token, action))
		}
		lines = append(lines, "", selectorHelp(w.style, "↑/↓ move   Enter select   b back   q cancel"))
		*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		switch key.kind {
		case selectorKeyPageUp:
			draft.reviewTop -= reviewPageSize
			if draft.reviewTop < 0 {
				draft.reviewTop = 0
			}
		case selectorKeyPageDown:
			draft.reviewTop += reviewPageSize
			if draft.reviewTop > maxTop {
				draft.reviewTop = maxTop
			}
		case selectorKeyUp:
			draft.reviewAction = (draft.reviewAction + 2) % 3
		case selectorKeyDown:
			draft.reviewAction = (draft.reviewAction + 1) % 3
		case selectorKeyEnter, selectorKeyApply:
			switch draft.reviewAction {
			case 0:
				changed, refreshed, revalidateErr := w.revalidateBootstrap(ctx, selection.Bootstrap)
				if revalidateErr != nil {
					message = revalidateErr.Error()
					continue
				}
				if changed {
					draft.bootstrap = refreshed
					message = "Host bootstrap configuration changed during review. Review the refreshed settings."
					continue
				}
				return contextCreateNavigateNext, nil
			case 1:
				navigation, editErr := w.editContextCreateSettingsRaw(ctx, in, out, lineCount, draft)
				if editErr != nil {
					if errors.Is(editErr, context.Canceled) {
						return contextCreateNavigateCancel, nil
					}
					return contextCreateNavigateCancel, editErr
				}
				if navigation == contextCreateNavigateCancel {
					return contextCreateNavigateCancel, nil
				}
			case 2:
				return contextCreateNavigateCancel, nil
			}
		case selectorKeyBack:
			return contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return contextCreateNavigateCancel, nil
		default:
			message = "Choose Create Workspace Manifest, Edit settings, or Cancel."
		}
	}
}

func contextCreateSelectionFromDraft(draft contextCreateRawDraft) (contextCreateSelection, error) {
	policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
	if err != nil {
		return contextCreateSelection{}, err
	}
	selection := contextCreateSelection{
		CopyFrom: draft.base,
		Name:     draft.name, RuntimeSelection: draft.runtimeSelection,
		NativeReadiness: draft.nativeReadiness,
		SourceAccess: []tobari.ManifestSourceAccess{
			tobari.ManifestSourceAccessReadWrite,
			tobari.ManifestSourceAccessReadOnly,
		}[draft.sourceIndex],
		MethodPolicy: policy,
	}
	if draft.bootstrap != nil {
		copy := draft.bootstrap.Clone()
		selection.Bootstrap = &copy
		selection.AWSBootstrapProfile = copy.AWS.Profile
		if copy.EKS != nil {
			selection.EKSBootstrapContext = copy.EKS.WorkspaceManifestName
		}
	}
	return selection, nil
}

func contextCreateReviewLines(style bool, selection contextCreateSelection) []string {
	runtimeSelection := selection.RuntimeSelection
	if runtimeSelection == "" {
		runtimeSelection = tobari.StandardRuntimeName
	}
	runtimeSelection = contextRuntimeDisplaySelection(runtimeSelection)
	lines := []string{
		applyStyleToken(style, styleText, "Workspace Manifest"),
		selectorDetail(style, "Base", contextCreateBaseDisplay(selection.CopyFrom), styleText),
		selectorDetail(style, "Name", safeExternalText(selection.Name), styleText),
		"", applyStyleToken(style, styleText, "Filesystem"),
		applyStyleToken(style, styleMuted, "  LOCATION                  ACCESS"),
		fmt.Sprintf("  %-25s %s", "Project source", selection.SourceAccess),
		fmt.Sprintf("  %-25s %s", "Workspace home", "read-write"),
		fmt.Sprintf("  %-25s %s", "Temporary files", "read-write"),
		"", applyStyleToken(style, styleText, "Network"),
		applyStyleToken(style, styleMuted, "  TRAFFIC                   POLICY"),
		fmt.Sprintf("  %-25s %s", "Claude/Codex routine", contextCreateRoutineTrafficPolicy(style, selection.NativeReadiness)),
		"", applyStyleToken(style, styleMuted, "  METHOD                    OTHER DESTINATIONS"),
	}
	lines = append(lines, contextCreateEffectiveMethodLines(style, selection.MethodPolicy, false)...)
	lines = append(lines,
		"", applyStyleToken(style, styleMuted, "  DESTINATIONS              CEILING"),
		fmt.Sprintf("  %-25s %s", "Private and unsafe", applyStyleToken(style, styleWarning, "deny")),
		"", applyStyleToken(style, styleText, "Runtime"),
		applyStyleToken(style, styleMuted, "  REVISION                  STATUS"),
		fmt.Sprintf("  %-25s %s", safeExternalText(runtimeSelection), applyStyleToken(style, styleSuccess, "ready")),
		"", applyStyleToken(style, styleText, "Workspace bootstrap"),
	)
	if selection.Bootstrap == nil {
		lines = append(lines,
			selectorDetail(style, "AWS IAM Identity Center", "not configured", styleMuted),
			selectorDetail(style, "Amazon EKS", "not configured", styleMuted),
		)
	} else {
		aws := selection.Bootstrap.AWS
		lines = append(lines,
			selectorDetail(style, "AWS profile", safeExternalText(aws.Profile), styleText),
			selectorDetail(style, "Account / role", safeExternalText(aws.AccountID+" / "+aws.RoleName), styleText),
			selectorDetail(style, "SSO session / region", safeExternalText(aws.SSOSession+" / "+aws.SSORegion), styleText),
		)
		if selection.Bootstrap.EKS == nil {
			lines = append(lines, selectorDetail(style, "Amazon EKS", "not configured", styleMuted))
		} else {
			lines = append(lines, selectorDetail(style, "Amazon EKS", safeExternalText(selection.Bootstrap.EKS.WorkspaceManifestName+" / "+selection.Bootstrap.EKS.ClusterName), styleText))
		}
	}
	lines = append(lines, selectorHelp(style, "Applied only to newly created Workspace homes."))
	return lines
}

func contextCreateBaseDisplay(base *tobari.ManifestCopySnapshot) string {
	if base == nil {
		return "Tobari recommended settings"
	}
	return safeExternalText(base.Name) + " (draft initializer only)"
}

func contextCreateRoutineTrafficPolicy(style bool, readiness tobari.ManifestNativeReadiness) string {
	if readiness == tobari.ManifestNativeReadinessDisabled {
		return applyStyleToken(style, styleWarning, "not pre-authorized")
	}
	return applyStyleToken(style, styleSuccess, "allow")
}

func editContextCreateFilesystemLine(ctx context.Context, in io.Reader, out io.Writer, style bool, draft *contextCreateRawDraft) error {
	chooser := &terminalContextConfigurationWizard{mode: nil, style: style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Workspace Manifest", contextName: draft.name, current: "draft",
		information: []string{"Workspace home and temporary files stay read-write in both choices."},
		prompt:      "Project source access", options: []configurationWizardOption{
			{label: "Read-write", description: "Allow direct project-source changes.", value: string(tobari.ManifestSourceAccessReadWrite)},
			{label: "Read-only", description: "Prevent direct project-source changes.", value: string(tobari.ManifestSourceAccessReadOnly)},
		},
	})
	if err == nil {
		draft.sourceIndex = index
	}
	return err
}

func editContextCreateNetworkLine(ctx context.Context, in io.Reader, out io.Writer, style bool, draft *contextCreateRawDraft) error {
	policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, strings.Join(append([]string{
		"Tobari · Create Workspace Manifest · Network access", "", "Agent traffic",
		"  Claude Code and Codex routine traffic  allow", "", "Other destinations", "  METHOD                    POLICY",
	}, append(contextCreateEffectiveMethodLines(false, policy, false), "", "Network ceiling", "  Private and unsafe destinations       deny")...), "\n")); err != nil {
		return err
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Network access", contextName: draft.name, current: "draft", prompt: "Action",
		options: []configurationWizardOption{
			{label: "Continue with these settings", description: "Keep every effective value shown above.", value: "continue"},
			{label: "Customize method policies", description: "Set the default and exact method overrides.", value: "customize"},
		},
	})
	if err != nil || index == 0 {
		return err
	}
	selected, err := selectContextCreateMethodPolicyLine(ctx, in, out, draft.name, []tobari.ManifestSourceAccess{tobari.ManifestSourceAccessReadWrite, tobari.ManifestSourceAccessReadOnly}[draft.sourceIndex])
	if err != nil {
		return err
	}
	draft.methodDefault = selected.Default
	draft.methodOverrides = make(map[string]tobari.ManifestMethodDecision, len(selected.Overrides))
	for _, override := range selected.Overrides {
		draft.methodOverrides[override.Method] = override.Decision
	}
	return nil
}

func (w *terminalContextCreateWizard) editContextCreateSettingsLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	options := []configurationWizardOption{}
	if len(w.bases) > 0 {
		options = append(options, configurationWizardOption{label: "Base", description: "Reset all Base-owned draft settings.", value: "base"})
	}
	options = append(options,
		configurationWizardOption{label: "Workspace Manifest name", value: "name"},
		configurationWizardOption{label: "Filesystem access", value: "filesystem"},
		configurationWizardOption{label: "Network access", value: "network"},
		configurationWizardOption{label: "Runtime", value: "runtime"},
		configurationWizardOption{label: "Workspace bootstrap", value: "bootstrap"},
	)
	options = append(options, configurationWizardOption{label: "Return to review", value: "review"})
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Workspace Manifest · Edit settings", contextName: draft.name, current: "draft", prompt: "What do you want to edit?",
		options: options,
	})
	if err != nil {
		return err
	}
	switch options[index].value {
	case "base":
		err = w.editContextCreateBaseLine(ctx, in, out, draft)
	case "name":
		draft.name, err = readContextCreateName(ctx, in, out)
	case "filesystem":
		err = editContextCreateFilesystemLine(ctx, in, out, w.style, draft)
	case "network":
		err = editContextCreateNetworkLine(ctx, in, out, w.style, draft)
	case "bootstrap":
		err = w.editContextCreateBootstrapLine(ctx, in, out, draft)
	case "runtime":
		err = w.editContextCreateRuntimeLine(ctx, in, out, draft)
	}
	return err
}

func (w *terminalContextCreateWizard) contextCreateBaseOptions(current *tobari.ManifestCopySnapshot) ([]configurationWizardOption, int) {
	options := []configurationWizardOption{{label: "Tobari recommended settings", description: "Reset to the stable product defaults.", value: ""}}
	initial := 0
	for _, item := range w.bases {
		if current != nil && item.Name == current.Name {
			initial = len(options)
		}
		options = append(options, configurationWizardOption{
			label: safeExternalText(item.Name), description: "Initialize a standalone draft; no lineage is created.", value: item.Name,
		})
	}
	return options, initial
}

func (w *terminalContextCreateWizard) editContextCreateBaseLine(
	ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft,
) error {
	options, initial := w.contextCreateBaseOptions(draft.base)
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	selected, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Workspace Manifest · Base", contextName: draft.name,
		current: contextCreateBaseDisplay(draft.base), prompt: "Base", options: options, initial: initial,
	})
	if err != nil {
		return err
	}
	name := options[selected].value
	if (draft.base == nil && name == "") || (draft.base != nil && draft.base.Name == name) {
		return nil
	}
	confirmed, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Workspace Manifest · Reset Base", contextName: draft.name,
		information: []string{"Changing Base replaces every Base-owned draft setting, including prior customizations."},
		prompt:      "Reset draft", options: []configurationWizardOption{
			{label: "Reset draft", description: "Replace settings with the selected Base.", value: "reset"},
			{label: "Keep current draft", description: "Return without changing settings.", value: "keep"},
		}, initial: 1,
	})
	if err != nil || confirmed != 0 {
		return err
	}
	return w.applyContextCreateBase(ctx, draft, name)
}

func (w *terminalContextCreateWizard) applyContextCreateBase(
	ctx context.Context, draft *contextCreateRawDraft, name string,
) error {
	if name == "" {
		resetContextCreateDraftBase(draft, nil)
		return nil
	}
	if w.baseRead == nil {
		return fmt.Errorf("Workspace Manifest copy-source reader is unavailable")
	}
	base, err := w.baseRead.CopySnapshot(ctx, name)
	if err != nil {
		return err
	}
	resetContextCreateDraftBase(draft, &base)
	return nil
}

func (w *terminalContextCreateWizard) readyRuntimeOptions() []configurationWizardOption {
	options := []configurationWizardOption{{label: "standard@1", description: "Built-in immutable Tobari Runtime.", value: tobari.StandardRuntimeName}}
	for _, runtime := range w.runtimes {
		if runtime.Kind == tobari.RuntimeKindManaged && runtime.Ready {
			selection := fmt.Sprintf("%s@%d", runtime.Name, runtime.Head)
			options = append(options, configurationWizardOption{label: selection, description: "Ready immutable revision " + runtime.Revision[:12] + ".", value: selection})
		}
	}
	return options
}

func (w *terminalContextCreateWizard) editContextCreateRuntimeLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	options := w.readyRuntimeOptions()
	current := 0
	for index := range options {
		if options[index].value == draft.runtimeSelection {
			current = index
			break
		}
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	selected, err := chooser.choose(ctx, in, out, configurationWizardMenu{title: "Tobari · Create Workspace Manifest · Runtime", contextName: draft.name, current: contextRuntimeDisplaySelection(draft.runtimeSelection), information: []string{"Only already built immutable revisions can be selected."}, prompt: "Ready Runtime revision", options: options})
	if err == nil {
		draft.runtimeSelection = options[selected].value
	}
	_ = current // line-mode chooser has no independent initial cursor.
	return err
}

func (w *terminalContextCreateWizard) reviewContextCreateBootstrapLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	options := contextCreateBootstrapStepOptions(draft.bootstrap)
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title:       "Tobari · Create Workspace Manifest · Workspace bootstrap",
		contextName: draft.name,
		current:     contextCreateBootstrapStepCurrent(draft.bootstrap),
		information: []string{"Applied only to newly created Workspace homes.", "Host configuration is read only after Configure from host is selected."},
		prompt:      "Workspace bootstrap",
		options:     options,
	})
	if err != nil || index == 0 {
		return err
	}
	if options[index].value == "remove" {
		draft.bootstrap = nil
		return nil
	}
	return w.editContextCreateBootstrapLine(ctx, in, out, draft)
}

func contextCreateBootstrapStepOptions(bootstrap *tobari.ManifestBootstrapSnapshot) []configurationWizardOption {
	options := []configurationWizardOption{
		{label: "Continue with this setting", description: contextCreateBootstrapStepCurrent(bootstrap), value: "continue"},
		{label: "Configure from host", description: "Review compatible AWS and optional Amazon EKS settings.", value: "configure"},
	}
	if bootstrap != nil {
		options = append(options, configurationWizardOption{label: "Remove bootstrap", description: "Keep future Workspace homes unconfigured.", value: "remove"})
	}
	return options
}

func contextCreateBootstrapStepCurrent(bootstrap *tobari.ManifestBootstrapSnapshot) string {
	if bootstrap == nil {
		return "not configured"
	}
	value := "AWS " + safeExternalText(bootstrap.AWS.Profile)
	if bootstrap.EKS != nil {
		value += " · EKS " + safeExternalText(bootstrap.EKS.WorkspaceManifestName)
	}
	return value
}

func (w *terminalContextCreateWizard) editContextCreateBootstrapLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	if w.bootstrap == nil {
		return fmt.Errorf("Workspace bootstrap discovery is unavailable")
	}
	discovery, err := w.bootstrap.DiscoverAWSBootstraps(ctx)
	if err != nil {
		return err
	}
	information := []string{"Credentials and SSO caches are never read."}
	if discovery.Reason != "" {
		information = append(information, "Reason: "+safeExternalText(discovery.Reason), "Next: tobari template show")
	}
	options := []configurationWizardOption{{label: "Continue without AWS bootstrap", description: "Keep future Workspace homes unconfigured.", value: "none"}}
	available := []*tobari.ManifestBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ManifestBootstrapCandidateUnavailable {
			information = append(information, "Unavailable: "+safeExternalText(candidate.Profile)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		aws := candidate.Snapshot.AWS
		options = append(options, configurationWizardOption{label: safeExternalText(aws.Profile), description: safeExternalText("account " + aws.AccountID + " · role " + aws.RoleName + " · SSO " + aws.SSOSession + " / " + aws.SSORegion + " · workload " + aws.Region), value: aws.Profile})
		available = append(available, candidate.Snapshot)
	}
	if len(available) == 1 {
		information = append(information, "No compatible IAM Identity Center profiles were found.")
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{title: "Tobari · AWS IAM Identity Center", contextName: draft.name, current: "draft", information: information, prompt: "Profile", options: options})
	if err != nil {
		return err
	}
	if index == 0 {
		draft.bootstrap = nil
		return nil
	}
	selected := available[index].Clone()
	draft.bootstrap = &selected
	return w.editContextCreateEKSLine(ctx, in, out, draft)
}

func (w *terminalContextCreateWizard) editContextCreateEKSLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	discovery, err := w.bootstrap.DiscoverEKSBootstraps(ctx, draft.bootstrap.Clone())
	if err != nil {
		return err
	}
	information := []string{"AWS profile: " + safeExternalText(draft.bootstrap.AWS.Profile)}
	if discovery.Reason != "" {
		information = append(information, "Reason: "+safeExternalText(discovery.Reason))
	}
	options := []configurationWizardOption{{label: "Do not configure Amazon EKS", description: "Continue with AWS only.", value: "none"}}
	available := []*tobari.ManifestBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ManifestBootstrapCandidateUnavailable {
			information = append(information, "Unavailable: "+safeExternalText(candidate.WorkspaceManifestName)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		eks := candidate.Snapshot.EKS
		options = append(options, configurationWizardOption{label: safeExternalText(eks.WorkspaceManifestName), description: safeExternalText("cluster " + eks.ClusterName + " · region " + eks.Region), value: eks.WorkspaceManifestName})
		available = append(available, candidate.Snapshot)
	}
	if len(available) == 1 {
		information = append(information, "No compatible Amazon EKS contexts were found.")
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{title: "Tobari · Amazon EKS", contextName: draft.name, current: "draft", information: information, prompt: "Target", options: options})
	if err != nil {
		return err
	}
	if index > 0 {
		selected := available[index].Clone()
		draft.bootstrap = &selected
	}
	return nil
}

func (w *terminalContextCreateWizard) revalidateBootstrap(ctx context.Context, reviewed *tobari.ManifestBootstrapSnapshot) (bool, *tobari.ManifestBootstrapSnapshot, error) {
	if reviewed == nil {
		return false, nil, nil
	}
	if w.bootstrap == nil {
		return false, nil, fmt.Errorf("Workspace bootstrap discovery is unavailable")
	}
	refreshed, err := w.bootstrap.PrepareAWSBootstrap(ctx, reviewed.AWS.Profile)
	if err != nil {
		return false, nil, err
	}
	if reviewed.EKS != nil {
		refreshed, err = w.bootstrap.PrepareEKSBootstrap(ctx, refreshed, reviewed.EKS.WorkspaceManifestName)
		if err != nil {
			return false, nil, err
		}
	}
	if refreshed.Revision == reviewed.Revision {
		copy := reviewed.Clone()
		return false, &copy, nil
	}
	return true, &refreshed, nil
}

func (w *terminalContextCreateWizard) editContextCreateSettingsRaw(ctx context.Context, in io.Reader, out io.Writer, lineCount *int, draft *contextCreateRawDraft) (contextCreateRawNavigation, error) {
	options := []configurationWizardOption{}
	if len(w.bases) > 0 {
		options = append(options, configurationWizardOption{label: "Base", value: "base"})
	}
	options = append(options,
		configurationWizardOption{label: "Workspace Manifest name", value: "name"},
		configurationWizardOption{label: "Filesystem access", value: "filesystem"},
		configurationWizardOption{label: "Network access", value: "network"},
		configurationWizardOption{label: "Runtime", value: "runtime"},
		configurationWizardOption{label: "Workspace bootstrap", value: "bootstrap"},
		configurationWizardOption{label: "Return to review", value: "review"},
	)
	index, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, draft.name, nil, "What do you want to edit?", options, draft.editSection)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	if navigation == contextCreateNavigateCancel {
		return contextCreateNavigateCancel, nil
	}
	if navigation == contextCreateNavigateBack || index == len(options)-1 {
		return contextCreateNavigateBack, nil
	}
	draft.editSection = index
	staged := cloneContextCreateRawDraft(*draft)
	switch options[index].value {
	case "base":
		baseOptions, initial := w.contextCreateBaseOptions(staged.base)
		value, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, staged.name, []string{"Changing Base replaces every Base-owned draft setting after confirmation."}, "Base", baseOptions, initial)
		if err != nil || navigation != contextCreateNavigateNext {
			return navigation, err
		}
		name := baseOptions[value].value
		if (staged.base == nil && name == "") || (staged.base != nil && staged.base.Name == name) {
			return contextCreateNavigateNext, nil
		}
		confirm, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, staged.name, []string{"Every Base-owned customization in this draft will be replaced."}, "Reset draft", []configurationWizardOption{{label: "Reset draft"}, {label: "Keep current draft"}}, 1)
		if err != nil || navigation != contextCreateNavigateNext || confirm != 0 {
			return navigation, err
		}
		if err := w.applyContextCreateBase(ctx, &staged, name); err != nil {
			return contextCreateNavigateCancel, err
		}
		*draft = staged
		return contextCreateNavigateNext, nil
	case "name":
		value, navigation, err := editContextCreateTextRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, "Workspace Manifest name", staged.name, maxContextCreateNameBytes, "Must match [a-z][a-z0-9-]{0,62}.", func(value string) error { return tobari.ValidateName(strings.TrimSpace(value)) }, true)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			staged.name = strings.TrimSpace(value)
			*draft = staged
		}
		return navigation, nil
	case "filesystem":
		value, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, staged.name, []string{"Workspace home and temporary files stay read-write."}, "Project source access", []configurationWizardOption{{label: "Read-write", description: "Allow direct project-source changes."}, {label: "Read-only", description: "Prevent direct project-source changes."}}, staged.sourceIndex)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			staged.sourceIndex = value
			*draft = staged
		}
		return navigation, nil
	case "network":
		navigation, err := reviewContextCreateNetworkRaw(ctx, in, out, lineCount, w.style, &staged)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			*draft = staged
		}
		return navigation, nil
	case "runtime":
		navigation, err := w.editContextCreateRuntimeRaw(ctx, in, out, lineCount, contextCreateStepReview, &staged)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			*draft = staged
		}
		return navigation, nil
	case "bootstrap":
		navigation, err := w.editContextCreateBootstrapRaw(ctx, in, out, lineCount, &staged)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			*draft = staged
		}
		return navigation, nil
	}
	return contextCreateNavigateBack, nil
}

func (w *terminalContextCreateWizard) editContextCreateRuntimeRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	step contextCreateRawStep,
	draft *contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	runtimeOptions := w.readyRuntimeOptions()
	current := 0
	for candidate := range runtimeOptions {
		if runtimeOptions[candidate].value == draft.runtimeSelection {
			current = candidate
			break
		}
	}
	value, navigation, err := editContextCreateChoiceRaw(
		ctx, in, out, lineCount, w.style, step, draft.name,
		[]string{"Only already built immutable revisions can be selected."},
		"Ready Runtime revision", runtimeOptions, current,
	)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	if navigation == contextCreateNavigateNext {
		draft.runtimeSelection = runtimeOptions[value].value
	}
	return navigation, nil
}

func (w *terminalContextCreateWizard) reviewContextCreateBootstrapRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	step contextCreateRawStep,
	draft *contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	for {
		options := contextCreateBootstrapStepOptions(draft.bootstrap)
		value, navigation, err := editContextCreateChoiceRaw(
			ctx, in, out, lineCount, w.style, step, draft.name,
			[]string{
				"Current: " + contextCreateBootstrapStepCurrent(draft.bootstrap),
				"Applied only to newly created Workspace homes.",
				"Host configuration is read only after Configure from host is selected.",
			},
			"Workspace bootstrap", options, 0,
		)
		if err != nil || navigation != contextCreateNavigateNext {
			return navigation, err
		}
		switch options[value].value {
		case "continue":
			return contextCreateNavigateNext, nil
		case "remove":
			draft.bootstrap = nil
			return contextCreateNavigateNext, nil
		case "configure":
			navigation, err = w.editContextCreateBootstrapRaw(ctx, in, out, lineCount, draft)
			if err != nil {
				return contextCreateNavigateCancel, err
			}
			if navigation == contextCreateNavigateBack {
				continue
			}
			return navigation, nil
		}
	}
}

func cloneContextCreateRawDraft(draft contextCreateRawDraft) contextCreateRawDraft {
	cloned := draft
	cloned.methodOverrides = make(map[string]tobari.ManifestMethodDecision, len(draft.methodOverrides))
	for method, decision := range draft.methodOverrides {
		cloned.methodOverrides[method] = decision
	}
	if draft.bootstrap != nil {
		bootstrap := draft.bootstrap.Clone()
		cloned.bootstrap = &bootstrap
	}
	return cloned
}

func (w *terminalContextCreateWizard) editContextCreateBootstrapRaw(ctx context.Context, in io.Reader, out io.Writer, lineCount *int, draft *contextCreateRawDraft) (contextCreateRawNavigation, error) {
	if w.bootstrap == nil {
		return contextCreateNavigateCancel, fmt.Errorf("Workspace bootstrap discovery is unavailable")
	}
	discovery, err := w.bootstrap.DiscoverAWSBootstraps(ctx)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	information := []string{"Credentials and SSO caches are never read."}
	if discovery.Reason != "" {
		information = append(information, "Reason: "+safeExternalText(discovery.Reason), "Next: tobari template show")
	}
	options := []configurationWizardOption{{label: "Continue without AWS bootstrap", description: "Keep future Workspace homes unconfigured."}}
	available := []*tobari.ManifestBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ManifestBootstrapCandidateUnavailable {
			information = append(information, "Unavailable: "+safeExternalText(candidate.Profile)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		aws := candidate.Snapshot.AWS
		options = append(options, configurationWizardOption{label: safeExternalText(aws.Profile), description: safeExternalText("account " + aws.AccountID + " · role " + aws.RoleName + " · SSO " + aws.SSOSession + " / " + aws.SSORegion + " · workload " + aws.Region)})
		available = append(available, candidate.Snapshot)
	}
	if len(available) == 1 {
		information = append(information, "No compatible IAM Identity Center profiles were found.")
	}
	index, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, draft.name, information, "AWS profile", options, 0)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	if navigation != contextCreateNavigateNext {
		return navigation, nil
	}
	if index == 0 {
		draft.bootstrap = nil
		return contextCreateNavigateNext, nil
	}
	selected := available[index].Clone()
	draft.bootstrap = &selected
	discoveredEKS, err := w.bootstrap.DiscoverEKSBootstraps(ctx, selected)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	eksInfo := []string{"AWS profile: " + safeExternalText(selected.AWS.Profile)}
	if discoveredEKS.Reason != "" {
		eksInfo = append(eksInfo, "Reason: "+safeExternalText(discoveredEKS.Reason))
	}
	eksOptions := []configurationWizardOption{{label: "Do not configure Amazon EKS", description: "Continue with AWS only."}}
	eksAvailable := []*tobari.ManifestBootstrapSnapshot{nil}
	for _, candidate := range discoveredEKS.Candidates {
		if candidate.State == tobari.ManifestBootstrapCandidateUnavailable {
			eksInfo = append(eksInfo, "Unavailable: "+safeExternalText(candidate.WorkspaceManifestName)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		eksOptions = append(eksOptions, configurationWizardOption{label: safeExternalText(candidate.WorkspaceManifestName), description: safeExternalText("cluster " + candidate.Snapshot.EKS.ClusterName + " · region " + candidate.Snapshot.EKS.Region)})
		eksAvailable = append(eksAvailable, candidate.Snapshot)
	}
	if len(eksAvailable) == 1 {
		eksInfo = append(eksInfo, "No compatible Amazon EKS contexts were found.")
	}
	eksIndex, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, draft.name, eksInfo, "Amazon EKS", eksOptions, 0)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	if navigation != contextCreateNavigateNext {
		return navigation, nil
	}
	if eksIndex > 0 {
		composed := eksAvailable[eksIndex].Clone()
		draft.bootstrap = &composed
	}
	return contextCreateNavigateNext, nil
}

func contextCreateStepLabel(step contextCreateRawStep) string {
	labels := []string{"1 of 6 · Name", "2 of 6 · Filesystem", "3 of 6 · Network", "4 of 6 · Runtime", "5 of 6 · Workspace bootstrap", "6 of 6 · Review & Create"}
	if int(step) < 0 || int(step) >= len(labels) {
		return "unknown"
	}
	return labels[step]
}

func readContextCreateName(ctx context.Context, in io.Reader, out io.Writer) (string, error) {
	return readContextCreateNameWithDefault(ctx, in, out, "")
}

func readContextCreateNameWithDefault(ctx context.Context, in io.Reader, out io.Writer, initial string) (string, error) {
	for {
		label := "Workspace Manifest name"
		if initial != "" {
			label += " [" + safeExternalText(initial) + "]"
		}
		name, err := readConfigurationWizardValue(ctx, in, out, label, maxContextCreateNameBytes)
		if err != nil {
			return "", err
		}
		name = strings.TrimSpace(name)
		if name == "" && initial != "" {
			name = initial
		}
		if err := tobari.ValidateName(name); err == nil {
			return name, nil
		}
		if _, err := fmt.Fprintln(out, "Use a valid portable Workspace Manifest name."); err != nil {
			return "", err
		}
	}
}

func selectContextCreateMethodPolicyLine(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	name string,
	sourceAccess tobari.ManifestSourceAccess,
) (tobari.ManifestMethodPolicy, error) {
	if _, err := fmt.Fprintf(out, "Tobari · Network method policy\nWorkspace Manifest: %s\nFilesystem: source %s · home read-write · tmpfs read-write\n\n", safeExternalText(name), sourceAccess); err != nil {
		return tobari.ManifestMethodPolicy{}, err
	}
	defaultDecision, err := readContextCreateMethodDecision(ctx, in, out, "Other methods (default)", tobari.ManifestMethodExactReview)
	if err != nil {
		return tobari.ManifestMethodPolicy{}, err
	}
	explicit := make(map[string]tobari.ManifestMethodDecision)
	for _, method := range contextCreateHTTPMethods {
		decision, err := readContextCreateMethodDecision(ctx, in, out, method, defaultDecision)
		if err != nil {
			return tobari.ManifestMethodPolicy{}, err
		}
		explicit[method] = decision
	}
	return buildContextCreateMethodPolicy(defaultDecision, explicit)
}

func readContextCreateMethodDecision(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	label string,
	fallback tobari.ManifestMethodDecision,
) (tobari.ManifestMethodDecision, error) {
	for {
		if _, err := fmt.Fprintf(out, "%s [a allow, e exact review, d deny] [%s]: ", label, displayMethodDecision(fallback)); err != nil {
			return "", err
		}
		value, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "":
			return fallback, nil
		case "a", "allow":
			return tobari.ManifestMethodAllow, nil
		case "e", "exact", "exact-review", "exact_review", "review":
			return tobari.ManifestMethodExactReview, nil
		case "d", "deny":
			return tobari.ManifestMethodDeny, nil
		case "q", "quit":
			return "", context.Canceled
		default:
			if _, err := fmt.Fprintln(out, "Choose a, e, d, or q."); err != nil {
				return "", err
			}
		}
	}
}

func buildContextCreateMethodPolicy(
	defaultDecision tobari.ManifestMethodDecision,
	explicit map[string]tobari.ManifestMethodDecision,
) (tobari.ManifestMethodPolicy, error) {
	policy := tobari.ManifestMethodPolicy{Default: defaultDecision, Overrides: []tobari.ManifestMethodOverride{}}
	for _, method := range contextCreateHTTPMethods {
		if decision, ok := explicit[method]; ok && decision != defaultDecision {
			policy.Overrides = append(policy.Overrides, tobari.ManifestMethodOverride{Method: method, Decision: decision})
		}
	}
	return tobari.NormalizeContextMethodPolicy(policy)
}

func displayMethodDecision(decision tobari.ManifestMethodDecision) string {
	if decision == tobari.ManifestMethodExactReview {
		return "exact review"
	}
	return string(decision)
}

func methodDecisionStyle(decision tobari.ManifestMethodDecision) styleToken {
	switch decision {
	case tobari.ManifestMethodAllow:
		return styleSuccess
	case tobari.ManifestMethodDeny:
		return styleWarning
	default:
		return styleText
	}
}
