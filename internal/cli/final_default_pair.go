package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

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
	if c == nil || c.statusHome == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.statusHome.Snapshot(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	if format == successFormatJSON {
		encoded, err := marshalCommandJSON(command.Path, map[string]any{"schema_version": tobari.StatusHomeSchemaVersion, "status": status})
		if err != nil {
			return c.fail(ctx, err)
		}
		return c.emitResult(ctx, append(encoded, '\n'))
	}
	return c.emitResult(ctx, renderStatusHome(status))
}

func renderStatusHome(status tobari.StatusHomeSnapshot) []byte {
	var text strings.Builder
	fmt.Fprintf(&text, "Project   %s\n", safeExternalText(status.ProjectRoot))
	if status.Template == nil {
		text.WriteString("Template  no default Template\nCurrent   no Context or Workspace\n")
	} else {
		fmt.Fprintf(&text, "Template  %s · generation %d\n", safeExternalText(status.Template.Name), status.Template.Generation)
		if status.Context == nil {
			text.WriteString("Current   Context absent\n")
		} else {
			fmt.Fprintf(&text, "Current   Context selected · Workspace %s\n", status.Workspace.Presence)
			fmt.Fprintf(&text, "Policy    Template %s · Memory %s\n", status.Context.TemplatePolicyActivation, status.Context.PolicyMemoryActivation)
			fmt.Fprintf(&text, "Workspace %s · entry %s · runtime %s · %s\n", status.Workspace.Presence, status.Workspace.EntryState, status.Workspace.ObservedRuntimeState, status.Workspace.AttachmentState)
			fmt.Fprintf(&text, "Runtime   %s · %s · native %s\n", status.Runtime.Authority, status.Runtime.Availability, status.Runtime.Compatibility)
			fmt.Fprintf(&text, "Cluster   %s · receipt %s\n", status.Cluster.Runtime, status.Cluster.Receipt)
			fmt.Fprintf(&text, "Review    %d permissions · %d services pending · %d active\n", status.Permissions.PendingCount, status.Services.PendingCount, status.Services.ActiveCount)
		}
	}
	if len(status.Siblings) > 0 {
		fmt.Fprintf(&text, "Other     %d same-root Contexts\n", len(status.Siblings))
	}
	if status.Next.Path != nil {
		path := *status.Next.Path
		if path == WorkspaceEntryCommandPath {
			fmt.Fprintf(&text, "Next      tobari — %s\n", status.Next.Reason)
		} else {
			fmt.Fprintf(&text, "Next      tobari %s — %s\n", path, status.Next.Reason)
		}
	} else if status.Next.Guidance != nil {
		fmt.Fprintf(&text, "Next      %s — %s\n", *status.Next.Guidance, status.Next.Reason)
	}
	return []byte(text.String())
}
