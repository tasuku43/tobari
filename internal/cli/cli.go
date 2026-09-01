// Package cli owns command routing and presentation.
package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/app/completioncmd"
	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/app/doctorcmd"
	"github.com/tasuku43/tobari/internal/app/installationmigrationcmd"
	"github.com/tasuku43/tobari/internal/app/permissionwaitcmd"
	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/serviceexposurecmd"
	"github.com/tasuku43/tobari/internal/app/statuscmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/configuratorstore"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/systemdoctor"
	"github.com/tasuku43/tobari/internal/infra/terminal"
	"github.com/tasuku43/tobari/internal/infra/terminalstyle"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritydoctor"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthorityresources"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysession"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

// finalWorkspaceAuthorityAdapter is composition only: each embedded adapter
// continues to own its task-specific boundary while the application services
// receive one complete final-authority port. It contains no predecessor reader
// or fallback selector.
type finalWorkspaceAuthorityAdapter struct {
	*workspaceauthorityresources.Adapter
	*workspaceauthoritystore.ContextEntryAdapter
	*workspaceauthoritystore.HostLoopbackPolicyAdapter
}

type finalPolicyAuthorityAdapter struct {
	*finalWorkspaceAuthorityAdapter
	*workspaceauthoritystore.FinalPolicyCandidateAdapter
}

func (a *finalPolicyAuthorityAdapter) ListPolicyCandidatesIncludingAttachments(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	return a.FinalPolicyCandidateAdapter.ListPolicyCandidatesIncludingAttachments(ctx)
}

func (a *finalPolicyAuthorityAdapter) ReadPolicyMemoryReviewSnapshot(ctx context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	return a.FinalPolicyCandidateAdapter.ReadPolicyMemoryReviewSnapshot(ctx)
}

func (a *finalPolicyAuthorityAdapter) ApplyAttachmentPolicyCandidate(ctx context.Context, ref string, decision tobari.PolicyMemoryDecision) (tobari.AttachmentGrantPublication, bool, error) {
	return a.FinalPolicyCandidateAdapter.ApplyAttachmentPolicyCandidate(ctx, ref, decision)
}

func (a *finalPolicyAuthorityAdapter) AllowPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return a.FinalPolicyCandidateAdapter.AllowPolicyCandidateByReference(ctx, ref)
}

func (a *finalPolicyAuthorityAdapter) DenyPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return a.FinalPolicyCandidateAdapter.DenyPolicyCandidateByReference(ctx, ref)
}

type finalDefaultPairEntry interface {
	Select(context.Context, io.Reader, io.Writer) (workspaceauthoritycmd.SelectedDefaultPair, error)
	ResolveSelected(context.Context, operation.Intent, *tobari.WorkspaceTemplateBody, workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error)
	ResolveSelectedWithTemplateID(context.Context, operation.Intent, *tobari.WorkspaceTemplateBody, tobari.WorkspaceTemplateID, workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error)
	ResolveSelectedWithConfiguratorIDs(context.Context, operation.Intent, *tobari.WorkspaceTemplateBody, tobari.WorkspaceTemplateID, tobari.ContextID, workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error)
	RefreshAfterCluster(context.Context, workspaceauthoritycmd.DefaultPairResolution, workspaceauthoritycmd.FinalClusterReconciliation) (workspaceauthoritycmd.DefaultPairResolution, error)
	EnterResolved(context.Context, workspaceauthoritycmd.DefaultPairResolution, tobari.WorkspaceSessionRequest, tobari.FirstEntryProgressSink, io.Reader, io.Writer, io.Writer) (workspaceauthoritycmd.ContextEntryResult, error)
	EnterResolvedCurrent(context.Context, workspaceauthoritycmd.DefaultPairResolution, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (workspaceauthoritycmd.ContextEntryResult, error)
}

type finalWorkspaceEntryReadiness interface {
	Check(context.Context) error
}

var (
	_ workspaceauthoritycmd.PolicyCandidatePort                 = (*finalPolicyAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.PolicyRulePort                      = (*finalPolicyAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.PolicyReviewedPort                  = (*finalPolicyAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.AttachmentPolicyCandidatePort       = (*finalPolicyAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.PolicyMemoryReviewPort              = (*finalPolicyAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateReadPort                    = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateDraftCreatePort             = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateDraftCopyPort               = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplatePlanPort                    = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateApplyPort                   = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplatePolicyMigrationPlanPort     = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplatePolicyMigrationApplyPort    = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateDefaultPort                 = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.TemplateDeletePort                  = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextReadPort                     = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextDraftCreatePort              = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextPlanPort                     = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextApplyPlanPort                = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextEnterPort                    = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.DefaultPairContextEnterPort         = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.DefaultPairContextEnterProgressPort = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.ContextDeletePort                   = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.WorkspaceReadPort                   = (*finalWorkspaceAuthorityAdapter)(nil)
	_ workspaceauthoritycmd.WorkspaceDeletePort                 = (*finalWorkspaceAuthorityAdapter)(nil)
)

// CLI contains injected streams and application services.
type CLI struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version string
	Commit  string

	// processLifetime is injected by the composition root and remains the
	// parent for bounded settlement after an invocation is canceled.
	processLifetime context.Context

	catalog                Catalog
	doctor                 *doctorcmd.Service
	tobari                 *tobaricmd.Service
	context                *contextcmd.Service
	runtime                *runtimecmd.Service
	finalAuth              *authcmd.FinalContextService
	completion             *completioncmd.Service
	serviceExposure        *serviceexposurecmd.Service
	statusHome             *statuscmd.Service
	serviceExposureInitErr error
	researchCLIState
	permissionWait        *permissionwaitcmd.Service
	permissionWaitInitErr error
	authorityStore        *workspaceauthoritystore.Store
	finalTemplates        *workspaceauthoritycmd.TemplateService
	installationMigration *installationmigrationcmd.Service
	finalContexts         *workspaceauthoritycmd.ContextService
	finalWorkspaces       *workspaceauthoritycmd.WorkspaceService
	finalPolicy           *workspaceauthoritycmd.PolicyMemoryService
	finalDefaultPair      finalDefaultPairEntry
	finalEntryReadiness   finalWorkspaceEntryReadiness
	finalClusterCLIState
	finalProjectRoot       finalProjectRootAuthority
	config                 contextConfigurationWizard
	contextCreate          contextCreateWizard
	firstUse               recommendedFirstUseReviewer
	firstUseSetup          firstUseSetupSelector
	configuratorReview     configuratorSubmissionReviewer
	configuratorPlanReview configuratorPlanReviewer
	configurator           *configuratorcmd.Service
	runtimeChoice          runtimeChoiceWizard
	authLogin              authLoginProviderSelector
	serviceNotify          func(io.Writer, string) error
	serviceReviewMode      terminal.Mode
	interactive            func(io.Reader, io.Writer, io.Writer) bool
	firstUseTemplateBody   func(context.Context) (tobari.WorkspaceTemplateBody, error)
	firstUseCustomize      func(context.Context, tobari.RecommendedFirstUseDraft) (tobari.WorkspaceTemplateBody, error)
	noColor                bool
}

// New builds the production CLI with the Docker-backed Tobari runtime.
func New(lifetime context.Context, in io.Reader, out, errOut io.Writer) *CLI {
	command := newCLI(in, out, errOut, DefaultCatalog(), systemdoctor.New())
	command.processLifetime = lifetime
	command.noColor = noColorFromEnvironment()
	command.config = newContextConfigurationWizardWithStyle(!command.noColor)
	command.contextCreate = newContextCreateWizardWithStyle(!command.noColor)
	command.firstUse = newRecommendedFirstUseReviewerWithStyle(!command.noColor)
	command.firstUseSetup = newFirstUseSetupSelectorWithStyle(!command.noColor)
	command.configuratorReview = newConfiguratorSubmissionReviewerWithStyle(!command.noColor)
	command.runtimeChoice = newRuntimeChoiceWizardWithStyle(!command.noColor)
	command.authLogin = newAuthLoginProviderSelectorWithStyle(!command.noColor)
	command.serviceNotify = terminal.WriteServiceReviewNotification
	configureResearchCLI(command)
	runtime, err := dockerruntime.New(lifetime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	command.finalEntryReadiness = workspaceauthoritycmd.NewWorkspaceEntryReadinessService(runtime)
	command.doctor = doctorcmd.New(runtime)
	authorityRoot, err := runtime.FinalWorkspaceAuthorityRoot()
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	authorityStore, err := workspaceauthoritystore.NewFinalOnly(authorityRoot, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	if err := runtime.BindFinalRuntimeProtectionSource(authorityStore); err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	mutator, err := workspaceauthoritystore.NewMutator(lifetime, authorityStore, runtime, runtime, runtime, runtime, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	sourceRoot, err := runtime.ResourceSourceRoot()
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	sourceStore, err := workspaceauthoritysource.New(sourceRoot)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	configuratorRoot, err := runtime.ConfiguratorRoot()
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	contextHomeRoot, err := runtime.ContextHomeRoot()
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	configuratorDrafts, err := configuratorstore.New(configuratorRoot, contextHomeRoot, mutator, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	resources, err := workspaceauthorityresources.New(
		authorityStore, mutator, sourceStore,
		runtime.ObserveInstallationRuntimeMigration,
		func(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, recovery bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
			return runtime.PrepareInstallationRuntimeMigration(ctx, collection, recovery)
		},
		runtime,
	)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	command.configurator = configuratorcmd.New(configuratorDrafts, runtime, resources, resources)
	finalAuthDoctor, err := configureFinalContextAuth(command, lifetime, authorityStore, mutator, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	clusterLifecycle, err := workspaceauthoritystore.NewClusterLifecycleAdapter(authorityStore, mutator, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	clusterReads, err := workspaceauthoritystore.NewClusterReadAdapter(authorityStore, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	if err := configureFinalClusterCLI(command, mutator, clusterLifecycle, clusterReads); err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	finalDoctor, err := workspaceauthoritydoctor.New(authorityStore, clusterLifecycle, runtime, finalAuthDoctor)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	command.doctor = doctorcmd.New(finalDoctor)
	sessions, err := workspaceauthoritysession.New(runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	entry, err := workspaceauthoritystore.NewContextEntryAdapter(mutator, runtime, runtime, sessions, lifetime, resources)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	hostLoopbackPolicy, err := workspaceauthoritystore.NewHostLoopbackPolicyAdapter(authorityStore, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	finalAuthority := &finalWorkspaceAuthorityAdapter{
		Adapter: resources, ContextEntryAdapter: entry,
		HostLoopbackPolicyAdapter: hostLoopbackPolicy,
	}
	finalPolicyAuthority, err := workspaceauthoritystore.NewFinalPolicyCandidateAdapter(authorityStore, runtime, mutator, hostLoopbackPolicy)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	finalPolicyPort := &finalPolicyAuthorityAdapter{finalWorkspaceAuthorityAdapter: finalAuthority, FinalPolicyCandidateAdapter: finalPolicyAuthority}
	command.authorityStore = authorityStore
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(finalAuthority)
	command.installationMigration = installationmigrationcmd.New(resources)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalAuthority)
	command.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(finalAuthority)
	command.finalPolicy = workspaceauthoritycmd.NewPolicyMemoryService(finalPolicyPort)
	defaultPair, err := workspaceauthoritystore.NewDefaultPairAdapter(authorityStore, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	command.finalDefaultPair = workspaceauthoritycmd.NewDefaultPairService(defaultPair, resources, command.finalContexts, newFinalDefaultPairSelectorWithStyle(!command.noColor))
	statusHome, err := workspaceauthoritystore.NewStatusHomeAdapter(authorityStore, runtime, runtime)
	if err != nil {
		command.doctor = doctorcmd.New(systemdoctor.New(err))
		return command
	}
	command.statusHome = statuscmd.New(statusHome)
	command.finalProjectRoot = runtime
	command.runtime = runtimecmd.New(runtime)
	command.completion = completioncmd.New(runtime)
	command.serviceExposure = serviceexposurecmd.New(runtime)
	return command
}

// NewExposureHelper builds only the attachment-local helper view of the
// canonical Catalog. It does not construct or expose the host Docker CLI.
func NewExposureHelper(in io.Reader, out, errOut io.Writer) *CLI {
	command := newCLI(in, out, errOut, DefaultCatalog().ForProgram(ExposureProgramName), systemdoctor.New())
	command.noColor = noColorFromEnvironment()
	client, err := dockerruntime.NewServiceExposureClientFromEnvironment()
	if err != nil {
		command.serviceExposureInitErr = err
		return command
	}
	command.serviceExposure = serviceexposurecmd.New(client)
	return command
}

// NewPermissionHelper builds only the attachment-local wait observer view of
// the canonical Catalog. It has no host policy or Docker composition service.
func NewPermissionHelper(in io.Reader, out, errOut io.Writer) *CLI {
	command := newCLI(in, out, errOut, DefaultCatalog().ForProgram(PermissionProgramName), systemdoctor.New())
	command.noColor = noColorFromEnvironment()
	client, err := dockerruntime.NewPermissionWaitClientFromEnvironment()
	if err != nil {
		command.permissionWaitInitErr = err
		return command
	}
	command.permissionWait = permissionwaitcmd.New(client)
	return command
}

func noColorFromEnvironment() bool {
	return terminalstyle.NoColorRequested()
}

func newCLI(in io.Reader, out, errOut io.Writer, catalog Catalog, inspector doctorcmd.InspectorPort) *CLI {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	return &CLI{
		In: in, Out: out, Err: errOut,
		Version:       "dev",
		catalog:       catalog,
		doctor:        doctorcmd.New(inspector),
		config:        newContextConfigurationWizard(),
		contextCreate: newContextCreateWizardWithStyle(true),
		runtimeChoice: newRuntimeChoiceWizardWithStyle(true),
		authLogin:     newAuthLoginProviderSelector(),
		serviceNotify: terminal.WriteServiceReviewNotification,
		interactive: func(in io.Reader, out, errOut io.Writer) bool {
			return terminal.IsTerminal(in) && terminal.IsTerminal(out) && terminal.IsTerminal(errOut)
		},
	}
}

// RunContext validates global options and the catalog, resolves one command,
// and propagates the same context to the selected application boundary.
func (c *CLI) RunContext(ctx context.Context, args []string) int {
	if c == nil {
		return ExitInternal
	}
	if ctx == nil {
		return c.fail(nil, fault.New(
			fault.KindContract,
			"missing_context",
			"The command context is not configured.",
			false,
			fault.NextAction{Command: "help", Reason: "Retry through a context-aware CLI entry point."},
		))
	}
	options, commandArgs, err := parseRootOptions(args)
	ctx = withErrorFormat(ctx, options.ErrorFormat)
	if err != nil {
		return c.failUsage(ctx, "invalid_root_options", err.Error(), "help", "Correct the global options.")
	}
	if err := ctx.Err(); err != nil {
		return c.fail(ctx, err)
	}
	if err := c.catalog.Validate(); err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract,
			"invalid_catalog",
			"The command catalog is invalid.",
			false,
			err,
			fault.NextAction{Command: "help", Reason: "Repair the catalog before dispatch."},
		))
	}
	if len(commandArgs) == 0 {
		// The root invocation is the primary interactive outcome. Help remains
		// explicit through `help` or `--help` and is handled before this branch.
		commandArgs = []string{WorkspaceEntryCommandPath}
	} else if commandArgs[0] == "--" {
		// A delimiter-led root invocation selects the existing catalog-owned
		// root entry; the delimiter remains available to the typed parser.
		commandArgs = append([]string{WorkspaceEntryCommandPath}, commandArgs...)
	} else if c.catalog.programName() == ExposureProgramName && commandArgs[0] != "help" && commandArgs[0] != "status" && commandArgs[0] != "stop" {
		// The helper's primary command is its exact port positional. Canonical
		// routing still resolves through the program root declaration.
		commandArgs = append([]string{ExposureProgramName}, commandArgs...)
	}

	commandArgs = normalizeRootAlias(commandArgs)
	commandArgs = normalizeTrailingHelpAlias(c.catalog, commandArgs)
	commandArgs = normalizeBareNamespace(c.catalog, commandArgs)
	command, rest, found := c.catalog.Match(commandArgs)
	if !found {
		suggestions := catalogCommandSuggestions(c.catalog, strings.Join(commandArgs, " "))
		message := fmt.Sprintf("Unknown command %q.", boundedHumanCommand(strings.Join(commandArgs, " ")))
		recovery := "help"
		reason := "Discover an exact command path or namespace."
		if len(suggestions) > 0 {
			message += " Did you mean " + strings.Join(suggestions, ", ") + "?"
			recovery = "help " + suggestions[0]
			reason = "Inspect the nearest exact catalog command or namespace."
		}
		return c.failUsage(ctx, "unknown_command", message, recovery, reason)
	}
	ctx = withCommandPath(ctx, command.Path)
	if err := ctx.Err(); err != nil {
		return c.fail(ctx, err)
	}
	inputs, err := parseCommandInputs(command, rest)
	if err != nil {
		var nextActions []fault.NextAction
		for _, declared := range command.Agent.Errors {
			if declared.Code == "invalid_arguments" {
				nextActions = cloneSlice(declared.NextActions)
				break
			}
		}
		return c.fail(ctx, fault.Wrap(
			fault.KindInvalidInput,
			"invalid_arguments",
			err.Error()+"; usage: "+command.Usage(),
			false,
			err,
			nextActions...,
		))
	}

	intent := operation.Intent{Command: command.Path, Effect: command.Effect}
	if command.Effect == operation.EffectRead {
		if err := intent.Validate(); err != nil {
			return c.fail(ctx, fault.Wrap(
				fault.KindContract,
				"invalid_intent",
				"The command intent is invalid.",
				false,
				err,
				fault.NextAction{Command: "help " + command.Path, Reason: "Repair the command declaration."},
			))
		}
	}
	return command.handler(ctx, c, command, intent, inputs)
}

func normalizeTrailingHelpAlias(catalog Catalog, args []string) []string {
	if len(args) < 2 || !isHelpFlag(args[len(args)-1]) {
		return args
	}
	selector := strings.Join(args[:len(args)-1], " ")
	commands, _ := catalog.Select(selector)
	if len(commands) == 0 {
		return args
	}
	return append([]string{"help"}, args[:len(args)-1]...)
}

// normalizeBareNamespace turns only a catalog-proven canonical namespace into
// its existing help selector. User argv is never appended to a recovery path.
func normalizeBareNamespace(catalog Catalog, args []string) []string {
	selector := strings.Join(args, " ")
	commands, exact := catalog.Select(selector)
	if selector == "" || exact || len(commands) == 0 {
		return args
	}
	return append([]string{"help"}, strings.Fields(selector)...)
}

const (
	maxCommandSuggestions  = 3
	maxUnknownCommandRunes = 96
)

type commandSuggestion struct {
	selector string
	score    int
	order    int
}

func catalogCommandSuggestions(catalog Catalog, attempted string) []string {
	attempted = boundedHumanCommand(attempted)
	candidates := catalogSuggestionSelectors(catalog)
	ranked := make([]commandSuggestion, 0, len(candidates))
	for index, candidate := range candidates {
		distance := editDistance([]rune(attempted), []rune(candidate))
		limit := 2
		if len([]rune(candidate)) >= 12 {
			limit = 3
		}
		if strings.HasPrefix(candidate, attempted) || strings.HasPrefix(attempted, candidate) {
			limit++
		}
		if distance <= limit {
			ranked = append(ranked, commandSuggestion{selector: candidate, score: distance, order: index})
		}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].score != ranked[right].score {
			return ranked[left].score < ranked[right].score
		}
		return ranked[left].order < ranked[right].order
	})
	if len(ranked) > maxCommandSuggestions {
		ranked = ranked[:maxCommandSuggestions]
	}
	result := make([]string, len(ranked))
	for index, suggestion := range ranked {
		result[index] = suggestion.selector
	}
	return result
}

func catalogSuggestionSelectors(catalog Catalog) []string {
	selectors := make([]string, 0, len(catalog.Commands())*2)
	seen := make(map[string]struct{})
	for _, command := range catalog.Commands() {
		if boundary := strings.IndexByte(command.Path, ' '); boundary > 0 {
			namespace := command.Path[:boundary]
			if _, found := seen[namespace]; !found {
				seen[namespace] = struct{}{}
				selectors = append(selectors, namespace)
			}
		}
		if _, found := seen[command.Path]; !found {
			seen[command.Path] = struct{}{}
			selectors = append(selectors, command.Path)
		}
	}
	return selectors
}

func boundedHumanCommand(value string) string {
	projected := []rune(safeExternalText(value))
	if len(projected) <= maxUnknownCommandRunes {
		return string(projected)
	}
	return string(projected[:maxUnknownCommandRunes-1]) + "…"
}

func editDistance(left, right []rune) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range right {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[rightIndex+1] = min(
				current[rightIndex]+1,
				previous[rightIndex+1]+1,
				previous[rightIndex]+cost,
			)
		}
		previous = current
	}
	return previous[len(right)]
}

func normalizeRootAlias(args []string) []string {
	switch args[0] {
	case "--help", "-h":
		return append([]string{"help"}, args[1:]...)
	case "--version", "-v":
		return append([]string{"version"}, args[1:]...)
	default:
		return args
	}
}

func isHelpFlag(value string) bool {
	return value == "--help" || value == "-h"
}

type rootOptions struct {
	ErrorFormat errorFormat
}

func parseRootOptions(args []string) (rootOptions, []string, error) {
	options := rootOptions{ErrorFormat: errorFormatText}
	seenErrorFormat := false
	index := 0
	for index < len(args) {
		argument := args[index]
		var value string
		switch {
		case argument == "--error-format":
			if index+1 >= len(args) {
				return options, nil, fmt.Errorf("--error-format requires text or json")
			}
			index++
			value = args[index]
		case strings.HasPrefix(argument, "--error-format="):
			value = strings.TrimPrefix(argument, "--error-format=")
		default:
			if argument == "--help" || argument == "-h" || argument == "--version" || argument == "-v" {
				return options, args[index:], nil
			}
			if argument == "--" {
				return options, args[index:], nil
			}
			if strings.HasPrefix(argument, "--") {
				return options, nil, fmt.Errorf("unknown global option %q", boundedHumanCommand(argument))
			}
			return options, args[index:], nil
		}
		if seenErrorFormat {
			return options, nil, fmt.Errorf("--error-format may be specified only once")
		}
		parsed, err := parseErrorFormat(value)
		if err != nil {
			return options, nil, err
		}
		options.ErrorFormat = parsed
		seenErrorFormat = true
		index++
	}
	return options, args[index:], nil
}
