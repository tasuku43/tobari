//go:build tobari_dev && tobari_research

package cli

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/operatorconsole"
)

type operatorConsoleRunner interface {
	Run(context.Context, operatorconsole.Backend, bool, func(operatorconsole.Session) error) error
}

func newOperatorConsoleRunner() operatorConsoleRunner { return operatorconsole.New() }

func serveSpec() CommandSpec {
	return CommandSpec{
		Path: "serve", Summary: "Open the local Operator Console",
		Args: "[--no-open]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "operator.console",
			Outcome:      "Run one foreground host-browser console for typed cluster, Workspace, pending-permission, and learned-rule inspection, with explicit staging and one canonical reviewed Apply",
			Inputs: []CommandInput{{
				Name: "--no-open", Source: InputSourceFlag, Required: false,
				ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
				Description:   "Print the session URL without opening the purpose-limited host browser.",
				AllowedValues: []string{}, DefaultValue: stringPointer("false"),
			}},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText,
				TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "url", Type: OutputFieldTypeString, Description: "Ephemeral IPv4-loopback URL carrying the process-memory session bearer in its initial fragment."},
					{Name: "browser_opened", Type: OutputFieldTypeBoolean, Description: "Whether the purpose-limited host browser opener succeeded."},
					{Name: "bind_scope", Type: OutputFieldTypeString, Description: "Always IPv4 loopback on a random port."},
					{Name: "stop", Type: OutputFieldTypeString, Description: "Foreground process stop action."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{"The shared cluster is configured and ready, and every Workspace Template typed policy data plus Context Policy Memory is valid."},
			Errors: append(readCommandErrors("serve", true,
				declaredCommandError(fault.KindContract, "invalid_operator_console", false, "doctor", "Repair the local Operator Console configuration."),
				declaredCommandError(fault.KindContract, "invalid_operator_console_snapshot", false, "doctor", "Repair the typed cluster, Workspace, or policy observation."),
				declaredCommandError(fault.KindInternal, "operator_console_session_failed", false, "doctor", "Retry after the host random source is available."),
				declaredCommandError(fault.KindUnavailable, "operator_console_unavailable", false, "doctor", "Retry after local IPv4 loopback is available."),
				declaredCommandError(fault.KindInternal, "operator_console_failed", false, "doctor", "Restart the foreground Operator Console."),
				declaredCommandError(fault.KindInternal, "operator_console_shutdown_failed", false, "cluster status", "Inspect the local process and cluster state."),
				declaredCommandError(fault.KindInternal, "state_read_failed", false, "doctor", "Inspect local state."),
				declaredCommandError(fault.KindInternal, "status_failed", false, "doctor", "Inspect Docker and cluster state."),
				declaredCommandErrorWithActions(fault.KindUnavailable, "cluster_reconcile_interrupted", false,
					fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster."},
					fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."}),
				declaredCommandError(fault.KindContract, "invalid_status_contract", false, "doctor", "Repair the cluster status contract."),
				declaredCommandError(fault.KindInvalidInput, "invalid_root", false, "doctor", "Validate the current directory."),
				declaredCommandError(fault.KindInternal, "runtime_status_failed", false, "status", "Inspect Workspace runtime state."),
				declaredCommandError(fault.KindContract, "invalid_list_contract", false, "doctor", "Repair Workspace list semantics."),
				declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster denials", "Inspect retained denial evidence."),
				declaredCommandError(fault.KindRejected, "policy_data_invalid", false, "doctor", "Repair owner-only policy data."),
				declaredCommandError(fault.KindContract, "invalid_candidate_contract", false, "cluster denials", "Repair retained denial compatibility."),
				declaredCommandError(fault.KindContract, "invalid_policy_rule_report", false, "doctor", "Repair the learned-rule inventory."),
				declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
			), policyClusterReadinessErrors()...),
		},
		handler: runServe,
	}
}

type operatorConsoleBackend struct {
	policy  *workspaceauthoritycmd.PolicyMemoryService
	cluster *workspaceauthoritycmd.FinalClusterLifecycleService
	apply   CommandSpec
}

func (b operatorConsoleBackend) Snapshot(ctx context.Context) (tobari.FinalOperatorConsoleSnapshot, error) {
	first, err := b.policy.ReviewSnapshot(ctx)
	if err != nil {
		return tobari.FinalOperatorConsoleSnapshot{}, err
	}
	cluster, err := b.cluster.Status(ctx)
	if err != nil {
		return tobari.FinalOperatorConsoleSnapshot{}, err
	}
	second, err := b.policy.ReviewSnapshot(ctx)
	if err != nil {
		return tobari.FinalOperatorConsoleSnapshot{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return tobari.FinalOperatorConsoleSnapshot{}, fault.New(fault.KindAmbiguous, "final_authority_changed", "Final authority changed during Operator Console observation.", false)
	}
	return tobari.NewFinalOperatorConsoleSnapshot(cluster, second)
}

func (b operatorConsoleBackend) ApplyReviewed(
	ctx context.Context, set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedResult, error) {
	intent := operation.Intent{
		Command: b.apply.Path, Effect: b.apply.Effect,
		Target: operation.TargetRef{Kind: b.apply.Agent.FixedTarget.Kind, ParentID: b.apply.Agent.FixedTarget.ID},
		Impact: b.apply.Agent.Mutation.Impact,
	}
	publication, err := b.policy.ApplyReviewed(withCommandPath(ctx, b.apply.Path), intent, set)
	if err != nil {
		return tobari.PolicyMemoryReviewedResult{}, err
	}
	return tobari.NewPolicyMemoryReviewedResult(publication)
}

func runServe(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.finalPolicy == nil || c.finalClusterLifecycle == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	if c.console == nil {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_operator_console",
			"The operator console is not configured.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the local Tobari installation."},
		))
	}
	apply, found := c.catalog.lookupRegistered("policy apply-reviewed")
	if !found || apply.Agent.Mutation == nil || apply.Agent.FixedTarget == nil {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "The reviewed policy Apply contract is missing.", false,
			fault.NextAction{Command: "help review permissions", Reason: "Repair the catalog-owned review workflow."},
		))
	}
	noOpen, _ := inputs.Boolean("--no-open")
	backend := operatorConsoleBackend{policy: c.finalPolicy, cluster: c.finalClusterLifecycle, apply: apply}
	err := c.console.Run(ctx, backend, !noOpen, func(session operatorconsole.Session) error {
		opened := "manual URL ready"
		if session.BrowserOpened {
			opened = "opened in host browser"
		}
		output := fmt.Sprintf(
			"Operator Console ready\n  URL      %s\n  Browser  %s\n  Bind     IPv4 loopback, random port\n  Stop     Ctrl-C\n",
			session.URL, opened,
		)
		written, err := io.WriteString(c.Out, output)
		if err == nil && written != len(output) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return fault.Wrap(
				fault.KindInternal, "output_write_failed",
				"The operator console URL could not be written.", true, err,
				fault.NextAction{Command: command.Path, Reason: "Retry with a writable output stream."},
			)
		}
		return nil
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return ExitOK
}
