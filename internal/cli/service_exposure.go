package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

func (c *CLI) requireServiceExposure(ctx context.Context) int {
	if c.serviceExposure != nil {
		return ExitOK
	}
	if c.serviceExposureInitErr != nil {
		return c.fail(ctx, fault.Wrap(fault.KindUnavailable, "service_attachment_unavailable", "Workspace service attachment is unavailable.", false, c.serviceExposureInitErr, fault.NextAction{Command: "list", Reason: "Run the helper from a live attached Workspace."}))
	}
	return c.fail(ctx, missingRuntimeFault())
}

func serviceMutationIntent(command CommandSpec) operation.Intent {
	intent := operation.Intent{Command: command.Path, Effect: command.Effect}
	if command.Agent.Mutation != nil {
		intent.Impact = command.Agent.Mutation.Impact
	}
	return intent
}

func runExposureRequest(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	port, ok := inputs.Integer("port")
	if !ok {
		return c.failUsage(ctx, "invalid_arguments", "Workspace service port is invalid", "help "+ExposureProgramName, "Choose an exact non-privileged port.")
	}
	instruction := []byte("Waiting for trusted-host review.\nRun on the host: " + invocationForPath("review services") + "\n")
	if _, err := writeOnce(c.Err, instruction); err != nil {
		return c.fail(ctx, fault.Wrap(fault.KindInternal, "service_instruction_write_failed", "The trusted-host review instruction could not be written.", false, err, fault.NextAction{Command: "list", Reason: "Retry with a writable terminal before another request."}))
	}
	result, err := c.serviceExposure.Request(ctx, serviceMutationIntent(command), int(port))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderExposure(result, true))
}

func runExposureList(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderExposureList(result))
}

func runExposureStop(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	id := inputs.One("exposure-ref")
	if err := c.serviceExposure.Stop(ctx, serviceMutationIntent(command), id); err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, []byte("Workspace service exposure stopped.\n"))
}

func runServiceRequests(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.Pending(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderServiceRequests(result))
}

func runServiceAllow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.Allow(ctx, serviceMutationIntent(command), inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderExposure(result, false))
}

func runServiceDeny(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	if err := c.serviceExposure.Deny(ctx, serviceMutationIntent(command), inputs.One("--id")); err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, []byte("Workspace service request denied.\n"))
}

func renderExposure(exposure tobari.ServiceExposure, includeStop bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "Workspace service ready:")
	fmt.Fprintf(&output, "  Workspace   %s\n", safeExternalText(exposure.Workspace))
	fmt.Fprintf(&output, "  Service     127.0.0.1:%d\n", exposure.TargetPort)
	fmt.Fprintf(&output, "  Host URL    %s\n", safeExternalText(exposure.URL))
	fmt.Fprintf(&output, "  Exposure    %s\n", exposure.ID)
	fmt.Fprintf(&output, "  Lifetime    current Workspace attachment\n")
	if includeStop {
		fmt.Fprintf(&output, "  Stop        %s stop %s\n", ExposureProgramName, exposure.ID)
	}
	return output.Bytes()
}

func renderExposureList(result tobari.ServiceExposureList) []byte {
	var output bytes.Buffer
	if len(result.Exposures) == 0 {
		return []byte("No Workspace service exposures in this attachment.\n")
	}
	fmt.Fprintln(&output, "Workspace service exposures:")
	for _, exposure := range result.Exposures {
		fmt.Fprintf(&output, "  %s  %s -> 127.0.0.1:%d  %s\n", exposure.ID, safeExternalText(exposure.URL), exposure.TargetPort, safeExternalText(exposure.State))
		fmt.Fprintf(&output, "    Stop: %s stop %s\n", ExposureProgramName, exposure.ID)
	}
	return output.Bytes()
}

func renderServiceRequests(result tobari.ServiceRequestList) []byte {
	var output bytes.Buffer
	if len(result.Requests) == 0 {
		return []byte("No pending Workspace service requests.\n")
	}
	fmt.Fprintln(&output, "Pending Workspace service requests:")
	for index, request := range result.Requests {
		fmt.Fprintf(&output, "  %d. %s\n", index+1, safeExternalText(request.Workspace))
		fmt.Fprintf(&output, "     Service 127.0.0.1:%d\n", request.TargetPort)
		fmt.Fprintf(&output, "     Request %s\n", request.ID)
	}
	return output.Bytes()
}

func readBoundedReviewLine(input io.Reader) (string, error) {
	data := make([]byte, 0, 32)
	one := []byte{0}
	for len(data) <= 128 {
		n, err := input.Read(one)
		if n == 1 {
			if one[0] == '\n' {
				return strings.TrimSpace(string(data)), nil
			}
			if one[0] != '\r' {
				data = append(data, one[0])
			}
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("review input is too long")
}

func runServiceReview(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	if !terminal.IsTerminal(c.In) || !terminal.IsTerminal(c.Out) {
		pending, err := c.serviceExposure.Pending(ctx)
		if err != nil {
			return c.fail(ctx, err)
		}
		return c.emitResult(ctx, renderServiceRequests(pending))
	}
	return runServiceReviewLoop(ctx, c)
}

func runServiceReviewLoop(ctx context.Context, c *CLI) int {
	for {
		pending, err := c.serviceExposure.Pending(ctx)
		if err != nil {
			return c.fail(ctx, err)
		}
		if _, err := c.Out.Write(renderServiceRequests(pending)); err != nil {
			return c.fail(ctx, err)
		}
		if len(pending.Requests) == 0 {
			return ExitOK
		}
		if _, err := fmt.Fprint(c.Out, "Select a request number, or [b] Back: "); err != nil {
			return c.fail(ctx, err)
		}
		choice, err := readBoundedReviewLine(c.In)
		if err != nil || strings.EqualFold(choice, "b") {
			return ExitOK
		}
		selected, numberErr := strconv.Atoi(choice)
		if numberErr != nil || selected < 1 || selected > len(pending.Requests) {
			continue
		}
		request := pending.Requests[selected-1]
		fmt.Fprintf(c.Out, "Workspace %s\nService 127.0.0.1:%d\n[a] Allow once  [d] Deny  [b] Back\n> ", safeExternalText(request.Workspace), request.TargetPort)
		action, actionErr := readBoundedReviewLine(c.In)
		if actionErr != nil || strings.EqualFold(action, "b") {
			continue
		}
		if action != "a" && action != "d" {
			continue
		}
		fmt.Fprint(c.Out, "Confirm [y/N]: ")
		confirmed, confirmErr := readBoundedReviewLine(c.In)
		if confirmErr != nil || !strings.EqualFold(confirmed, "y") {
			continue
		}
		path := "service deny"
		if action == "a" {
			path = "service allow"
		}
		spec, found := c.catalog.lookupRegistered(path)
		if !found {
			return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Service review action is missing.", false))
		}
		inputs := ParsedInputs{values: map[string][]string{"--id": {request.ID}}, provided: map[string]bool{"--id": true}, defaults: map[string]bool{}}
		if action == "a" {
			return runServiceAllow(ctx, c, spec, operation.Intent{}, inputs)
		}
		return runServiceDeny(ctx, c, spec, operation.Intent{}, inputs)
	}
}
