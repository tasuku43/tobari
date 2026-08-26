package tobari

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	InstallationMigrationPlanSchemaVersion = 1
	InstallationMigrationPlanReferenceKind = "installation-migration-plan"
	installationMigrationPlanRefPrefix     = "implan1_"
)

type InstallationMigrationPlan struct {
	SchemaVersion       int            `json:"schema_version"`
	PlanRef             string         `json:"plan_ref"`
	SourceDigest        SemanticDigest `json:"source_digest"`
	SourceGeneration    uint64         `json:"source_generation"`
	SourceRevision      SemanticDigest `json:"source_revision"`
	RuntimeSourceDigest SemanticDigest `json:"runtime_source_digest"`
	TemplateCount       int            `json:"template_count"`
	ContextCount        int            `json:"context_count"`
	PolicyMemoryCount   int            `json:"policy_memory_count"`
	WorkspaceCount      int            `json:"workspace_count"`
	TargetGeneration    uint64         `json:"target_generation"`
}

type installationMigrationPlanAuthority InstallationMigrationPlan

func NewInstallationMigrationPlan(sourceDigest, runtimeSourceDigest SemanticDigest, collection WorkspaceAuthorityCollection) (InstallationMigrationPlan, error) {
	if sourceDigest.Validate() != nil || runtimeSourceDigest.Validate() != nil || collection.Validate() != nil {
		return InstallationMigrationPlan{}, fmt.Errorf("installation migration source is invalid")
	}
	plan := InstallationMigrationPlan{SchemaVersion: InstallationMigrationPlanSchemaVersion, SourceDigest: sourceDigest, RuntimeSourceDigest: runtimeSourceDigest, SourceGeneration: collection.Generation, SourceRevision: collection.Revision, TemplateCount: len(collection.Templates), ContextCount: len(collection.Contexts), PolicyMemoryCount: len(collection.Contexts), WorkspaceCount: len(collection.Workspaces), TargetGeneration: collection.Generation}
	digest, err := semanticIdentity(installationMigrationPlanAuthority(plan))
	if err != nil {
		return InstallationMigrationPlan{}, err
	}
	plan.PlanRef = installationMigrationPlanRefPrefix + strings.TrimPrefix(string(digest), "sha256:")
	return plan, plan.Validate()
}

func ParseInstallationMigrationPlanRef(value string) error {
	if !strings.HasPrefix(value, installationMigrationPlanRefPrefix) || len(strings.TrimPrefix(value, installationMigrationPlanRefPrefix)) != 64 {
		return fmt.Errorf("installation migration plan reference is invalid")
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, installationMigrationPlanRefPrefix))
	return err
}

func (p InstallationMigrationPlan) Validate() error {
	if p.SchemaVersion != InstallationMigrationPlanSchemaVersion || ParseInstallationMigrationPlanRef(p.PlanRef) != nil || p.SourceDigest.Validate() != nil || p.RuntimeSourceDigest.Validate() != nil || p.SourceRevision.Validate() != nil || p.SourceGeneration == 0 || p.TargetGeneration != p.SourceGeneration || p.TemplateCount < 0 || p.ContextCount < 0 || p.PolicyMemoryCount != p.ContextCount || p.WorkspaceCount < 0 {
		return fmt.Errorf("installation migration plan is invalid")
	}
	copy := p
	copy.PlanRef = ""
	digest, err := semanticIdentity(installationMigrationPlanAuthority(copy))
	if err != nil {
		return err
	}
	want := installationMigrationPlanRefPrefix + strings.TrimPrefix(string(digest), "sha256:")
	if p.PlanRef != want {
		return fmt.Errorf("installation migration plan reference does not match authority")
	}
	return nil
}

type InstallationMigrationResult struct {
	SchemaVersion    int            `json:"schema_version"`
	PlanRef          string         `json:"plan_ref"`
	ActiveGeneration uint64         `json:"active_generation"`
	ActiveRevision   SemanticDigest `json:"active_revision"`
	Changed          bool           `json:"changed"`
}

func (r InstallationMigrationResult) Validate() error {
	if r.SchemaVersion != 1 || ParseInstallationMigrationPlanRef(r.PlanRef) != nil || r.ActiveGeneration == 0 || r.ActiveRevision.Validate() != nil || !r.Changed {
		return fmt.Errorf("installation migration result is invalid")
	}
	return nil
}
