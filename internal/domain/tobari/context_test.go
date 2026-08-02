package tobari

import (
	"path/filepath"
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
