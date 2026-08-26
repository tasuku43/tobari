package tobari

import "fmt"

// RecommendedFirstUseSessionKind identifies the already-validated foreground
// session without exposing presentation text or reconstructing child argv.
type RecommendedFirstUseSessionKind string

const (
	RecommendedFirstUseSessionBash   RecommendedFirstUseSessionKind = "bash"
	RecommendedFirstUseSessionDirect RecommendedFirstUseSessionKind = "direct"
)

type RecommendedFirstUseSession struct {
	Kind       RecommendedFirstUseSessionKind
	Executable string
}

type RecommendedHostConfigurationState string

const RecommendedHostConfigurationNotImported RecommendedHostConfigurationState = "not_imported"

func (s RecommendedHostConfigurationState) Validate() error {
	if s != RecommendedHostConfigurationNotImported {
		return fmt.Errorf("recommended host-configuration state is invalid: %q", s)
	}
	return nil
}

func recommendedFirstUseSession(request WorkspaceSessionRequest) (RecommendedFirstUseSession, error) {
	if err := request.Validate(); err != nil {
		return RecommendedFirstUseSession{}, err
	}
	if !request.Direct() {
		return RecommendedFirstUseSession{Kind: RecommendedFirstUseSessionBash}, nil
	}
	return RecommendedFirstUseSession{Kind: RecommendedFirstUseSessionDirect, Executable: request.Argv()[0]}, nil
}

func (s RecommendedFirstUseSession) Validate() error {
	switch s.Kind {
	case RecommendedFirstUseSessionBash:
		if s.Executable != "" {
			return fmt.Errorf("Bash first-use session cannot carry an executable")
		}
	case RecommendedFirstUseSessionDirect:
		if s.Executable == "" {
			return fmt.Errorf("direct first-use session executable is required")
		}
	default:
		return fmt.Errorf("recommended first-use session kind is invalid: %q", s.Kind)
	}
	return nil
}

// RecommendedFirstUseDraft is the complete reviewed root-only first-use
// proposal. Its creation composition is derived from the same values that are
// rendered; presentation never infers policy or mutation defaults.
type RecommendedFirstUseDraft struct {
	ProjectRoot           string
	WorkspaceManifestName string
	Access                ManifestAccessSummary
	RuntimeSelection      string
	NativeReadiness       ManifestNativeReadiness
	HostConfiguration     RecommendedHostConfigurationState
	Session               RecommendedFirstUseSession
}

func NewRecommendedFirstUseDraft(root string, session WorkspaceSessionRequest) (RecommendedFirstUseDraft, error) {
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		return RecommendedFirstUseDraft{}, fmt.Errorf("default Workspace Manifest policy is unavailable")
	}
	access, err := SummarizeContextAccess(policy, ManifestSourceAccessReadWrite, ManifestNativeReadinessEnabled)
	if err != nil {
		return RecommendedFirstUseDraft{}, err
	}
	summary, err := recommendedFirstUseSession(session)
	if err != nil {
		return RecommendedFirstUseDraft{}, err
	}
	draft := RecommendedFirstUseDraft{
		ProjectRoot: root, WorkspaceManifestName: DefaultManifestName,
		Access: access, RuntimeSelection: StandardRuntimeName + "@1",
		NativeReadiness:   ManifestNativeReadinessEnabled,
		HostConfiguration: RecommendedHostConfigurationNotImported, Session: summary,
	}
	return draft, draft.Validate()
}

func (d RecommendedFirstUseDraft) Validate() error {
	if err := ValidateCanonicalRoot(d.ProjectRoot); err != nil {
		return err
	}
	if d.WorkspaceManifestName != DefaultManifestName ||
		d.RuntimeSelection != StandardRuntimeName+"@1" || d.NativeReadiness != ManifestNativeReadinessEnabled ||
		d.HostConfiguration != RecommendedHostConfigurationNotImported {
		return fmt.Errorf("recommended first-use settings do not match the supported draft")
	}
	if err := d.Access.Validate(); err != nil {
		return err
	}
	if err := d.HostConfiguration.Validate(); err != nil {
		return err
	}
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		return fmt.Errorf("default Workspace Manifest policy is unavailable")
	}
	expected, err := SummarizeContextAccess(policy, ManifestSourceAccessReadWrite, ManifestNativeReadinessEnabled)
	if err != nil {
		return err
	}
	if !sameContextAccessSummary(d.Access, expected) {
		return fmt.Errorf("recommended first-use Access does not match the supported draft")
	}
	return d.Session.Validate()
}

func sameContextAccessSummary(left, right ManifestAccessSummary) bool {
	if left.SourceAccess != right.SourceAccess || left.RoutineTraffic != right.RoutineTraffic ||
		left.PrivateTargets != right.PrivateTargets || left.MethodPolicy.Default != right.MethodPolicy.Default ||
		len(left.MethodPolicy.Overrides) != len(right.MethodPolicy.Overrides) {
		return false
	}
	for index := range left.MethodPolicy.Overrides {
		if left.MethodPolicy.Overrides[index] != right.MethodPolicy.Overrides[index] {
			return false
		}
	}
	return true
}

func (d RecommendedFirstUseDraft) Composition() ManifestCreateComposition {
	methodPolicy := d.Access.MethodPolicy.Clone()
	return ManifestCreateComposition{
		NativeReadiness: d.NativeReadiness, MethodPolicy: &methodPolicy,
		RuntimeSelection: d.RuntimeSelection,
	}
}
