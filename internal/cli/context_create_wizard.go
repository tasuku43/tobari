package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

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
}

type contextCreateWizard interface {
	Compose(context.Context, io.Reader, io.Writer) (contextCreateSelection, error)
}

type terminalContextCreateWizard struct {
	mode  terminal.Mode
	style bool
}

func newContextCreateWizardWithStyle(style bool) *terminalContextCreateWizard {
	return &terminalContextCreateWizard{mode: terminal.New(), style: style}
}

func (w *terminalContextCreateWizard) Compose(
	ctx context.Context, in io.Reader, out io.Writer,
) (contextCreateSelection, error) {
	name, err := readContextCreateName(ctx, in, out)
	if err != nil {
		return contextCreateSelection{}, err
	}
	chooser := &terminalContextConfigurationWizard{mode: w.mode, style: w.style}
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
	policy, err := w.chooseMethodPolicy(ctx, in, out, name, sourceAccess)
	if err != nil {
		return contextCreateSelection{}, err
	}
	bootstrapIndex, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Workspace bootstrap", contextName: name, current: "not created",
		information: []string{"A typed snapshot is applied once only to newly created Workspace homes.", "Credentials, caches, helpers, and unknown directives are never copied."},
		prompt:      "Bootstrap", options: []configurationWizardOption{
			{label: "None", description: "Start future Workspace homes without imported tool configuration.", value: "none"},
			{label: "AWS IAM Identity Center", description: "Normalize one host AWS shared-config profile.", value: "aws"},
		},
	})
	if err != nil {
		return contextCreateSelection{}, err
	}
	bootstrapProfile := ""
	if bootstrapIndex == 1 {
		bootstrapProfile, err = readConfigurationWizardValue(ctx, in, out, "AWS profile", 64)
		if err != nil {
			return contextCreateSelection{}, err
		}
		bootstrapProfile = strings.TrimSpace(bootstrapProfile)
		if bootstrapProfile == "" {
			return contextCreateSelection{}, fmt.Errorf("AWS profile is required")
		}
	}
	selection := contextCreateSelection{Name: name, SourceAccess: sourceAccess, MethodPolicy: policy, AWSBootstrapProfile: bootstrapProfile}
	if err := tobari.ValidateName(selection.Name); err != nil {
		return contextCreateSelection{}, err
	}
	if err := selection.SourceAccess.Validate(); err != nil {
		return contextCreateSelection{}, err
	}
	if err := selection.MethodPolicy.Validate(); err != nil {
		return contextCreateSelection{}, err
	}
	return selection, nil
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

func (w *terminalContextCreateWizard) chooseMethodPolicy(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	name string,
	sourceAccess tobari.ContextSourceAccess,
) (tobari.PolicyPresetMethodPolicy, error) {
	if w != nil && w.mode != nil {
		restore, rawErr := w.mode.Enter(in)
		if rawErr == nil {
			policy, selectErr := selectContextCreateMethodPolicyRaw(ctx, in, out, name, sourceAccess, w.style)
			restoreErr := restore()
			if selectErr != nil {
				return tobari.PolicyPresetMethodPolicy{}, selectErr
			}
			if restoreErr != nil {
				return tobari.PolicyPresetMethodPolicy{}, restoreErr
			}
			return policy, nil
		}
	}
	return selectContextCreateMethodPolicyLine(ctx, in, out, name, sourceAccess)
}

func selectContextCreateMethodPolicyRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	name string,
	sourceAccess tobari.ContextSourceAccess,
	style bool,
) (tobari.PolicyPresetMethodPolicy, error) {
	defaultDecision := tobari.PolicyPresetMethodExactReview
	explicit := make(map[string]tobari.PolicyPresetMethodDecision)
	selected := 0
	lineCount := 0
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			finishSelectorScreen(out, lineCount)
			return tobari.PolicyPresetMethodPolicy{}, err
		}
		lines := []string{
			selectorTitle(style, "Tobari · Network method policy"),
			selectorDetail(style, "Context", safeExternalText(name), styleText),
			selectorDetail(style, "Filesystem", "source "+string(sourceAccess)+" · home read-write · tmpfs read-write", styleText),
			selectorHelp(style, "Every method resolves to allow, exact review, or deny."),
			"",
		}
		rows := append([]string{"Other methods (default)"}, contextCreateHTTPMethods...)
		for index, row := range rows {
			marker := "  "
			labelToken := styleText
			if index == selected {
				marker = "❯ "
				labelToken = styleAccent
			}
			decision := defaultDecision
			staged := " "
			if index > 0 {
				if value, ok := explicit[row]; ok {
					decision = value
					staged = "*"
				}
			}
			lines = append(lines, fmt.Sprintf("%s%s %-24s %s", marker, staged, applyStyleToken(style, labelToken, row), applyStyleToken(style, methodDecisionStyle(decision), displayMethodDecision(decision))))
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, message))
		}
		lines = append(lines, "", selectorHelp(style, "↑/↓ move   a allow   e exact review   d deny"), selectorHelp(style, "p Create   q cancel"))
		var err error
		lineCount, err = renderSelectorScreen(out, lines, lineCount)
		if err != nil {
			return tobari.PolicyPresetMethodPolicy{}, err
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishSelectorScreen(out, lineCount)
			return tobari.PolicyPresetMethodPolicy{}, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = (selected - 1 + len(rows)) % len(rows)
		case selectorKeyDown:
			selected = (selected + 1) % len(rows)
		case selectorKeyHome:
			selected = 0
		case selectorKeyEnd:
			selected = len(rows) - 1
		case selectorKeyAllow, selectorKeyExact, selectorKeyDeny:
			decision := tobari.PolicyPresetMethodExactReview
			if key.kind == selectorKeyAllow {
				decision = tobari.PolicyPresetMethodAllow
			} else if key.kind == selectorKeyDeny {
				decision = tobari.PolicyPresetMethodDeny
			}
			if selected == 0 {
				defaultDecision = decision
			} else {
				explicit[rows[selected]] = decision
			}
			message = rows[selected] + " staged as " + displayMethodDecision(decision) + "."
		case selectorKeyApply:
			finishSelectorScreen(out, lineCount)
			return buildContextCreateMethodPolicy(defaultDecision, explicit)
		case selectorKeyCancel:
			finishSelectorScreen(out, lineCount)
			return tobari.PolicyPresetMethodPolicy{}, context.Canceled
		default:
			message = "Use a/e/d to stage a decision, p to Create, or q to cancel."
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
