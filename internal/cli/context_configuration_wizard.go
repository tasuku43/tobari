package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

type contextConfigurationWizard interface {
	ConfigureShell(
		context.Context, tobari.ContextReport, io.Reader, io.Writer,
	) (tobari.ContextShellEnvironmentSetting, error)
	ConfigureGit(
		context.Context, tobari.ContextReport, io.Reader, io.Writer,
	) (tobari.ContextGitIdentitySetting, error)
}

type terminalContextConfigurationWizard struct {
	mode  terminal.Mode
	style bool
}

type configurationWizardOption struct {
	label       string
	description string
	value       string
}

const maxConfigurationWizardChoiceBytes = 64

func newContextConfigurationWizard() *terminalContextConfigurationWizard {
	return newContextConfigurationWizardWithStyle(true)
}

func newContextConfigurationWizardWithStyle(enabled bool) *terminalContextConfigurationWizard {
	return &terminalContextConfigurationWizard{mode: terminal.New(), style: enabled}
}

func (w *terminalContextConfigurationWizard) ConfigureShell(
	ctx context.Context, current tobari.ContextReport, in io.Reader, out io.Writer,
) (tobari.ContextShellEnvironmentSetting, error) {
	variables := tobari.ContextShellEnvironmentVariables()
	variableOptions := make([]configurationWizardOption, 0, len(variables))
	for _, variable := range variables {
		variableOptions = append(variableOptions, configurationWizardOption{label: variable, value: variable})
	}
	variableIndex, err := w.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Shell configuration", contextName: current.Name,
		current: "Choose a variable to inspect and change.",
		prompt:  "Choose a shell variable", options: variableOptions,
	})
	if err != nil {
		return tobari.ContextShellEnvironmentSetting{}, err
	}
	variable := variableOptions[variableIndex].value
	setting, found := contextShellSetting(current.ShellEnvironment, variable)
	if !found {
		return tobari.ContextShellEnvironmentSetting{}, fmt.Errorf("current Context shell setting %q is absent", variable)
	}

	sourceOptions := []configurationWizardOption{
		{label: "Use Tobari default", description: "Remove this Context override.", value: string(tobari.ContextShellEnvironmentDefault)},
		{label: "Inherit host value", description: "Read the exported host value on each Workspace entry.", value: string(tobari.ContextShellEnvironmentInherit)},
		{label: "Use a fixed value", description: "Store one Context-owned literal.", value: string(tobari.ContextShellEnvironmentLiteral)},
	}
	sourceIndex, err := w.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Shell configuration", contextName: current.Name,
		current: shellSettingSummary(setting), prompt: "Choose a source for " + variable,
		options: sourceOptions,
	})
	if err != nil {
		return tobari.ContextShellEnvironmentSetting{}, err
	}
	change := tobari.ContextShellEnvironmentSetting{
		Variable: variable,
		Source:   tobari.ContextShellEnvironmentSource(sourceOptions[sourceIndex].value),
	}
	if change.Source == tobari.ContextShellEnvironmentLiteral {
		value, readErr := readConfigurationWizardValue(
			ctx, in, out, "Fixed value", tobari.MaxContextShellValueBytes,
		)
		if readErr != nil {
			return tobari.ContextShellEnvironmentSetting{}, readErr
		}
		change.Value = &value
	}

	selected := shellSettingSummary(change)
	review, err := w.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Shell configuration", contextName: current.Name,
		current: selected, prompt: "Apply this setting?",
		options: []configurationWizardOption{
			{label: "Apply", description: "Atomically update the Context setting."},
			{label: "Cancel", description: "Leave the Context unchanged."},
		},
	})
	if err != nil {
		return tobari.ContextShellEnvironmentSetting{}, err
	}
	if review != 0 {
		return tobari.ContextShellEnvironmentSetting{}, context.Canceled
	}
	return change, nil
}

func (w *terminalContextConfigurationWizard) ConfigureGit(
	ctx context.Context, current tobari.ContextReport, in io.Reader, out io.Writer,
) (tobari.ContextGitIdentitySetting, error) {
	sourceOptions := []configurationWizardOption{
		{label: "Inherit host identity", description: "Resolve host global user.name and user.email on Workspace entry.", value: string(tobari.ContextGitIdentityInherit)},
		{label: "Use a fixed identity", description: "Store one Context-owned name and email pair.", value: string(tobari.ContextGitIdentityLiteral)},
		{label: "No Context fallback", description: "Remove the Context identity projection.", value: string(tobari.ContextGitIdentityDefault)},
	}
	sourceIndex, err := w.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Git identity", contextName: current.Name,
		current: gitIdentitySummary(current.GitIdentity),
		information: []string{
			"Only user.name and user.email are projected.",
			"Authentication, signing, helpers, aliases, and other Git configuration stay isolated.",
		},
		prompt: "Choose an identity source", options: sourceOptions,
	})
	if err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	change := tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentitySource(sourceOptions[sourceIndex].value),
	}
	if change.Source == tobari.ContextGitIdentityLiteral {
		name, readErr := readConfigurationWizardValue(
			ctx, in, out, "Git user.name", tobari.MaxContextGitIdentityValueBytes,
		)
		if readErr != nil {
			return tobari.ContextGitIdentitySetting{}, readErr
		}
		email, readErr := readConfigurationWizardValue(
			ctx, in, out, "Git user.email", tobari.MaxContextGitIdentityValueBytes,
		)
		if readErr != nil {
			return tobari.ContextGitIdentitySetting{}, readErr
		}
		change.Name, change.Email = &name, &email
	}

	review, err := w.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Git identity", contextName: current.Name,
		current: gitIdentitySummary(change),
		information: []string{
			"Only user.name and user.email are projected.",
			"Authentication, signing, helpers, aliases, and other Git configuration stay isolated.",
		},
		prompt: "Apply this setting?",
		options: []configurationWizardOption{
			{label: "Apply", description: "Atomically update the Context identity policy."},
			{label: "Cancel", description: "Leave the Context unchanged."},
		},
	})
	if err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	if review != 0 {
		return tobari.ContextGitIdentitySetting{}, context.Canceled
	}
	return change, nil
}

type configurationWizardMenu struct {
	title       string
	contextName string
	current     string
	information []string
	prompt      string
	options     []configurationWizardOption
}

func (w *terminalContextConfigurationWizard) choose(
	ctx context.Context, in io.Reader, out io.Writer, menu configurationWizardMenu,
) (int, error) {
	if len(menu.options) == 0 {
		return 0, fmt.Errorf("configuration wizard has no options")
	}
	if w != nil && w.mode != nil {
		restore, rawErr := w.mode.Enter(in)
		if rawErr == nil {
			selected, selectErr := selectConfigurationWizardRaw(ctx, in, out, menu, w.style)
			restoreErr := restore()
			if selectErr != nil {
				return 0, selectErr
			}
			if restoreErr != nil {
				return 0, restoreErr
			}
			return selected, nil
		}
	}
	return selectConfigurationWizardLine(ctx, in, out, menu)
}

func selectConfigurationWizardRaw(
	ctx context.Context, in io.Reader, out io.Writer, menu configurationWizardMenu, style bool,
) (int, error) {
	selected := 0
	message := ""
	lineCount := 0
	for {
		if err := ctx.Err(); err != nil {
			finishSelectorScreen(out, lineCount)
			return 0, err
		}
		currentLines, err := renderConfigurationWizardRaw(out, menu, selected, message, lineCount, style)
		if err != nil {
			finishSelectorScreen(out, lineCount)
			return 0, err
		}
		lineCount = currentLines
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishSelectorScreen(out, lineCount)
			return 0, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = (selected - 1 + len(menu.options)) % len(menu.options)
			message = ""
		case selectorKeyDown:
			selected = (selected + 1) % len(menu.options)
			message = ""
		case selectorKeyHome:
			selected = 0
			message = ""
		case selectorKeyEnd:
			selected = len(menu.options) - 1
			message = ""
		case selectorKeyNumber:
			if key.index >= 0 && key.index < len(menu.options) {
				finishSelectorScreen(out, lineCount)
				return key.index, nil
			}
			message = "That option does not exist."
		case selectorKeyEnter:
			finishSelectorScreen(out, lineCount)
			return selected, nil
		case selectorKeyCancel:
			finishSelectorScreen(out, lineCount)
			return 0, context.Canceled
		default:
			message = "Use ↑/↓ to move, Enter to choose, or q to cancel."
		}
	}
}

func renderConfigurationWizardRaw(
	out io.Writer, menu configurationWizardMenu, selected int, message string, previousLines int, style bool,
) (int, error) {
	lines := []string{
		selectorTitle(style, menu.title),
		selectorDetail(style, "Context", safeExternalText(menu.contextName), styleText),
		selectorDetail(style, "Current", safeExternalText(menu.current), styleText),
	}
	for _, information := range menu.information {
		lines = append(lines, selectorHelp(style, safeExternalText(information)))
	}
	lines = append(lines, "", applyStyleToken(style, styleText, menu.prompt+":"))
	for index, option := range menu.options {
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
	lines = append(lines, "", selectorHelp(style, "↑/↓ move   Enter choose   q cancel"))
	if previousLines > 0 {
		if _, err := fmt.Fprintf(out, "\x1b[%dA\r\x1b[J", previousLines); err != nil {
			return 0, err
		}
	}
	if _, err := io.WriteString(out, "\x1b[?25l"+strings.Join(lines, "\n")+"\n"); err != nil {
		return 0, err
	}
	return len(lines), nil
}

func selectConfigurationWizardLine(
	ctx context.Context, in io.Reader, out io.Writer, menu configurationWizardMenu,
) (int, error) {
	if _, err := fmt.Fprintf(out, "%s\nContext: %s\nCurrent: %s\n", menu.title, safeExternalText(menu.contextName), safeExternalText(menu.current)); err != nil {
		return 0, err
	}
	for _, information := range menu.information {
		if _, err := fmt.Fprintln(out, safeExternalText(information)); err != nil {
			return 0, err
		}
	}
	if _, err := fmt.Fprintf(out, "\n%s:\n", menu.prompt); err != nil {
		return 0, err
	}
	for index, option := range menu.options {
		if _, err := fmt.Fprintf(out, "  %d. %s", index+1, option.label); err != nil {
			return 0, err
		}
		if option.description != "" {
			if _, err := fmt.Fprintf(out, " — %s", option.description); err != nil {
				return 0, err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return 0, err
		}
	}
	if _, err := fmt.Fprint(out, "Choose a number, or q to cancel [1]: "); err != nil {
		return 0, err
	}
	for {
		line, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
		if err != nil {
			return 0, err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			return 0, nil
		}
		if strings.EqualFold(value, "q") || strings.EqualFold(value, "quit") || strings.EqualFold(value, "esc") {
			return 0, context.Canceled
		}
		choice, parseErr := strconv.Atoi(value)
		if parseErr == nil && choice >= 1 && choice <= len(menu.options) {
			return choice - 1, nil
		}
		if _, writeErr := fmt.Fprint(out, "Enter a listed number or q: "); writeErr != nil {
			return 0, writeErr
		}
	}
}

func readConfigurationWizardValue(
	ctx context.Context, in io.Reader, out io.Writer, label string, maxBytes int,
) (string, error) {
	if _, err := fmt.Fprintf(out, "%s: ", label); err != nil {
		return "", err
	}
	return readConfigurationWizardLine(ctx, in, maxBytes)
}

func readConfigurationWizardLine(ctx context.Context, in io.Reader, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		return "", fmt.Errorf("configuration wizard input bound is invalid")
	}
	var value strings.Builder
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var octet [1]byte
		count, err := in.Read(octet[:])
		if count > 0 {
			switch octet[0] {
			case '\n', '\r':
				return value.String(), nil
			case 3, 4, 27:
				return "", context.Canceled
			default:
				if value.Len() >= maxBytes {
					return "", fmt.Errorf("configuration wizard input exceeds %d bytes", maxBytes)
				}
				value.WriteByte(octet[0])
			}
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if value.Len() == 0 {
					return "", context.Canceled
				}
				return value.String(), nil
			}
			return "", err
		}
	}
}

func contextShellSetting(
	settings []tobari.ContextShellEnvironmentSetting, variable string,
) (tobari.ContextShellEnvironmentSetting, bool) {
	for _, setting := range settings {
		if setting.Variable == variable {
			return setting, true
		}
	}
	return tobari.ContextShellEnvironmentSetting{}, false
}

func shellSettingSummary(setting tobari.ContextShellEnvironmentSetting) string {
	value := setting.Variable + " · " + string(setting.Source)
	if setting.Source == tobari.ContextShellEnvironmentLiteral && setting.Value != nil {
		value += " · " + fmt.Sprintf("%q", safeExternalText(*setting.Value))
	}
	return value
}

func gitIdentitySummary(setting tobari.ContextGitIdentitySetting) string {
	value := string(setting.Source)
	if setting.Source == tobari.ContextGitIdentityLiteral && setting.Name != nil && setting.Email != nil {
		value += " · " + fmt.Sprintf("%q <%s>", safeExternalText(*setting.Name), safeExternalText(*setting.Email))
	}
	return value
}
