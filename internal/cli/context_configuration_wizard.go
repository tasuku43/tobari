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
	) ([]tobari.ContextShellEnvironmentSetting, error)
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
) ([]tobari.ContextShellEnvironmentSetting, error) {
	pending := cloneShellSettings(current.ShellEnvironment)
	staged := make(map[string]tobari.ContextShellEnvironmentSetting)
	selected := 0
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if w != nil && w.mode != nil {
			restore, rawErr := w.mode.Enter(in)
			if rawErr == nil {
				action, nextSelected, selectErr := selectConfigurationShellRaw(
					ctx, in, out, current.Name, pending, staged, selected, message, w.style,
				)
				restoreErr := restore()
				if selectErr != nil {
					return nil, selectErr
				}
				if restoreErr != nil {
					return nil, restoreErr
				}
				selected = nextSelected
				switch action {
				case configurationShellActionApply:
					return stagedShellChanges(staged), nil
				case configurationShellActionLiteral:
					value, readErr := readConfigurationWizardValue(
						ctx, in, out, "Fixed value for "+pending[selected].Variable, tobari.MaxContextShellValueBytes,
					)
					if readErr != nil {
						return nil, readErr
					}
					setting := tobari.ContextShellEnvironmentSetting{
						Variable: pending[selected].Variable, Source: tobari.ContextShellEnvironmentLiteral, Value: &value,
					}
					pending[selected] = setting
					staged[setting.Variable] = setting
					message = setting.Variable + " staged as literal."
					continue
				default:
					return nil, context.Canceled
				}
			}
		}
		return selectConfigurationShellLine(ctx, current.Name, pending, in, out)
	}
}

func (w *terminalContextConfigurationWizard) ConfigureGit(
	ctx context.Context, current tobari.ContextReport, in io.Reader, out io.Writer,
) (tobari.ContextGitIdentitySetting, error) {
	pending := cloneGitIdentitySetting(current.GitIdentity)
	selected := gitIdentitySourceIndex(pending.Source)
	staged := false
	message := ""
	for {
		if err := ctx.Err(); err != nil {
			return tobari.ContextGitIdentitySetting{}, err
		}
		if w != nil && w.mode != nil {
			restore, rawErr := w.mode.Enter(in)
			if rawErr == nil {
				action, next, nextSelected, selectErr := selectConfigurationGitRaw(
					ctx, in, out, current.Name, current.GitIdentity, pending, staged, selected, message, w.style,
				)
				restoreErr := restore()
				if selectErr != nil {
					return tobari.ContextGitIdentitySetting{}, selectErr
				}
				if restoreErr != nil {
					return tobari.ContextGitIdentitySetting{}, restoreErr
				}
				pending, selected = next, nextSelected
				switch action {
				case configurationGitActionApply:
					return pending, nil
				case configurationGitActionLiteral:
					name, readErr := readConfigurationWizardValue(ctx, in, out, "Git user.name", tobari.MaxContextGitIdentityValueBytes)
					if readErr != nil {
						return tobari.ContextGitIdentitySetting{}, readErr
					}
					email, readErr := readConfigurationWizardValue(ctx, in, out, "Git user.email", tobari.MaxContextGitIdentityValueBytes)
					if readErr != nil {
						return tobari.ContextGitIdentitySetting{}, readErr
					}
					pending = tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityLiteral, Name: &name, Email: &email}
					staged = true
					message = "Fixed identity staged."
					continue
				default:
					return tobari.ContextGitIdentitySetting{}, context.Canceled
				}
			}
		}
		return selectConfigurationGitLine(ctx, current.Name, current.GitIdentity, in, out)
	}
}

type configurationShellAction uint8

const (
	configurationShellActionCancel configurationShellAction = iota
	configurationShellActionLiteral
	configurationShellActionApply
)

func selectConfigurationShellRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	contextName string,
	pending []tobari.ContextShellEnvironmentSetting,
	staged map[string]tobari.ContextShellEnvironmentSetting,
	selected int,
	message string,
	style bool,
) (configurationShellAction, int, error) {
	if len(pending) == 0 || selected < 0 || selected >= len(pending) {
		return configurationShellActionCancel, selected, fmt.Errorf("configuration shell editor state is invalid")
	}
	lineCount := 0
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishSelectorScreen(out, lineCount)
			return configurationShellActionCancel, selected, err
		}
		if needsRender {
			currentLines, err := renderConfigurationShellRaw(
				out, contextName, pending, staged, selected, message, lineCount, style,
			)
			if err != nil {
				finishSelectorScreen(out, lineCount)
				return configurationShellActionCancel, selected, err
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishSelectorScreen(out, lineCount)
			return configurationShellActionCancel, selected, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = (selected - 1 + len(pending)) % len(pending)
			message = ""
			needsRender = true
		case selectorKeyDown:
			selected = (selected + 1) % len(pending)
			message = ""
			needsRender = true
		case selectorKeyHome:
			selected = 0
			message = ""
			needsRender = true
		case selectorKeyEnd:
			selected = len(pending) - 1
			message = ""
			needsRender = true
		case selectorKeyDeny:
			setting := tobari.ContextShellEnvironmentSetting{
				Variable: pending[selected].Variable, Source: tobari.ContextShellEnvironmentDefault,
			}
			pending[selected] = setting
			staged[setting.Variable] = setting
			message = setting.Variable + " staged as default."
			needsRender = true
		case selectorKeyInherit:
			setting := tobari.ContextShellEnvironmentSetting{
				Variable: pending[selected].Variable, Source: tobari.ContextShellEnvironmentInherit,
			}
			pending[selected] = setting
			staged[setting.Variable] = setting
			message = setting.Variable + " staged as inherit."
			needsRender = true
		case selectorKeyLiteral:
			finishSelectorScreen(out, lineCount)
			return configurationShellActionLiteral, selected, nil
		case selectorKeyApply:
			if len(staged) == 0 {
				message = "Stage at least one change before Apply."
				needsRender = true
				continue
			}
			finishSelectorScreen(out, lineCount)
			return configurationShellActionApply, selected, nil
		case selectorKeyCancel:
			finishSelectorScreen(out, lineCount)
			return configurationShellActionCancel, selected, context.Canceled
		default:
			message = "Use ↑/↓ to move, d/h/l to stage, p to Apply, or q to cancel."
			needsRender = true
		}
	}
}

func renderConfigurationShellRaw(
	out io.Writer,
	contextName string,
	pending []tobari.ContextShellEnvironmentSetting,
	staged map[string]tobari.ContextShellEnvironmentSetting,
	selected int,
	message string,
	previousLines int,
	style bool,
) (int, error) {
	lines := []string{
		selectorTitle(style, "Tobari · Shell configuration"),
		selectorDetail(style, "Context", safeExternalText(contextName), styleText),
		"",
		applyStyleToken(style, styleMuted, "  Variable     Source     Value"),
	}
	for index, setting := range pending {
		marker := "  "
		variableToken := styleText
		if index == selected {
			marker = "❯ "
			variableToken = styleAccent
		}
		stagedMarker := " "
		if _, ok := staged[setting.Variable]; ok {
			stagedMarker = "*"
		}
		lines = append(lines, fmt.Sprintf(
			"%s%s %-12s %-10s %s",
			marker, stagedMarker,
			applyStyleToken(style, variableToken, setting.Variable),
			applyStyleToken(style, styleText, string(setting.Source)),
			applyStyleToken(style, styleMuted, shellSettingDisplayValue(setting)),
		))
	}
	lines = append(lines, "", applyStyleToken(style, styleMuted, fmt.Sprintf("Pending: %d change%s", len(staged), pluralSuffix(len(staged)))))
	if message != "" {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, "",
		selectorHelp(style, "↑/↓ move   d default   h inherit   l literal"),
		selectorHelp(style, "p Apply    q cancel"),
	)
	return renderConfigurationWizardLines(out, lines, previousLines)
}

func selectConfigurationShellLine(
	ctx context.Context,
	contextName string,
	pending []tobari.ContextShellEnvironmentSetting,
	in io.Reader,
	out io.Writer,
) ([]tobari.ContextShellEnvironmentSetting, error) {
	staged := make(map[string]tobari.ContextShellEnvironmentSetting)
	for {
		if _, err := fmt.Fprintf(out, "Tobari · Shell configuration\nContext: %s\n\n", safeExternalText(contextName)); err != nil {
			return nil, err
		}
		for index, setting := range pending {
			marker := " "
			if _, ok := staged[setting.Variable]; ok {
				marker = "*"
			}
			if _, err := fmt.Fprintf(out, "  %d. %s %s · %s · %s\n", index+1, marker, setting.Variable, setting.Source, shellSettingDisplayValue(setting)); err != nil {
				return nil, err
			}
		}
		if _, err := fmt.Fprintf(out, "\nChoose a variable, p to Apply %d change%s, or q to cancel: ", len(staged), pluralSuffix(len(staged))); err != nil {
			return nil, err
		}
		choice, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
		if err != nil {
			return nil, err
		}
		value := strings.ToLower(strings.TrimSpace(choice))
		if value == "q" || value == "quit" || value == "esc" {
			return nil, context.Canceled
		}
		if value == "p" || value == "apply" {
			if len(staged) > 0 {
				return stagedShellChanges(staged), nil
			}
			if _, err := fmt.Fprintln(out, "Stage at least one change before Apply."); err != nil {
				return nil, err
			}
			continue
		}
		index, parseErr := strconv.Atoi(value)
		if parseErr != nil || index < 1 || index > len(pending) {
			if _, err := fmt.Fprintln(out, "Choose a listed variable, p, or q."); err != nil {
				return nil, err
			}
			continue
		}
		selected := index - 1
		if _, err := fmt.Fprintf(out, "Source for %s [d default, h inherit, l literal]: ", pending[selected].Variable); err != nil {
			return nil, err
		}
		source, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
		if err != nil {
			return nil, err
		}
		setting := tobari.ContextShellEnvironmentSetting{Variable: pending[selected].Variable}
		switch strings.ToLower(strings.TrimSpace(source)) {
		case "d", "default":
			setting.Source = tobari.ContextShellEnvironmentDefault
		case "h", "inherit":
			setting.Source = tobari.ContextShellEnvironmentInherit
		case "l", "literal":
			setting.Source = tobari.ContextShellEnvironmentLiteral
			literal, readErr := readConfigurationWizardValue(ctx, in, out, "Fixed value", tobari.MaxContextShellValueBytes)
			if readErr != nil {
				return nil, readErr
			}
			setting.Value = &literal
		default:
			if _, err := fmt.Fprintln(out, "Choose d, h, or l."); err != nil {
				return nil, err
			}
			continue
		}
		pending[selected] = setting
		staged[setting.Variable] = setting
	}
}

func cloneShellSettings(settings []tobari.ContextShellEnvironmentSetting) []tobari.ContextShellEnvironmentSetting {
	result := make([]tobari.ContextShellEnvironmentSetting, len(settings))
	for index, setting := range settings {
		result[index] = setting
		if setting.Value != nil {
			value := *setting.Value
			result[index].Value = &value
		}
	}
	return result
}

func stagedShellChanges(staged map[string]tobari.ContextShellEnvironmentSetting) []tobari.ContextShellEnvironmentSetting {
	result := make([]tobari.ContextShellEnvironmentSetting, 0, len(staged))
	for _, variable := range tobari.ContextShellEnvironmentVariables() {
		if setting, ok := staged[variable]; ok {
			result = append(result, setting)
		}
	}
	return result
}

func shellSettingDisplayValue(setting tobari.ContextShellEnvironmentSetting) string {
	if setting.Source != tobari.ContextShellEnvironmentLiteral || setting.Value == nil {
		return "—"
	}
	return fmt.Sprintf("%q", truncateSelectorPath(safeExternalText(*setting.Value), 36))
}

type configurationGitAction uint8

const (
	configurationGitActionCancel configurationGitAction = iota
	configurationGitActionLiteral
	configurationGitActionApply
)

func configurationGitOptions() []configurationWizardOption {
	return []configurationWizardOption{
		{label: "Inherit host identity", description: "Resolve host global user.name and user.email on Workspace entry.", value: string(tobari.ContextGitIdentityInherit)},
		{label: "Use a fixed identity", description: "Store one Context-owned name and email pair.", value: string(tobari.ContextGitIdentityLiteral)},
		{label: "No Context fallback", description: "Remove the Context identity projection.", value: string(tobari.ContextGitIdentityDefault)},
	}
}

func selectConfigurationGitRaw(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	contextName string,
	current tobari.ContextGitIdentitySetting,
	pending tobari.ContextGitIdentitySetting,
	staged bool,
	selected int,
	message string,
	style bool,
) (configurationGitAction, tobari.ContextGitIdentitySetting, int, error) {
	options := configurationGitOptions()
	lineCount := 0
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishSelectorScreen(out, lineCount)
			return configurationGitActionCancel, pending, selected, err
		}
		if needsRender {
			currentLines, err := renderConfigurationGitRaw(
				out, contextName, current, pending, staged, options, selected, message, lineCount, style,
			)
			if err != nil {
				finishSelectorScreen(out, lineCount)
				return configurationGitActionCancel, pending, selected, err
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishSelectorScreen(out, lineCount)
			return configurationGitActionCancel, pending, selected, err
		}
		source := tobari.ContextGitIdentitySource("")
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = (selected - 1 + len(options)) % len(options)
			message = ""
			needsRender = true
			continue
		case selectorKeyDown:
			selected = (selected + 1) % len(options)
			message = ""
			needsRender = true
			continue
		case selectorKeyHome:
			selected = 0
			message = ""
			needsRender = true
			continue
		case selectorKeyEnd:
			selected = len(options) - 1
			message = ""
			needsRender = true
			continue
		case selectorKeyEnter:
			source = tobari.ContextGitIdentitySource(options[selected].value)
		case selectorKeyDeny:
			source = tobari.ContextGitIdentityDefault
			selected = gitIdentitySourceIndex(source)
		case selectorKeyInherit:
			source = tobari.ContextGitIdentityInherit
			selected = gitIdentitySourceIndex(source)
		case selectorKeyLiteral:
			source = tobari.ContextGitIdentityLiteral
			selected = gitIdentitySourceIndex(source)
		case selectorKeyApply:
			if !staged {
				message = "Stage an identity source before Apply."
				needsRender = true
				continue
			}
			finishSelectorScreen(out, lineCount)
			return configurationGitActionApply, pending, selected, nil
		case selectorKeyCancel:
			finishSelectorScreen(out, lineCount)
			return configurationGitActionCancel, pending, selected, context.Canceled
		default:
			message = "Use ↑/↓ and Enter or d/h/l to stage, p to Apply, or q to cancel."
			needsRender = true
			continue
		}
		if source == tobari.ContextGitIdentityLiteral {
			finishSelectorScreen(out, lineCount)
			return configurationGitActionLiteral, pending, selected, nil
		}
		pending = tobari.ContextGitIdentitySetting{Source: source}
		staged = true
		message = "Git identity staged as " + string(source) + "."
		needsRender = true
	}
}

func renderConfigurationGitRaw(
	out io.Writer,
	contextName string,
	current tobari.ContextGitIdentitySetting,
	pending tobari.ContextGitIdentitySetting,
	staged bool,
	options []configurationWizardOption,
	selected int,
	message string,
	previousLines int,
	style bool,
) (int, error) {
	lines := []string{
		selectorTitle(style, "Tobari · Git identity"),
		selectorDetail(style, "Context", safeExternalText(contextName), styleText),
		selectorDetail(style, "Current", safeExternalText(gitIdentitySummary(current)), styleText),
		selectorHelp(style, "Only user.name and user.email are projected."),
		selectorHelp(style, "Authentication and signing stay isolated."),
		"",
	}
	for index, option := range options {
		marker := "  "
		labelToken := styleText
		if index == selected {
			marker = "❯ "
			labelToken = styleAccent
		}
		pendingMarker := " "
		if staged && pending.Source == tobari.ContextGitIdentitySource(option.value) {
			pendingMarker = "*"
		}
		lines = append(lines,
			marker+pendingMarker+" "+applyStyleToken(style, labelToken, option.label),
			"    "+applyStyleToken(style, styleMuted, option.description),
		)
	}
	if staged {
		lines = append(lines, "", selectorDetail(style, "Pending", safeExternalText(gitIdentitySummary(pending)), styleText))
	} else {
		lines = append(lines, "", selectorDetail(style, "Pending", "none", styleMuted))
	}
	if message != "" {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, "",
		selectorHelp(style, "↑/↓ move   Enter stage   d default   h inherit   l literal"),
		selectorHelp(style, "p Apply    q cancel"),
	)
	return renderConfigurationWizardLines(out, lines, previousLines)
}

func selectConfigurationGitLine(
	ctx context.Context,
	contextName string,
	current tobari.ContextGitIdentitySetting,
	in io.Reader,
	out io.Writer,
) (tobari.ContextGitIdentitySetting, error) {
	if _, err := fmt.Fprintf(
		out,
		"Tobari · Git identity\nContext: %s\nCurrent: %s\nOnly user.name and user.email are projected.\n\nSource [d default, h inherit, l literal]: ",
		safeExternalText(contextName), safeExternalText(gitIdentitySummary(current)),
	); err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	input, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
	if err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	if value := strings.ToLower(strings.TrimSpace(input)); value == "q" || value == "quit" || value == "esc" {
		return tobari.ContextGitIdentitySetting{}, context.Canceled
	}
	change := tobari.ContextGitIdentitySetting{}
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "d", "default":
		change.Source = tobari.ContextGitIdentityDefault
	case "h", "inherit":
		change.Source = tobari.ContextGitIdentityInherit
	case "l", "literal":
		change.Source = tobari.ContextGitIdentityLiteral
		name, readErr := readConfigurationWizardValue(ctx, in, out, "Git user.name", tobari.MaxContextGitIdentityValueBytes)
		if readErr != nil {
			return tobari.ContextGitIdentitySetting{}, readErr
		}
		email, readErr := readConfigurationWizardValue(ctx, in, out, "Git user.email", tobari.MaxContextGitIdentityValueBytes)
		if readErr != nil {
			return tobari.ContextGitIdentitySetting{}, readErr
		}
		change.Name, change.Email = &name, &email
	default:
		return tobari.ContextGitIdentitySetting{}, fmt.Errorf("Git identity source must be d, h, or l")
	}
	if _, err := fmt.Fprintf(out, "Pending: %s\nApply this change? [Y/n]: ", safeExternalText(gitIdentitySummary(change))); err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	confirm, err := readConfigurationWizardLine(ctx, in, maxConfigurationWizardChoiceBytes)
	if err != nil {
		return tobari.ContextGitIdentitySetting{}, err
	}
	if value := strings.ToLower(strings.TrimSpace(confirm)); value == "n" || value == "no" || value == "q" {
		return tobari.ContextGitIdentitySetting{}, context.Canceled
	}
	return change, nil
}

func cloneGitIdentitySetting(setting tobari.ContextGitIdentitySetting) tobari.ContextGitIdentitySetting {
	result := setting
	if setting.Name != nil {
		value := *setting.Name
		result.Name = &value
	}
	if setting.Email != nil {
		value := *setting.Email
		result.Email = &value
	}
	return result
}

func gitIdentitySourceIndex(source tobari.ContextGitIdentitySource) int {
	for index, option := range configurationGitOptions() {
		if tobari.ContextGitIdentitySource(option.value) == source {
			return index
		}
	}
	return 0
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
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishSelectorScreen(out, lineCount)
			return 0, err
		}
		if needsRender {
			currentLines, err := renderConfigurationWizardRaw(out, menu, selected, message, lineCount, style)
			if err != nil {
				finishSelectorScreen(out, lineCount)
				return 0, err
			}
			lineCount = currentLines
			needsRender = false
		}
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
			needsRender = true
		case selectorKeyDown:
			selected = (selected + 1) % len(menu.options)
			message = ""
			needsRender = true
		case selectorKeyHome:
			selected = 0
			message = ""
			needsRender = true
		case selectorKeyEnd:
			selected = len(menu.options) - 1
			message = ""
			needsRender = true
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
			needsRender = true
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
	return renderConfigurationWizardLines(out, lines, previousLines)
}

func renderConfigurationWizardLines(out io.Writer, lines []string, previousLines int) (int, error) {
	return renderSelectorScreen(out, lines, previousLines)
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
