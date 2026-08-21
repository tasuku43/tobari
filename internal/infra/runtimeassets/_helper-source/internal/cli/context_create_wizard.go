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
	Name                string
	RuntimeSelection    string
	SourceAccess        tobari.ContextSourceAccess
	NativeReadiness     tobari.ContextNativeReadiness
	MethodPolicy        tobari.ContextMethodPolicy
	AWSBootstrapProfile string
	EKSBootstrapContext string
	Bootstrap           *tobari.ContextBootstrapSnapshot
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
}

type contextCreateBootstrapDiscovery interface {
	DiscoverAWSBootstraps(context.Context) (tobari.ContextAWSBootstrapDiscovery, error)
	DiscoverEKSBootstraps(context.Context, tobari.ContextBootstrapSnapshot) (tobari.ContextEKSBootstrapDiscovery, error)
	PrepareAWSBootstrap(context.Context, string) (tobari.ContextBootstrapSnapshot, error)
	PrepareEKSBootstrap(context.Context, tobari.ContextBootstrapSnapshot, string) (tobari.ContextBootstrapSnapshot, error)
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
	name             string
	sourceIndex      int
	methodSelected   int
	methodDefault    tobari.ContextMethodDecision
	methodOverrides  map[string]tobari.ContextMethodDecision
	bootstrap        *tobari.ContextBootstrapSnapshot
	runtimeSelection string
	nativeReadiness  tobari.ContextNativeReadiness
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
			finishSelectorScreen(out, 0)
			restoreErr := restore()
			if composeErr != nil {
				return contextCreateSelection{}, composeErr
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
				if _, writeErr := fmt.Fprintln(out, "Host bootstrap configuration could not be revalidated. Context has not been created. Edit Workspace bootstrap or cancel."); writeErr != nil {
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
				ctx, in, out, &lineCount, w.style, step, "Context name", draft.name,
				maxContextCreateNameBytes, "Must match [a-z][a-z0-9-]{0,62}.",
				func(value string) error { return tobari.ValidateName(strings.TrimSpace(value)) }, false,
			)
			draft.name = strings.TrimSpace(draft.name)
		case contextCreateStepFilesystem:
			draft.sourceIndex, navigation, err = editContextCreateChoiceRaw(
				ctx, in, out, &lineCount, w.style, step, draft.name,
				[]string{"Workspace home and tmpfs stay read-write in both choices."},
				"Project source access", []configurationWizardOption{
					{label: "Read-write", description: "Allow direct project-source changes.", value: string(tobari.ContextSourceAccessReadWrite)},
					{label: "Read-only", description: "Prevent direct project-source changes.", value: string(tobari.ContextSourceAccessReadOnly)},
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
			return contextCreateSelection{}, fmt.Errorf("Context creation wizard step is invalid")
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
		name:             seed.Selection.Name,
		sourceIndex:      0,
		runtimeSelection: seed.Selection.RuntimeSelection,
		nativeReadiness:  seed.Selection.NativeReadiness,
		methodDefault:    seed.Selection.MethodPolicy.Default,
		methodOverrides:  make(map[string]tobari.ContextMethodDecision),
	}
	if draft.runtimeSelection == "" {
		draft.runtimeSelection = tobari.StandardRuntimeName
	}
	if draft.nativeReadiness == "" {
		draft.nativeReadiness = tobari.ContextNativeReadinessEnabled
	}
	if draft.methodDefault == "" {
		draft.methodDefault = tobari.ContextMethodExactReview
	}
	for _, override := range seed.Selection.MethodPolicy.Overrides {
		draft.methodOverrides[override.Method] = override.Decision
	}
	if seed.Selection.SourceAccess == tobari.ContextSourceAccessReadOnly {
		draft.sourceIndex = 1
	}
	if seed.Selection.Bootstrap != nil {
		copy := seed.Selection.Bootstrap.Clone()
		draft.bootstrap = &copy
	}
	return draft
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
				selectorTitle(style, "Tobari · Create Context"),
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
			selectorTitle(style, "Tobari · Create Context"),
			selectorDetail(style, "Step", contextCreateStepLabel(step), styleText),
			selectorDetail(style, "Context", safeExternalText(name), styleText),
		}
		for _, detail := range information {
			lines = append(lines, selectorHelp(style, detail))
		}
		lines = append(lines, "", applyStyleToken(style, styleText, prompt+":"))
		for index, option := range options {
			marker := "  "
			labelToken := styleText
			if index == selected {
				marker = "❯ "
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
		sourceAccess := []tobari.ContextSourceAccess{
			tobari.ContextSourceAccessReadWrite,
			tobari.ContextSourceAccessReadOnly,
		}[draft.sourceIndex]
		lines := []string{
			selectorTitle(style, "Tobari · Create Context"),
			selectorDetail(style, "Step", contextCreateStepLabel(contextCreateStepNetwork), styleText),
			selectorDetail(style, "Context", safeExternalText(draft.name), styleText),
			selectorDetail(style, "Filesystem", "source "+string(sourceAccess)+" · home read-write · tmpfs read-write", styleText),
			selectorHelp(style, "Every method resolves to allow, exact review, or deny."),
			"",
			applyStyleToken(style, styleMuted, "  METHOD                    POLICY          SOURCE"),
		}
		for index, row := range rows {
			marker := "  "
			labelToken := styleText
			if index == draft.methodSelected {
				marker = "❯ "
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
			decision := tobari.ContextMethodExactReview
			if key.kind == selectorKeyAllow {
				decision = tobari.ContextMethodAllow
			} else if key.kind == selectorKeyDeny {
				decision = tobari.ContextMethodDeny
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
			draft.methodDefault = tobari.ContextMethodExactReview
			draft.methodOverrides = make(map[string]tobari.ContextMethodDecision)
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
			selectorTitle(style, "Tobari · Create Context"),
			selectorDetail(style, "Step", contextCreateStepLabel(contextCreateStepNetwork), styleText),
			selectorDetail(style, "Context", safeExternalText(draft.name), styleText),
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
				marker, token = "❯ ", styleAccent
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

func contextCreateEffectiveMethodLines(style bool, policy tobari.ContextMethodPolicy, includeSource bool) []string {
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
	style bool, marker, label string, labelToken styleToken, decision tobari.ContextMethodDecision, source string,
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
		title: "Tobari · Create Context · Review & Create", contextName: selection.Name, current: "draft",
		prompt: "Action", options: []configurationWizardOption{
			{label: "Create Context", description: "Create this exact reviewed boundary.", value: "create"},
			{label: "Edit settings", description: "Change one section, then return here.", value: "edit"},
			{label: "Cancel", description: "Leave the Context collection unchanged.", value: "cancel"},
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
		lines := append([]string{selectorTitle(w.style, "Tobari · Create Context"), selectorDetail(w.style, "Step", contextCreateStepLabel(contextCreateStepReview), styleText), ""}, details[draft.reviewTop:end]...)
		if len(details) > reviewPageSize {
			lines = append(lines, "", selectorHelp(w.style, fmt.Sprintf("Details %d-%d of %d · PgUp/PgDn scroll", draft.reviewTop+1, end, len(details))))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(w.style, styleWarning, message))
		}
		lines = append(lines, "")
		actions := []string{"Create Context", "Edit settings", "Cancel"}
		for index, action := range actions {
			marker, token := "  ", styleText
			if index == draft.reviewAction {
				marker, token = "❯ ", styleAccent
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
			message = "Choose Create Context, Edit settings, or Cancel."
		}
	}
}

func contextCreateSelectionFromDraft(draft contextCreateRawDraft) (contextCreateSelection, error) {
	policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
	if err != nil {
		return contextCreateSelection{}, err
	}
	selection := contextCreateSelection{
		Name: draft.name, RuntimeSelection: draft.runtimeSelection,
		NativeReadiness: draft.nativeReadiness,
		SourceAccess: []tobari.ContextSourceAccess{
			tobari.ContextSourceAccessReadWrite,
			tobari.ContextSourceAccessReadOnly,
		}[draft.sourceIndex],
		MethodPolicy: policy,
	}
	if draft.bootstrap != nil {
		copy := draft.bootstrap.Clone()
		selection.Bootstrap = &copy
		selection.AWSBootstrapProfile = copy.AWS.Profile
		if copy.EKS != nil {
			selection.EKSBootstrapContext = copy.EKS.ContextName
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
		applyStyleToken(style, styleText, "Context"),
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
			lines = append(lines, selectorDetail(style, "Amazon EKS", safeExternalText(selection.Bootstrap.EKS.ContextName+" / "+selection.Bootstrap.EKS.ClusterName), styleText))
		}
	}
	lines = append(lines, selectorHelp(style, "Applied only to newly created Workspace homes."))
	return lines
}

func contextCreateRoutineTrafficPolicy(style bool, readiness tobari.ContextNativeReadiness) string {
	if readiness == tobari.ContextNativeReadinessDisabled {
		return applyStyleToken(style, styleWarning, "not pre-authorized")
	}
	return applyStyleToken(style, styleSuccess, "allow")
}

func editContextCreateFilesystemLine(ctx context.Context, in io.Reader, out io.Writer, style bool, draft *contextCreateRawDraft) error {
	chooser := &terminalContextConfigurationWizard{mode: nil, style: style}
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Context", contextName: draft.name, current: "draft",
		information: []string{"Workspace home and temporary files stay read-write in both choices."},
		prompt:      "Project source access", options: []configurationWizardOption{
			{label: "Read-write", description: "Allow direct project-source changes.", value: string(tobari.ContextSourceAccessReadWrite)},
			{label: "Read-only", description: "Prevent direct project-source changes.", value: string(tobari.ContextSourceAccessReadOnly)},
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
		"Tobari · Create Context · Network access", "", "Agent traffic",
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
	selected, err := selectContextCreateMethodPolicyLine(ctx, in, out, draft.name, []tobari.ContextSourceAccess{tobari.ContextSourceAccessReadWrite, tobari.ContextSourceAccessReadOnly}[draft.sourceIndex])
	if err != nil {
		return err
	}
	draft.methodDefault = selected.Default
	draft.methodOverrides = make(map[string]tobari.ContextMethodDecision, len(selected.Overrides))
	for _, override := range selected.Overrides {
		draft.methodOverrides[override.Method] = override.Decision
	}
	return nil
}

func (w *terminalContextCreateWizard) editContextCreateSettingsLine(ctx context.Context, in io.Reader, out io.Writer, draft *contextCreateRawDraft) error {
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	options := []configurationWizardOption{
		{label: "Context name", value: "name"}, {label: "Filesystem access", value: "filesystem"},
		{label: "Network access", value: "network"}, {label: "Runtime", value: "runtime"},
		{label: "Workspace bootstrap", value: "bootstrap"},
	}
	options = append(options, configurationWizardOption{label: "Return to review", value: "review"})
	index, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Create Context · Edit settings", contextName: draft.name, current: "draft", prompt: "What do you want to edit?",
		options: options,
	})
	if err != nil {
		return err
	}
	switch options[index].value {
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
	selected, err := chooser.choose(ctx, in, out, configurationWizardMenu{title: "Tobari · Create Context · Runtime", contextName: draft.name, current: contextRuntimeDisplaySelection(draft.runtimeSelection), information: []string{"Only already built immutable revisions can be selected."}, prompt: "Ready Runtime revision", options: options})
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
		title:       "Tobari · Create Context · Workspace bootstrap",
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

func contextCreateBootstrapStepOptions(bootstrap *tobari.ContextBootstrapSnapshot) []configurationWizardOption {
	options := []configurationWizardOption{
		{label: "Continue with this setting", description: contextCreateBootstrapStepCurrent(bootstrap), value: "continue"},
		{label: "Configure from host", description: "Review compatible AWS and optional Amazon EKS settings.", value: "configure"},
	}
	if bootstrap != nil {
		options = append(options, configurationWizardOption{label: "Remove bootstrap", description: "Keep future Workspace homes unconfigured.", value: "remove"})
	}
	return options
}

func contextCreateBootstrapStepCurrent(bootstrap *tobari.ContextBootstrapSnapshot) string {
	if bootstrap == nil {
		return "not configured"
	}
	value := "AWS " + safeExternalText(bootstrap.AWS.Profile)
	if bootstrap.EKS != nil {
		value += " · EKS " + safeExternalText(bootstrap.EKS.ContextName)
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
		information = append(information, "Reason: "+safeExternalText(discovery.Reason), "Next: tobari help config bootstrap aws")
	}
	options := []configurationWizardOption{{label: "Continue without AWS bootstrap", description: "Keep future Workspace homes unconfigured.", value: "none"}}
	available := []*tobari.ContextBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ContextBootstrapCandidateUnavailable {
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
	available := []*tobari.ContextBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ContextBootstrapCandidateUnavailable {
			information = append(information, "Unavailable: "+safeExternalText(candidate.ContextName)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		eks := candidate.Snapshot.EKS
		options = append(options, configurationWizardOption{label: safeExternalText(eks.ContextName), description: safeExternalText("cluster " + eks.ClusterName + " · region " + eks.Region), value: eks.ContextName})
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

func (w *terminalContextCreateWizard) revalidateBootstrap(ctx context.Context, reviewed *tobari.ContextBootstrapSnapshot) (bool, *tobari.ContextBootstrapSnapshot, error) {
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
		refreshed, err = w.bootstrap.PrepareEKSBootstrap(ctx, refreshed, reviewed.EKS.ContextName)
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
	options := []configurationWizardOption{{label: "Context name"}, {label: "Filesystem access"}, {label: "Network access"}, {label: "Runtime"}, {label: "Workspace bootstrap"}, {label: "Return to review"}}
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
	switch index {
	case 0:
		value, navigation, err := editContextCreateTextRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, "Context name", staged.name, maxContextCreateNameBytes, "Must match [a-z][a-z0-9-]{0,62}.", func(value string) error { return tobari.ValidateName(strings.TrimSpace(value)) }, true)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			staged.name = strings.TrimSpace(value)
			*draft = staged
		}
		return navigation, nil
	case 1:
		value, navigation, err := editContextCreateChoiceRaw(ctx, in, out, lineCount, w.style, contextCreateStepReview, staged.name, []string{"Workspace home and temporary files stay read-write."}, "Project source access", []configurationWizardOption{{label: "Read-write", description: "Allow direct project-source changes."}, {label: "Read-only", description: "Prevent direct project-source changes."}}, staged.sourceIndex)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			staged.sourceIndex = value
			*draft = staged
		}
		return navigation, nil
	case 2:
		navigation, err := reviewContextCreateNetworkRaw(ctx, in, out, lineCount, w.style, &staged)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			*draft = staged
		}
		return navigation, nil
	case 3:
		navigation, err := w.editContextCreateRuntimeRaw(ctx, in, out, lineCount, contextCreateStepReview, &staged)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		if navigation == contextCreateNavigateNext {
			*draft = staged
		}
		return navigation, nil
	case 4:
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
	cloned.methodOverrides = make(map[string]tobari.ContextMethodDecision, len(draft.methodOverrides))
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
		information = append(information, "Reason: "+safeExternalText(discovery.Reason), "Next: tobari help config bootstrap aws")
	}
	options := []configurationWizardOption{{label: "Continue without AWS bootstrap", description: "Keep future Workspace homes unconfigured."}}
	available := []*tobari.ContextBootstrapSnapshot{nil}
	for _, candidate := range discovery.Candidates {
		if candidate.State == tobari.ContextBootstrapCandidateUnavailable {
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
	eksAvailable := []*tobari.ContextBootstrapSnapshot{nil}
	for _, candidate := range discoveredEKS.Candidates {
		if candidate.State == tobari.ContextBootstrapCandidateUnavailable {
			eksInfo = append(eksInfo, "Unavailable: "+safeExternalText(candidate.ContextName)+" — "+safeExternalText(candidate.Reason))
			continue
		}
		eksOptions = append(eksOptions, configurationWizardOption{label: safeExternalText(candidate.ContextName), description: safeExternalText("cluster " + candidate.Snapshot.EKS.ClusterName + " · region " + candidate.Snapshot.EKS.Region)})
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
		label := "Context name"
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
		if _, err := fmt.Fprintln(out, "Use a valid portable Context name."); err != nil {
			return "", err
		}
	}
}

func selectContextCreateMethodPolicyLine(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	name string,
	sourceAccess tobari.ContextSourceAccess,
) (tobari.ContextMethodPolicy, error) {
	if _, err := fmt.Fprintf(out, "Tobari · Network method policy\nContext: %s\nFilesystem: source %s · home read-write · tmpfs read-write\n\n", safeExternalText(name), sourceAccess); err != nil {
		return tobari.ContextMethodPolicy{}, err
	}
	defaultDecision, err := readContextCreateMethodDecision(ctx, in, out, "Other methods (default)", tobari.ContextMethodExactReview)
	if err != nil {
		return tobari.ContextMethodPolicy{}, err
	}
	explicit := make(map[string]tobari.ContextMethodDecision)
	for _, method := range contextCreateHTTPMethods {
		decision, err := readContextCreateMethodDecision(ctx, in, out, method, defaultDecision)
		if err != nil {
			return tobari.ContextMethodPolicy{}, err
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
	fallback tobari.ContextMethodDecision,
) (tobari.ContextMethodDecision, error) {
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
			return tobari.ContextMethodAllow, nil
		case "e", "exact", "exact-review", "exact_review", "review":
			return tobari.ContextMethodExactReview, nil
		case "d", "deny":
			return tobari.ContextMethodDeny, nil
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
	defaultDecision tobari.ContextMethodDecision,
	explicit map[string]tobari.ContextMethodDecision,
) (tobari.ContextMethodPolicy, error) {
	policy := tobari.ContextMethodPolicy{Default: defaultDecision, Overrides: []tobari.ContextMethodOverride{}}
	for _, method := range contextCreateHTTPMethods {
		if decision, ok := explicit[method]; ok && decision != defaultDecision {
			policy.Overrides = append(policy.Overrides, tobari.ContextMethodOverride{Method: method, Decision: decision})
		}
	}
	return tobari.NormalizeContextMethodPolicy(policy)
}

func displayMethodDecision(decision tobari.ContextMethodDecision) string {
	if decision == tobari.ContextMethodExactReview {
		return "exact review"
	}
	return string(decision)
}

func methodDecisionStyle(decision tobari.ContextMethodDecision) styleToken {
	switch decision {
	case tobari.ContextMethodAllow:
		return styleSuccess
	case tobari.ContextMethodDeny:
		return styleWarning
	default:
		return styleText
	}
}
