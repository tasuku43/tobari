package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalTemplateProjection struct {
	TemplateRef        string                           `json:"template_ref,omitempty"`
	CurrentRevisionRef string                           `json:"current_revision_ref,omitempty"`
	TemplateID         string                           `json:"workspace_template_id"`
	Name               string                           `json:"name"`
	Generation         uint64                           `json:"generation"`
	Revision           string                           `json:"revision"`
	RuntimeID          string                           `json:"runtime_id"`
	RuntimeRevision    string                           `json:"runtime_revision"`
	SourceAccess       string                           `json:"source_access"`
	GraphQLEndpoints   []tobari.ManifestPolicyExactRule `json:"graphql_endpoints"`
	PolicySliceDigest  string                           `json:"policy_slice_digest,omitempty"`
	EntrySliceDigest   string                           `json:"entry_slice_digest,omitempty"`
}

type finalContextProjection struct {
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

func finalTemplateFrom(viewTemplate tobari.WorkspaceTemplate, templateRef, revisionRef string, exposeRevisionRef bool) finalTemplateProjection {
	revision := viewTemplate.Current
	result := finalTemplateProjection{TemplateRef: templateRef, TemplateID: string(viewTemplate.ID), Name: viewTemplate.Name, Generation: revision.Generation, Revision: string(revision.Revision), RuntimeID: revision.Slices.RuntimeID, RuntimeRevision: string(revision.Slices.RuntimeRevision), SourceAccess: string(revision.Body.Boundary.SourceAccess), GraphQLEndpoints: append([]tobari.ManifestPolicyExactRule{}, revision.Body.Policy.GraphQLEndpoints...), PolicySliceDigest: string(revision.Slices.PolicySliceDigest), EntrySliceDigest: string(revision.Slices.EntrySliceDigest)}
	if exposeRevisionRef {
		result.CurrentRevisionRef = revisionRef
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
		return []byte(fmt.Sprintf("Template %s\nReference %s\nRevision %s\nSource access %s\nGraphQL endpoints %s\n", safeExternalText(value.Name), value.TemplateRef, value.CurrentRevisionRef, value.SourceAccess, finalTemplateGraphQLEndpointText(value.GraphQLEndpoints)))
	}
	return []byte(fmt.Sprintf("Template %s\nGeneration %d\nRevision %s\nSource access %s\nGraphQL endpoints %s\n", safeExternalText(value.Name), value.Generation, value.Revision, value.SourceAccess, finalTemplateGraphQLEndpointText(value.GraphQLEndpoints)))
}

func finalContextFrom(snapshot tobari.ContextAuthoritySnapshot, contextRef string) (finalContextProjection, error) {
	axes, err := tobari.NewContextAuthorityAxes(snapshot)
	if err != nil {
		return finalContextProjection{}, err
	}
	result := finalContextProjection{
		ContextRef: contextRef, ContextID: string(snapshot.Context.ID), TemplateID: string(snapshot.Context.TemplateID), TemplateName: snapshot.Template.Name, ProjectRoot: snapshot.Context.ProjectRoot,
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
	return result, nil
}

func finalContextText(value finalContextProjection) []byte {
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
	items := make([]finalTemplateProjection, len(result.Items))
	var text strings.Builder
	for i, item := range result.Items {
		items[i] = finalTemplateFrom(item.Template, item.TemplateRef, item.CurrentRevisionRef, false)
		items[i].PolicySliceDigest = ""
		items[i].EntrySliceDigest = ""
		fmt.Fprintf(&text, "%s  %s  generation %d  source %s\n", item.TemplateRef, safeExternalText(item.Template.Name), item.Template.Current.Generation, item.Template.Current.Body.Boundary.SourceAccess)
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
	result, err := c.finalTemplates.Show(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateFrom(result.Template, result.TemplateRef, result.CurrentRevisionRef, true)
	text := finalTemplateText(value, true)
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
	view, err := c.finalTemplates.Create(ctx, intent, inputs.One("--name"), body)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalTemplateMutation(ctx, command, inputs, view.Template, view.TemplateRef, view.CurrentRevisionRef)
}

func runFinalTemplateCopy(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: inputs.One("--from")}
	intent.Impact = command.Agent.Mutation.Impact
	view, err := c.finalTemplates.Copy(ctx, intent, inputs.One("--from"), inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalTemplateMutation(ctx, command, inputs, view.Template, view.TemplateRef, view.CurrentRevisionRef)
}

func (c *CLI) emitFinalTemplateMutation(ctx context.Context, command CommandSpec, inputs ParsedInputs, template tobari.WorkspaceTemplate, _, _ string) int {
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value := finalTemplateFrom(template, "", "", false)
	output, err := finalAuthorityOutput(command.Path, "template", value, format, finalTemplateText(value, false))
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
		endpoint, err := tobari.ParseBoundedGraphQLEndpoint(inputs.One("--graphql-endpoint"))
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
	items := make([]finalContextProjection, len(result.Items))
	var text strings.Builder
	for i, item := range result.Items {
		items[i], err = finalContextFrom(item.Snapshot, item.ContextRef)
		if err != nil {
			return c.fail(ctx, err)
		}
		fmt.Fprintf(&text, "%s  %s  %s\n", item.ContextRef, safeExternalText(item.Snapshot.Template.Name), safeExternalText(item.Snapshot.Context.ProjectRoot))
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
	view, err := c.finalContexts.Show(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value, err := finalContextFrom(view.Snapshot, view.ContextRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := finalAuthorityOutput(command.Path, "context", value, format, finalContextText(value))
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
	view, err := c.finalContexts.Create(ctx, intent, ref, root)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	value, err := finalContextFrom(view.Snapshot, view.ContextRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := finalAuthorityOutput(command.Path, "context", value, format, []byte(fmt.Sprintf("Context %s\nProject %s\n", view.ContextRef, safeExternalText(root))))
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

func runFinalConfigShell(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	var value *string
	if inputs.Provided("--value") {
		v := inputs.One("--value")
		value = &v
	}
	change := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: inputs.One("--variable"), Source: tobari.ManifestShellEnvironmentSource(inputs.One("--source")), Value: value}}}
	return c.runFinalTemplateChange(ctx, command, intent, inputs, change)
}

func runFinalConfigGit(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	source := tobari.ManifestGitIdentitySource(inputs.One("--source"))
	hasName, hasEmail := inputs.Provided("--name"), inputs.Provided("--email")
	if source == tobari.ManifestGitIdentityLiteral {
		if !hasName || !hasEmail {
			return c.failUsage(ctx, "invalid_arguments", "literal Git identity requires both --name and --email; usage: "+command.Usage(), "help config git", "Supply the complete literal identity.")
		}
	} else if hasName || hasEmail {
		return c.failUsage(ctx, "invalid_arguments", "default and inherit Git identity do not accept --name or --email; usage: "+command.Usage(), "help config git", "Remove literal identity fields or select literal.")
	}
	setting := tobari.ManifestGitIdentitySetting{Source: source}
	if inputs.Provided("--name") {
		v := inputs.One("--name")
		setting.Name = &v
	}
	if inputs.Provided("--email") {
		v := inputs.One("--email")
		setting.Email = &v
	}
	return c.runFinalTemplateChange(ctx, command, intent, inputs, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeGit, Git: &setting})
}

func runFinalTemplateRuntimeSet(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	return c.runFinalTemplateChange(ctx, command, intent, inputs, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: inputs.One("--runtime")})
}

func runFinalConfigBootstrapAWS(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	return c.runFinalTemplateBootstrap(ctx, command, intent, inputs, tobari.WorkspaceTemplateChangeBootstrapAWS, "--profile")
}

func runFinalConfigBootstrapEKS(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	return c.runFinalTemplateBootstrap(ctx, command, intent, inputs, tobari.WorkspaceTemplateChangeBootstrapEKS, "--kube-context")
}

func (c *CLI) runFinalTemplateBootstrap(ctx context.Context, command CommandSpec, intent operation.Intent, inputs ParsedInputs, kind tobari.WorkspaceTemplateChangeKind, selectorInput string) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	request := tobari.WorkspaceTemplateBootstrapRequest{Kind: kind}
	actions := 0
	if inputs.Provided(selectorInput) {
		request.Action, request.Selector = tobari.WorkspaceTemplateBootstrapConfigure, inputs.One(selectorInput)
		actions++
	}
	if inputs.Provided("--refresh") {
		refresh, ok := inputs.Boolean("--refresh")
		if !ok || !refresh {
			return c.failUsage(ctx, "invalid_arguments", "--refresh must be true when provided", "help "+command.Path, "Choose exactly one bootstrap action.")
		}
		request.Action = tobari.WorkspaceTemplateBootstrapRefresh
		actions++
	}
	if inputs.Provided("--remove") {
		remove, ok := inputs.Boolean("--remove")
		if !ok || !remove {
			return c.failUsage(ctx, "invalid_arguments", "--remove must be true when provided", "help "+command.Path, "Choose exactly one bootstrap action.")
		}
		request.Action = tobari.WorkspaceTemplateBootstrapRemove
		actions++
	}
	if actions != 1 {
		return c.failUsage(ctx, "invalid_arguments", "exactly one bootstrap configure, refresh, or remove action is required", "help "+command.Path, "Choose exactly one bootstrap action.")
	}
	if request.Action == tobari.WorkspaceTemplateBootstrapRemove {
		return c.runFinalTemplateChange(ctx, command, intent, inputs, tobari.WorkspaceTemplateChange{Kind: kind})
	}
	ref := inputs.One("--id")
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: ref}
	intent.Impact = command.Agent.Mutation.Impact
	publication, err := c.finalTemplates.UpdateBootstrap(ctx, intent, ref, request)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitFinalTemplateMutation(ctx, command, inputs, publication.Template, "", "")
}

func (c *CLI) runFinalTemplateChange(ctx context.Context, command CommandSpec, intent operation.Intent, inputs ParsedInputs, change tobari.WorkspaceTemplateChange) int {
	if c == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ref := inputs.One("--id")
	parent := ""
	if change.Kind == tobari.WorkspaceTemplateChangeRuntime {
		parent = change.RuntimeRevisionRef
	}
	intent.Target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: ref, ParentID: parent}
	intent.Impact = command.Agent.Mutation.Impact
	publication, err := c.finalTemplates.UpdateConfiguration(ctx, intent, ref, change)
	if err != nil {
		return c.fail(ctx, err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(publication.Template.ID)
	revisionRef, _ := tobari.WorkspaceTemplateRevisionRef(publication.Template.ID, publication.Template.Current.Revision)
	return c.emitFinalTemplateMutation(ctx, command, inputs, publication.Template, templateRef, revisionRef)
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
	body := tobari.WorkspaceTemplateBody{Boundary: tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: policy.DestinationCeiling, MethodPolicy: policy.MethodPolicy}, Policy: tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: policy.BaselineGrants, BaselineTemplates: policy.BaselineTemplates, MCPBaselineGrants: policy.MCPBaselineGrants, BaselineDenies: policy.BaselineDenies, GraphQLEndpoints: policy.GraphQLEndpoints, MCPEndpoints: policy.MCPEndpoints}, EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: binding}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{}}
	if err := body.Validate(); err != nil {
		return tobari.WorkspaceTemplateBody{}, err
	}
	return body, nil
}
