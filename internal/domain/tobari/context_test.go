package tobari

import (
	"path/filepath"
	"strings"
	"testing"
)

func validContextManifest() ContextManifest {
	return ContextManifest{
		SchemaVersion:  ContextSchemaVersion,
		ID:             "018bcfe5-687b-7000-8000-000000000000",
		Name:           "project-tools",
		AgentProfile:   DefaultProfile,
		Image:          OfficialRuntimeBase,
		PolicyMode:     ContextPolicyModeAdvanced,
		SourceAccess:   ContextSourceAccessReadWrite,
		PolicyRevision: DefaultContextPolicyRevision(),
	}
}

func TestContextNativeReadinessCompatibility(t *testing.T) {
	for _, test := range []struct {
		name  string
		value ContextNativeReadiness
		want  ContextNativeReadiness
	}{
		{"enabled", ContextNativeReadinessEnabled, ContextNativeReadinessEnabled},
		{"disabled", ContextNativeReadinessDisabled, ContextNativeReadinessDisabled},
		{"omitted", "", ContextNativeReadinessEnabled},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveContextNativeReadiness(test.value)
			if err != nil || got != test.want {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
	if _, err := ResolveContextNativeReadiness("sometimes"); err == nil {
		t.Fatal("invalid explicit native readiness was accepted")
	}
}

func TestContextCreateCompositionClonesMethodPolicyAndDeleteResultIsTerminal(t *testing.T) {
	policy := ContextMethodPolicy{
		Default:   ContextMethodExactReview,
		Overrides: []ContextMethodOverride{{Method: "GET", Decision: ContextMethodAllow}},
	}
	composition := ContextCreateComposition{
		NativeReadiness: ContextNativeReadinessEnabled,
		MethodPolicy:    &policy,
	}
	if err := composition.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := composition.Clone()
	clone.MethodPolicy.Overrides[0].Decision = ContextMethodDeny
	if composition.MethodPolicy.Overrides[0].Decision != ContextMethodAllow {
		t.Fatal("Context composition clone aliases the caller's method policy")
	}
	result := ContextDeleteResult{
		Task: TaskContextDelete, ID: "018bcfe5-687b-7000-8000-000000000099", Name: "coding",
		Deleted: true, Cluster: ContextClusterStatusRequiresReconcile,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Deleted = false
	if err := result.Validate(); err == nil {
		t.Fatal("unconfirmed Context deletion was accepted")
	}
}

func TestContextCreateBaseIsCompleteValidatedAndDeepCloned(t *testing.T) {
	literal := "prompt"
	base := ContextCreateBase{
		ID: "018bcfe5-687b-7000-8000-000000000123", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: ContextPolicyModeAdvanced,
		SourceAccess: ContextSourceAccessReadOnly, NativeReadiness: ContextNativeReadinessDisabled,
		MethodPolicy:     ContextMethodPolicy{Default: ContextMethodDeny, Overrides: []ContextMethodOverride{{Method: "GET", Decision: ContextMethodAllow}}},
		RuntimeSelection: StandardRuntimeName,
		ShellEnvironment: DefaultContextShellEnvironmentReport(), GitIdentity: DefaultContextGitIdentityReport(),
	}
	base.ShellEnvironment[2] = ContextShellEnvironmentSetting{Variable: "PS1", Source: ContextShellEnvironmentLiteral, Value: &literal}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := base.Clone()
	*clone.ShellEnvironment[2].Value = "changed"
	clone.MethodPolicy.Overrides[0].Decision = ContextMethodDeny
	if *base.ShellEnvironment[2].Value != literal || base.MethodPolicy.Overrides[0].Decision != ContextMethodAllow {
		t.Fatal("Context creation Base clone aliases copyable settings")
	}
	for name, mutate := range map[string]func(*ContextCreateBase){
		"identity": func(value *ContextCreateBase) { value.ID = "engineering" },
		"revision": func(value *ContextCreateBase) { value.Revision = DefaultContextPolicyRevision()[:20] },
		"runtime":  func(value *ContextCreateBase) { value.RuntimeSelection = "missing@0" },
		"shell":    func(value *ContextCreateBase) { value.ShellEnvironment = value.ShellEnvironment[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base.Clone()
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid Context creation Base was accepted: %+v", candidate)
			}
		})
	}
}

func TestContextManifestValidatesRuntimeImageAndMode(t *testing.T) {
	manifest := validContextManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid Context manifest rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ContextManifest){
		"invalid name":          func(value *ContextManifest) { value.Name = "Project" },
		"invalid image":         func(value *ContextManifest) { value.Image = "--pull=always" },
		"invalid mode":          func(value *ContextManifest) { value.PolicyMode = "manual" },
		"missing source access": func(value *ContextManifest) { value.SourceAccess = "" },
		"invalid source access": func(value *ContextManifest) { value.SourceAccess = "snapshot" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := manifest
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid Context manifest was accepted: %+v", candidate)
			}
		})
	}
}

func TestContextShellEnvironmentIsAllowlistedAndPreservesExplicitEmptyLiteral(t *testing.T) {
	empty := ""
	overrides, err := ApplyContextShellEnvironmentSetting(nil, ContextShellEnvironmentSetting{
		Variable: "PS1", Source: ContextShellEnvironmentLiteral, Value: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	complete, err := CompleteContextShellEnvironment(overrides)
	if err != nil {
		t.Fatal(err)
	}
	if len(complete) != 4 || complete[2].Variable != "PS1" || complete[2].Source != ContextShellEnvironmentLiteral ||
		complete[2].Value == nil || *complete[2].Value != "" {
		t.Fatalf("complete shell environment = %+v", complete)
	}
	overrides, err = ApplyContextShellEnvironmentSetting(overrides, ContextShellEnvironmentSetting{
		Variable: "PS1", Source: ContextShellEnvironmentDefault,
	})
	if err != nil || len(overrides) != 0 {
		t.Fatalf("default shell environment update = %+v, %v", overrides, err)
	}
}

func TestContextShellEnvironmentRejectsUnsafeOrAmbiguousSettings(t *testing.T) {
	value := "value"
	tooLarge := strings.Repeat("x", MaxContextShellValueBytes+1)
	for name, setting := range map[string]ContextShellEnvironmentSetting{
		"unlisted variable":     {Variable: "PATH", Source: ContextShellEnvironmentInherit},
		"inherit with value":    {Variable: "PS1", Source: ContextShellEnvironmentInherit, Value: &value},
		"literal without value": {Variable: "PS1", Source: ContextShellEnvironmentLiteral},
		"oversized literal":     {Variable: "PS1", Source: ContextShellEnvironmentLiteral, Value: &tooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplyContextShellEnvironmentSetting(nil, setting); err == nil {
				t.Fatalf("invalid setting was accepted: %+v", setting)
			}
		})
	}
	duplicate := []ContextShellEnvironmentSetting{
		{Variable: "TERM", Source: ContextShellEnvironmentInherit},
		{Variable: "TERM", Source: ContextShellEnvironmentInherit},
	}
	if _, err := CompleteContextShellEnvironment(duplicate); err == nil {
		t.Fatal("duplicate shell environment setting was accepted")
	}
}

func TestContextShellEnvironmentAppliesDistinctStagedChangesAtomically(t *testing.T) {
	literal := "truecolor"
	changes := []ContextShellEnvironmentSetting{
		{Variable: "COLORTERM", Source: ContextShellEnvironmentLiteral, Value: &literal},
		{Variable: "PS1", Source: ContextShellEnvironmentDefault},
	}
	result, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), changes)
	if err != nil {
		t.Fatalf("ApplyContextShellEnvironmentSettings() error = %v", err)
	}
	if len(result) != 1 || result[0].Variable != "COLORTERM" || result[0].Value == nil || *result[0].Value != literal {
		t.Fatalf("result = %+v", result)
	}
	duplicate := append(append([]ContextShellEnvironmentSetting(nil), changes...), changes[0])
	if invalid, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), duplicate); err == nil || invalid != nil {
		t.Fatalf("duplicate staged result/error = %+v / %v", invalid, err)
	}
	if invalid, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), nil); err == nil || invalid != nil {
		t.Fatalf("empty staged result/error = %+v / %v", invalid, err)
	}
}

func TestContextGitIdentityAcceptsAtomicSourcesAndLiteralPair(t *testing.T) {
	name, email := "Tobari User", "tobari@example.com"
	for _, setting := range []ContextGitIdentitySetting{
		{Source: ContextGitIdentityDefault},
		{Source: ContextGitIdentityInherit},
		{Source: ContextGitIdentityLiteral, Name: &name, Email: &email},
	} {
		if err := setting.Validate(true); err != nil {
			t.Fatalf("valid Git identity rejected: %+v: %v", setting, err)
		}
	}
	if err := (ContextGitIdentitySetting{Source: ContextGitIdentityDefault}).Validate(false); err == nil {
		t.Fatal("default Git identity was accepted as a persisted override")
	}
}

func TestContextGitIdentityRejectsPartialOrUnsafeLiteralValues(t *testing.T) {
	validName, validEmail := "Tobari User", "tobari@example.com"
	empty := ""
	tooLarge := strings.Repeat("x", MaxContextGitIdentityValueBytes+1)
	for name, setting := range map[string]ContextGitIdentitySetting{
		"unknown source":        {Source: "host"},
		"default with name":     {Source: ContextGitIdentityDefault, Name: &validName},
		"inherit with email":    {Source: ContextGitIdentityInherit, Email: &validEmail},
		"literal without name":  {Source: ContextGitIdentityLiteral, Email: &validEmail},
		"literal without email": {Source: ContextGitIdentityLiteral, Name: &validName},
		"empty name":            {Source: ContextGitIdentityLiteral, Name: &empty, Email: &validEmail},
		"oversized email":       {Source: ContextGitIdentityLiteral, Name: &validName, Email: &tooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			if err := setting.Validate(true); err == nil {
				t.Fatalf("invalid Git identity was accepted: %+v", setting)
			}
		})
	}

	for name, unsafe := range map[string]string{
		"nul":                         "a\x00b",
		"carriage return":             "a\rb",
		"line feed":                   "a\nb",
		"C0 control":                  "a\x1fb",
		"C1 control":                  "a\u0085b",
		"Unicode line separator":      "a\u2028b",
		"Unicode paragraph separator": "a\u2029b",
		"format control":              "a\u200db",
		"invalid UTF-8":               string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			setting := ContextGitIdentitySetting{
				Source: ContextGitIdentityLiteral, Name: &validName, Email: &unsafe,
			}
			if err := setting.Validate(true); err == nil {
				t.Fatalf("unsafe Git identity was accepted: %q", unsafe)
			}
		})
	}
}

func TestContextManifestPersistsOnlyNonDefaultGitIdentityOverrides(t *testing.T) {
	manifest := validContextManifest()
	defaultSetting := DefaultContextGitIdentityReport()
	manifest.GitIdentity = &defaultSetting
	if err := manifest.Validate(); err == nil {
		t.Fatal("current Context manifest persisted a default Git identity setting")
	}

	manifest.GitIdentity = &ContextGitIdentitySetting{Source: ContextGitIdentityInherit}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("inherited Git identity override rejected: %v", err)
	}
	manifest.SchemaVersion = ContextSchemaVersion + 1
	if err := manifest.Validate(); err == nil {
		t.Fatal("unsupported Context manifest schema accepted")
	}
}

func TestContextRuntimeRecipeValidatesFixedRecipeAndDigests(t *testing.T) {
	recipe := ContextRuntimeRecipe{
		Kind:          ContextRuntimeKindDockerfile,
		File:          ContextRuntimeRecipeFile,
		BaseReference: OfficialRuntimeBase,
		SourceDigest:  "sha256:" + strings.Repeat("a", 64),
		LastBuild: &ContextRuntimeBuild{
			Image:        "tobari-context-project-tools:abcdef123456",
			ImageDigest:  "sha256:" + strings.Repeat("b", 64),
			SourceDigest: "sha256:" + strings.Repeat("a", 64),
		},
	}
	if err := recipe.Validate(); err != nil {
		t.Fatalf("valid runtime recipe rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ContextRuntimeRecipe){
		"wrong kind":     func(value *ContextRuntimeRecipe) { value.Kind = ContextRuntimeKindOfficial },
		"wrong file":     func(value *ContextRuntimeRecipe) { value.File = "runtime/custom.Dockerfile" },
		"invalid base":   func(value *ContextRuntimeRecipe) { value.BaseReference = "builtin" },
		"invalid digest": func(value *ContextRuntimeRecipe) { value.SourceDigest = "sha256:short" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := recipe
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid runtime recipe was accepted: %+v", candidate)
			}
		})
	}
}

func TestContextReportAcceptsRuntimeTasksAndStatuses(t *testing.T) {
	report := ContextRuntimeReport{
		Kind: ContextRuntimeKindDockerfile, Status: ContextRuntimeStatusPendingBuild,
		Dockerfile:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "runtime", "Dockerfile"),
		BaseReference: OfficialRuntimeBase,
		SourceDigest:  "sha256:" + strings.Repeat("a", 64),
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("valid runtime report rejected: %v", err)
	}
	manifest := validContextManifest()
	manifest.Runtime = &ContextRuntimeRecipe{Kind: ContextRuntimeKindDockerfile, File: ContextRuntimeRecipeFile, BaseReference: OfficialRuntimeBase}
	contextReport := ContextReport{
		Task: TaskRuntimeBuild, ContextState: ContextObservationPersisted, ID: manifest.ID, Name: manifest.Name, Active: true,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		SourceAccess:   manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		Cluster: ContextClusterStatusNotApplicable,
		Stores: ContextStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
		},
		Runtime:          report,
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Authentication:   ContextAuthentication{BrokerState: ContextAuthBrokerNotApplicable},
	}
	if err := contextReport.Validate(); err != nil {
		t.Fatalf("valid runtime Context report rejected: %v", err)
	}
}

func TestContextReportAcceptsConfigurationTasksAndRequiresCompleteGitIdentity(t *testing.T) {
	manifest := validContextManifest()
	base := ContextReport{
		ContextState: ContextObservationPersisted, ID: manifest.ID, Name: manifest.Name, AgentProfile: manifest.AgentProfile,
		Image: manifest.Image, PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Stores: ContextStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
		},
		Runtime:        ContextRuntimeReport{Kind: ContextRuntimeKindOfficial, Status: ContextRuntimeStatusOfficial},
		Cluster:        ContextClusterStatusNotApplicable,
		Authentication: ContextAuthentication{BrokerState: ContextAuthBrokerNotApplicable},
	}
	for _, task := range []string{TaskConfigShell, TaskConfigGit} {
		report := base
		report.Task = task
		if err := report.Validate(); err != nil {
			t.Fatalf("configuration task %q rejected: %v", task, err)
		}
	}
	base.Task = TaskConfigGit
	base.GitIdentity = ContextGitIdentitySetting{}
	if err := base.Validate(); err == nil {
		t.Fatal("Context report without an explicit Git identity source was accepted")
	}
}

func mustCompleteContextShellEnvironment(t *testing.T, overrides []ContextShellEnvironmentSetting) []ContextShellEnvironmentSetting {
	t.Helper()
	result, err := CompleteContextShellEnvironment(overrides)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestContextClusterStatusValidatesKnownOutcomes(t *testing.T) {
	for _, status := range []ContextClusterStatus{
		ContextClusterStatusNotApplicable, ContextClusterStatusNotConfigured,
		ContextClusterStatusNotRunning, ContextClusterStatusAlreadyReady,
		ContextClusterStatusReconciled,
	} {
		if err := status.Validate(); err != nil {
			t.Fatalf("status %q rejected: %v", status, err)
		}
	}
	if err := ContextClusterStatus("failed").Validate(); err == nil {
		t.Fatal("unknown Context cluster status was accepted")
	}
}

func TestContextListRequiresOneMatchingActiveItem(t *testing.T) {
	items := []ContextSummary{
		{ID: "018bcfe5-687b-7000-8000-000000000000", Name: "default", ContextState: ContextObservationPersisted, Active: true, AgentProfile: DefaultProfile, Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided, SourceAccess: ContextSourceAccessReadWrite, PolicyRevision: DefaultContextPolicyRevision(), MethodPolicy: ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}}, RuntimeStatus: ContextRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1"},
		{ID: "018bcfe5-687b-7000-8000-000000000001", Name: "project-tools", ContextState: ContextObservationPersisted, AgentProfile: DefaultProfile, Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeAdvanced, SourceAccess: ContextSourceAccessReadOnly, PolicyRevision: DefaultContextPolicyRevision(), MethodPolicy: ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}}, RuntimeStatus: ContextRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1"},
	}
	result := ContextListResult{Task: TaskContextList, ContextState: ContextObservationPersisted, Active: "default", Items: items}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid Context list rejected: %v", err)
	}

	items[0].Active = false
	if err := (ContextListResult{Task: TaskContextList, ContextState: ContextObservationPersisted, Active: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Context list without an active item was accepted")
	}
	items[0].Active = true
	items[1].Active = true
	if err := (ContextListResult{Task: TaskContextList, ContextState: ContextObservationPersisted, Active: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Context list with two active items was accepted")
	}
}

func TestContextListAllowsOnlyExplicitSyntheticDefaultWithoutAuthority(t *testing.T) {
	t.Parallel()
	result := ContextListResult{
		Task: TaskContextList, ContextState: ContextObservationSyntheticDefault,
		Active: DefaultContextName, Items: []ContextSummary{},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("synthetic Context list = %v", err)
	}
	result.Items = []ContextSummary{{
		ID: "018bcfe5-687b-7000-8000-000000000000", Name: DefaultContextName,
		ContextState: ContextObservationPersisted, Active: true, AgentProfile: DefaultProfile,
		Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided,
	}}
	if err := result.Validate(); err == nil {
		t.Fatal("synthetic Context list accepted a configured item")
	}
}

func TestContextListRequiresTopLevelStateToMatchActiveItem(t *testing.T) {
	t.Parallel()
	result := ContextListResult{
		Task: TaskContextList, ContextState: ContextObservationSyntheticDefault,
		Active: DefaultContextName,
		Items: []ContextSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000000", Name: DefaultContextName,
			ContextState: ContextObservationPersisted, Active: true, AgentProfile: DefaultProfile,
			Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided, SourceAccess: ContextSourceAccessReadWrite,
		}},
	}
	if err := result.Validate(); err == nil {
		t.Fatal("Context list accepted a top-level state different from its active item")
	}
}

func TestSyntheticContextReportCannotClaimAuthorityOrStores(t *testing.T) {
	t.Parallel()
	report := ContextReport{
		Task: TaskContextShow, ContextState: ContextObservationSyntheticDefault,
		Name: DefaultContextName, Active: true, AgentProfile: DefaultProfile,
		Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided, SourceAccess: ContextSourceAccessReadWrite,
		MethodPolicy:     ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		ShellEnvironment: DefaultContextShellEnvironmentReport(),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Runtime:          ContextRuntimeReport{Kind: ContextRuntimeKindOfficial, Status: ContextRuntimeStatusOfficial, BaseReference: OfficialRuntimeBase},
		Cluster:          ContextClusterStatusNotApplicable,
		Authentication:   ContextAuthentication{BrokerState: ContextAuthBrokerUnavailable, Providers: []ContextAuthProvider{}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("synthetic Context report = %v", err)
	}
	report.ID = "018bcfe5-687b-7000-8000-000000000000"
	if err := report.Validate(); err == nil {
		t.Fatal("synthetic Context report accepted an authority ID")
	}
}

func TestContextStorePathsRequireCanonicalAbsolutePaths(t *testing.T) {
	paths := ContextStorePaths{
		PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
	}
	if err := paths.Validate(); err != nil {
		t.Fatalf("valid Context stores rejected: %v", err)
	}
	paths.PolicyDirectory = "relative/policy"
	if err := paths.Validate(); err == nil {
		t.Fatal("relative Context store path was accepted")
	}
}
