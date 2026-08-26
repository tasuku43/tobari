package tobari

import (
	"path/filepath"
	"strings"
	"testing"
)

const testRuntimeImage = "tobari-runtime:test"

func validContextManifest() WorkspaceManifest {
	return WorkspaceManifest{
		SchemaVersion:  WorkspaceManifestSchemaVersion,
		ID:             "018bcfe5-687b-7000-8000-000000000000",
		Name:           "project-tools",
		AgentProfile:   DefaultProfile,
		Image:          BuiltinImageSelector,
		PolicyMode:     ManifestPolicyModeAdvanced,
		SourceAccess:   ManifestSourceAccessReadWrite,
		PolicyRevision: DefaultContextPolicyRevision(),
	}
}

func TestContextNativeReadinessCompatibility(t *testing.T) {
	for _, test := range []struct {
		name  string
		value ManifestNativeReadiness
		want  ManifestNativeReadiness
	}{
		{"enabled", ManifestNativeReadinessEnabled, ManifestNativeReadinessEnabled},
		{"disabled", ManifestNativeReadinessDisabled, ManifestNativeReadinessDisabled},
		{"omitted", "", ManifestNativeReadinessEnabled},
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
	policy := ManifestMethodPolicy{
		Default:   ManifestMethodExactReview,
		Overrides: []ManifestMethodOverride{{Method: "GET", Decision: ManifestMethodAllow}},
	}
	composition := ManifestCreateComposition{
		NativeReadiness: ManifestNativeReadinessEnabled,
		MethodPolicy:    &policy,
	}
	if err := composition.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := composition.Clone()
	clone.MethodPolicy.Overrides[0].Decision = ManifestMethodDeny
	if composition.MethodPolicy.Overrides[0].Decision != ManifestMethodAllow {
		t.Fatal("Workspace Manifest composition clone aliases the caller's method policy")
	}
	result := ManifestDeleteResult{
		Task: TaskManifestDelete, ID: "018bcfe5-687b-7000-8000-000000000099", Name: "coding",
		Deleted: true, Cluster: ManifestClusterStatusRequiresReconcile,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Deleted = false
	if err := result.Validate(); err == nil {
		t.Fatal("unconfirmed Workspace Manifest deletion was accepted")
	}
}

func TestContextCreateBaseIsCompleteValidatedAndDeepCloned(t *testing.T) {
	literal := "prompt"
	base := ManifestCopySnapshot{
		ID: "018bcfe5-687b-7000-8000-000000000123", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: ManifestPolicyModeAdvanced,
		Desired:      WorkspaceManifestRevision{Generation: 1, Revision: "sha256:" + strings.Repeat("a", 64), BoundaryRevision: "sha256:" + strings.Repeat("b", 64), ClusterProjectionRevision: "sha256:" + strings.Repeat("c", 64), EntryRevision: "sha256:" + strings.Repeat("d", 64), SessionDefaultsRevision: "sha256:" + strings.Repeat("e", 64), CreationDefaultsRevision: "sha256:" + strings.Repeat("f", 64)},
		SourceAccess: ManifestSourceAccessReadOnly, NativeReadiness: ManifestNativeReadinessDisabled,
		MethodPolicy:     ManifestMethodPolicy{Default: ManifestMethodDeny, Overrides: []ManifestMethodOverride{{Method: "GET", Decision: ManifestMethodAllow}}},
		RuntimeSelection: StandardRuntimeName + "@1",
		RuntimeBinding:   RuntimeBinding{RuntimeID: StandardRuntimeID, Name: StandardRuntimeName, Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1, Image: testRuntimeImage},
		ShellEnvironment: DefaultContextShellEnvironmentReport(), GitIdentity: DefaultContextGitIdentityReport(),
	}
	base.ShellEnvironment[2] = ManifestShellEnvironmentSetting{Variable: "PS1", Source: ManifestShellEnvironmentLiteral, Value: &literal}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := base.Clone()
	*clone.ShellEnvironment[2].Value = "changed"
	clone.MethodPolicy.Overrides[0].Decision = ManifestMethodDeny
	if *base.ShellEnvironment[2].Value != literal || base.MethodPolicy.Overrides[0].Decision != ManifestMethodAllow {
		t.Fatal("Workspace Manifest creation Base clone aliases copyable settings")
	}
	for name, mutate := range map[string]func(*ManifestCopySnapshot){
		"identity": func(value *ManifestCopySnapshot) { value.ID = "engineering" },
		"revision": func(value *ManifestCopySnapshot) { value.Revision = DefaultContextPolicyRevision()[:20] },
		"runtime":  func(value *ManifestCopySnapshot) { value.RuntimeSelection = "missing@0" },
		"shell":    func(value *ManifestCopySnapshot) { value.ShellEnvironment = value.ShellEnvironment[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base.Clone()
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid Workspace Manifest creation Base was accepted: %+v", candidate)
			}
		})
	}
}

func TestContextManifestValidatesRuntimeImageAndMode(t *testing.T) {
	manifest := validContextManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid Workspace Manifest manifest rejected: %v", err)
	}

	for name, mutate := range map[string]func(*WorkspaceManifest){
		"invalid name":          func(value *WorkspaceManifest) { value.Name = "Project" },
		"invalid image":         func(value *WorkspaceManifest) { value.Image = "--pull=always" },
		"invalid mode":          func(value *WorkspaceManifest) { value.PolicyMode = "manual" },
		"missing source access": func(value *WorkspaceManifest) { value.SourceAccess = "" },
		"invalid source access": func(value *WorkspaceManifest) { value.SourceAccess = "snapshot" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := manifest
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid Workspace Manifest manifest was accepted: %+v", candidate)
			}
		})
	}
}

func TestWorkspaceManifestBindingAllowsBuiltinButRejectsContradictoryPortableImage(t *testing.T) {
	manifest := validContextManifest()
	manifest.RuntimeBinding = &RuntimeBinding{
		RuntimeID: StandardRuntimeID, Name: StandardRuntimeName,
		Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: testRuntimeImage,
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("builtin selector with resolved binding was rejected: %v", err)
	}

	matching := manifest
	matching.Image = testRuntimeImage
	if err := matching.Validate(); err != nil {
		t.Fatalf("matching portable selector with resolved binding was rejected: %v", err)
	}

	mismatch := manifest
	mismatch.Image = "example.com/custom:a"
	if err := mismatch.Validate(); err == nil {
		t.Fatal("contradictory portable selector and Runtime binding were accepted")
	}
}

func TestContextShellEnvironmentIsAllowlistedAndPreservesExplicitEmptyLiteral(t *testing.T) {
	empty := ""
	overrides, err := ApplyContextShellEnvironmentSetting(nil, ManifestShellEnvironmentSetting{
		Variable: "PS1", Source: ManifestShellEnvironmentLiteral, Value: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	complete, err := CompleteContextShellEnvironment(overrides)
	if err != nil {
		t.Fatal(err)
	}
	if len(complete) != 4 || complete[2].Variable != "PS1" || complete[2].Source != ManifestShellEnvironmentLiteral ||
		complete[2].Value == nil || *complete[2].Value != "" {
		t.Fatalf("complete shell environment = %+v", complete)
	}
	overrides, err = ApplyContextShellEnvironmentSetting(overrides, ManifestShellEnvironmentSetting{
		Variable: "PS1", Source: ManifestShellEnvironmentDefault,
	})
	if err != nil || len(overrides) != 0 {
		t.Fatalf("default shell environment update = %+v, %v", overrides, err)
	}
}

func TestContextShellEnvironmentRejectsUnsafeOrAmbiguousSettings(t *testing.T) {
	value := "value"
	tooLarge := strings.Repeat("x", MaxContextShellValueBytes+1)
	for name, setting := range map[string]ManifestShellEnvironmentSetting{
		"unlisted variable":     {Variable: "PATH", Source: ManifestShellEnvironmentInherit},
		"inherit with value":    {Variable: "PS1", Source: ManifestShellEnvironmentInherit, Value: &value},
		"literal without value": {Variable: "PS1", Source: ManifestShellEnvironmentLiteral},
		"oversized literal":     {Variable: "PS1", Source: ManifestShellEnvironmentLiteral, Value: &tooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplyContextShellEnvironmentSetting(nil, setting); err == nil {
				t.Fatalf("invalid setting was accepted: %+v", setting)
			}
		})
	}
	duplicate := []ManifestShellEnvironmentSetting{
		{Variable: "TERM", Source: ManifestShellEnvironmentInherit},
		{Variable: "TERM", Source: ManifestShellEnvironmentInherit},
	}
	if _, err := CompleteContextShellEnvironment(duplicate); err == nil {
		t.Fatal("duplicate shell environment setting was accepted")
	}
}

func TestContextShellEnvironmentAppliesDistinctStagedChangesAtomically(t *testing.T) {
	literal := "truecolor"
	changes := []ManifestShellEnvironmentSetting{
		{Variable: "COLORTERM", Source: ManifestShellEnvironmentLiteral, Value: &literal},
		{Variable: "PS1", Source: ManifestShellEnvironmentDefault},
	}
	result, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), changes)
	if err != nil {
		t.Fatalf("ApplyWorkspace ManifestShellEnvironmentSettings() error = %v", err)
	}
	if len(result) != 1 || result[0].Variable != "COLORTERM" || result[0].Value == nil || *result[0].Value != literal {
		t.Fatalf("result = %+v", result)
	}
	duplicate := append(append([]ManifestShellEnvironmentSetting(nil), changes...), changes[0])
	if invalid, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), duplicate); err == nil || invalid != nil {
		t.Fatalf("duplicate staged result/error = %+v / %v", invalid, err)
	}
	if invalid, err := ApplyContextShellEnvironmentSettings(InitialContextShellEnvironment(), nil); err == nil || invalid != nil {
		t.Fatalf("empty staged result/error = %+v / %v", invalid, err)
	}
}

func TestContextGitIdentityAcceptsAtomicSourcesAndLiteralPair(t *testing.T) {
	name, email := "Tobari User", "tobari@example.com"
	for _, setting := range []ManifestGitIdentitySetting{
		{Source: ManifestGitIdentityDefault},
		{Source: ManifestGitIdentityInherit},
		{Source: ManifestGitIdentityLiteral, Name: &name, Email: &email},
	} {
		if err := setting.Validate(true); err != nil {
			t.Fatalf("valid Git identity rejected: %+v: %v", setting, err)
		}
	}
	if err := (ManifestGitIdentitySetting{Source: ManifestGitIdentityDefault}).Validate(false); err == nil {
		t.Fatal("default Git identity was accepted as a persisted override")
	}
}

func TestContextGitIdentityRejectsPartialOrUnsafeLiteralValues(t *testing.T) {
	validName, validEmail := "Tobari User", "tobari@example.com"
	empty := ""
	tooLarge := strings.Repeat("x", MaxContextGitIdentityValueBytes+1)
	for name, setting := range map[string]ManifestGitIdentitySetting{
		"unknown source":        {Source: "host"},
		"default with name":     {Source: ManifestGitIdentityDefault, Name: &validName},
		"inherit with email":    {Source: ManifestGitIdentityInherit, Email: &validEmail},
		"literal without name":  {Source: ManifestGitIdentityLiteral, Email: &validEmail},
		"literal without email": {Source: ManifestGitIdentityLiteral, Name: &validName},
		"empty name":            {Source: ManifestGitIdentityLiteral, Name: &empty, Email: &validEmail},
		"oversized email":       {Source: ManifestGitIdentityLiteral, Name: &validName, Email: &tooLarge},
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
			setting := ManifestGitIdentitySetting{
				Source: ManifestGitIdentityLiteral, Name: &validName, Email: &unsafe,
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
		t.Fatal("current Workspace Manifest manifest persisted a default Git identity setting")
	}

	manifest.GitIdentity = &ManifestGitIdentitySetting{Source: ManifestGitIdentityInherit}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("inherited Git identity override rejected: %v", err)
	}
	manifest.SchemaVersion = WorkspaceManifestSchemaVersion + 1
	if err := manifest.Validate(); err == nil {
		t.Fatal("unsupported Workspace Manifest manifest schema accepted")
	}
}

func TestContextRuntimeRecipeValidatesFixedRecipeAndDigests(t *testing.T) {
	recipe := ManifestRuntimeRecipe{
		Kind:          ManifestRuntimeKindDockerfile,
		File:          ManifestRuntimeRecipeFile,
		BaseReference: testRuntimeImage,
		SourceDigest:  "sha256:" + strings.Repeat("a", 64),
		LastBuild: &ManifestRuntimeBuild{
			Image:        "tobari-context-project-tools:abcdef123456",
			ImageDigest:  "sha256:" + strings.Repeat("b", 64),
			SourceDigest: "sha256:" + strings.Repeat("a", 64),
		},
	}
	if err := recipe.Validate(); err != nil {
		t.Fatalf("valid runtime recipe rejected: %v", err)
	}
	officialReport := ManifestRuntimeReport{
		Kind: ManifestRuntimeKindOfficial, Status: ManifestRuntimeStatusOfficial,
		BaseReference: testRuntimeImage,
	}
	if err := officialReport.Validate(); err != nil {
		t.Fatalf("valid official runtime report rejected: %v", err)
	}
	officialReport.BaseReference = BuiltinImageSelector
	if err := officialReport.Validate(); err == nil {
		t.Fatal("official runtime report accepted the unresolved builtin selector")
	}

	for name, mutate := range map[string]func(*ManifestRuntimeRecipe){
		"wrong kind":     func(value *ManifestRuntimeRecipe) { value.Kind = ManifestRuntimeKindOfficial },
		"wrong file":     func(value *ManifestRuntimeRecipe) { value.File = "runtime/custom.Dockerfile" },
		"invalid base":   func(value *ManifestRuntimeRecipe) { value.BaseReference = "builtin" },
		"invalid digest": func(value *ManifestRuntimeRecipe) { value.SourceDigest = "sha256:short" },
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
	report := ManifestRuntimeReport{
		Kind: ManifestRuntimeKindDockerfile, Status: ManifestRuntimeStatusPendingBuild,
		Dockerfile:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "runtime", "Dockerfile"),
		BaseReference: testRuntimeImage,
		SourceDigest:  "sha256:" + strings.Repeat("a", 64),
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("valid runtime report rejected: %v", err)
	}
	manifest := validContextManifest()
	manifest.Runtime = &ManifestRuntimeRecipe{Kind: ManifestRuntimeKindDockerfile, File: ManifestRuntimeRecipeFile, BaseReference: testRuntimeImage}
	contextReport := ManifestReport{
		Task: TaskRuntimeBuild, ManifestState: ManifestObservationPersisted, ID: manifest.ID, Name: manifest.Name, Default: true,
		Desired:      testManifestDesiredRevision(),
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		SourceAccess:   manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		Cluster: ManifestClusterStatusNotApplicable,
		Stores: ManifestStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
		},
		Runtime:          report,
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Authentication:   ManifestAuthentication{BrokerState: ManifestAuthBrokerNotApplicable},
	}
	if err := contextReport.Validate(); err != nil {
		t.Fatalf("valid runtime Workspace Manifest report rejected: %v", err)
	}
}

func TestContextReportKeepsStableSelectorSeparateFromResolvedRuntimeMaterial(t *testing.T) {
	manifest := validContextManifest()
	resolved := ManifestRuntimeReport{
		Kind: ManifestRuntimeKindOfficial, Status: ManifestRuntimeStatusOfficial,
		Image: testRuntimeImage, RuntimeID: StandardRuntimeID, Name: StandardRuntimeName,
		Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1,
	}
	report := ManifestReport{
		Task: TaskManifestShow, ManifestState: ManifestObservationPersisted, ID: manifest.ID, Name: manifest.Name,
		Default: true, Desired: testManifestDesiredRevision(), AgentProfile: manifest.AgentProfile,
		Image: BuiltinImageSelector, PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil), GitIdentity: DefaultContextGitIdentityReport(),
		Stores:  ManifestStorePaths{PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy")},
		Runtime: resolved, Cluster: ManifestClusterStatusNotApplicable,
		Authentication: ManifestAuthentication{Mode: ManifestAuthenticationModeNative, Providers: []ManifestAuthProvider{}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("selector/material-separated report rejected: %v", err)
	}
}

func TestManifestReportRejectsContradictoryPortableBindingImage(t *testing.T) {
	manifest := validContextManifest()
	report := ManifestReport{
		Task: TaskManifestShow, ManifestState: ManifestObservationPersisted, ID: manifest.ID, Name: manifest.Name,
		Default: true, Desired: testManifestDesiredRevision(), AgentProfile: manifest.AgentProfile,
		Image: "example.com/custom:a", PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil), GitIdentity: DefaultContextGitIdentityReport(),
		Stores: ManifestStorePaths{PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy")},
		Runtime: ManifestRuntimeReport{
			Kind: ManifestRuntimeKindOfficial, Status: ManifestRuntimeStatusOfficial,
			Image: testRuntimeImage, RuntimeID: StandardRuntimeID, Name: StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1,
		},
		Cluster:        ManifestClusterStatusNotApplicable,
		Authentication: ManifestAuthentication{Mode: ManifestAuthenticationModeNative, Providers: []ManifestAuthProvider{}},
	}
	if err := report.Validate(); err == nil {
		t.Fatal("Manifest report accepted a portable selector that contradicts its Runtime binding")
	}
}

func TestContextReportAcceptsConfigurationTasksAndRequiresCompleteGitIdentity(t *testing.T) {
	manifest := validContextManifest()
	base := ManifestReport{
		ManifestState: ManifestObservationPersisted, ID: manifest.ID, Name: manifest.Name, AgentProfile: manifest.AgentProfile,
		Desired: testManifestDesiredRevision(),
		Image:   manifest.Image, PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		PolicyRevision: manifest.PolicyRevision, MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Stores: ManifestStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
		},
		Runtime:        ManifestRuntimeReport{Kind: ManifestRuntimeKindOfficial, Status: ManifestRuntimeStatusOfficial},
		Cluster:        ManifestClusterStatusNotApplicable,
		Authentication: ManifestAuthentication{BrokerState: ManifestAuthBrokerNotApplicable},
	}
	for _, task := range []string{TaskConfigShell, TaskConfigGit} {
		report := base
		report.Task = task
		if err := report.Validate(); err != nil {
			t.Fatalf("configuration task %q rejected: %v", task, err)
		}
	}
	base.Task = TaskConfigGit
	base.GitIdentity = ManifestGitIdentitySetting{}
	if err := base.Validate(); err == nil {
		t.Fatal("Workspace Manifest report without an explicit Git identity source was accepted")
	}
}

func mustCompleteContextShellEnvironment(t *testing.T, overrides []ManifestShellEnvironmentSetting) []ManifestShellEnvironmentSetting {
	t.Helper()
	result, err := CompleteContextShellEnvironment(overrides)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestContextClusterStatusValidatesKnownOutcomes(t *testing.T) {
	for _, status := range []ManifestClusterStatus{
		ManifestClusterStatusNotApplicable, ManifestClusterStatusNotConfigured,
		ManifestClusterStatusNotRunning, ManifestClusterStatusAlreadyReady,
		ManifestClusterStatusReconciled,
	} {
		if err := status.Validate(); err != nil {
			t.Fatalf("status %q rejected: %v", status, err)
		}
	}
	if err := ManifestClusterStatus("failed").Validate(); err == nil {
		t.Fatal("unknown Workspace Manifest cluster status was accepted")
	}
}

func TestContextListRequiresOneMatchingActiveItem(t *testing.T) {
	items := []ManifestSummary{
		{ID: "018bcfe5-687b-7000-8000-000000000000", Name: "default", ManifestState: ManifestObservationPersisted, Default: true, AgentProfile: DefaultProfile, Image: BuiltinImageSelector, PolicyMode: ManifestPolicyModeGuided, SourceAccess: ManifestSourceAccessReadWrite, PolicyRevision: DefaultContextPolicyRevision(), MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}}, RuntimeStatus: ManifestRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1"},
		{ID: "018bcfe5-687b-7000-8000-000000000001", Name: "project-tools", ManifestState: ManifestObservationPersisted, AgentProfile: DefaultProfile, Image: BuiltinImageSelector, PolicyMode: ManifestPolicyModeAdvanced, SourceAccess: ManifestSourceAccessReadOnly, PolicyRevision: DefaultContextPolicyRevision(), MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}}, RuntimeStatus: ManifestRuntimeStatusOfficial, RuntimeSelection: StandardRuntimeName + "@1"},
	}
	for index := range items {
		items[index].Desired = testManifestDesiredRevision()
	}
	result := ManifestListResult{Task: TaskManifestList, ManifestState: ManifestObservationPersisted, DefaultManifestID: items[0].ID, DefaultManifest: "default", Items: items}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid Workspace Manifest list rejected: %v", err)
	}

	items[0].Default = false
	if err := (ManifestListResult{Task: TaskManifestList, ManifestState: ManifestObservationPersisted, DefaultManifestID: items[0].ID, DefaultManifest: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Workspace Manifest list without an active item was accepted")
	}
	items[0].Default = true
	items[1].Default = true
	if err := (ManifestListResult{Task: TaskManifestList, ManifestState: ManifestObservationPersisted, DefaultManifestID: items[0].ID, DefaultManifest: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Workspace Manifest list with two active items was accepted")
	}
}

func testManifestDesiredRevision() WorkspaceManifestRevision {
	digest := "sha256:" + strings.Repeat("f", 64)
	return WorkspaceManifestRevision{
		Generation: 1, Revision: digest, BoundaryRevision: digest,
		ClusterProjectionRevision: digest, EntryRevision: digest,
		SessionDefaultsRevision: digest, CreationDefaultsRevision: digest,
	}
}

func TestWorkspaceManifestPublicationBindsCanonicalBodyAndGeneration(t *testing.T) {
	manifest := WorkspaceManifest{
		SchemaVersion: WorkspaceManifestSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000", Name: "default",
		AgentProfile: DefaultProfile, Image: BuiltinImageSelector,
		PolicyMode: ManifestPolicyModeGuided, SourceAccess: ManifestSourceAccessReadWrite,
		PolicyRevision:   DefaultContextPolicyRevision(),
		RuntimeBinding:   &RuntimeBinding{RuntimeID: StandardRuntimeID, Name: StandardRuntimeName, Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: testRuntimeImage},
		ShellEnvironment: InitialContextShellEnvironment(),
	}
	published, err := PublishWorkspaceManifest(manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := published.ValidatePublished(); err != nil {
		t.Fatalf("published Manifest = %+v, error = %v", published.Desired, err)
	}
	noOp, err := PublishWorkspaceManifest(published, &published)
	if err != nil || noOp.Desired != published.Desired {
		t.Fatalf("semantic no-op changed correlation generation or digest: before=%+v after=%+v err=%v", published.Desired, noOp.Desired, err)
	}
	value := "xterm-256color"
	updated := published
	updated.ShellEnvironment = []ManifestShellEnvironmentSetting{{Variable: "TERM", Source: ManifestShellEnvironmentLiteral, Value: &value}}
	updated, err = PublishWorkspaceManifest(updated, &published)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Desired.Generation != 2 || updated.Desired.SessionDefaultsRevision == published.Desired.SessionDefaultsRevision {
		t.Fatalf("updated revision = %+v, previous = %+v", updated.Desired, published.Desired)
	}
	if err := updated.ValidatePublished(); err != nil {
		t.Fatalf("updated Manifest = %+v, error = %v", updated.Desired, err)
	}
	reverted := updated
	reverted.ShellEnvironment = published.ShellEnvironment
	reverted, err = PublishWorkspaceManifest(reverted, &updated)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.Desired.Generation != 3 || reverted.Desired.Revision != published.Desired.Revision {
		t.Fatalf("A-B-A history must retain semantic authority while advancing correlation: A=%+v B=%+v A2=%+v", published.Desired, updated.Desired, reverted.Desired)
	}
}

func TestManifestListAllowsAnAbsentDefaultWithoutAuthority(t *testing.T) {
	t.Parallel()
	result := ManifestListResult{
		Task: TaskManifestList, ManifestState: ManifestObservationAbsent, Items: []ManifestSummary{},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("synthetic Workspace Manifest list = %v", err)
	}
	result.Items = []ManifestSummary{{
		ID: "018bcfe5-687b-7000-8000-000000000000", Name: DefaultManifestName,
		ManifestState: ManifestObservationPersisted, Default: true, AgentProfile: DefaultProfile,
		Image: BuiltinImageSelector, PolicyMode: ManifestPolicyModeGuided,
	}}
	if err := result.Validate(); err == nil {
		t.Fatal("synthetic Workspace Manifest list accepted a configured item")
	}
}

func TestContextListRequiresTopLevelStateToMatchActiveItem(t *testing.T) {
	t.Parallel()
	result := ManifestListResult{
		Task: TaskManifestList, ManifestState: ManifestObservationAbsent,
		Items: []ManifestSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000000", Name: DefaultManifestName,
			ManifestState: ManifestObservationPersisted, Default: true, AgentProfile: DefaultProfile,
			Image: BuiltinImageSelector, PolicyMode: ManifestPolicyModeGuided, SourceAccess: ManifestSourceAccessReadWrite,
		}},
	}
	if err := result.Validate(); err == nil {
		t.Fatal("Workspace Manifest list accepted a top-level state different from its active item")
	}
}

func TestAbsentManifestReportCannotClaimAuthorityOrStores(t *testing.T) {
	t.Parallel()
	report := ManifestReport{
		Task: TaskManifestShow, ManifestState: ManifestObservationAbsent,
		Name: DefaultManifestName, Default: true, AgentProfile: DefaultProfile,
		Image: BuiltinImageSelector, PolicyMode: ManifestPolicyModeGuided, SourceAccess: ManifestSourceAccessReadWrite,
		MethodPolicy:     ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}},
		ShellEnvironment: DefaultContextShellEnvironmentReport(),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Runtime:          ManifestRuntimeReport{Kind: ManifestRuntimeKindOfficial, Status: ManifestRuntimeStatusOfficial, BaseReference: testRuntimeImage},
		Cluster:          ManifestClusterStatusNotApplicable,
		Authentication:   ManifestAuthentication{BrokerState: ManifestAuthBrokerUnavailable, Providers: []ManifestAuthProvider{}},
	}
	if err := report.Validate(); err == nil {
		t.Fatal("absent Manifest report claimed authority")
	}
	report.ID = "018bcfe5-687b-7000-8000-000000000000"
	if err := report.Validate(); err == nil {
		t.Fatal("synthetic Workspace Manifest report accepted an authority ID")
	}
}

func TestContextStorePathsRequireCanonicalAbsolutePaths(t *testing.T) {
	paths := ManifestStorePaths{
		PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
	}
	if err := paths.Validate(); err != nil {
		t.Fatalf("valid Workspace Manifest stores rejected: %v", err)
	}
	paths.PolicyDirectory = "relative/policy"
	if err := paths.Validate(); err == nil {
		t.Fatal("relative Workspace Manifest store path was accepted")
	}
}
