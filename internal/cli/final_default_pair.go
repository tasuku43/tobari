package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalDefaultPairProjection struct {
	AuthorityState                   string                        `json:"authority_state"`
	ProjectRoot                      string                        `json:"project_root"`
	DefaultTemplateState             string                        `json:"default_template_state"`
	WorkspaceTemplateID              *string                       `json:"workspace_template_id"`
	TemplateName                     *string                       `json:"template_name"`
	DesiredTemplateGeneration        *uint64                       `json:"desired_template_generation"`
	DesiredTemplateRevision          *string                       `json:"desired_template_revision"`
	DesiredTemplatePolicySliceDigest *string                       `json:"desired_template_policy_slice_digest"`
	ActiveTemplatePolicySliceDigest  *string                       `json:"active_template_policy_slice_digest"`
	ContextID                        *string                       `json:"context_id"`
	CurrentPolicyMemoryRevision      *string                       `json:"current_policy_memory_revision"`
	ActivePolicyMemoryRevision       *string                       `json:"active_policy_memory_revision"`
	WorkspaceID                      *string                       `json:"workspace_id"`
	WorkspaceRef                     *string                       `json:"workspace_ref,omitempty"`
	WorkspaceHome                    *string                       `json:"workspace_home"`
	AppliedEntry                     *tobari.WorkspaceAppliedEntry `json:"applied_entry"`
}

func finalDefaultPairFrom(status workspaceauthoritycmd.DefaultPairStatus) (finalDefaultPairProjection, error) {
	if err := status.Validate(); err != nil {
		return finalDefaultPairProjection{}, err
	}
	result := finalDefaultPairProjection{AuthorityState: status.AuthorityState, ProjectRoot: status.ProjectRoot, DefaultTemplateState: status.DefaultTemplateState, AppliedEntry: status.AppliedEntry}
	if status.DefaultTemplateState == "selected" {
		templateID, name := string(status.WorkspaceTemplateID), status.TemplateName
		generation, revision, policy := status.DesiredTemplateGeneration, string(status.DesiredTemplateRevision), string(status.DesiredTemplatePolicySliceDigest)
		result.WorkspaceTemplateID, result.TemplateName = &templateID, &name
		result.DesiredTemplateGeneration, result.DesiredTemplateRevision, result.DesiredTemplatePolicySliceDigest = &generation, &revision, &policy
	}
	if status.ContextID != "" {
		contextID, memory := string(status.ContextID), string(status.CurrentPolicyMemoryRevision)
		result.ContextID, result.CurrentPolicyMemoryRevision = &contextID, &memory
	}
	if status.ActiveTemplatePolicySliceDigest != nil {
		value := string(*status.ActiveTemplatePolicySliceDigest)
		result.ActiveTemplatePolicySliceDigest = &value
	}
	if status.ActivePolicyMemoryRevision != nil {
		value := string(*status.ActivePolicyMemoryRevision)
		result.ActivePolicyMemoryRevision = &value
	}
	if status.WorkspaceID != "" {
		id, ref, home := string(status.WorkspaceID), status.WorkspaceRef, status.WorkspaceHome
		result.WorkspaceID, result.WorkspaceRef, result.WorkspaceHome = &id, &ref, &home
	}
	return result, nil
}

func runFinalDefaultPairEnter(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalDefaultPair == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	session := tobari.NewWorkspaceShellSession()
	if inputs.Provided("command") {
		var err error
		session, err = tobari.NewWorkspaceDirectSession(inputs.Values("command"))
		if err != nil {
			return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help tobari", "Supply exact child argv after --.")
		}
	}
	body, err := c.reviewedStandardTemplateBody(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	intent.Target = operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.finalDefaultPair.Enter(ctx, intent, body, session, c.In, c.Out, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	return result.Outcome.ExitCode
}

func runFinalDefaultPairStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalDefaultPair == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.finalDefaultPair.Status(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	projection, err := finalDefaultPairFrom(status)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	if format == successFormatJSON {
		encoded, err := marshalCommandJSON(command.Path, map[string]any{"schema_version": 3, "status": projection})
		if err != nil {
			return c.fail(ctx, err)
		}
		return c.emitResult(ctx, append(encoded, '\n'))
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Final authority %s\nProject %s\nDefault Template %s\n", projection.AuthorityState, safeExternalText(projection.ProjectRoot), projection.DefaultTemplateState)
	if projection.ContextID != nil {
		fmt.Fprintf(&text, "Context %s\nDesired Template generation %d\nDesired Template revision %s\nDesired Template policy %s\n", *projection.ContextID, *projection.DesiredTemplateGeneration, *projection.DesiredTemplateRevision, *projection.DesiredTemplatePolicySliceDigest)
	}
	if projection.ActiveTemplatePolicySliceDigest != nil {
		fmt.Fprintf(&text, "Active Template policy %s\n", *projection.ActiveTemplatePolicySliceDigest)
	} else {
		text.WriteString("Active Template policy absent\n")
	}
	if projection.CurrentPolicyMemoryRevision != nil {
		fmt.Fprintf(&text, "Current Policy Memory %s\n", *projection.CurrentPolicyMemoryRevision)
	}
	if projection.ActivePolicyMemoryRevision != nil {
		fmt.Fprintf(&text, "Active Policy Memory %s\n", *projection.ActivePolicyMemoryRevision)
	} else {
		text.WriteString("Active Policy Memory absent\n")
	}
	if projection.AppliedEntry != nil {
		fmt.Fprintf(&text, "Applied entry %s / %s\n", projection.AppliedEntry.TemplateRevision, projection.AppliedEntry.EntrySliceDigest)
	} else {
		text.WriteString("Applied entry absent\n")
	}
	return c.emitResult(ctx, []byte(text.String()))
}
