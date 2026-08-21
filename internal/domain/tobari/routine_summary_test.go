package tobari

import (
	"strings"
	"testing"
)

func TestContextAccessSummaryAccountsForReadinessAndBothCeilings(t *testing.T) {
	t.Parallel()
	base := ContextPolicy{
		SchemaVersion: ContextPolicySchemaVersion,
		Name:          "summary",
		DestinationCeiling: ContextPolicyDestinationCeiling{
			Mode: "public_https", Authorities: []ContextPolicyAuthority{},
		},
		MethodPolicy:      ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		BaselineGrants:    []ContextPolicyExactRule{},
		BaselineTemplates: []ContextPolicyPathTemplateRule{},
		MCPBaselineGrants: []ContextPolicyMCPRule{},
		BaselineDenies:    []ContextPolicyExactRule{},
		GraphQLEndpoints:  []ContextPolicyExactRule{},
		MCPEndpoints:      []ContextPolicyExactRule{},
	}

	ready, err := SummarizeContextAccess(base, ContextSourceAccessReadWrite, ContextNativeReadinessEnabled)
	if err != nil || ready.RoutineTraffic != ContextRoutineTrafficReady || ready.PrivateTargets != ContextMethodDeny {
		t.Fatalf("ready access summary = %+v, error = %v", ready, err)
	}

	methodLimited := base
	methodLimited.MethodPolicy = ContextMethodPolicy{
		Default:   ContextMethodExactReview,
		Overrides: []ContextMethodOverride{{Method: "POST", Decision: ContextMethodDeny}},
	}
	limited, err := SummarizeContextAccess(methodLimited, ContextSourceAccessReadOnly, ContextNativeReadinessEnabled)
	if err != nil || limited.RoutineTraffic != ContextRoutineTrafficLimited || limited.SourceAccess != ContextSourceAccessReadOnly {
		t.Fatalf("method-limited access summary = %+v, error = %v", limited, err)
	}

	destinationLimited := base
	destinationLimited.DestinationCeiling = ContextPolicyDestinationCeiling{
		Mode:        "exact",
		Authorities: []ContextPolicyAuthority{{Scheme: "https", Host: "api.anthropic.com", Port: 443}},
	}
	limited, err = SummarizeContextAccess(destinationLimited, ContextSourceAccessReadWrite, ContextNativeReadinessEnabled)
	if err != nil || limited.RoutineTraffic != ContextRoutineTrafficLimited {
		t.Fatalf("destination-limited access summary = %+v, error = %v", limited, err)
	}

	disabled, err := SummarizeContextAccess(base, ContextSourceAccessReadWrite, ContextNativeReadinessDisabled)
	if err != nil || disabled.RoutineTraffic != ContextRoutineTrafficNotEnabled {
		t.Fatalf("disabled access summary = %+v, error = %v", disabled, err)
	}
}

func TestContextRoutineSummaryOwnsDefaultsRuntimeAndAction(t *testing.T) {
	t.Parallel()
	empty := ""
	report := ContextReport{
		Task: TaskContextShow, ContextState: ContextObservationPersisted,
		ID: "018bcfe5-687b-7000-8000-000000000099", Name: "review", Active: false,
		AgentProfile: DefaultProfile, Image: "tobari-context-review:123456789abc",
		PolicyMode: ContextPolicyModeGuided, SourceAccess: ContextSourceAccessReadWrite,
		PolicyRevision: DefaultContextPolicyRevision(), NativeReadiness: ContextNativeReadinessDisabled,
		MethodPolicy: ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		ShellEnvironment: []ContextShellEnvironmentSetting{
			{Variable: "COLORTERM", Source: ContextShellEnvironmentDefault},
			{Variable: "NO_COLOR", Source: ContextShellEnvironmentLiteral, Value: &empty},
			{Variable: "PS1", Source: ContextShellEnvironmentInherit},
			{Variable: "TERM", Source: ContextShellEnvironmentDefault},
		},
		GitIdentity: ContextGitIdentitySetting{Source: ContextGitIdentityInherit},
		Stores:      ContextStorePaths{PolicyDirectory: "/config/contexts/review/policy"},
		Runtime: ContextRuntimeReport{
			Kind: ContextRuntimeKindManaged, Status: ContextRuntimeStatusReady,
			Image:     "tobari-context-review:123456789abc",
			RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend",
			Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4,
		},
		Cluster:        ContextClusterStatusNotApplicable,
		Authentication: ContextAuthentication{Mode: ContextAuthenticationModeNative, Providers: []ContextAuthProvider{}},
		Bootstrap:      ContextBootstrapReportFrom(nil),
	}
	summary, err := report.RoutineSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.RuntimeSelection != "frontend@4" || summary.Action != ContextRoutineActionEnterNamed ||
		summary.Defaults.Shell != ContextShellDefaultCustomized || summary.Defaults.Git != ContextGitDefaultInherited ||
		summary.Defaults.Bootstrap != ContextBootstrapDefaultNone || summary.AuthenticationMode != ContextAuthenticationModeNative {
		t.Fatalf("Context routine summary = %+v", summary)
	}

	listSummary, err := (ContextSummary{
		ID: report.ID, Name: report.Name, ContextState: report.ContextState, Active: false,
		AgentProfile: report.AgentProfile, Image: report.Image, PolicyMode: report.PolicyMode,
		SourceAccess: report.SourceAccess, PolicyRevision: report.PolicyRevision,
		NativeReadiness: report.NativeReadiness, MethodPolicy: report.MethodPolicy,
		RuntimeStatus: ContextRuntimeStatusInvalid, RuntimeSelection: "frontend@4",
		Bootstrap: ContextBootstrapReportFrom(nil),
	}).RoutineSummary()
	if err != nil || listSummary.Action != ContextRoutineActionSelectThenBuild {
		t.Fatalf("invalid inactive Runtime action = %+v, error = %v", listSummary, err)
	}
}

func TestContextRoutineSummaryRejectsMismatchedEffectiveAccess(t *testing.T) {
	t.Parallel()
	methods := ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}}
	access := ContextAccessSummary{
		SourceAccess: ContextSourceAccessReadOnly, RoutineTraffic: ContextRoutineTrafficReady,
		MethodPolicy: methods.Clone(), PrivateTargets: ContextMethodDeny,
	}
	summary := ContextSummary{
		ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default",
		ContextState: ContextObservationPersisted, Active: true, AgentProfile: DefaultProfile,
		Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided,
		SourceAccess: ContextSourceAccessReadWrite, PolicyRevision: DefaultContextPolicyRevision(),
		NativeReadiness: ContextNativeReadinessEnabled, MethodPolicy: methods, RoutineAccess: &access,
		RuntimeStatus: ContextRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1",
	}
	if _, err := summary.RoutineSummary(); err == nil {
		t.Fatal("RoutineSummary() accepted Access from a different source boundary")
	}
}

func TestProjectRoutineSummarySeparatesLifecycleActionFromBootstrapAttention(t *testing.T) {
	t.Parallel()
	status := ProjectStatus{
		Task: TaskStatus, ContextState: ContextObservationPersisted, Exists: true,
		Root: "/workspace/example", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/state/example/home", ContextID: "018bcfe5-687b-7000-8000-000000000099",
		ContextName: "default", Runtime: RuntimeDiagnosticReady, Attachment: AttachmentDetached,
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
