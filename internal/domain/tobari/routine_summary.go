package tobari

import "fmt"

// ContextRoutineTrafficState is the effective availability of the trusted
// binary's reviewed native-client traffic inside one Context Boundary.
type ContextRoutineTrafficState string

const (
	ContextRoutineTrafficReady      ContextRoutineTrafficState = "ready"
	ContextRoutineTrafficLimited    ContextRoutineTrafficState = "limited"
	ContextRoutineTrafficNotEnabled ContextRoutineTrafficState = "not_enabled"
)

func (s ContextRoutineTrafficState) Validate() error {
	switch s {
	case ContextRoutineTrafficReady, ContextRoutineTrafficLimited, ContextRoutineTrafficNotEnabled:
		return nil
	default:
		return fmt.Errorf("Context routine traffic state is invalid: %q", s)
	}
}

type ContextShellDefaultState string

const (
	ContextShellDefaultStandard   ContextShellDefaultState = "standard"
	ContextShellDefaultInherited  ContextShellDefaultState = "inherited"
	ContextShellDefaultCustomized ContextShellDefaultState = "customized"
)

func (s ContextShellDefaultState) Validate() error {
	switch s {
	case ContextShellDefaultStandard, ContextShellDefaultInherited, ContextShellDefaultCustomized:
		return nil
	default:
		return fmt.Errorf("Context shell default state is invalid: %q", s)
	}
}

type ContextGitDefaultState string

const (
	ContextGitDefaultNotImported ContextGitDefaultState = "not_imported"
	ContextGitDefaultInherited   ContextGitDefaultState = "inherited"
	ContextGitDefaultConfigured  ContextGitDefaultState = "configured"
)

func (s ContextGitDefaultState) Validate() error {
	switch s {
	case ContextGitDefaultNotImported, ContextGitDefaultInherited, ContextGitDefaultConfigured:
		return nil
	default:
		return fmt.Errorf("Context Git default state is invalid: %q", s)
	}
}

type ContextBootstrapDefaultState string

const (
	ContextBootstrapDefaultNone       ContextBootstrapDefaultState = "none"
	ContextBootstrapDefaultConfigured ContextBootstrapDefaultState = "configured"
)

func (s ContextBootstrapDefaultState) Validate() error {
	switch s {
	case ContextBootstrapDefaultNone, ContextBootstrapDefaultConfigured:
		return nil
	default:
		return fmt.Errorf("Context bootstrap default state is invalid: %q", s)
	}
}

// ContextRoutineAction identifies the next task implied by the selected
// Context state without carrying presentation text.
type ContextRoutineAction string

const (
	ContextRoutineActionEnterCurrent    ContextRoutineAction = "enter_current"
	ContextRoutineActionEnterNamed      ContextRoutineAction = "enter_named"
	ContextRoutineActionBuildRuntime    ContextRoutineAction = "build_runtime"
	ContextRoutineActionSelectThenBuild ContextRoutineAction = "select_then_build"
)

func (a ContextRoutineAction) Validate() error {
	switch a {
	case ContextRoutineActionEnterCurrent, ContextRoutineActionEnterNamed,
		ContextRoutineActionBuildRuntime, ContextRoutineActionSelectThenBuild:
		return nil
	default:
		return fmt.Errorf("Context routine action is invalid: %q", a)
	}
}

type ContextAccessSummary struct {
	SourceAccess   ContextSourceAccess
	RoutineTraffic ContextRoutineTrafficState
	MethodPolicy   ContextMethodPolicy
	PrivateTargets ContextMethodDecision
}

func (s ContextAccessSummary) Validate() error {
	if err := s.SourceAccess.Validate(); err != nil {
		return err
	}
	if err := s.RoutineTraffic.Validate(); err != nil {
		return err
	}
	if err := s.MethodPolicy.Validate(); err != nil {
		return err
	}
	if s.PrivateTargets != ContextMethodDeny {
		return fmt.Errorf("private targets must remain denied in the routine Context summary")
	}
	return nil
}

func validateRoutineAccessProjection(summary *ContextAccessSummary, source ContextSourceAccess, methods ContextMethodPolicy) error {
	if summary == nil {
		return nil
	}
	if err := summary.Validate(); err != nil {
		return err
	}
	if summary.SourceAccess != source || summary.MethodPolicy.Default != methods.Default ||
		len(summary.MethodPolicy.Overrides) != len(methods.Overrides) {
		return fmt.Errorf("Context routine Access does not match its source or method projection")
	}
	for index := range methods.Overrides {
		if summary.MethodPolicy.Overrides[index] != methods.Overrides[index] {
			return fmt.Errorf("Context routine Access does not match its method overrides")
		}
	}
	return nil
}

type ContextWorkspaceDefaultsSummary struct {
	Shell     ContextShellDefaultState
	Git       ContextGitDefaultState
	Bootstrap ContextBootstrapDefaultState
}

func (s ContextWorkspaceDefaultsSummary) Validate() error {
	if err := s.Shell.Validate(); err != nil {
		return err
	}
	if err := s.Git.Validate(); err != nil {
		return err
	}
	return s.Bootstrap.Validate()
}

type ContextRoutineSummary struct {
	Access              ContextAccessSummary
	RuntimeSelection    string
	RuntimeStatus       ContextRuntimeStatus
	Defaults            ContextWorkspaceDefaultsSummary
	AuthenticationMode  string
	RecommendedNotSaved bool
	Action              ContextRoutineAction
}

func (s ContextRoutineSummary) Validate() error {
	if err := s.Access.Validate(); err != nil {
		return err
	}
	if s.RuntimeStatus == ContextRuntimeStatusOfficial || s.RuntimeStatus == ContextRuntimeStatusReady {
		if err := validateRuntimeDisplaySelection(s.RuntimeSelection); err != nil {
			return err
		}
	}
	if err := s.RuntimeStatus.Validate(); err != nil {
		return err
	}
	if err := s.Defaults.Validate(); err != nil {
		return err
	}
	switch s.AuthenticationMode {
	case ContextAuthenticationModeNative, ContextAuthenticationModeBroker, ContextAuthenticationModeNotApplicable:
	default:
		return fmt.Errorf("Context routine authentication mode is invalid: %q", s.AuthenticationMode)
	}
	return s.Action.Validate()
}

// SummarizeContextAccess evaluates the reviewed routine-client overlay against
// the actual Context destination and method ceilings. It does not infer
// process identity or general network availability.
func SummarizeContextAccess(
	policy ContextPolicy, source ContextSourceAccess, readiness ContextNativeReadiness,
) (ContextAccessSummary, error) {
	if err := source.Validate(); err != nil {
		return ContextAccessSummary{}, err
	}
	normalized, _, _, err := NormalizeContextPolicy(policy)
	if err != nil {
		return ContextAccessSummary{}, err
	}
	resolved, err := ResolveContextNativeReadiness(readiness)
	if err != nil {
		return ContextAccessSummary{}, err
	}
	traffic := ContextRoutineTrafficNotEnabled
	if resolved == ContextNativeReadinessEnabled {
		traffic = ContextRoutineTrafficReady
		for _, bundle := range nativeToolAuthReadinessBundles() {
			for _, rule := range append(append([]ContextPolicyExactRule(nil), bundle.BaselineGrants...), bundle.GraphQLEndpoints...) {
				if !contextPolicyRuleInsideDestination(normalized.DestinationCeiling, rule) ||
					normalized.MethodPolicy.Decision(rule.Method) == ContextMethodDeny {
					traffic = ContextRoutineTrafficLimited
					break
				}
			}
			if traffic == ContextRoutineTrafficLimited {
				break
			}
		}
	}
	summary := ContextAccessSummary{
		SourceAccess: source, RoutineTraffic: traffic,
		MethodPolicy: normalized.MethodPolicy.Clone(), PrivateTargets: ContextMethodDeny,
	}
	return summary, summary.Validate()
}

func summarizeContextWorkspaceDefaults(
	shell []ContextShellEnvironmentSetting, git ContextGitIdentitySetting, bootstrap ContextBootstrapReport,
) (ContextWorkspaceDefaultsSummary, error) {
	if err := validateContextShellEnvironment(shell, true); err != nil {
		return ContextWorkspaceDefaultsSummary{}, err
	}
	shellState := ContextShellDefaultStandard
	for _, setting := range shell {
		switch setting.Source {
		case ContextShellEnvironmentLiteral:
			shellState = ContextShellDefaultCustomized
		case ContextShellEnvironmentInherit:
			if shellState != ContextShellDefaultCustomized {
				shellState = ContextShellDefaultInherited
			}
		}
	}
	if err := git.Validate(true); err != nil {
		return ContextWorkspaceDefaultsSummary{}, err
	}
	gitState := ContextGitDefaultNotImported
	switch git.Source {
	case ContextGitIdentityInherit:
		gitState = ContextGitDefaultInherited
	case ContextGitIdentityLiteral:
		gitState = ContextGitDefaultConfigured
	}
	resolvedBootstrap := bootstrap.Resolved()
	if err := resolvedBootstrap.Validate(); err != nil {
		return ContextWorkspaceDefaultsSummary{}, err
	}
	bootstrapState := ContextBootstrapDefaultNone
	if resolvedBootstrap.State == ContextBootstrapConfigured {
		bootstrapState = ContextBootstrapDefaultConfigured
	}
	summary := ContextWorkspaceDefaultsSummary{Shell: shellState, Git: gitState, Bootstrap: bootstrapState}
	return summary, summary.Validate()
}

func (r ContextRuntimeReport) Selection() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Name == StandardRuntimeName && r.Ordinal == 1 {
		return StandardRuntimeName + "@1", nil
	}
	if r.Kind == ContextRuntimeKindDockerfile {
		return "context-owned Dockerfile", nil
	}
	selection := fmt.Sprintf("%s@%d", r.Name, r.Ordinal)
	if _, _, err := ParseRuntimeSelection(selection); err != nil {
		return "", err
	}
	return selection, nil
}

func validateRuntimeDisplaySelection(selection string) error {
	if selection == StandardRuntimeName+"@1" {
		return nil
	}
	_, _, err := ParseRuntimeSelection(selection)
	return err
}

func (r ContextReport) RoutineSummary() (ContextRoutineSummary, error) {
	if err := validateRoutineAccessProjection(r.RoutineAccess, r.SourceAccess, r.MethodPolicy); err != nil {
		return ContextRoutineSummary{}, err
	}
	access := ContextAccessSummary{}
	if r.RoutineAccess != nil {
		access = *r.RoutineAccess
	} else {
		policy, ok := DefaultContextPolicySnapshot()
		if !ok {
			return ContextRoutineSummary{}, fmt.Errorf("default Context policy is unavailable")
		}
		policy.MethodPolicy = r.MethodPolicy.Clone()
		var err error
		access, err = SummarizeContextAccess(policy, r.SourceAccess, r.NativeReadiness)
		if err != nil {
			return ContextRoutineSummary{}, err
		}
	}
	defaults, err := summarizeContextWorkspaceDefaults(r.ShellEnvironment, r.GitIdentity, r.Bootstrap)
	if err != nil {
		return ContextRoutineSummary{}, err
	}
	selection, err := r.Runtime.Selection()
	if err != nil {
		return ContextRoutineSummary{}, err
	}
	action := ContextRoutineActionEnterCurrent
	if !r.Active && r.ContextState == ContextObservationPersisted {
		action = ContextRoutineActionEnterNamed
	}
	if r.Runtime.Status == ContextRuntimeStatusPendingBuild || r.Runtime.Status == ContextRuntimeStatusInvalid {
		action = ContextRoutineActionBuildRuntime
		if !r.Active && r.ContextState == ContextObservationPersisted {
			action = ContextRoutineActionSelectThenBuild
		}
	}
	summary := ContextRoutineSummary{
		Access: access, RuntimeSelection: selection, RuntimeStatus: r.Runtime.Status,
		Defaults: defaults, AuthenticationMode: contextRoutineAuthenticationMode(r.Authentication),
		RecommendedNotSaved: r.ContextState == ContextObservationSyntheticDefault, Action: action,
	}
	return summary, summary.Validate()
}

func contextRoutineAuthenticationMode(authentication ContextAuthentication) string {
	if authentication.Mode != "" {
		return authentication.Mode
	}
	if authentication.BrokerState == ContextAuthBrokerNotApplicable {
		return ContextAuthenticationModeNotApplicable
	}
	return ContextAuthenticationModeBroker
}

func (s ContextSummary) RoutineSummary() (ContextRoutineSummary, error) {
	if err := validateRoutineAccessProjection(s.RoutineAccess, s.SourceAccess, s.MethodPolicy); err != nil {
		return ContextRoutineSummary{}, err
	}
	access := ContextAccessSummary{}
	if s.RoutineAccess != nil {
		access = *s.RoutineAccess
	} else {
		policy, ok := DefaultContextPolicySnapshot()
		if !ok {
			return ContextRoutineSummary{}, fmt.Errorf("default Context policy is unavailable")
		}
		policy.MethodPolicy = s.MethodPolicy.Clone()
		var err error
		access, err = SummarizeContextAccess(policy, s.SourceAccess, s.NativeReadiness)
		if err != nil {
			return ContextRoutineSummary{}, err
		}
	}
	if s.RuntimeStatus == ContextRuntimeStatusOfficial || s.RuntimeStatus == ContextRuntimeStatusReady {
		if err := validateRuntimeDisplaySelection(s.RuntimeSelection); err != nil {
			return ContextRoutineSummary{}, err
		}
	}
	action := ContextRoutineActionEnterNamed
	if s.Active {
		action = ContextRoutineActionEnterCurrent
	}
	if s.RuntimeStatus == ContextRuntimeStatusPendingBuild || s.RuntimeStatus == ContextRuntimeStatusInvalid {
		action = ContextRoutineActionBuildRuntime
		if !s.Active {
			action = ContextRoutineActionSelectThenBuild
		}
	}
	summary := ContextRoutineSummary{
		Access: access, RuntimeSelection: s.RuntimeSelection, RuntimeStatus: s.RuntimeStatus,
		Defaults: ContextWorkspaceDefaultsSummary{
			Shell: ContextShellDefaultStandard, Git: ContextGitDefaultNotImported, Bootstrap: ContextBootstrapDefaultNone,
		},
		AuthenticationMode: ContextAuthenticationModeNotApplicable, Action: action,
	}
	return summary, summary.Validate()
}

type ProjectRoutineAction string

const (
	ProjectRoutineActionCreateOrEnter ProjectRoutineAction = "create_or_enter"
	ProjectRoutineActionEnter         ProjectRoutineAction = "enter"
	ProjectRoutineActionInspect       ProjectRoutineAction = "inspect_runtime"
)

func (a ProjectRoutineAction) Validate() error {
	switch a {
	case ProjectRoutineActionCreateOrEnter, ProjectRoutineActionEnter, ProjectRoutineActionInspect:
		return nil
	default:
		return fmt.Errorf("Workspace routine action is invalid: %q", a)
	}
}

type ProjectRoutineSummary struct {
	Action             ProjectRoutineAction
	BootstrapAttention bool
	RuntimeSelection   string
}

func (s ProjectRoutineSummary) Validate() error {
	return s.Action.Validate()
}

func (s ProjectStatus) RoutineSummary() (ProjectRoutineSummary, error) {
	if err := s.Validate(); err != nil {
		return ProjectRoutineSummary{}, err
	}
	action := ProjectRoutineActionCreateOrEnter
	if s.Exists {
		action = ProjectRoutineActionEnter
		if s.Runtime != RuntimeDiagnosticReady {
			action = ProjectRoutineActionInspect
		}
	}
	bootstrap := s.Bootstrap.Resolved()
	summary := ProjectRoutineSummary{
		Action:             action,
		BootstrapAttention: s.Exists && (bootstrap.State == WorkspaceBootstrapNotApplied || bootstrap.State == WorkspaceBootstrapOlder),
		RuntimeSelection:   s.RuntimeSelection,
	}
	return summary, summary.Validate()
}
