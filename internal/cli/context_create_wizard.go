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

var contextCreateHTTPMethods = []string{
	"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE",
}

type contextCreateSelection struct {
	Name                string
	SourceAccess        tobari.ContextSourceAccess
	MethodPolicy        tobari.PolicyPresetMethodPolicy
	AWSBootstrapProfile string
	EKSBootstrapContext string
}

type contextCreateWizard interface {
	Compose(context.Context, io.Reader, io.Writer) (contextCreateSelection, error)
}

type terminalContextCreateWizard struct {
	mode  terminal.Mode
	style bool
}

type contextCreateRawStep uint8

const (
	contextCreateStepName contextCreateRawStep = iota
	contextCreateStepFilesystem
	contextCreateStepNetwork
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
	name                string
	sourceIndex         int
	methodSelected      int
	methodDefault       tobari.PolicyPresetMethodDecision
	methodOverrides     map[string]tobari.PolicyPresetMethodDecision
	bootstrapIndex      int
	bootstrapProfile    string
	bootstrapEKSContext string
}

func newContextCreateWizardWithStyle(style bool) *terminalContextCreateWizard {
	return &terminalContextCreateWizard{mode: terminal.New(), style: style}
}

func (w *terminalContextCreateWizard) Compose(
	ctx context.Context, in io.Reader, out io.Writer,
) (contextCreateSelection, error) {
	if w != nil && w.mode != nil {
		restore, rawErr := w.mode.Enter(in)
		if rawErr == nil {
			selection, composeErr := w.composeRaw(ctx, in, out)
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
	return w.composeLine(ctx, in, out)
}

func (w *terminalContextCreateWizard) composeLine(
	ctx context.Context, in io.Reader, out io.Writer,
) (contextCreateSelection, error) {
	name, err := readContextCreateName(ctx, in, out)
	if err != nil {
		return contextCreateSelection{}, err
	}
	chooser := &terminalContextConfigurationWizard{mode: nil, style: w.style}
	sourceIndex, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title:       "Tobari · Create Context",
		contextName: name,
		current:     "not created",
		information: []string{"Workspace home and tmpfs stay read-write in both choices."},
		prompt:      "Project source access",
		options: []configurationWizardOption{
			{label: "Read-write", description: "Allow direct project-source changes.", value: string(tobari.ContextSourceAccessReadWrite)},
			{label: "Read-only", description: "Prevent direct project-source changes.", value: string(tobari.ContextSourceAccessReadOnly)},
		},
	})
	if err != nil {
		return contextCreateSelection{}, err
	}
	sourceAccess := tobari.ContextSourceAccess([]tobari.ContextSourceAccess{
		tobari.ContextSourceAccessReadWrite,
		tobari.ContextSourceAccessReadOnly,
	}[sourceIndex])
	policy, err := selectContextCreateMethodPolicyLine(ctx, in, out, name, sourceAccess)
	if err != nil {
		return contextCreateSelection{}, err
	}
	bootstrapIndex, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Workspace bootstrap", contextName: name, current: "not created",
		information: []string{"A typed snapshot is applied once only to newly created Workspace homes.", "Credentials, caches, helpers, and unknown directives are never copied."},
		prompt:      "Bootstrap", options: []configurationWizardOption{
			{label: "None", description: "Start future Workspace homes without imported tool configuration.", value: "none"},
			{label: "AWS IAM Identity Center", description: "Normalize one host AWS shared-config profile.", value: "aws"},
			{label: "AWS + Amazon EKS", description: "Add one reviewed EKS context using the same AWS profile.", value: "eks"},
		},
	})
	if err != nil {
		return contextCreateSelection{}, err
	}
	bootstrapProfile := ""
	if bootstrapIndex > 0 {
		bootstrapProfile, err = readConfigurationWizardValue(ctx, in, out, "AWS profile", 64)
		if err != nil {
			return contextCreateSelection{}, err
		}
		bootstrapProfile = strings.TrimSpace(bootstrapProfile)
		if bootstrapProfile == "" {
			return contextCreateSelection{}, fmt.Errorf("AWS profile is required")
		}
	}
	bootstrapEKSContext := ""
	if bootstrapIndex == 2 {
		bootstrapEKSContext, err = readConfigurationWizardValue(ctx, in, out, "Kubernetes context", 253)
		if err != nil {
			return contextCreateSelection{}, err
		}
		bootstrapEKSContext = strings.TrimSpace(bootstrapEKSContext)
		if bootstrapEKSContext == "" {
			return contextCreateSelection{}, fmt.Errorf("Kubernetes context is required")
		}
	}
	selection := contextCreateSelection{Name: name, SourceAccess: sourceAccess, MethodPolicy: policy, AWSBootstrapProfile: bootstrapProfile, EKSBootstrapContext: bootstrapEKSContext}
	if err := tobari.ValidateName(selection.Name); err != nil {
		return contextCreateSelection{}, err
	}
	if err := selection.SourceAccess.Validate(); err != nil {
		return contextCreateSelection{}, err
	}
	if err := selection.MethodPolicy.Validate(); err != nil {
		return contextCreateSelection{}, err
	}
	if err := reviewContextCreateLine(ctx, in, out, selection); err != nil {
		return contextCreateSelection{}, err
	}
	return selection, nil
}

func (w *terminalContextCreateWizard) composeRaw(
	ctx context.Context, in io.Reader, out io.Writer,
) (contextCreateSelection, error) {
	draft := contextCreateRawDraft{
		sourceIndex:     0,
		methodDefault:   tobari.PolicyPresetMethodExactReview,
		methodOverrides: make(map[string]tobari.PolicyPresetMethodDecision),
		bootstrapIndex:  0,
	}
	step := contextCreateStepName
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
			navigation, err = editContextCreateMethodPolicyRaw(ctx, in, out, &lineCount, w.style, &draft)
		case contextCreateStepBootstrap:
			for {
				draft.bootstrapIndex, navigation, err = editContextCreateChoiceRaw(
					ctx, in, out, &lineCount, w.style, step, draft.name,
					[]string{
						"A typed snapshot is applied once only to newly created Workspace homes.",
						"Credentials, caches, helpers, and unknown directives are never copied.",
					},
					"Workspace bootstrap", []configurationWizardOption{
						{label: "None", description: "Start without imported tool configuration.", value: "none"},
						{label: "AWS IAM Identity Center", description: "Normalize one host AWS shared-config profile.", value: "aws"},
						{label: "AWS + Amazon EKS", description: "Add one reviewed EKS context using the same AWS profile.", value: "eks"},
					}, draft.bootstrapIndex,
				)
				if err != nil || navigation != contextCreateNavigateNext || draft.bootstrapIndex == 0 {
					break
				}
				draft.bootstrapProfile, navigation, err = editContextCreateTextRaw(
					ctx, in, out, &lineCount, w.style, step, "AWS profile", draft.bootstrapProfile,
					64, "Profile name from the host AWS shared config.",
					func(value string) error {
						if strings.TrimSpace(value) == "" {
							return fmt.Errorf("AWS profile is required")
						}
						return nil
					}, true,
				)
				draft.bootstrapProfile = strings.TrimSpace(draft.bootstrapProfile)
				if err != nil || navigation == contextCreateNavigateCancel {
					break
				}
				if navigation == contextCreateNavigateBack {
					continue
				}
				if draft.bootstrapIndex == 2 {
					draft.bootstrapEKSContext, navigation, err = editContextCreateTextRaw(
						ctx, in, out, &lineCount, w.style, step, "Kubernetes context", draft.bootstrapEKSContext,
						253, "Context name from fixed host ~/.kube/config.",
						func(value string) error {
							if strings.TrimSpace(value) == "" {
								return fmt.Errorf("Kubernetes context is required")
							}
							return nil
						}, true,
					)
					draft.bootstrapEKSContext = strings.TrimSpace(draft.bootstrapEKSContext)
					if navigation == contextCreateNavigateBack {
						continue
					}
				}
				break
			}
		case contextCreateStepReview:
			navigation, err = reviewContextCreateRaw(ctx, in, out, &lineCount, w.style, draft)
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
			if step > contextCreateStepName {
				step--
			}
			continue
		}
		if step == contextCreateStepReview {
			break
		}
		step++
	}

	policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
	if err != nil {
		return contextCreateSelection{}, err
	}
	selection := contextCreateSelection{
		Name: draft.name,
		SourceAccess: []tobari.ContextSourceAccess{
			tobari.ContextSourceAccessReadWrite,
			tobari.ContextSourceAccessReadOnly,
		}[draft.sourceIndex],
		MethodPolicy: policy,
	}
	if draft.bootstrapIndex > 0 {
		selection.AWSBootstrapProfile = draft.bootstrapProfile
	}
	if draft.bootstrapIndex == 2 {
		selection.EKSBootstrapContext = draft.bootstrapEKSContext
	}
	return selection, nil
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
		}
		for index, row := range rows {
			marker := "  "
			labelToken := styleText
			if index == draft.methodSelected {
				marker = "❯ "
				labelToken = styleAccent
			}
			decision := draft.methodDefault
			staged := " "
			if index > 0 {
				if value, ok := draft.methodOverrides[row]; ok {
					decision = value
					staged = "*"
				}
			}
			lines = append(lines, fmt.Sprintf("%s%s %-24s %s", marker, staged, applyStyleToken(style, labelToken, row), applyStyleToken(style, methodDecisionStyle(decision), displayMethodDecision(decision))))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "↑/↓ move   a allow   e exact review   d deny"), selectorHelp(style, "Enter continue   b back   q cancel"))
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
			decision := tobari.PolicyPresetMethodExactReview
			if key.kind == selectorKeyAllow {
				decision = tobari.PolicyPresetMethodAllow
			} else if key.kind == selectorKeyDeny {
				decision = tobari.PolicyPresetMethodDeny
			}
			if draft.methodSelected == 0 {
				draft.methodDefault = decision
			} else {
				draft.methodOverrides[rows[draft.methodSelected]] = decision
			}
			message = rows[draft.methodSelected] + " staged as " + displayMethodDecision(decision) + "."
		case selectorKeyEnter, selectorKeyApply:
			return contextCreateNavigateNext, nil
		case selectorKeyBack:
			return contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return contextCreateNavigateCancel, nil
		default:
			message = "Use a/e/d to stage a decision, Enter to continue, b to go back, or q to cancel."
		}
	}
}

func reviewContextCreateLine(
	ctx context.Context, in io.Reader, out io.Writer, selection contextCreateSelection,
) error {
	overrides := "none"
	if len(selection.MethodPolicy.Overrides) > 0 {
		parts := make([]string, 0, len(selection.MethodPolicy.Overrides))
		for _, override := range selection.MethodPolicy.Overrides {
			parts = append(parts, override.Method+"="+displayMethodDecision(override.Decision))
		}
		overrides = strings.Join(parts, " · ")
	}
	bootstrap := "none"
	if selection.AWSBootstrapProfile != "" {
		bootstrap = "AWS IAM Identity Center · profile " + safeExternalText(selection.AWSBootstrapProfile)
	}
	if selection.EKSBootstrapContext != "" {
		bootstrap += " · EKS context " + safeExternalText(selection.EKSBootstrapContext)
	}
	if _, err := fmt.Fprintf(
		out,
		"Tobari · Create Context · Review & Create\nName: %s\nRuntime: standard Tobari runtime (builtin)\nProject source: %s\nWorkspace home: read-write\nTmpfs: read-write\nNetwork default: %s\nMethod overrides: %s\nBootstrap: %s\n\nCreate this Context? [y/N]: ",
		safeExternalText(selection.Name), selection.SourceAccess,
		displayMethodDecision(selection.MethodPolicy.Default), overrides, bootstrap,
	); err != nil {
		return err
	}
	for {
		answer, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return nil
		case "", "n", "no", "q", "quit":
			return context.Canceled
		default:
			if _, err := fmt.Fprint(out, "Enter y to create or n to cancel: "); err != nil {
				return err
			}
		}
	}
}

func reviewContextCreateRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	lineCount *int,
	style bool,
	draft contextCreateRawDraft,
) (contextCreateRawNavigation, error) {
	policy, err := buildContextCreateMethodPolicy(draft.methodDefault, draft.methodOverrides)
	if err != nil {
		return contextCreateNavigateCancel, err
	}
	sourceAccess := []tobari.ContextSourceAccess{
		tobari.ContextSourceAccessReadWrite,
		tobari.ContextSourceAccessReadOnly,
	}[draft.sourceIndex]
	overrides := "none"
	if len(policy.Overrides) > 0 {
		parts := make([]string, 0, len(policy.Overrides))
		for _, override := range policy.Overrides {
			parts = append(parts, override.Method+"="+displayMethodDecision(override.Decision))
		}
		overrides = strings.Join(parts, " · ")
	}
	bootstrap := "none"
	if draft.bootstrapIndex == 1 {
		bootstrap = "AWS IAM Identity Center · profile " + safeExternalText(draft.bootstrapProfile)
	} else if draft.bootstrapIndex == 2 {
		bootstrap = "AWS IAM Identity Center · profile " + safeExternalText(draft.bootstrapProfile) + " · EKS context " + safeExternalText(draft.bootstrapEKSContext)
	}
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			return contextCreateNavigateCancel, err
		}
		lines := []string{
			selectorTitle(style, "Tobari · Create Context"),
			selectorDetail(style, "Step", contextCreateStepLabel(contextCreateStepReview), styleText),
			"",
			selectorDetail(style, "Name", safeExternalText(draft.name), styleText),
			selectorDetail(style, "Runtime", "standard Tobari runtime (builtin)", styleText),
			selectorDetail(style, "Project source", string(sourceAccess), styleText),
			selectorDetail(style, "Workspace home", "read-write", styleText),
			selectorDetail(style, "Tmpfs", "read-write", styleText),
			selectorDetail(style, "Network default", displayMethodDecision(policy.Default), methodDecisionStyle(policy.Default)),
			selectorDetail(style, "Method overrides", overrides, styleText),
			selectorDetail(style, "Bootstrap", bootstrap, styleText),
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "Enter Create   b back   q cancel"))
		*lineCount, err = renderSelectorScreen(out, lines, *lineCount)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return contextCreateNavigateCancel, err
		}
		switch key.kind {
		case selectorKeyEnter, selectorKeyApply, selectorKeyCreate:
			return contextCreateNavigateNext, nil
		case selectorKeyBack:
			return contextCreateNavigateBack, nil
		case selectorKeyCancel:
			return contextCreateNavigateCancel, nil
		default:
			message = "Press Enter to create, b to go back, or q to cancel."
		}
	}
}

func contextCreateStepLabel(step contextCreateRawStep) string {
	labels := []string{"1 of 5 · Name", "2 of 5 · Filesystem", "3 of 5 · Network", "4 of 5 · Workspace bootstrap", "5 of 5 · Review & Create"}
	if int(step) < 0 || int(step) >= len(labels) {
		return "unknown"
	}
	return labels[step]
}

func readContextCreateName(ctx context.Context, in io.Reader, out io.Writer) (string, error) {
	for {
		name, err := readConfigurationWizardValue(ctx, in, out, "Context name", maxContextCreateNameBytes)
		if err != nil {
			return "", err
		}
		name = strings.TrimSpace(name)
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
) (tobari.PolicyPresetMethodPolicy, error) {
	if _, err := fmt.Fprintf(out, "Tobari · Network method policy\nContext: %s\nFilesystem: source %s · home read-write · tmpfs read-write\n\n", safeExternalText(name), sourceAccess); err != nil {
		return tobari.PolicyPresetMethodPolicy{}, err
	}
	defaultDecision, err := readContextCreateMethodDecision(ctx, in, out, "Other methods (default)", tobari.PolicyPresetMethodExactReview)
	if err != nil {
		return tobari.PolicyPresetMethodPolicy{}, err
	}
	explicit := make(map[string]tobari.PolicyPresetMethodDecision)
	for _, method := range contextCreateHTTPMethods {
		decision, err := readContextCreateMethodDecision(ctx, in, out, method, defaultDecision)
		if err != nil {
			return tobari.PolicyPresetMethodPolicy{}, err
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
	fallback tobari.PolicyPresetMethodDecision,
) (tobari.PolicyPresetMethodDecision, error) {
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
			return tobari.PolicyPresetMethodAllow, nil
		case "e", "exact", "exact-review", "exact_review", "review":
			return tobari.PolicyPresetMethodExactReview, nil
		case "d", "deny":
			return tobari.PolicyPresetMethodDeny, nil
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
	defaultDecision tobari.PolicyPresetMethodDecision,
	explicit map[string]tobari.PolicyPresetMethodDecision,
) (tobari.PolicyPresetMethodPolicy, error) {
	policy := tobari.PolicyPresetMethodPolicy{Default: defaultDecision, Overrides: []tobari.PolicyPresetMethodOverride{}}
	for _, method := range contextCreateHTTPMethods {
		if decision, ok := explicit[method]; ok && decision != defaultDecision {
			policy.Overrides = append(policy.Overrides, tobari.PolicyPresetMethodOverride{Method: method, Decision: decision})
		}
	}
	return tobari.NormalizePolicyPresetMethodPolicy(policy)
}

func displayMethodDecision(decision tobari.PolicyPresetMethodDecision) string {
	if decision == tobari.PolicyPresetMethodExactReview {
		return "exact review"
	}
	return string(decision)
}

func methodDecisionStyle(decision tobari.PolicyPresetMethodDecision) styleToken {
	switch decision {
	case tobari.PolicyPresetMethodAllow:
		return styleSuccess
	case tobari.PolicyPresetMethodDeny:
		return styleWarning
	default:
		return styleText
	}
}
