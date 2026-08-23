package tobari

import "fmt"

// ManifestRoutineTrafficState is the effective availability of the trusted
// binary's reviewed native-client traffic inside one Context Boundary.
type ManifestRoutineTrafficState string

const (
	ManifestRoutineTrafficReady      ManifestRoutineTrafficState = "ready"
	ManifestRoutineTrafficLimited    ManifestRoutineTrafficState = "limited"
	ManifestRoutineTrafficNotEnabled ManifestRoutineTrafficState = "not_enabled"
)

func (s ManifestRoutineTrafficState) Validate() error {
	switch s {
	case ManifestRoutineTrafficReady, ManifestRoutineTrafficLimited, ManifestRoutineTrafficNotEnabled:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest routine traffic state is invalid: %q", s)
	}
}

type ManifestShellDefaultState string

const (
	ManifestShellDefaultStandard   ManifestShellDefaultState = "standard"
	ManifestShellDefaultInherited  ManifestShellDefaultState = "inherited"
	ManifestShellDefaultCustomized ManifestShellDefaultState = "customized"
)

func (s ManifestShellDefaultState) Validate() error {
	switch s {
	case ManifestShellDefaultStandard, ManifestShellDefaultInherited, ManifestShellDefaultCustomized:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest shell default state is invalid: %q", s)
	}
}

type ManifestGitDefaultState string

const (
	ManifestGitDefaultNotImported ManifestGitDefaultState = "not_imported"
	ManifestGitDefaultInherited   ManifestGitDefaultState = "inherited"
	ManifestGitDefaultConfigured  ManifestGitDefaultState = "configured"
)

func (s ManifestGitDefaultState) Validate() error {
	switch s {
	case ManifestGitDefaultNotImported, ManifestGitDefaultInherited, ManifestGitDefaultConfigured:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest Git default state is invalid: %q", s)
	}
}

type ManifestBootstrapDefaultState string

const (
	ManifestBootstrapDefaultNone       ManifestBootstrapDefaultState = "none"
	ManifestBootstrapDefaultConfigured ManifestBootstrapDefaultState = "configured"
)

func (s ManifestBootstrapDefaultState) Validate() error {
	switch s {
	case ManifestBootstrapDefaultNone, ManifestBootstrapDefaultConfigured:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest bootstrap default state is invalid: %q", s)
	}
}

// ManifestRoutineAction identifies the next task implied by the selected
// Context state without carrying presentation text.
type ManifestRoutineAction string

const (
	ManifestRoutineActionEnterCurrent    ManifestRoutineAction = "enter_current"
	ManifestRoutineActionEnterNamed      ManifestRoutineAction = "enter_named"
	ManifestRoutineActionBuildRuntime    ManifestRoutineAction = "build_runtime"
	ManifestRoutineActionSelectThenBuild ManifestRoutineAction = "select_then_build"
)

func (a ManifestRoutineAction) Validate() error {
	switch a {
	case ManifestRoutineActionEnterCurrent, ManifestRoutineActionEnterNamed,
		ManifestRoutineActionBuildRuntime, ManifestRoutineActionSelectThenBuild:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest routine action is invalid: %q", a)
	}
}

type ManifestAccessSummary struct {
	SourceAccess   ManifestSourceAccess
	RoutineTraffic ManifestRoutineTrafficState
	MethodPolicy   ManifestMethodPolicy
	PrivateTargets ManifestMethodDecision
}

func (s ManifestAccessSummary) Validate() error {
	if err := s.SourceAccess.Validate(); err != nil {
		return err
	}
	if err := s.RoutineTraffic.Validate(); err != nil {
		return err
	}
	if err := s.MethodPolicy.Validate(); err != nil {
		return err
	}
	if s.PrivateTargets != ManifestMethodDeny {
		return fmt.Errorf("private targets must remain denied in the routine Workspace Manifest summary")
	}
	return nil
}

func validateRoutineAccessProjection(summary *ManifestAccessSummary, source ManifestSourceAccess, methods ManifestMethodPolicy) error {
	if summary == nil {
		return nil
	}
	if err := summary.Validate(); err != nil {
		return err
	}
	if summary.SourceAccess != source || summary.MethodPolicy.Default != methods.Default ||
		len(summary.MethodPolicy.Overrides) != len(methods.Overrides) {
		return fmt.Errorf("Workspace Manifest routine Access does not match its source or method projection")
	}
	for index := range methods.Overrides {
		if summary.MethodPolicy.Overrides[index] != methods.Overrides[index] {
			return fmt.Errorf("Workspace Manifest routine Access does not match its method overrides")
		}
	}
	return nil
}

type ManifestWorkspaceDefaultsSummary struct {
	Shell     ManifestShellDefaultState
	Git       ManifestGitDefaultState
	Bootstrap ManifestBootstrapDefaultState
}

func (s ManifestWorkspaceDefaultsSummary) Validate() error {
	if err := s.Shell.Validate(); err != nil {
		return err
	}
	if err := s.Git.Validate(); err != nil {
		return err
	}
	return s.Bootstrap.Validate()
}

type ManifestRoutineSummary struct {
	Access              ManifestAccessSummary
	RuntimeSelection    string
	RuntimeStatus       ManifestRuntimeStatus
	Defaults            ManifestWorkspaceDefaultsSummary
	AuthenticationMode  string
	RecommendedNotSaved bool
	Action              ManifestRoutineAction
}

func (s ManifestRoutineSummary) Validate() error {
	if err := s.Access.Validate(); err != nil {
		return err
	}
	if s.RuntimeStatus == ManifestRuntimeStatusOfficial || s.RuntimeStatus == ManifestRuntimeStatusReady {
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
	case ManifestAuthenticationModeNative, ManifestAuthenticationModeBroker, ManifestAuthenticationModeNotApplicable:
	default:
		return fmt.Errorf("Workspace Manifest routine authentication mode is invalid: %q", s.AuthenticationMode)
	}
	return s.Action.Validate()
}

// SummarizeContextAccess evaluates the reviewed routine-client overlay against
// the actual Context destination and method ceilings. It does not infer
// process identity or general network availability.
func SummarizeContextAccess(
	policy ManifestPolicy, source ManifestSourceAccess, readiness ManifestNativeReadiness,
) (ManifestAccessSummary, error) {
	if err := source.Validate(); err != nil {
		return ManifestAccessSummary{}, err
	}
	normalized, _, _, err := NormalizeContextPolicy(policy)
	if err != nil {
		return ManifestAccessSummary{}, err
	}
	resolved, err := ResolveContextNativeReadiness(readiness)
	if err != nil {
		return ManifestAccessSummary{}, err
	}
	traffic := ManifestRoutineTrafficNotEnabled
	if resolved == ManifestNativeReadinessEnabled {
		traffic = ManifestRoutineTrafficReady
		for _, bundle := range nativeToolAuthReadinessBundles() {
			for _, rule := range append(append([]ManifestPolicyExactRule(nil), bundle.BaselineGrants...), bundle.GraphQLEndpoints...) {
				if !contextPolicyRuleInsideDestination(normalized.DestinationCeiling, rule) ||
					normalized.MethodPolicy.Decision(rule.Method) == ManifestMethodDeny {
					traffic = ManifestRoutineTrafficLimited
					break
				}
			}
			if traffic == ManifestRoutineTrafficLimited {
				break
			}
		}
	}
	summary := ManifestAccessSummary{
		SourceAccess: source, RoutineTraffic: traffic,
		MethodPolicy: normalized.MethodPolicy.Clone(), PrivateTargets: ManifestMethodDeny,
	}
	return summary, summary.Validate()
}

func summarizeContextWorkspaceDefaults(
	shell []ManifestShellEnvironmentSetting, git ManifestGitIdentitySetting, bootstrap ManifestBootstrapReport,
) (ManifestWorkspaceDefaultsSummary, error) {
	if err := validateContextShellEnvironment(shell, true); err != nil {
		return ManifestWorkspaceDefaultsSummary{}, err
	}
	shellState := ManifestShellDefaultStandard
	for _, setting := range shell {
		switch setting.Source {
		case ManifestShellEnvironmentLiteral:
			shellState = ManifestShellDefaultCustomized
		case ManifestShellEnvironmentInherit:
			if shellState != ManifestShellDefaultCustomized {
				shellState = ManifestShellDefaultInherited
			}
		}
	}
	if err := git.Validate(true); err != nil {
		return ManifestWorkspaceDefaultsSummary{}, err
	}
	gitState := ManifestGitDefaultNotImported
	switch git.Source {
	case ManifestGitIdentityInherit:
		gitState = ManifestGitDefaultInherited
	case ManifestGitIdentityLiteral:
		gitState = ManifestGitDefaultConfigured
	}
	resolvedBootstrap := bootstrap.Resolved()
	if err := resolvedBootstrap.Validate(); err != nil {
		return ManifestWorkspaceDefaultsSummary{}, err
	}
	bootstrapState := ManifestBootstrapDefaultNone
	if resolvedBootstrap.State == ManifestBootstrapConfigured {
		bootstrapState = ManifestBootstrapDefaultConfigured
	}
	summary := ManifestWorkspaceDefaultsSummary{Shell: shellState, Git: gitState, Bootstrap: bootstrapState}
	return summary, summary.Validate()
}

func (r ManifestRuntimeReport) Selection() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Name == StandardRuntimeName && r.Ordinal == 1 {
		return StandardRuntimeName + "@1", nil
	}
	if r.Kind == ManifestRuntimeKindDockerfile {
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

func (r ManifestReport) RoutineSummary() (ManifestRoutineSummary, error) {
	if err := validateRoutineAccessProjection(r.RoutineAccess, r.SourceAccess, r.MethodPolicy); err != nil {
		return ManifestRoutineSummary{}, err
	}
	access := ManifestAccessSummary{}
	if r.RoutineAccess != nil {
		access = *r.RoutineAccess
	} else {
		policy, ok := DefaultContextPolicySnapshot()
		if !ok {
			return ManifestRoutineSummary{}, fmt.Errorf("default Workspace Manifest policy is unavailable")
		}
		policy.MethodPolicy = r.MethodPolicy.Clone()
		var err error
		access, err = SummarizeContextAccess(policy, r.SourceAccess, r.NativeReadiness)
		if err != nil {
			return ManifestRoutineSummary{}, err
		}
	}
	defaults, err := summarizeContextWorkspaceDefaults(r.ShellEnvironment, r.GitIdentity, r.Bootstrap)
	if err != nil {
		return ManifestRoutineSummary{}, err
	}
	selection, err := r.Runtime.Selection()
	if err != nil {
		return ManifestRoutineSummary{}, err
	}
	action := ManifestRoutineActionEnterCurrent
	if !r.Default && r.ManifestState == ManifestObservationPersisted {
		action = ManifestRoutineActionEnterNamed
	}
	if r.Runtime.Status == ManifestRuntimeStatusPendingBuild || r.Runtime.Status == ManifestRuntimeStatusInvalid {
		action = ManifestRoutineActionBuildRuntime
		if !r.Default && r.ManifestState == ManifestObservationPersisted {
			action = ManifestRoutineActionSelectThenBuild
		}
	}
	summary := ManifestRoutineSummary{
		Access: access, RuntimeSelection: selection, RuntimeStatus: r.Runtime.Status,
		Defaults: defaults, AuthenticationMode: contextRoutineAuthenticationMode(r.Authentication),
		RecommendedNotSaved: r.ManifestState == ManifestObservationAbsent, Action: action,
	}
	return summary, summary.Validate()
}

func contextRoutineAuthenticationMode(authentication ManifestAuthentication) string {
	if authentication.Mode != "" {
		return authentication.Mode
	}
	if authentication.BrokerState == ManifestAuthBrokerNotApplicable {
		return ManifestAuthenticationModeNotApplicable
	}
	return ManifestAuthenticationModeBroker
}

func (s ManifestSummary) RoutineSummary() (ManifestRoutineSummary, error) {
	if err := validateRoutineAccessProjection(s.RoutineAccess, s.SourceAccess, s.MethodPolicy); err != nil {
		return ManifestRoutineSummary{}, err
	}
	access := ManifestAccessSummary{}
	if s.RoutineAccess != nil {
		access = *s.RoutineAccess
	} else {
		policy, ok := DefaultContextPolicySnapshot()
		if !ok {
			return ManifestRoutineSummary{}, fmt.Errorf("default Workspace Manifest policy is unavailable")
		}
		policy.MethodPolicy = s.MethodPolicy.Clone()
		var err error
		access, err = SummarizeContextAccess(policy, s.SourceAccess, s.NativeReadiness)
		if err != nil {
			return ManifestRoutineSummary{}, err
		}
	}
	if s.RuntimeStatus == ManifestRuntimeStatusOfficial || s.RuntimeStatus == ManifestRuntimeStatusReady {
		if err := validateRuntimeDisplaySelection(s.RuntimeSelection); err != nil {
			return ManifestRoutineSummary{}, err
		}
	}
	action := ManifestRoutineActionEnterNamed
	if s.Default {
		action = ManifestRoutineActionEnterCurrent
	}
	if s.RuntimeStatus == ManifestRuntimeStatusPendingBuild || s.RuntimeStatus == ManifestRuntimeStatusInvalid {
		action = ManifestRoutineActionBuildRuntime
		if !s.Default {
			action = ManifestRoutineActionSelectThenBuild
		}
	}
	summary := ManifestRoutineSummary{
		Access: access, RuntimeSelection: s.RuntimeSelection, RuntimeStatus: s.RuntimeStatus,
		Defaults: ManifestWorkspaceDefaultsSummary{
			Shell: ManifestShellDefaultStandard, Git: ManifestGitDefaultNotImported, Bootstrap: ManifestBootstrapDefaultNone,
		},
		AuthenticationMode: ManifestAuthenticationModeNotApplicable, Action: action,
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

func (s WorkspaceStatus) RoutineSummary() (ProjectRoutineSummary, error) {
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
