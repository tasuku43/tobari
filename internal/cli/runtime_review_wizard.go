package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type readyRuntimeChoice struct {
	selection string
	label     string
	revision  string
	latest    bool
}

func runtimeReviewChooser(c *CLI) *terminalContextConfigurationWizard {
	if c != nil {
		if chooser, ok := c.config.(*terminalContextConfigurationWizard); ok && chooser != nil {
			return chooser
		}
	}
	return newContextConfigurationWizard()
}

func runtimeReviewAvailable(ctx context.Context, c *CLI, format successFormat) bool {
	return format == successFormatText && invocationErrorFormat(ctx) != errorFormatJSON &&
		c != nil && c.tobari != nil && c.tobari.IsInteractive(c.In, c.Err)
}

func runtimeReviewUnavailable(ctx context.Context, c *CLI, command CommandSpec, selector string) int {
	return c.failUsage(
		ctx,
		"runtime_review_unavailable",
		"Omitted "+selector+" requires text success/error output and interactive terminal stdin and stderr; usage: "+command.Usage(),
		"help "+command.Path,
		"Supply "+selector+" or run the Review wizard with text success/error output on interactive stdin and stderr.",
	)
}

func normalizeRuntimeReviewError(path string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}
	return fault.Wrap(
		fault.KindInternal,
		"runtime_review_failed",
		"The Runtime Review failed before applying a change.",
		false,
		err,
		fault.NextAction{Command: "help " + path, Reason: "Retry with a complete selector or repair the interactive terminal streams."},
	)
}

func chooseRuntimeBuild(ctx context.Context, c *CLI) (string, error) {
	catalog, err := c.runtime.List(ctx)
	if err != nil {
		return "", err
	}
	managed := make([]tobari.RuntimeSummary, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		if item.Kind == tobari.RuntimeKindManaged {
			managed = append(managed, item)
		}
	}
	if len(managed) == 0 {
		return "", fault.New(
			fault.KindNotFound,
			"managed_runtime_not_found",
			"No managed Runtime exists to build.",
			false,
			fault.NextAction{Command: "help runtime create", Reason: "Create a managed Runtime source tree first."},
		)
	}
	chooser := runtimeReviewChooser(c)
	selected := 0
	for {
		options := make([]configurationWizardOption, 0, len(managed))
		for _, item := range managed {
			description := "draft · no successful revision"
			if item.Ready {
				description = fmt.Sprintf("ready · head %s@%d", item.Name, item.Head)
			}
			options = append(options, configurationWizardOption{label: item.Name, description: description, value: item.Name})
		}
		index, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Tobari · Build Runtime", current: managed[selected].Name,
			information: []string{"Choose one installation-wide managed Runtime."},
			prompt:      "Managed Runtime", options: options,
		})
		if chooseErr != nil {
			return "", chooseErr
		}
		selected = index
		report, showErr := c.runtime.Show(ctx, managed[selected].Name)
		if showErr != nil {
			return "", showErr
		}
		if report.Runtime.RuntimeRef != managed[selected].RuntimeRef {
			return "", fault.New(
				fault.KindRejected,
				"runtime_selection_changed",
				"The selected Runtime changed while its details were being reviewed.",
				false,
				fault.NextAction{Command: "review runtimes", Reason: "Restart from a fresh Runtime catalog."},
			)
		}
		current := "draft · no successful revision"
		if head, ok := report.Runtime.Head(); ok {
			current = fmt.Sprintf("%s@%d · %s", report.Runtime.Name, head.Ordinal, shortRuntimeRevision(head.Revision))
		}
		action, reviewErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title: "Tobari · Build Runtime",
			details: []configurationWizardDetail{
				{label: "Runtime", value: report.Runtime.Name},
				{label: "Source", value: report.Runtime.SourcePath},
				{label: "Current", value: current},
			},
			information: []string{
				"Changed source creates one immutable revision; unchanged source creates none.",
				"No Workspace Manifest Runtime binding will change.",
			},
			prompt: "Action",
			options: []configurationWizardOption{
				{label: "Build Runtime", description: "Snapshot, build, validate, and record this Runtime.", value: "build"},
				{label: "Change Runtime", description: "Review another managed Runtime.", value: "change"},
				{label: "Cancel", description: "Build nothing and keep Runtime history unchanged.", value: "cancel"},
			},
		})
		if reviewErr != nil {
			return "", reviewErr
		}
		switch action {
		case 0:
			if report.Runtime.Kind != tobari.RuntimeKindManaged || report.Runtime.RuntimeRef == "" {
				return "", fault.New(fault.KindContract, "invalid_runtime_report", "The selected managed Runtime has no stable reference.", false)
			}
			return managed[selected].RuntimeRef, nil
		case 1:
			continue
		default:
			return "", context.Canceled
		}
	}
}

func confirmRuntimeBuildRecovery(ctx context.Context, c *CLI, recovery tobari.RuntimeBuildRecovery) (bool, error) {
	if err := recovery.Validate(); err != nil {
		return false, fault.Wrap(fault.KindContract, "runtime_recovery_contract_invalid", "The Runtime recovery target is invalid.", false, err)
	}
	title := "Tobari · Recover Runtime Build"
	reference := recovery.RuntimeRef
	action := "Recover interrupted build"
	description := "Run the exact bounded recovery for this Runtime reference."
	if recovery.RevisionRef != "" {
		title = "Tobari · Recover Runtime Restore"
		reference = recovery.RevisionRef
		action = "Recover interrupted restore"
		description = "Resume the exact retained revision restore without changing history or Workspaces."
	}
	index, err := runtimeReviewChooser(c).choose(ctx, c.In, c.Err, configurationWizardMenu{
		title: title,
		details: []configurationWizardDetail{
			{label: "Runtime", value: recovery.Name},
			{label: "Reference", value: reference},
			{label: "Recovery", value: string(recovery.Kind)},
		},
		information: []string{
			"Re-observe and resume only the retained journal authority.",
			"No Workspace Manifest, Workspace ID, home, or applied receipt is removed.",
		},
		prompt: "Action",
		options: []configurationWizardOption{
			{label: action, description: description, value: "recover"},
			{label: "Cancel", description: "Keep the journal and all Runtime material unchanged.", value: "cancel"},
		},
	})
	if err != nil {
		return false, err
	}
	return index == 0, nil
}

func confirmRuntimeDeleteRecovery(ctx context.Context, c *CLI, runtime tobari.RuntimeSummary) (bool, error) {
	if err := runtime.Validate(); err != nil || runtime.Kind != tobari.RuntimeKindManaged {
		if err == nil {
			err = fmt.Errorf("Runtime deletion recovery target is not managed")
		}
		return false, fault.WithClassification(
			fault.Wrap(fault.KindContract, "runtime_recovery_contract_invalid", "The Runtime deletion recovery target is invalid.", false, err,
				fault.NextAction{Command: "review runtimes", Reason: "Reconcile the current Runtime catalog."}),
			fault.PhaseObservation, fault.ChangeNotApplicable,
		)
	}
	index, err := runtimeReviewChooser(c).choose(ctx, c.In, c.Err, configurationWizardMenu{
		title: "Tobari · Recover Runtime Delete",
		details: []configurationWizardDetail{
			{label: "Runtime", value: runtime.Name},
			{label: "Reference", value: runtime.RuntimeRef},
		},
		information: []string{
			"Resume only the exact retained whole-Runtime deletion journal.",
			"Source, snapshots, and history continue forward to deletion; Workspace Manifests, Workspaces, IDs, homes, and applied receipts remain preserved.",
		},
		prompt: "Action",
		options: []configurationWizardOption{
			{label: "Recover interrupted delete", description: "Revalidate and resume this exact Runtime deletion.", value: "recover"},
			{label: "Cancel", description: "Keep the deletion journal and remaining Runtime material unchanged.", value: "cancel"},
		},
	})
	if err != nil {
		return false, err
	}
	return index == 0, nil
}

func chooseRuntimeCreateBase(ctx context.Context, c *CLI, targetName string) (string, error) {
	catalog, err := c.runtime.List(ctx)
	if err != nil {
		return "", err
	}
	managed := make([]tobari.RuntimeSummary, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		if item.Kind == tobari.RuntimeKindManaged {
			managed = append(managed, item)
		}
	}
	if len(managed) == 0 {
		return tobari.StandardRuntimeName, nil
	}
	options := []configurationWizardOption{{
		label: tobari.StandardRuntimeName, description: "Tobari built-in editable starter source.", value: tobari.StandardRuntimeName,
	}}
	for _, item := range managed {
		options = append(options, configurationWizardOption{
			label: item.Name, description: "Current editable source · build history is not copied", value: item.Name,
		})
	}
	index, err := runtimeReviewChooser(c).choose(ctx, c.In, c.Err, configurationWizardMenu{
		title:   "Tobari · Create Runtime · Base",
		details: []configurationWizardDetail{{label: "New Runtime", value: targetName}},
		information: []string{
			"Copy one current editable source tree without building it.",
			"Revisions, history, and lineage are not copied.",
			"The new Runtime is standalone and no Workspace Manifest binding changes.",
		},
		prompt: "Source Base", options: options, initial: 0,
	})
	if err != nil {
		return "", err
	}
	return options[index].value, nil
}

func loadReadyRuntimeChoices(ctx context.Context, c *CLI) ([]readyRuntimeChoice, error) {
	catalog, err := c.runtime.List(ctx)
	if err != nil {
		return nil, err
	}
	choices := make([]readyRuntimeChoice, 0)
	for _, item := range catalog.Items {
		if !item.Ready {
			continue
		}
		if item.Kind == tobari.RuntimeKindBuiltin {
			choices = append(choices, readyRuntimeChoice{
				selection: tobari.StandardRuntimeName,
				label:     tobari.StandardRuntimeName + "@1",
				revision:  item.Revision,
				latest:    true,
			})
			continue
		}
		history, historyErr := c.runtime.History(ctx, item.Name)
		if historyErr != nil {
			return nil, historyErr
		}
		for index := len(history.Runtime.Revisions) - 1; index >= 0; index-- {
			revision := history.Runtime.Revisions[index]
			choices = append(choices, readyRuntimeChoice{
				selection: fmt.Sprintf("%s@%d", item.Name, revision.Ordinal),
				label:     fmt.Sprintf("%s@%d", item.Name, revision.Ordinal),
				revision:  revision.Revision,
				latest:    revision.Ordinal == item.Head,
			})
		}
	}
	if len(choices) == 0 {
		return nil, fault.New(
			fault.KindNotFound,
			"runtime_not_found",
			"No ready Runtime revision exists.",
			false,
			fault.NextAction{Command: "runtime list", Reason: "Inspect the installation Runtime catalog."},
		)
	}
	return choices, nil
}

func runtimeChoiceOptions(choices []readyRuntimeChoice, current, selected string) []configurationWizardOption {
	ordered := make([]readyRuntimeChoice, 0, len(choices))
	for _, choice := range choices {
		if choice.selection == selected {
			ordered = append(ordered, choice)
			break
		}
	}
	for _, choice := range choices {
		if choice.selection != selected {
			ordered = append(ordered, choice)
		}
	}
	options := make([]configurationWizardOption, 0, len(ordered))
	for _, choice := range ordered {
		states := make([]string, 0, 3)
		if choice.selection == current {
			states = append(states, "current")
		}
		if choice.latest {
			states = append(states, "latest")
		}
		states = append(states, shortRuntimeRevision(choice.revision))
		options = append(options, configurationWizardOption{
			label: choice.label, description: strings.Join(states, " · "), value: choice.selection,
		})
	}
	return options
}

func shortRuntimeRevision(revision string) string {
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}

func contextRuntimeSelection(report tobari.ManifestReport) string {
	if report.Runtime.RuntimeID == tobari.StandardRuntimeID || report.Runtime.Name == tobari.StandardRuntimeName {
		return tobari.StandardRuntimeName
	}
	return fmt.Sprintf("%s@%d", report.Runtime.Name, report.Runtime.Ordinal)
}

func contextRuntimeDisplaySelection(selection string) string {
	if selection == tobari.StandardRuntimeName {
		return tobari.StandardRuntimeName + "@1"
	}
	return selection
}

func chooseContextRuntime(ctx context.Context, c *CLI, inputs ParsedInputs) (string, string, error) {
	contextName, err := selectedConfigurationContext(ctx, inputs)
	if err != nil {
		return "", "", err
	}
	current, err := c.context.Show(ctx, contextName)
	if err != nil {
		return "", "", err
	}
	if current.ManifestState != tobari.ManifestObservationPersisted {
		return "", "", fault.New(
			fault.KindNotFound,
			"manifest_not_found",
			"No persisted Workspace Manifest exists to receive a Runtime binding.",
			false,
			fault.NextAction{Command: "manifest list", Reason: "Inspect the persisted Workspace Manifest collection."},
		)
	}
	choices, err := loadReadyRuntimeChoices(ctx, c)
	if err != nil {
		return "", "", err
	}
	contextLocked := inputs.Provided("--manifest") || executionContextName(ctx) != ""
	selected := contextRuntimeSelection(current)
	chooser := runtimeReviewChooser(c)
	for {
		actual := contextRuntimeSelection(current)
		changed := selected != actual
		contextLabel := current.Name
		if current.Default {
			contextLabel += " · current"
		}
		information := []string{"Existing Workspace homes remain unchanged; the selected image applies on next entry."}
		if changed {
			actions := []configurationWizardOption{
				{label: "Apply change", description: "Replace this Workspace Manifest's exact Runtime binding.", value: "apply"},
				{label: "Back to Runtime list", description: "Choose another ready immutable revision.", value: "runtime"},
				{label: "Cancel", description: "Keep every Workspace Manifest Runtime binding unchanged.", value: "cancel"},
			}
			index, reviewErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
				title: "Tobari · Set Workspace Manifest Runtime · Review",
				details: []configurationWizardDetail{
					{label: "Workspace Manifest", value: contextLabel},
					{label: "Runtime", value: contextRuntimeDisplaySelection(actual) + " → " + contextRuntimeDisplaySelection(selected)},
					{label: "Applies", value: "next Workspace entry"},
				},
				information: information,
				prompt:      "Action", options: actions,
			})
			if reviewErr != nil {
				return "", "", reviewErr
			}
			switch actions[index].value {
			case "apply":
				return current.Name, selected, nil
			case "runtime":
				// Continue below to reopen the Runtime list without mutating.
			default:
				return "", "", context.Canceled
			}
		} else {
			actions := []configurationWizardOption{
				{label: "Change Runtime", description: "Choose standard or any successful immutable revision.", value: "runtime"},
			}
			if !contextLocked {
				actions = append(actions, configurationWizardOption{label: "Change Workspace Manifest", description: "Choose another persisted Workspace Manifest.", value: "context"})
			}
			actions = append(actions, configurationWizardOption{label: "Cancel", description: "Keep every Workspace Manifest Runtime binding unchanged.", value: "cancel"})
			index, reviewErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
				title: "Tobari · Set Workspace Manifest Runtime",
				details: []configurationWizardDetail{
					{label: "Workspace Manifest", value: contextLabel},
					{label: "Runtime", value: contextRuntimeDisplaySelection(actual)},
					{label: "Applies", value: "next Workspace entry"},
				},
				information: information,
				prompt:      "Action", options: actions,
			})
			if reviewErr != nil {
				return "", "", reviewErr
			}
			switch actions[index].value {
			case "runtime":
				// Continue below to choose a Runtime.
			case "context":
				list, listErr := c.context.List(ctx)
				if listErr != nil {
					return "", "", listErr
				}
				if len(list.Items) == 0 {
					return "", "", fault.New(
						fault.KindNotFound, "manifest_not_found", "No persisted Workspace Manifest exists.", false,
						fault.NextAction{Command: "manifest list", Reason: "Inspect the persisted Workspace Manifest collection."},
					)
				}
				contextOptions := contextReviewOptions(list.Items, current.Name)
				choice, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
					title:   "Tobari · Set Workspace Manifest Runtime · Workspace Manifest",
					current: current.Name,
					prompt:  "Persisted Workspace Manifest", options: contextOptions,
				})
				if chooseErr != nil {
					return "", "", chooseErr
				}
				current, err = c.context.Show(ctx, contextOptions[choice].value)
				if err != nil {
					return "", "", err
				}
				selected = contextRuntimeSelection(current)
				continue
			default:
				return "", "", context.Canceled
			}
		}
		options := runtimeChoiceOptions(choices, actual, selected)
		choice, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
			title:       "Tobari · Set Workspace Manifest Runtime · Runtime",
			contextName: current.Name,
			current:     contextRuntimeDisplaySelection(actual),
			information: []string{"Only already built immutable revisions can be selected."},
			prompt:      "Ready Runtime revision", options: options,
		})
		if chooseErr != nil {
			return "", "", chooseErr
		}
		selected = options[choice].value
	}
}

func contextReviewOptions(items []tobari.ManifestSummary, selected string) []configurationWizardOption {
	ordered := make([]tobari.ManifestSummary, 0, len(items))
	for _, item := range items {
		if item.Name == selected {
			ordered = append(ordered, item)
			break
		}
	}
	for _, item := range items {
		if item.Name != selected {
			ordered = append(ordered, item)
		}
	}
	options := make([]configurationWizardOption, 0, len(ordered))
	for _, item := range ordered {
		description := "persisted"
		if item.Default {
			description += " · current"
		}
		options = append(options, configurationWizardOption{label: item.Name, description: description, value: item.Name})
	}
	return options
}
