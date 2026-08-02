package tobari

import (
	"path/filepath"
	"strings"
	"testing"
)

func validContextManifest() ContextManifest {
	return ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		Name:          "project-tools",
		AgentProfile:  DefaultProfile,
		Image:         "tobari-runtime:local",
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
		Task: TaskRuntimeBuild, Name: manifest.Name, Active: true,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		Stores: ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", "default", "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", "default", "credentials"),
		},
		Runtime: report,
	}
	if err := contextReport.Validate(); err != nil {
		t.Fatalf("valid runtime Context report rejected: %v", err)
	}
}

func TestContextListRequiresOneMatchingActiveItem(t *testing.T) {
	items := []ContextSummary{
		{Name: "default", Active: true, AgentProfile: DefaultProfile, Image: BuiltinImageSelector, PolicyMode: ContextPolicyModeGuided},
		{Name: "project-tools", AgentProfile: DefaultProfile, Image: "tobari-runtime:local", PolicyMode: ContextPolicyModeAdvanced},
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
