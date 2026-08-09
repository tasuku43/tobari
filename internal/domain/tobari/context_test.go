package tobari

import (
	"path/filepath"
	"strings"
	"testing"
)

func validContextManifest() ContextManifest {
	return ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000",
		Name:          "project-tools",
		AgentProfile:  DefaultProfile,
		Image:         OfficialRuntimeBase,
		PolicyMode:    ContextPolicyModeAdvanced,
	}
}

func TestContextManifestValidatesRuntimeImageAndMode(t *testing.T) {
	manifest := validContextManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid Context manifest rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ContextManifest){
		"invalid name":  func(value *ContextManifest) { value.Name = "Project" },
		"invalid image": func(value *ContextManifest) { value.Image = "--pull=always" },
		"invalid mode":  func(value *ContextManifest) { value.PolicyMode = "manual" },
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
	manifest.SchemaVersion = LegacyContextSchemaVersion4
	if err := manifest.Validate(); err == nil {
		t.Fatal("schema-4 Context manifest accepted a Git identity field")
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
		Task: TaskRuntimeBuild, ID: manifest.ID, Name: manifest.Name, Active: true,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		Cluster: ContextClusterStatusNotApplicable,
		Stores: ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials"),
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
		ID: manifest.ID, Name: manifest.Name, AgentProfile: manifest.AgentProfile,
		Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		ShellEnvironment: mustCompleteContextShellEnvironment(t, nil),
		GitIdentity:      DefaultContextGitIdentityReport(),
		Stores: ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials"),
		},
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
		{ID: "018bcfe5-687b-7000-8000-000000000000", Name: "default", Active: true, AgentProfile: DefaultProfile, Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeGuided},
		{ID: "018bcfe5-687b-7000-8000-000000000001", Name: "project-tools", AgentProfile: DefaultProfile, Image: OfficialRuntimeBase, PolicyMode: ContextPolicyModeAdvanced},
	}
	result := ContextListResult{Task: TaskContextList, Active: "default", Items: items}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid Context list rejected: %v", err)
	}

	items[0].Active = false
	if err := (ContextListResult{Task: TaskContextList, Active: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Context list without an active item was accepted")
	}
	items[0].Active = true
	items[1].Active = true
	if err := (ContextListResult{Task: TaskContextList, Active: "default", Items: items}).Validate(); err == nil {
		t.Fatal("Context list with two active items was accepted")
	}
}

func TestContextStorePathsRequireCanonicalAbsolutePaths(t *testing.T) {
	paths := ContextStorePaths{
		PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
		CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials.json"),
		CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials"),
	}
	if err := paths.Validate(); err != nil {
		t.Fatalf("valid Context stores rejected: %v", err)
	}
	paths.PolicyDirectory = "relative/policy"
	if err := paths.Validate(); err == nil {
		t.Fatal("relative Context store path was accepted")
	}
}
