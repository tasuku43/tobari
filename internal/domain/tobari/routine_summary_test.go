package tobari

import (
	"strings"
	"testing"
)

func TestContextAccessSummaryAccountsForReadinessAndBothCeilings(t *testing.T) {
	t.Parallel()
	base := ManifestPolicy{
		SchemaVersion: ManifestPolicySchemaVersion,
		Name:          "summary",
		DestinationCeiling: ManifestPolicyDestinationCeiling{
			Mode: "public_https", Authorities: []ManifestPolicyAuthority{},
		},
		MethodPolicy:      ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		BaselineGrants:    []ManifestPolicyExactRule{},
		BaselineTemplates: []ManifestPolicyPathTemplateRule{},
		MCPBaselineGrants: []ManifestPolicyMCPRule{},
		BaselineDenies:    []ManifestPolicyExactRule{},
		GraphQLEndpoints:  []ManifestPolicyExactRule{},
		MCPEndpoints:      []ManifestPolicyExactRule{},
	}

	ready, err := SummarizeContextAccess(base, ManifestSourceAccessReadWrite, ManifestNativeReadinessEnabled)
	if err != nil || ready.RoutineTraffic != ManifestRoutineTrafficReady || ready.PrivateTargets != ManifestMethodDeny {
		t.Fatalf("ready access summary = %+v, error = %v", ready, err)
	}

	methodLimited := base
	methodLimited.MethodPolicy = ManifestMethodPolicy{
		Default:   ManifestMethodExactReview,
		Overrides: []ManifestMethodOverride{{Method: "POST", Decision: ManifestMethodDeny}},
	}
	limited, err := SummarizeContextAccess(methodLimited, ManifestSourceAccessReadOnly, ManifestNativeReadinessEnabled)
	if err != nil || limited.RoutineTraffic != ManifestRoutineTrafficLimited || limited.SourceAccess != ManifestSourceAccessReadOnly {
		t.Fatalf("method-limited access summary = %+v, error = %v", limited, err)
	}

	destinationLimited := base
	destinationLimited.DestinationCeiling = ManifestPolicyDestinationCeiling{
		Mode:        "exact",
		Authorities: []ManifestPolicyAuthority{{Scheme: "https", Host: "api.anthropic.com", Port: 443}},
	}
	limited, err = SummarizeContextAccess(destinationLimited, ManifestSourceAccessReadWrite, ManifestNativeReadinessEnabled)
	if err != nil || limited.RoutineTraffic != ManifestRoutineTrafficLimited {
		t.Fatalf("destination-limited access summary = %+v, error = %v", limited, err)
	}

	disabled, err := SummarizeContextAccess(base, ManifestSourceAccessReadWrite, ManifestNativeReadinessDisabled)
	if err != nil || disabled.RoutineTraffic != ManifestRoutineTrafficNotEnabled {
		t.Fatalf("disabled access summary = %+v, error = %v", disabled, err)
	}
}

func TestContextRoutineSummaryOwnsDefaultsRuntimeAndAction(t *testing.T) {
	t.Parallel()
	empty := ""
	report := ManifestReport{
		Task: TaskManifestShow, ManifestState: ManifestObservationPersisted,
		ID: "018bcfe5-687b-7000-8000-000000000099", Name: "review", Default: false,
		AgentProfile: DefaultProfile, Image: "tobari-context-review:123456789abc",
		PolicyMode: ManifestPolicyModeGuided, SourceAccess: ManifestSourceAccessReadWrite,
		PolicyRevision: DefaultContextPolicyRevision(), NativeReadiness: ManifestNativeReadinessDisabled,
		MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		ShellEnvironment: []ManifestShellEnvironmentSetting{
			{Variable: "COLORTERM", Source: ManifestShellEnvironmentDefault},
			{Variable: "NO_COLOR", Source: ManifestShellEnvironmentLiteral, Value: &empty},
			{Variable: "PS1", Source: ManifestShellEnvironmentInherit},
			{Variable: "TERM", Source: ManifestShellEnvironmentDefault},
		},
		GitIdentity: ManifestGitIdentitySetting{Source: ManifestGitIdentityInherit},
		Stores:      ManifestStorePaths{PolicyDirectory: "/config/contexts/review/policy"},
		Runtime: ManifestRuntimeReport{
			Kind: ManifestRuntimeKindManaged, Status: ManifestRuntimeStatusReady,
			Image:     "tobari-context-review:123456789abc",
			RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend",
			Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4,
		},
		Cluster:        ManifestClusterStatusNotApplicable,
		Authentication: ManifestAuthentication{Mode: ManifestAuthenticationModeNative, Providers: []ManifestAuthProvider{}},
		Bootstrap:      ManifestBootstrapReportFrom(nil),
	}
	summary, err := report.RoutineSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.RuntimeSelection != "frontend@4" || summary.Action != ManifestRoutineActionEnterNamed ||
		summary.Defaults.Shell != ManifestShellDefaultCustomized || summary.Defaults.Git != ManifestGitDefaultInherited ||
		summary.Defaults.Bootstrap != ManifestBootstrapDefaultNone || summary.AuthenticationMode != ManifestAuthenticationModeNative {
		t.Fatalf("Workspace Manifest routine summary = %+v", summary)
	}

	listSummary, err := (ManifestSummary{
		ID: report.ID, Name: report.Name, ManifestState: report.ManifestState, Default: false,
		AgentProfile: report.AgentProfile, Image: report.Image, PolicyMode: report.PolicyMode,
		SourceAccess: report.SourceAccess, PolicyRevision: report.PolicyRevision,
		NativeReadiness: report.NativeReadiness, MethodPolicy: report.MethodPolicy,
		RuntimeStatus: ManifestRuntimeStatusInvalid, RuntimeSelection: "frontend@4",
		Bootstrap: ManifestBootstrapReportFrom(nil),
	}).RoutineSummary()
	if err != nil || listSummary.Action != ManifestRoutineActionSelectThenBuild {
		t.Fatalf("invalid inactive Runtime action = %+v, error = %v", listSummary, err)
	}
}

func TestContextRoutineSummaryRejectsMismatchedEffectiveAccess(t *testing.T) {
	t.Parallel()
	methods := ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}}
	access := ManifestAccessSummary{
		SourceAccess: ManifestSourceAccessReadOnly, RoutineTraffic: ManifestRoutineTrafficReady,
		MethodPolicy: methods.Clone(), PrivateTargets: ManifestMethodDeny,
	}
	summary := ManifestSummary{
		ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default",
		ManifestState: ManifestObservationPersisted, Default: true, AgentProfile: DefaultProfile,
		Image: OfficialRuntimeBase, PolicyMode: ManifestPolicyModeGuided,
		SourceAccess: ManifestSourceAccessReadWrite, PolicyRevision: DefaultContextPolicyRevision(),
		NativeReadiness: ManifestNativeReadinessEnabled, MethodPolicy: methods, RoutineAccess: &access,
		RuntimeStatus: ManifestRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1",
	}
	if _, err := summary.RoutineSummary(); err == nil {
		t.Fatal("RoutineSummary() accepted Access from a different source boundary")
	}
}

func TestProjectRoutineSummarySeparatesLifecycleActionFromBootstrapAttention(t *testing.T) {
	t.Parallel()
	status := WorkspaceStatus{
		Task: TaskStatus, ManifestState: ManifestObservationPersisted, Exists: true,
		Root: "/workspace/example", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/state/example/home", WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000099",
		WorkspaceManifestName: "default", Runtime: RuntimeDiagnosticReady, Attachment: AttachmentDetached,
		Adoption: WorkspaceAdoptionNeverApplied, Next: ptrDesiredEntry(testDesiredEntry()),
		Bootstrap: WorkspaceBootstrapReport{
			State:           WorkspaceBootstrapOlder,
			AppliedRevision: "sha256:" + strings.Repeat("a", 64),
			CurrentRevision: "sha256:" + strings.Repeat("b", 64),
		},
	}
	summary, err := status.RoutineSummary()
	if err != nil || summary.Action != ProjectRoutineActionEnter || !summary.BootstrapAttention {
		t.Fatalf("ready status summary = %+v, error = %v", summary, err)
	}
	status.Runtime = RuntimeDiagnosticMissing
	summary, err = status.RoutineSummary()
	if err != nil || summary.Action != ProjectRoutineActionInspect {
		t.Fatalf("missing runtime summary = %+v, error = %v", summary, err)
	}
}
