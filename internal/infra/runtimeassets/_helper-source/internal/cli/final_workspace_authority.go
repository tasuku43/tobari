package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalTemplateProjection struct {
	Lifecycle          string                           `json:"lifecycle"`
	TemplateRef        string                           `json:"template_ref,omitempty"`
	CurrentRevisionRef string                           `json:"current_revision_ref,omitempty"`
	TemplateID         string                           `json:"workspace_template_id"`
	Name               string                           `json:"name"`
	Generation         uint64                           `json:"generation,omitempty"`
	Revision           string                           `json:"revision,omitempty"`
	RuntimeID          string                           `json:"runtime_id,omitempty"`
	RuntimeRevision    string                           `json:"runtime_revision,omitempty"`
	SourceAccess       string                           `json:"source_access,omitempty"`
	GraphQLEndpoints   []tobari.ManifestPolicyExactRule `json:"graphql_endpoints,omitempty"`
	PolicySliceDigest  string                           `json:"policy_slice_digest,omitempty"`
	EntrySliceDigest   string                           `json:"entry_slice_digest,omitempty"`
	SourcePath         string                           `json:"source_path,omitempty"`
	SourceState        string                           `json:"source_state,omitempty"`
	SourceRevision     *string                          `json:"source_revision,omitempty"`
	ActiveRevision     string                           `json:"active_revision,omitempty"`
}

type finalTemplateDraftProjection struct {
	Lifecycle        string                           `json:"lifecycle"`
	TemplateRef      string                           `json:"template_ref"`
	TemplateID       string                           `json:"workspace_template_id"`
	Name             string                           `json:"name"`
	SourcePath       string                           `json:"source_path"`
	SourceState      string                           `json:"source_state"`
	SourceRevision   *string                          `json:"source_revision"`
	SourceAccess     string                           `json:"source_access"`
	GraphQLEndpoints []tobari.ManifestPolicyExactRule `json:"graphql_endpoints"`
}

type finalContextProjection struct {
	Lifecycle                        string                        `json:"lifecycle"`
	Current                          *bool                         `json:"current,omitempty"`
	ContextRef                       string                        `json:"context_ref,omitempty"`
	ContextID                        string                        `json:"context_id"`
	TemplateID                       string                        `json:"workspace_template_id"`
	TemplateName                     string                        `json:"template_name"`
	DesiredTemplateGeneration        uint64                        `json:"desired_template_generation"`
	DesiredTemplateRevision          string                        `json:"desired_template_revision"`
	DesiredTemplatePolicySliceDigest string                        `json:"desired_template_policy_slice_digest"`
	ActiveTemplatePolicySliceDigest  *string                       `json:"active_template_policy_slice_digest"`
	CurrentPolicyMemoryRevision      string                        `json:"current_policy_memory_revision"`
	ActivePolicyMemoryRevision       *string                       `json:"active_policy_memory_revision"`
	WorkspaceID                      string                        `json:"workspace_id,omitempty"`
	AppliedEntry                     *tobari.WorkspaceAppliedEntry `json:"applied_entry"`
	SourcePath                       string                        `json:"source_path,omitempty"`
	SourceState                      string                        `json:"source_state,omitempty"`
	SourceRevision                   *string                       `json:"source_revision,omitempty"`
	ActiveRevision                   string                        `json:"active_revision,omitempty"`
}

type finalContextDraftProjection struct {
	Lifecycle      string `json:"lifecycle"`
	ContextRef     string `json:"context_ref"`
	ContextID      string `json:"context_id"`
	TemplateID     string `json:"workspace_template_id"`
	SourcePath     string `json:"source_path"`
	SourceState    string `json:"source_state"`
	SourceRevision string `json:"source_revision"`
}

type finalContextPlanProjection struct {
	PlanRef              string `json:"plan_ref"`
	ContextRef           string `json:"context_ref"`
	SourceFingerprint    string `json:"source_fingerprint"`
	TemplateRef          string `json:"template_ref"`
	TemplateRevision     string `json:"template_revision"`
	DuplicateBinding     bool   `json:"duplicate_binding"`
	NoOp                 bool   `json:"no_op"`
	SourceAccess         string `json:"source_access"`
	RuntimeID            string `json:"runtime_id"`
	RuntimeRevision      string `json:"runtime_revision"`
	BoundaryFingerprint  string `json:"boundary_fingerprint"`
	PolicySliceDigest    string `json:"policy_slice_digest"`
	NewPolicyMemoryOwner string `json:"new_policy_memory_owner"`
}

type finalTemplateChangeContextProjection struct {
	ContextRef           string                `json:"context_ref"`
	PolicyMemoryRevision tobari.SemanticDigest `json:"policy_memory_revision"`
	WorkspaceRef         string                `json:"workspace_ref,omitempty"`
}

type finalTemplateChangePlanProjection struct {
	PlanRef                string                                 `json:"plan_ref"`
	TemplateRef            string                                 `json:"template_ref"`
	ActiveRevision         *tobari.SemanticDigest                 `json:"active_revision,omitempty"`
	ActiveMetadataRevision *tobari.SemanticDigest                 `json:"active_metadata_revision,omitempty"`
	BaseRevision           *tobari.SemanticDigest                 `json:"base_revision,omitempty"`
	SourceFingerprint      string                                 `json:"source_fingerprint"`
	SourceRevision         tobari.SemanticDigest                  `json:"source_revision"`
	Impact                 tobari.WorkspaceTemplateChangeImpact   `json:"impact"`
	Diff                   tobari.WorkspaceTemplateChangeDiff     `json:"diff"`
	Contexts               []finalTemplateChangeContextProjection `json:"contexts"`
	AffectedContextCount   int                                    `json:"affected_context_count"`
	RunningWorkspaceCount  int                                    `json:"running_workspace_count"`
}

func finalTemplateChangePlanFrom(plan tobari.WorkspaceTemplateChangePlan) finalTemplateChangePlanProjection {
	contexts := make([]finalTemplateChangeContextProjection, len(plan.Contexts))
	for index, item := range plan.Contexts {
		contexts[index] = finalTemplateChangeContextProjection{
			ContextRef: item.ContextRef, PolicyMemoryRevision: item.PolicyMemoryRevision, WorkspaceRef: item.WorkspaceRef,
		}
	}
	return finalTemplateChangePlanProjection{
		PlanRef: plan.PlanRef, TemplateRef: plan.TemplateRef, ActiveRevision: plan.ActiveRevision,
		ActiveMetadataRevision: plan.ActiveMetadataRevision, BaseRevision: plan.BaseRevision,
		SourceFingerprint: plan.SourceFingerprint, SourceRevision: plan.SourceRevision, Impact: plan.Impact,
		Diff: plan.Diff, Contexts: contexts, AffectedContextCount: plan.AffectedContextCount,
		RunningWorkspaceCount: plan.RunningWorkspaceCount,
	}
}

type finalTemplatePolicyMigrationPlanProjection struct {
	PlanRef           string                `json:"plan_ref"`
	TemplateRef       string                `json:"template_ref"`
	ActiveRevision    tobari.SemanticDigest `json:"active_revision"`
	SourceFingerprint string                `json:"source_fingerprint"`
	TargetFingerprint string                `json:"target_fingerprint"`
	SourceSchema      string                `json:"source_schema"`
	TargetSchema      string                `json:"target_schema"`
}

func finalTemplatePolicyMigrationPlanFrom(plan tobari.WorkspaceTemplatePolicyMigrationPlan) finalTemplatePolicyMigrationPlanProjection {
	return finalTemplatePolicyMigrationPlanProjection{
		PlanRef: plan.PlanRef, TemplateRef: plan.TemplateRef, ActiveRevision: plan.ActiveRevision,
		SourceFingerprint: plan.SourceFingerprint, TargetFingerprint: plan.TargetFingerprint,
		SourceSchema: plan.SourceSchema, TargetSchema: plan.TargetSchema,
	}
}

type finalWorkspaceProjection struct {
	WorkspaceRef         string `json:"workspace_ref,omitempty"`
	WorkspaceID          string `json:"workspace_id"`
	ContextID            string `json:"context_id"`
	TemplateID           string `json:"workspace_template_id"`
	TemplateName         string `json:"template_name"`
	ProjectRoot          string `json:"project_root"`
	WorkspaceHome        string `json:"workspace_home"`
	Applied              bool   `json:"applied"`
	AppliedEntryRevision string `json:"applied_entry_revision,omitempty"`
}

func projectTemplate(view interface{ Validate() error }) error { return view.Validate() }

func finalTemplateFrom(view workspaceauthoritycmd.TemplateView, exposeRevisionRef bool) finalTemplateProjection {
	revision := view.Template.Current
	result := finalTemplateProjection{Lifecycle: "active", TemplateRef: view.TemplateRef, TemplateID: string(view.Template.ID), Name: view.Template.Name, Generation: revision.Generation, Revision: string(revision.Revision), RuntimeID: revision.Slices.RuntimeID, RuntimeRevision: string(revision.Slices.RuntimeRevision), SourceAccess: string(revision.Body.Boundary.SourceAccess), GraphQLEndpoints: finalTemplateGraphQLEndpoints(revision.Body.Policy), PolicySliceDigest: string(revision.Slices.PolicySliceDigest), EntrySliceDigest: string(revision.Slices.EntrySliceDigest), ActiveRevision: string(revision.Revision)}
	if view.Source != nil {
		result.SourcePath = view.Source.Path
		result.SourceState = string(view.Source.State)
		if view.Source.SourceRevision != nil {
			value := string(*view.Source.SourceRevision)
			result.SourceRevision = &value
		}
	}
	if exposeRevisionRef {
		result.CurrentRevisionRef = view.CurrentRevisionRef
	}
	return result
}

func finalTemplateDraftFrom(view workspaceauthoritycmd.TemplateDraftView) finalTemplateDraftProjection {
	revision := ""
	if view.Draft.Source.SourceRevision != nil {
		revision = string(*view.Draft.Source.SourceRevision)
	}
	return finalTemplateDraftProjection{Lifecycle: "draft", TemplateRef: view.TemplateRef, TemplateID: string(view.Draft.ID), Name: view.Draft.Name, SourcePath: view.Draft.Source.Path, SourceState: string(view.Draft.Source.State), SourceRevision: &revision, SourceAccess: string(view.Draft.Body.Boundary.SourceAccess), GraphQLEndpoints: finalTemplateGraphQLEndpoints(view.Draft.Body.Policy)}
}

func finalTemplateDraftResourceFrom(view workspaceauthoritycmd.TemplateDraftView) finalTemplateProjection {
	value := finalTemplateDraftFrom(view)
	result := finalTemplateProjection{Lifecycle: value.Lifecycle, TemplateRef: value.TemplateRef, TemplateID: value.TemplateID, Name: value.Name, SourcePath: value.SourcePath, SourceState: value.SourceState, SourceRevision: value.SourceRevision}
	if view.Draft.Source.State != tobari.ResourceSourceInvalid {
		result.SourceAccess = value.SourceAccess
		result.GraphQLEndpoints = value.GraphQLEndpoints
	}
	return result
}

func finalTemplateGraphQLEndpointText(endpoints []tobari.ManifestPolicyExactRule) string {
	if len(endpoints) == 0 {
		return "none"
	}
	values := make([]string, len(endpoints))
	for index, endpoint := range endpoints {
		values[index] = safeExternalText(fmt.Sprintf("%s://%s:%d%s", endpoint.Scheme, endpoint.Host, endpoint.Port, endpoint.Path))
	}
	return strings.Join(values, ", ")
}

func finalTemplateGraphQLEndpoints(policy tobari.WorkspaceTemplatePolicyBody) []tobari.ManifestPolicyExactRule {
	modules, ok := policy.FinalSemanticModules()
	if !ok {
		return append([]tobari.ManifestPolicyExactRule{}, policy.GraphQLEndpoints...)
	}
	endpoints := modules.GraphQLEndpoints()
	result := make([]tobari.ManifestPolicyExactRule, 0, len(endpoints))
	for _, endpoint := range endpoints {
		result = append(result, tobari.ManifestPolicyExactRule{
			Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Method: "POST", Path: endpoint.Path,
		})
	}
	return result
}

type finalAuthorityHumanRow struct {
	label string
	value string
	token styleToken
}

func finalAuthorityHumanCard(color bool, marker, title string, markerToken styleToken, section string, rows []finalAuthorityHumanRow, nextCommand, nextReason string, details []finalAuthorityHumanRow) []byte {
	output := newHumanOutput(color)
	output.heading(marker, title, markerToken)
	if section != "" {
		output.section(section)
	}
	for _, row := range rows {
		output.row(row.label, row.value, row.token)
	}
	if nextCommand != "" {
		output.next(nextCommand, nextReason)
	}
	if len(details) > 0 {
		output.section("Details")
		for _, row := range details {
			output.row(row.label, row.value, row.token)
		}
	}
	return output.bytes()
}

func finalTemplateText(value finalTemplateProjection, includeReferences, color bool) []byte {
	section := safeExternalText(value.Name)
	if section == "" {
		section = "Template"
	}
	status := "✓ active · generation " + fmt.Sprint(value.Generation)
	statusToken := styleSuccess
	rows := []finalAuthorityHumanRow{
		{label: "Status", value: status, token: statusToken},
		{label: "Source access", value: value.SourceAccess, token: styleText},
		{label: "GraphQL", value: finalTemplateGraphQLEndpointText(value.GraphQLEndpoints), token: styleText},
	}
	if value.Lifecycle == "draft" {
		rows[0] = finalAuthorityHumanRow{label: "Status", value: "! draft · source " + value.SourceState, token: styleWarning}
		if value.SourcePath != "" {
			rows = append(rows, finalAuthorityHumanRow{label: "Source", value: safeExternalText(value.SourcePath), token: styleText})
		}
	}
	details := []finalAuthorityHumanRow{{label: "Reference", value: value.TemplateRef, token: styleText}}
	if includeReferences {
		revision := value.CurrentRevisionRef
		if revision == "" {
			revision = value.Revision
		}
		details = append(details, finalAuthorityHumanRow{label: "Revision", value: revision, token: styleText})
	} else if value.Revision != "" {
		details = append(details, finalAuthorityHumanRow{label: "Revision", value: value.Revision, token: styleText})
	}
	return finalAuthorityHumanCard(color, "·", "Template details", styleMuted, section, rows, "", "", details)
}

func finalContextFromView(view workspaceauthoritycmd.ContextView) (finalContextProjection, error) {
	snapshot, contextRef := view.Snapshot, view.ContextRef
	axes, err := tobari.NewContextAuthorityAxes(snapshot)
	if err != nil {
		return finalContextProjection{}, err
	}
	result := finalContextProjection{
		Lifecycle: "active", Current: view.Current, ContextRef: contextRef, ContextID: string(snapshot.Context.ID), TemplateID: string(snapshot.Context.TemplateID), TemplateName: snapshot.Template.Name,
		DesiredTemplateGeneration: axes.DesiredTemplateGeneration, DesiredTemplateRevision: string(axes.DesiredTemplateRevision), DesiredTemplatePolicySliceDigest: string(axes.DesiredTemplatePolicySliceDigest),
		CurrentPolicyMemoryRevision: string(axes.CurrentPolicyMemoryRevision), AppliedEntry: axes.AppliedEntry,
	}
	if axes.ActiveTemplatePolicySliceDigest != nil {
		value := string(*axes.ActiveTemplatePolicySliceDigest)
		result.ActiveTemplatePolicySliceDigest = &value
	}
	if axes.ActivePolicyMemoryRevision != nil {
		value := string(*axes.ActivePolicyMemoryRevision)
		result.ActivePolicyMemoryRevision = &value
	}
	if snapshot.Workspace != nil {
		result.WorkspaceID = string(snapshot.Workspace.ID)
	}
	if view.Source != nil {
		result.SourcePath = view.Source.Path
		result.SourceState = string(view.Source.State)
		if view.Source.ActiveRevision != nil {
			result.ActiveRevision = string(*view.Source.ActiveRevision)
		}
		if view.Source.SourceRevision != nil {
			value := string(*view.Source.SourceRevision)
			result.SourceRevision = &value
		}
	}
	return result, nil
}

func finalContextFromDraft(view workspaceauthoritycmd.ContextDraftView) finalContextProjection {
	draft := view.Draft
	result := finalContextProjection{Lifecycle: "draft", ContextRef: view.ContextRef, ContextID: string(draft.Source.ContextID), TemplateID: string(draft.Source.TemplateID), SourcePath: draft.Observation.Path, SourceState: string(draft.Observation.State)}
	if draft.Observation.SourceRevision != nil {
		value := string(*draft.Observation.SourceRevision)
		result.SourceRevision = &value
	}
	return result
}

func finalContextFrom(snapshot tobari.ContextAuthoritySnapshot, contextRef string) (finalContextProjection, error) {
	return finalContextFromView(workspaceauthoritycmd.ContextView{Snapshot: snapshot, ContextRef: contextRef})
}

func finalContextText(value finalContextProjection, color bool) []byte {
	if value.Lifecycle == "draft" {
		return finalAuthorityHumanCard(color, "·", "Context details", styleMuted, "Context draft", []finalAuthorityHumanRow{
			{label: "Status", value: "! draft · source " + value.SourceState, token: styleWarning},
			{label: "Source", value: safeExternalText(value.SourcePath), token: styleText},
			{label: "Template ID", value: value.TemplateID, token: styleText},
		}, "", "", []finalAuthorityHumanRow{{label: "Reference", value: value.ContextRef, token: styleText}})
	}
	activeTemplate := "absent"
	if value.ActiveTemplatePolicySliceDigest != nil {
		activeTemplate = *value.ActiveTemplatePolicySliceDigest
	}
	activeMemory := "absent"
	if value.ActivePolicyMemoryRevision != nil {
		activeMemory = *value.ActivePolicyMemoryRevision
	}
	applied := "absent"
	if value.AppliedEntry != nil {
		applied = string(value.AppliedEntry.TemplateRevision) + " / " + string(value.AppliedEntry.EntrySliceDigest)
	}
	section := safeExternalText(value.TemplateName)
	if section == "" {
		section = "Context"
	}
	return finalAuthorityHumanCard(color, "·", "Context details", styleMuted, section, []finalAuthorityHumanRow{
		{label: "Status", value: "✓ active", token: styleSuccess},
		{label: "Desired Template generation", value: fmt.Sprintf("%d", value.DesiredTemplateGeneration), token: styleText},
		{label: "Desired Template revision", value: value.DesiredTemplateRevision, token: styleText},
		{label: "Desired Template policy", value: value.DesiredTemplatePolicySliceDigest, token: styleText},
		{label: "Active Template policy", value: activeTemplate, token: styleText},
		{label: "Current Policy Memory", value: value.CurrentPolicyMemoryRevision, token: styleText},
		{label: "Active Policy Memory", value: activeMemory, token: styleText},
		{label: "Applied entry", value: applied, token: humanStatusToken(map[bool]string{true: "ready", false: "missing"}[value.AppliedEntry != nil])},
	}, "", "", []finalAuthorityHumanRow{{label: "Reference", value: value.ContextRef, token: styleText}})
}

func finalWorkspaceFrom(snapshot tobari.ContextAuthoritySnapshot, workspaceRef string) finalWorkspaceProjection {
	workspace := snapshot.Workspace
	result := finalWorkspaceProjection{WorkspaceRef: workspaceRef, WorkspaceID: string(workspace.ID), ContextID: string(snapshot.Context.ID), TemplateID: string(snapshot.Context.TemplateID), TemplateName: snapshot.Template.Name, ProjectRoot: workspace.ProjectRoot, WorkspaceHome: workspace.Home, Applied: workspace.LastSuccessfulEntry != nil}
	if workspace.LastSuccessfulEntry != nil {
		result.AppliedEntryRevision = string(workspace.LastSuccessfulEntry.EntrySliceDigest)
	}
	return result
}

func finalAuthorityOutput(path, envelope string, value any, format successFormat, text []byte) ([]byte, error) {
	if format == successFormatJSON {
		document := map[string]any{"schema_version": 1, envelope: value}
		encoded, err := marshalCommandJSON(path, document)
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	}
	if len(text) == 0 {
		return []byte("No final authority.\n"), nil
	}
	return text, nil
}

func finalCollectionOutput(color bool, kind string, count int, cards func(*humanOutput)) []byte {
	output := newHumanOutput(color)
	output.heading("·", fmt.Sprintf("%s · %d", kind, count), styleMuted)
	if count > 0 {
		cards(output)
	}
	return output.bytes()
}

func finalTemplateListText(items []finalTemplateProjection, color bool) []byte {
	return finalCollectionOutput(color, "Templates", len(items), func(output *humanOutput) {
		for _, item := range items {
			output.section(safeExternalText(item.Name))
			if item.Lifecycle == "draft" {
				output.row("Status", "! draft · source "+item.SourceState, styleWarning)
				output.row("Source", safeExternalText(item.SourcePath), styleText)
			} else {
				output.row("Status", fmt.Sprintf("✓ active · generation %d", item.Generation), styleSuccess)
				output.row("Source access", item.SourceAccess, styleText)
			}
			output.row("Reference", item.TemplateRef, styleText)
		}
	})
}

func finalContextListText(items []finalContextProjection, color bool) []byte {
	return finalCollectionOutput(color, "Contexts", len(items), func(output *humanOutput) {
		for _, item := range items {
			section := safeExternalText(item.TemplateName)
			if section == "" {
				section = "Context draft"
			}
			output.section(section)
			if item.Lifecycle == "draft" {
				output.row("Status", "! draft · source "+item.SourceState, styleWarning)
				output.row("Template ID", item.TemplateID, styleText)
				output.row("Source", safeExternalText(item.SourcePath), styleText)
			} else {
				status := "✓ active"
				if item.Current != nil && *item.Current {
					status += " · current"
				}
				output.row("Status", status, styleSuccess)
				output.row("Template", safeExternalText(item.TemplateName), styleText)
			}
			output.row("Reference", item.ContextRef, styleText)
		}
	})
}

func finalTemplateDraftText(value finalTemplateDraftProjection, action string, color bool) []byte {
	title := "Template source created"
	if action == "copy" {
		title = "Template source copied"
	}
	return finalAuthorityHumanCard(color, "✓", title, styleSuccess, safeExternalText(value.Name), []finalAuthorityHumanRow{
		{label: "Status", value: "! draft · ready to review", token: styleWarning},
		{label: "Source", value: safeExternalText(value.SourcePath), token: styleText},
		{label: "Source access", value: value.SourceAccess, token: styleText},
	}, "template plan --id "+value.TemplateRef, "Review and activate this Template source.", []finalAuthorityHumanRow{
		{label: "Reference", value: value.TemplateRef, token: styleText},
		{label: "Template ID", value: value.TemplateID, token: styleText},
	})
}

func finalTemplatePlanText(plan tobari.WorkspaceTemplateChangePlan, color bool) []byte {
	return finalAuthorityHumanCard(color, "·", "Template change plan", styleMuted, "Impact", []finalAuthorityHumanRow{
		{label: "Change", value: string(plan.Impact), token: humanStatusToken(string(plan.Impact))},
		{label: "Contexts", value: fmt.Sprintf("%d affected", plan.AffectedContextCount), token: styleText},
		{label: "Workspaces", value: fmt.Sprintf("%d running", plan.RunningWorkspaceCount), token: styleText},
	}, "template apply --plan "+plan.PlanRef, "Apply this exact reviewed plan.", []finalAuthorityHumanRow{
		{label: "Plan reference", value: plan.PlanRef, token: styleText},
		{label: "Template", value: plan.TemplateRef, token: styleText},
	})
}

func finalTemplateMigrationPlanText(plan tobari.WorkspaceTemplatePolicyMigrationPlan, color bool) []byte {
	return finalAuthorityHumanCard(color, "·", "Template policy migration plan", styleMuted, "Schema", []finalAuthorityHumanRow{
		{label: "Source", value: plan.SourceSchema, token: styleText},
		{label: "Target", value: plan.TargetSchema, token: styleAccent},
	}, "template migration apply --plan "+plan.PlanRef, "Apply this exact policy-source migration.", []finalAuthorityHumanRow{
		{label: "Plan reference", value: plan.PlanRef, token: styleText},
		{label: "Template", value: plan.TemplateRef, token: styleText},
	})
}

func finalTemplateMigrationResultText(result tobari.WorkspaceTemplatePolicyMigrationResult, color bool) []byte {
	return finalAuthorityHumanCard(color, "✓", "Template policy source migrated", styleSuccess, "Template", []finalAuthorityHumanRow{
		{label: "Status", value: "✓ source migrated", token: styleSuccess},
		{label: "Revision", value: string(result.ActiveRevision) + " · unchanged", token: styleText},
	}, "", "", []finalAuthorityHumanRow{
		{label: "Reference", value: result.TemplateRef, token: styleText},
		{label: "Source", value: result.SourceFingerprint, token: styleText},
	})
}

func finalContextPlanText(plan tobari.ContextActivationPlan, color bool) []byte {
	status, token := "ready to apply", styleAccent
	if plan.NoOp {
		status, token = "no change", styleMuted
	}
	return finalAuthorityHumanCard(color, "·", "Context activation plan", styleMuted, "Activation", []finalAuthorityHumanRow{
		{label: "Status", value: status, token: token},
		{label: "Template", value: plan.TemplateRef, token: styleText},
		{label: "Source access", value: string(plan.SourceAccess), token: styleText},
		{label: "Runtime", value: plan.Runtime.RuntimeID + " · " + string(plan.Runtime.Revision), token: styleText},
	}, "context apply --plan "+plan.PlanRef, "Apply this exact reviewed activation plan.", []finalAuthorityHumanRow{
		{label: "Plan reference", value: plan.PlanRef, token: styleText},
		{label: "Context", value: plan.ContextRef, token: styleText},
	})
}

func finalContextDraftText(value finalContextDraftProjection, templateRef string, color bool) []byte {
	return finalAuthorityHumanCard(color, "✓", "Context source created", styleSuccess, "Context draft", []finalAuthorityHumanRow{
		{label: "Status", value: "! draft · ready to review", token: styleWarning},
		{label: "Source", value: safeExternalText(value.SourcePath), token: styleText},
		{label: "Template", value: templateRef, token: styleText},
	}, "context plan --id "+value.ContextRef, "Review and activate this Context source.", []finalAuthorityHumanRow{
		{label: "Reference", value: value.ContextRef, token: styleText},
		{label: "Context ID", value: value.ContextID, token: styleText},
	})
}

func finalContextApplyText(value finalContextProjection, changed, color bool) []byte {
	marker, title, headingStyle := "✓", "Context activated", styleSuccess
	if !changed {
		marker, title, headingStyle = "·", "Context already active", styleMuted
	}
	section := safeExternalText(value.TemplateName)
	if section == "" {
		section = "Context"
	}
	return finalAuthorityHumanCard(color, marker, title, headingStyle, section, []finalAuthorityHumanRow{
		{label: "Changed", value: humanBool(changed), token: humanOutcomeBoolToken(changed)},
		{label: "Status", value: "✓ active", token: styleSuccess},
		{label: "Template revision", value: value.DesiredTemplateRevision, token: styleText},
	}, "context use --id "+value.ContextRef, "Select this location-free Context for later Context-aware commands.", []finalAuthorityHumanRow{{label: "Reference", value: value.ContextRef, token: styleText}})
}

func finalWorkspaceStatusText(value finalWorkspaceProjection, color bool) []byte {
	applied, token := "absent", styleWarning
	if value.Applied {
		applied, token = "present", styleSuccess
	}
	return finalAuthorityHumanCard(color, "·", "Workspace details", styleMuted, safeExternalText(value.ProjectRoot), []finalAuthorityHumanRow{
		{label: "Status", value: "✓ exists", token: styleSuccess},
		{label: "Template", value: safeExternalText(value.TemplateName), token: styleText},
		{label: "Applied entry", value: applied, token: token},
		{label: "Home", value: safeExternalText(value.WorkspaceHome), token: styleText},
	}, "", "", []finalAuthorityHumanRow{{label: "Reference", value: value.WorkspaceRef, token: styleText}})
}

func finalSimpleResultText(path, reference, id, stateName string, state, color bool) []byte {
	resource, action, idLabel := "Resource", stateName, "Authority ID"
	unchangedTitle := resource + " unchanged"
	switch path {
	case "template default set":
		resource, action, idLabel = "Default Template", "selected", "Template ID"
		unchangedTitle = "Default Template unchanged"
	case "template delete":
		resource, action, idLabel = "Template", "deleted", "Template ID"
		unchangedTitle = "Template already absent"
	case "context delete":
		resource, action, idLabel = "Context", "deleted", "Context ID"
		unchangedTitle = "Context already absent"
	case "workspace delete":
		resource, action, idLabel = "Workspace", "deleted", "Workspace ID"
		unchangedTitle = "Workspace already absent"
	}
	marker, title, headingToken := "·", unchangedTitle, styleMuted
	stateText, stateToken := "no", styleMuted
	if state {
		marker, title, headingToken = "✓", resource+" "+action, styleSuccess
		stateText, stateToken = "yes", styleSuccess
	}
	return finalAuthorityHumanCard(color, marker, title, headingToken, resource, []finalAuthorityHumanRow{
		{label: strings.ToUpper(stateName[:1]) + stateName[1:], value: stateText, token: stateToken},
	}, "", "", []finalAuthorityHumanRow{{label: "Reference", value: reference, token: styleText}, {label: idLabel, value: id, token: styleText}})
}

func finalWorkspaceListText(items []finalWorkspaceProjection, color bool) []byte {
	return finalCollectionOutput(color, "Workspaces", len(items), func(output *humanOutput) {
		for _, item := range items {
			output.section(safeExternalText(item.ProjectRoot))
			output.row("Status", "✓ exists", styleSuccess)
			output.row("Template", safeExternalText(item.TemplateName), styleText)
			output.row("Applied entry", map[bool]string{true: "present", false: "absent"}[item.Applied], humanOutcomeBoolToken(item.Applied))
			output.row("Reference", item.WorkspaceRef, styleText)
		}
	})
}

func finalFormat(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs) (successFormat, int, bool) {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return "", c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help "+command.Path, "Correct the command arguments."), false
	}
	return format, ExitOK, true
}

func runFinalTemplateList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalTemplates.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	items := make([]finalTemplateProjection, 0, len(result.Items)+len(result.Drafts))
	for _, item := range result.Items {
		value := finalTemplateFrom(item, false)
		value.PolicySliceDigest = ""
		value.EntrySliceDigest = ""
		items = append(items, value)
	}
	for _, draft := range result.Drafts {
		value := finalTemplateDraftResourceFrom(draft)
		items = append(items, value)
	}
	output, err := finalAuthorityOutput(command.Path, "templates", map[string]any{"items": items}, format, finalTemplateListText(items, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalTemplateShow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalTemplates.ShowResource(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	var value finalTemplateProjection
	var text []byte
	if result.Active != nil {
		value = finalTemplateFrom(*result.Active, true)
		text = finalTemplateText(value, true, humanStyleAllowed(ctx, c, c.Out))
	} else {
		value = finalTemplateDraftResourceFrom(*result.Draft)
		text = finalTemplateText(value, true, humanStyleAllowed(ctx, c, c.Out))
	}
	output, err := finalAuthorityOutput(command.Path, "template", value, format, text)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalTemplateCreate(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	body, err := c.reviewedStandardTemplateBody(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	if err := applyTemplateCreateInputs(&body, inputs); err != nil {
		return c.failUsage(ctx, "invalid_arguments", "template create options are invalid: "+err.Error()+"; usage: "+command.Usage(), "help "+command.Path, "Correct the creation options.")
	}
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateCatalogTargetKind, ParentID: tobari.WorkspaceTemplateCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	view, err := c.finalTemplates.CreateDraft(ctx, intent, inputs.One("--name"), body)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateDraftFrom(view)
	human := finalTemplateDraftText(value, "create", humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "template", value, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalTemplateCopy(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: inputs.One("--from")}
	intent.Impact = command.Agent.Mutation.Impact
	view, err := c.finalTemplates.CopyDraft(ctx, intent, inputs.One("--from"), inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateDraftFrom(view)
	human := finalTemplateDraftText(value, "copy", humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "template", value, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalTemplateApply(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--plan")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalTemplates.Apply(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateFrom(result.View, true)
	document := struct {
		finalTemplateProjection
		Changed bool `json:"changed"`
	}{finalTemplateProjection: value, Changed: result.Changed}
	marker, title, headingStyle := "✓", "Template activated", styleSuccess
	if !result.Changed {
		marker, title, headingStyle = "·", "Template already active", styleMuted
	}
	human := finalAuthorityHumanCard(humanStyleAllowed(ctx, c, c.Out), marker, title, headingStyle, safeExternalText(value.Name), []finalAuthorityHumanRow{
		{label: "Changed", value: humanBool(result.Changed), token: humanOutcomeBoolToken(result.Changed)},
		{label: "Status", value: fmt.Sprintf("✓ active · generation %d", value.Generation), token: styleSuccess},
		{label: "Source access", value: value.SourceAccess, token: styleText},
	}, "", "", []finalAuthorityHumanRow{{label: "Reference", value: value.TemplateRef, token: styleText}, {label: "Revision", value: value.CurrentRevisionRef, token: styleText}})
	output, err := finalAuthorityOutput(command.Path, "template", document, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalTemplatePlan(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	plan, err := c.finalTemplates.Plan(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	human := finalTemplatePlanText(plan, humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "template_change_plan", finalTemplateChangePlanFrom(plan), format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalTemplateMigrationPlan(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	plan, err := c.finalTemplates.PlanPolicyMigration(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	human := finalTemplateMigrationPlanText(plan, humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "template_policy_migration_plan", finalTemplatePolicyMigrationPlanFrom(plan), format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalTemplateMigrationApply(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--plan")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalTemplates.ApplyPolicyMigration(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	human := finalTemplateMigrationResultText(result, humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "template_policy_migration", result, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func (c *CLI) emitFinalTemplateMutation(ctx context.Context, command CommandSpec, inputs ParsedInputs, view workspaceauthoritycmd.TemplateView, includeReferences bool) int {
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateFrom(view, false)
	output, err := finalAuthorityOutput(command.Path, "template", value, format, finalTemplateText(value, includeReferences, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func applyTemplateCreateInputs(body *tobari.WorkspaceTemplateBody, inputs ParsedInputs) error {
	if body == nil {
		return fmt.Errorf("Template body is unavailable")
	}
	sourceAccess := inputs.One("--source-access")
	if sourceAccess == "" {
		sourceAccess = string(tobari.ManifestSourceAccessReadWrite)
	}
	body.Boundary.SourceAccess = tobari.ManifestSourceAccess(sourceAccess)
	if inputs.Provided("--graphql-endpoint") {
		endpoint, err := parseBoundedGraphQLEndpoint(inputs.One("--graphql-endpoint"))
		if err != nil {
			return err
		}
		if body.Policy.SemanticModules == nil {
			return fmt.Errorf("Template body is not compiled as semantic policy V1")
		}
		body.Policy.SemanticModules.Protocols.HTTP.GraphQL.Endpoints = append(
			body.Policy.SemanticModules.Protocols.HTTP.GraphQL.Endpoints,
			tobari.SemanticHTTPEndpoint{SemanticRuleAuthority: tobari.SemanticRuleAuthority{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port}, Path: endpoint.Path},
		)
	}
	return body.Validate()
}

func runFinalTemplateDefaultSet(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalTemplates.SetDefault(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalSimpleResult(ctx, command, inputs, "workspace_template_id", string(result.TemplateID), "selected", result.Selected)
}

func runFinalTemplateDelete(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalTemplates.Delete(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalSimpleResult(ctx, command, inputs, "workspace_template_id", string(result.TemplateID), "deleted", result.Deleted)
}

func runFinalContextList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalContexts.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	items := make([]finalContextProjection, 0, len(result.Items)+len(result.Drafts))
	for _, item := range result.Items {
		value, projectionErr := finalContextFromView(item)
		err = projectionErr
		if err != nil {
			return c.fail(ctx, err)
		}
		items = append(items, value)
	}
	for _, draft := range result.Drafts {
		value := finalContextFromDraft(draft)
		current := false
		value.Current = &current
		items = append(items, value)
	}
	output, err := finalAuthorityOutput(command.Path, "contexts", map[string]any{"items": items}, format, finalContextListText(items, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalContextShow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	resource, err := c.finalContexts.ShowResource(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	var value finalContextProjection
	if resource.Active != nil {
		value, err = finalContextFromView(*resource.Active)
		if err != nil {
			return c.fail(ctx, err)
		}
	} else if resource.Draft != nil {
		value = finalContextFromDraft(*resource.Draft)
	} else {
		return c.fail(ctx, fmt.Errorf("Context resource is absent"))
	}
	output, err := finalAuthorityOutput(command.Path, "context", value, format, finalContextText(value, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalContextApply(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--plan")
	intent.Target = operation.TargetRef{Kind: tobari.ContextActivationPlanReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalContexts.Apply(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value, err := finalContextFromView(result.View)
	if err != nil {
		return c.fail(ctx, err)
	}
	document := struct {
		finalContextProjection
		Changed bool `json:"changed"`
	}{finalContextProjection: value, Changed: result.Changed}
	human := finalContextApplyText(value, result.Changed, humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "context", document, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalContextPlan(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	plan, err := c.finalContexts.Plan(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	value := finalContextPlanProjection{PlanRef: plan.PlanRef, ContextRef: plan.ContextRef, SourceFingerprint: plan.SourceFingerprint, TemplateRef: plan.TemplateRef, TemplateRevision: string(plan.TemplateRevision), DuplicateBinding: plan.DuplicateBinding, NoOp: plan.NoOp, SourceAccess: string(plan.SourceAccess), RuntimeID: plan.Runtime.RuntimeID, RuntimeRevision: plan.Runtime.Revision, BoundaryFingerprint: string(plan.BoundaryFingerprint), PolicySliceDigest: string(plan.PolicySliceDigest), NewPolicyMemoryOwner: string(plan.NewPolicyMemoryOwner)}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	human := finalContextPlanText(plan, humanStyleAllowed(ctx, c, c.Out))
	output, err := finalAuthorityOutput(command.Path, "context_activation_plan", value, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalContextCreate(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--template")
	intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	view, err := c.finalContexts.CreateDraft(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	revision := ""
	if view.Draft.Observation.SourceRevision != nil {
		revision = string(*view.Draft.Observation.SourceRevision)
	}
	value := finalContextDraftProjection{Lifecycle: "draft", ContextRef: view.ContextRef, ContextID: string(view.Draft.Source.ContextID), TemplateID: string(view.Draft.Source.TemplateID), SourcePath: view.Draft.Observation.Path, SourceState: string(view.Draft.Observation.State), SourceRevision: revision}
	output, err := finalAuthorityOutput(command.Path, "context", value, format, finalContextDraftText(value, ref, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalContextUse(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalContexts.Use(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalSimpleResult(ctx, command, inputs, "context_id", string(result.ContextID), "selected", result.Selected)
}

func runFinalContextDelete(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalContexts.Delete(ctx, intent, ref)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalSimpleResult(ctx, command, inputs, "context_id", string(result.ContextID), "deleted", result.Deleted)
}

func runFinalWorkspaceList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalWorkspaces == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalWorkspaces.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	items := make([]finalWorkspaceProjection, len(result.Items))
	for i, item := range result.Items {
		items[i] = finalWorkspaceFrom(item.Snapshot, item.WorkspaceRef)
	}
	output, err := finalAuthorityOutput(command.Path, "workspaces", map[string]any{"items": items}, format, finalWorkspaceListText(items, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalWorkspaceStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalWorkspaces == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	view, err := c.finalWorkspaces.Status(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalWorkspaceFrom(view.Snapshot, view.WorkspaceRef)
	output, err := finalAuthorityOutput(command.Path, "workspace", value, format, finalWorkspaceStatusText(value, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalWorkspaceDelete(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalWorkspaces == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	force, ok := inputs.Boolean("--force")
	if !ok {
		return c.failUsage(ctx, "invalid_arguments", "--force must be boolean", "help workspace delete", "Correct the command arguments.")
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalWorkspaces.Delete(ctx, intent, ref, force)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalSimpleResult(ctx, command, inputs, "workspace_id", string(result.WorkspaceID), "deleted", result.Deleted)
}

func (c *CLI) emitFinalSimpleResult(ctx context.Context, command CommandSpec, inputs ParsedInputs, idName, id string, stateName string, state bool) int {
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := map[string]any{idName: id, stateName: state}
	output, err := finalAuthorityOutput(command.Path, "result", value, format, finalSimpleResultText(command.Path, inputs.One("--id"), id, stateName, state, humanStyleAllowed(ctx, c, c.Out)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func (c *CLI) reviewedStandardTemplateBody(ctx context.Context) (tobari.WorkspaceTemplateBody, error) {
	if c == nil || c.runtime == nil {
		return tobari.WorkspaceTemplateBody{}, missingRuntimeFault()
	}
	binding, err := c.runtime.BindingByReference(ctx, tobari.StandardRuntimeID, 1)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		return tobari.WorkspaceTemplateBody{}, fault.WithClassification(
			fault.New(fault.KindContract, "invalid_standard_template_body", "The built-in standard Template policy is unavailable.", false),
			fault.PhasePrecondition, fault.ChangeNone,
		)
	}
	body := tobari.WorkspaceTemplateBody{Boundary: tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: policy.DestinationCeiling, MethodPolicy: policy.MethodPolicy}, Policy: tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: policy.BaselineGrants, BaselineTemplates: policy.BaselineTemplates, MCPBaselineGrants: policy.MCPBaselineGrants, BaselineDenies: policy.BaselineDenies, GraphQLEndpoints: policy.GraphQLEndpoints, MCPEndpoints: policy.MCPEndpoints}, EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: binding}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{}}
	compiled, err := tobari.CompileWorkspaceTemplateBodyV1(body)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, fault.WithClassification(
			fault.Wrap(fault.KindContract, "invalid_standard_template_body", "The built-in standard Template body is invalid.", false, err),
			fault.PhasePrecondition, fault.ChangeNone,
		)
	}
	return compiled, nil
}
