//go:build tobari_experimental

package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
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
			Prerequisites: []string{"The shared cluster is configured and ready, and every Context policy source is valid."},
			Errors: append(readCommandErrors("serve", true,
				declaredCommandError(fault.KindContract, "invalid_operator_console", false, "doctor", "Repair the local Operator Console configuration."),
				declaredCommandError(fault.KindContract, "invalid_operator_console_snapshot", false, "doctor", "Repair the typed cluster, Workspace, or policy observation."),
				declaredCommandError(fault.KindInternal, "operator_console_session_failed", false, "serve", "Retry after the host random source is available."),
				declaredCommandError(fault.KindUnavailable, "operator_console_unavailable", false, "serve", "Retry after local IPv4 loopback is available."),
				declaredCommandError(fault.KindInternal, "operator_console_failed", false, "serve", "Restart the foreground Operator Console."),
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
	service *tobaricmd.Service
	apply   CommandSpec
	tail    int
}

func (b operatorConsoleBackend) Snapshot(ctx context.Context) (tobari.OperatorConsoleSnapshot, error) {
	return b.service.OperatorConsoleSnapshot(ctx, b.tail)
}

func (b operatorConsoleBackend) ApplyPolicyReview(
	ctx context.Context, set tobari.PolicyReviewDecisionSet,
) (tobari.PolicyReviewChange, error) {
	intent := operation.Intent{
		Command: b.apply.Path, Effect: b.apply.Effect,
		Target: operation.TargetRef{Kind: b.apply.Agent.FixedTarget.Kind, ID: b.apply.Agent.FixedTarget.ID},
		Impact: b.apply.Agent.Mutation.Impact,
	}
	return b.service.ApplyPolicyReviewDecisionSet(withCommandPath(ctx, b.apply.Path), intent, set)
}

func runServe(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
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
	backend := operatorConsoleBackend{service: c.tobari, apply: apply, tail: 10_000}
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
