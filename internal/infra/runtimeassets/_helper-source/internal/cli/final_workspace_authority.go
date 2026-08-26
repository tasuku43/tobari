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
	ContextRef                       string                        `json:"context_ref,omitempty"`
	ContextID                        string                        `json:"context_id"`
	TemplateID                       string                        `json:"workspace_template_id"`
	TemplateName                     string                        `json:"template_name"`
	ProjectRoot                      string                        `json:"project_root"`
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
	ProjectRoot    string `json:"project_root"`
	SourcePath     string `json:"source_path"`
	SourceState    string `json:"source_state"`
	SourceRevision string `json:"source_revision"`
}

type finalContextPlanProjection struct {
	PlanRef              string `json:"plan_ref"`
	ContextRef           string `json:"context_ref"`
	SourceFingerprint    string `json:"source_fingerprint"`
	ProjectRoot          string `json:"project_root"`
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

// finalProjectRootAuthority is the smallest non-creating final Context scope
// seam. It deliberately excludes predecessor Workspace/Manifest readers.
type finalProjectRootAuthority interface {
	CurrentDirectory(context.Context) (string, error)
	ResolveProjectRoot(context.Context, string) (string, error)
}

func resolveFinalProjectRoot(ctx context.Context, authority finalProjectRootAuthority) (string, error) {
	if authority == nil {
		return "", fault.New(fault.KindInternal, "missing_port", "The final Project-root observation adapter is not configured.", false)
	}
	cwd, err := authority.CurrentDirectory(ctx)
	if err != nil {
		return "", fault.Wrap(fault.KindInvalidInput, "invalid_root", "The current directory is unavailable.", false, err)
	}
	root, err := authority.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return "", fault.Wrap(fault.KindInvalidInput, "invalid_root", "The current Project root is not eligible for a Context.", false, err)
	}
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return "", fault.Wrap(fault.KindContract, "invalid_root", "The resolved Project root is invalid.", false, err)
	}
	return root, nil
}

func projectTemplate(view interface{ Validate() error }) error { return view.Validate() }

func finalTemplateFrom(view workspaceauthoritycmd.TemplateView, exposeRevisionRef bool) finalTemplateProjection {
	revision := view.Template.Current
	result := finalTemplateProjection{Lifecycle: "active", TemplateRef: view.TemplateRef, TemplateID: string(view.Template.ID), Name: view.Template.Name, Generation: revision.Generation, Revision: string(revision.Revision), RuntimeID: revision.Slices.RuntimeID, RuntimeRevision: string(revision.Slices.RuntimeRevision), SourceAccess: string(revision.Body.Boundary.SourceAccess), GraphQLEndpoints: append([]tobari.ManifestPolicyExactRule{}, revision.Body.Policy.GraphQLEndpoints...), PolicySliceDigest: string(revision.Slices.PolicySliceDigest), EntrySliceDigest: string(revision.Slices.EntrySliceDigest), ActiveRevision: string(revision.Revision)}
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
	return finalTemplateDraftProjection{Lifecycle: "draft", TemplateRef: view.TemplateRef, TemplateID: string(view.Draft.ID), Name: view.Draft.Name, SourcePath: view.Draft.Source.Path, SourceState: string(view.Draft.Source.State), SourceRevision: &revision, SourceAccess: string(view.Draft.Body.Boundary.SourceAccess), GraphQLEndpoints: append([]tobari.ManifestPolicyExactRule{}, view.Draft.Body.Policy.GraphQLEndpoints...)}
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

func finalTemplateText(value finalTemplateProjection, includeReferences bool) []byte {
	if includeReferences {
		revision := value.CurrentRevisionRef
		if revision == "" {
			revision = value.Revision
		}
		return []byte(fmt.Sprintf("Template %s\nReference %s\nRevision %s\nSource access %s\nGraphQL endpoints %s\n", safeExternalText(value.Name), value.TemplateRef, revision, value.SourceAccess, finalTemplateGraphQLEndpointText(value.GraphQLEndpoints)))
	}
	return []byte(fmt.Sprintf("Template %s\nGeneration %d\nRevision %s\nSource access %s\nGraphQL endpoints %s\n", safeExternalText(value.Name), value.Generation, value.Revision, value.SourceAccess, finalTemplateGraphQLEndpointText(value.GraphQLEndpoints)))
}

func finalContextFromView(view workspaceauthoritycmd.ContextView) (finalContextProjection, error) {
	snapshot, contextRef := view.Snapshot, view.ContextRef
	axes, err := tobari.NewContextAuthorityAxes(snapshot)
	if err != nil {
		return finalContextProjection{}, err
	}
	result := finalContextProjection{
		Lifecycle: "active", ContextRef: contextRef, ContextID: string(snapshot.Context.ID), TemplateID: string(snapshot.Context.TemplateID), TemplateName: snapshot.Template.Name, ProjectRoot: snapshot.Context.ProjectRoot,
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
	result := finalContextProjection{Lifecycle: "draft", ContextRef: view.ContextRef, ContextID: string(draft.Source.ContextID), TemplateID: string(draft.Source.TemplateID), ProjectRoot: draft.Source.ProjectRoot, SourcePath: draft.Observation.Path, SourceState: string(draft.Observation.State)}
	if draft.Observation.SourceRevision != nil {
		value := string(*draft.Observation.SourceRevision)
		result.SourceRevision = &value
	}
	return result
}

func finalContextFrom(snapshot tobari.ContextAuthoritySnapshot, contextRef string) (finalContextProjection, error) {
	return finalContextFromView(workspaceauthoritycmd.ContextView{Snapshot: snapshot, ContextRef: contextRef})
}

func finalContextText(value finalContextProjection) []byte {
	if value.Lifecycle == "draft" {
		return []byte(fmt.Sprintf("Context %s\nLifecycle draft\nProject %s\nSource %s\nSource state %s\n", value.ContextRef, safeExternalText(value.ProjectRoot), safeExternalText(value.SourcePath), value.SourceState))
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
	return []byte(fmt.Sprintf("Context %s\nTemplate %s\nProject %s\nDesired Template generation %d\nDesired Template revision %s\nDesired Template policy %s\nActive Template policy %s\nCurrent Policy Memory %s\nActive Policy Memory %s\nApplied entry %s\n",
		value.ContextRef, safeExternalText(value.TemplateName), safeExternalText(value.ProjectRoot), value.DesiredTemplateGeneration,
		value.DesiredTemplateRevision, value.DesiredTemplatePolicySliceDigest, activeTemplate, value.CurrentPolicyMemoryRevision, activeMemory, applied))
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
	var text strings.Builder
	for _, item := range result.Items {
		value := finalTemplateFrom(item, false)
		value.PolicySliceDigest = ""
		value.EntrySliceDigest = ""
		items = append(items, value)
		fmt.Fprintf(&text, "%s  %s  generation %d  source %s\n", item.TemplateRef, safeExternalText(item.Template.Name), item.Template.Current.Generation, item.Template.Current.Body.Boundary.SourceAccess)
	}
	for _, draft := range result.Drafts {
		value := finalTemplateDraftResourceFrom(draft)
		items = append(items, value)
		fmt.Fprintf(&text, "%s  %s  draft  source %s\n", draft.TemplateRef, safeExternalText(draft.Draft.Name), draft.Draft.Source.State)
	}
	output, err := finalAuthorityOutput(command.Path, "templates", map[string]any{"items": items}, format, []byte(text.String()))
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
		text = finalTemplateText(value, true)
	} else {
		value = finalTemplateDraftResourceFrom(*result.Draft)
		text = []byte(fmt.Sprintf("Template draft %s\nReference %s\nSource %s\n", safeExternalText(value.Name), value.TemplateRef, value.SourcePath))
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
	human := []byte(fmt.Sprintf("Template draft %s\nReference %s\nSource %s\nNext %s template plan --id %s\n", safeExternalText(value.Name), value.TemplateRef, value.SourcePath, ProgramName, value.TemplateRef))
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
	human := []byte(fmt.Sprintf("Template draft %s\nReference %s\nSource %s\nNext %s template plan --id %s\n", safeExternalText(value.Name), value.TemplateRef, value.SourcePath, ProgramName, value.TemplateRef))
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
	output, err := finalAuthorityOutput(command.Path, "template", document, format, finalTemplateText(value, true))
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
	human := []byte(fmt.Sprintf("Template change plan %s\nImpact %s\nTemplate %s\nContexts %d\nRunning Workspaces %d\nApply %s template apply --plan %s\n",
		plan.PlanRef, plan.Impact, plan.TemplateRef, plan.AffectedContextCount, plan.RunningWorkspaceCount, ProgramName, plan.PlanRef))
	output, err := finalAuthorityOutput(command.Path, "template_change_plan", plan, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func (c *CLI) emitFinalTemplateMutation(ctx context.Context, command CommandSpec, inputs ParsedInputs, view workspaceauthoritycmd.TemplateView, includeReferences bool) int {
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateFrom(view, false)
	output, err := finalAuthorityOutput(command.Path, "template", value, format, finalTemplateText(value, includeReferences))
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
		body.Policy.GraphQLEndpoints = append(body.Policy.GraphQLEndpoints, endpoint)
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
	var text strings.Builder
	for _, item := range result.Items {
		value, projectionErr := finalContextFromView(item)
		err = projectionErr
		if err != nil {
			return c.fail(ctx, err)
		}
		items = append(items, value)
		fmt.Fprintf(&text, "%s  %s  %s\n", item.ContextRef, safeExternalText(item.Snapshot.Template.Name), safeExternalText(item.Snapshot.Context.ProjectRoot))
	}
	for _, draft := range result.Drafts {
		value := finalContextFromDraft(draft)
		items = append(items, value)
		fmt.Fprintf(&text, "%s  draft  %s\n", draft.ContextRef, safeExternalText(draft.Draft.Observation.Path))
	}
	output, err := finalAuthorityOutput(command.Path, "contexts", map[string]any{"items": items}, format, []byte(text.String()))
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
	output, err := finalAuthorityOutput(command.Path, "context", value, format, finalContextText(value))
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
	output, err := finalAuthorityOutput(command.Path, "context", document, format, finalContextText(value))
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
	value := finalContextPlanProjection{PlanRef: plan.PlanRef, ContextRef: plan.ContextRef, SourceFingerprint: plan.SourceFingerprint, ProjectRoot: plan.ProjectRoot, TemplateRef: plan.TemplateRef, TemplateRevision: string(plan.TemplateRevision), DuplicateBinding: plan.DuplicateBinding, NoOp: plan.NoOp, SourceAccess: string(plan.SourceAccess), RuntimeID: plan.Runtime.RuntimeID, RuntimeRevision: plan.Runtime.Revision, BoundaryFingerprint: string(plan.BoundaryFingerprint), PolicySliceDigest: string(plan.PolicySliceDigest), NewPolicyMemoryOwner: string(plan.NewPolicyMemoryOwner)}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	human := []byte(fmt.Sprintf("Context activation plan %s\nContext %s\nProject %s\nTemplate %s\nApply %s context apply --plan %s\n", plan.PlanRef, plan.ContextRef, safeExternalText(plan.ProjectRoot), plan.TemplateRef, ProgramName, plan.PlanRef))
	output, err := finalAuthorityOutput(command.Path, "context_activation_plan", value, format, human)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalContextCreate(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil || c.finalProjectRoot == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	root, err := resolveFinalProjectRoot(ctx, c.finalProjectRoot)
	if err != nil {
		return c.fail(ctx, err)
	}
	ref := inputs.One("--template")
	intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	view, err := c.finalContexts.CreateDraft(ctx, intent, ref, root)
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
	value := finalContextDraftProjection{Lifecycle: "draft", ContextRef: view.ContextRef, ContextID: string(view.Draft.Source.ContextID), TemplateID: string(view.Draft.Source.TemplateID), ProjectRoot: view.Draft.Source.ProjectRoot, SourcePath: view.Draft.Observation.Path, SourceState: string(view.Draft.Observation.State), SourceRevision: revision}
	output, err := finalAuthorityOutput(command.Path, "context", value, format, []byte(fmt.Sprintf("Context draft %s\nProject %s\nSource %s\n", view.ContextRef, safeExternalText(root), view.Draft.Observation.Path)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalContextEnter(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalContexts == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	session := tobari.NewWorkspaceShellSession()
	if inputs.Provided("command") {
		var err error
		session, err = tobari.NewWorkspaceDirectSession(inputs.Values("command"))
		if err != nil {
			return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context enter", "Supply exact child argv after --.")
		}
	}
	ref := inputs.One("--id")
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalContexts.Enter(ctx, intent, ref, session, c.In, c.Out, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	value := map[string]any{"workspace_ref": result.WorkspaceRef, "workspace_id": string(result.Snapshot.Workspace.ID), "context_id": string(result.Snapshot.Context.ID), "exit_code": result.Outcome.ExitCode}
	text := []byte(fmt.Sprintf("Workspace %s\n", result.WorkspaceRef))
	if len(result.Outcome.CleanupIssues) > 0 {
		text = append(text, []byte(workspaceCleanupAttention+"\n")...)
	}
	output, err := finalAuthorityOutput(command.Path, "entry", value, format, text)
	if err != nil {
		return c.fail(ctx, err)
	}
	_ = c.emitMutationResultTo(ctx, command, output, c.Err)
	return result.Outcome.ExitCode
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
	var text strings.Builder
	for i, item := range result.Items {
		items[i] = finalWorkspaceFrom(item.Snapshot, item.WorkspaceRef)
		fmt.Fprintf(&text, "%s  %s\n", item.WorkspaceRef, safeExternalText(item.Snapshot.Workspace.ProjectRoot))
	}
	output, err := finalAuthorityOutput(command.Path, "workspaces", map[string]any{"items": items}, format, []byte(text.String()))
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
	output, err := finalAuthorityOutput(command.Path, "workspace", value, format, []byte(fmt.Sprintf("Workspace %s\nProject %s\n", view.WorkspaceRef, safeExternalText(value.ProjectRoot))))
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
	output, err := finalAuthorityOutput(command.Path, "result", value, format, []byte(fmt.Sprintf("%s %s\n", idName, id)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func (c *CLI) reviewedStandardTemplateBody(ctx context.Context) (tobari.WorkspaceTemplateBody, error) {
	if c == nil || c.runtime == nil {
		return tobari.WorkspaceTemplateBody{}, fault.New(fault.KindInternal, "missing_port", "The built-in Runtime authority is not configured.", false)
	}
	binding, err := c.runtime.BindingByReference(ctx, tobari.StandardRuntimeID, 1)
	if err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		return tobari.WorkspaceTemplateBody{}, fault.New(fault.KindContract, "invalid_template_body", "The built-in standard Template policy is unavailable.", false)
	}
	body := tobari.WorkspaceTemplateBody{Boundary: tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: policy.DestinationCeiling, MethodPolicy: policy.MethodPolicy}, Policy: tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: policy.BaselineGrants, BaselineTemplates: policy.BaselineTemplates, MCPBaselineGrants: policy.MCPBaselineGrants, BaselineDenies: policy.BaselineDenies, GraphQLEndpoints: policy.GraphQLEndpoints, MCPEndpoints: policy.MCPEndpoints}, EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: binding}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{}}
	if err := body.Validate(); err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	return body, nil
}
