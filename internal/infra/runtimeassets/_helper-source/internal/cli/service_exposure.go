package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

type serviceRequestProjection struct {
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	ContextID    string `json:"context_id"`
	WorkspaceID  string `json:"workspace_id"`
	Context      string `json:"context"`
	ProjectRoot  string `json:"project_root"`
	TargetPort   int    `json:"target_port"`
	State        string `json:"state"`
}

type serviceExposureProjection struct {
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	ContextID    string `json:"context_id"`
	WorkspaceID  string `json:"workspace_id"`
	Context      string `json:"context"`
	ProjectRoot  string `json:"project_root"`
	URL          string `json:"url"`
	TargetPort   int    `json:"target_port"`
	HostPort     int    `json:"host_port"`
	State        string `json:"state"`
	Connections  int    `json:"connections"`
}

type serviceObservationProjection struct {
	Scope                 string                         `json:"scope"`
	Anchor                string                         `json:"anchor"`
	Coverage              string                         `json:"coverage"`
	Observation           tobari.ServiceObservationState `json:"observation"`
	ObservedOwnerCount    int                            `json:"observed_owner_count"`
	UnavailableOwnerCount int                            `json:"unavailable_owner_count"`
}

type serviceReviewProjection struct {
	serviceObservationProjection
	Requests []serviceRequestProjection `json:"requests"`
}

type serviceStatusProjection struct {
	serviceObservationProjection
	Requests  []serviceRequestProjection  `json:"requests"`
	Exposures []serviceExposureProjection `json:"exposures"`
}

type serviceAttachmentStatusProjection struct {
	Scope        string                        `json:"scope"`
	AttachmentID string                        `json:"attachment_id"`
	Pending      []tobari.ServicePendingStatus `json:"pending"`
	Exposures    []serviceExposureProjection   `json:"exposures"`
}

func projectServiceRequest(value tobari.ServiceRequest) serviceRequestProjection {
	return serviceRequestProjection{ID: value.ID, AttachmentID: value.AttachmentID, ContextID: value.ContextID, WorkspaceID: value.WorkspaceID, Context: value.Context, ProjectRoot: value.ProjectRoot, TargetPort: value.TargetPort, State: value.State}
}

func projectServiceExposure(value tobari.ServiceExposure) serviceExposureProjection {
	return serviceExposureProjection{ID: value.ID, AttachmentID: value.AttachmentID, ContextID: value.ContextID, WorkspaceID: value.WorkspaceID, Context: value.Context, ProjectRoot: value.ProjectRoot, URL: value.URL, TargetPort: value.TargetPort, HostPort: value.HostPort, State: value.State, Connections: value.Connections}
}

func projectServiceObservation(value tobari.ServiceOwnerObservation) serviceObservationProjection {
	return serviceObservationProjection{Scope: value.Scope, Anchor: value.Anchor, Coverage: value.Coverage, Observation: value.Observation, ObservedOwnerCount: value.ObservedOwnerCount, UnavailableOwnerCount: value.UnavailableOwnerCount}
}

func projectServiceReview(value tobari.ServiceReviewSnapshot) serviceReviewProjection {
	requests := make([]serviceRequestProjection, 0, len(value.Requests))
	for _, request := range value.Requests {
		requests = append(requests, projectServiceRequest(request))
	}
	return serviceReviewProjection{serviceObservationProjection: projectServiceObservation(value.ServiceOwnerObservation), Requests: requests}
}

func projectServiceStatus(value tobari.ServiceStatusSnapshot) serviceStatusProjection {
	requests := make([]serviceRequestProjection, 0, len(value.Requests))
	exposures := make([]serviceExposureProjection, 0, len(value.Exposures))
	for _, request := range value.Requests {
		requests = append(requests, projectServiceRequest(request))
	}
	for _, exposure := range value.Exposures {
		exposures = append(exposures, projectServiceExposure(exposure))
	}
	return serviceStatusProjection{serviceObservationProjection: projectServiceObservation(value.ServiceOwnerObservation), Requests: requests, Exposures: exposures}
}

func projectAttachmentStatus(value tobari.ServiceAttachmentStatus) serviceAttachmentStatusProjection {
	exposures := make([]serviceExposureProjection, 0, len(value.Exposures))
	for _, exposure := range value.Exposures {
		exposures = append(exposures, projectServiceExposure(exposure))
	}
	return serviceAttachmentStatusProjection{Scope: value.Scope, AttachmentID: value.AttachmentID, Pending: value.Pending, Exposures: exposures}
}

func (c *CLI) requireServiceExposure(ctx context.Context) int {
	if c.serviceExposure != nil {
		return ExitOK
	}
	if c.serviceExposureInitErr != nil {
		return c.fail(ctx, fault.Wrap(fault.KindUnavailable, "service_attachment_unavailable", "Workspace service attachment is unavailable.", false, c.serviceExposureInitErr, fault.NextAction{Command: "status", Reason: "Run the helper from a live attached Workspace."}))
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

func serviceFormat(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs) (successFormat, int) {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil || (format != successFormatText && format != successFormatJSON) {
		return "", c.failUsage(ctx, "invalid_arguments", "--format must be text or json; usage: "+command.Usage(), "help "+command.Path, "Correct the command arguments.")
	}
	return format, ExitOK
}

func marshalServiceJSON(program, path, envelope string, value any) ([]byte, error) {
	document := map[string]any{"schema_version": 1, envelope: value}
	var encoded []byte
	var err error
	if program == ExposureProgramName {
		encoded, err = marshalCommandJSONForProgram(program, path, document)
	} else {
		encoded, err = marshalCommandJSON(path, document)
	}
	if err != nil {
		return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Service JSON output could not be encoded.", false, err)
	}
	return append(encoded, '\n'), nil
}

func renderServiceExposure(exposure tobari.ServiceExposure, includeStop bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "Workspace service ready:")
	fmt.Fprintf(&output, "  Context      %s\n", safeExternalText(exposure.Context))
	fmt.Fprintf(&output, "  Project      %s\n", safeExternalText(exposure.ProjectRoot))
	fmt.Fprintf(&output, "  Target       127.0.0.1:%d\n", exposure.TargetPort)
	fmt.Fprintf(&output, "  URL          %s\n", safeExternalText(exposure.URL))
	fmt.Fprintf(&output, "  Exposure     %s\n", exposure.ID)
	fmt.Fprintln(&output, "  Lifetime     current Workspace attachment")
	if includeStop {
		fmt.Fprintf(&output, "  Stop         %s stop %s\n", ExposureProgramName, exposure.ID)
	}
	return output.Bytes()
}

func renderAttachmentStatus(result tobari.ServiceAttachmentStatus) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "Workspace service status · %d pending · %d active\n", len(result.Pending), len(result.Exposures))
	for _, pending := range result.Pending {
		fmt.Fprintf(&output, "  Pending  127.0.0.1:%d\n", pending.TargetPort)
	}
	for _, exposure := range result.Exposures {
		fmt.Fprintf(&output, "  Active   %s  %s -> 127.0.0.1:%d  %s\n", exposure.ID, safeExternalText(exposure.URL), exposure.TargetPort, safeExternalText(exposure.State))
		fmt.Fprintf(&output, "           Stop: %s stop %s\n", ExposureProgramName, exposure.ID)
	}
	if len(result.Pending) == 0 && len(result.Exposures) == 0 {
		fmt.Fprintln(&output, "  No pending requests or active exposures.")
	}
	return output.Bytes()
}

func renderServiceReview(result tobari.ServiceReviewSnapshot) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "Service review · %s · %d observed · %d unavailable\n", result.Observation, result.ObservedOwnerCount, result.UnavailableOwnerCount)
	if len(result.Requests) == 0 {
		fmt.Fprintln(&output, "No pending Workspace service requests.")
		return output.Bytes()
	}
	for index, request := range result.Requests {
		fmt.Fprintf(&output, "  %d. %s · %s · 127.0.0.1:%d\n", index+1, safeExternalText(request.Context), safeExternalText(request.ProjectRoot), request.TargetPort)
		fmt.Fprintf(&output, "     Request %s\n", request.ID)
	}
	return output.Bytes()
}

func renderServiceStatus(result tobari.ServiceStatusSnapshot) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "Workspace services · %s · %d observed · %d unavailable\n", result.Observation, result.ObservedOwnerCount, result.UnavailableOwnerCount)
	fmt.Fprintf(&output, "Pending requests: %d\n", len(result.Requests))
	for _, request := range result.Requests {
		fmt.Fprintf(&output, "  %s  %s · %s · 127.0.0.1:%d\n", request.ID, safeExternalText(request.Context), safeExternalText(request.ProjectRoot), request.TargetPort)
	}
	fmt.Fprintf(&output, "Active exposures: %d\n", len(result.Exposures))
	for _, exposure := range result.Exposures {
		fmt.Fprintf(&output, "  %s  %s · %s · %s  %s\n", exposure.ID, safeExternalText(exposure.Context), safeExternalText(exposure.ProjectRoot), safeExternalText(exposure.URL), safeExternalText(exposure.State))
		fmt.Fprintf(&output, "    Open: %s  Stop: %s\n", invocationForPath("service open --id "+exposure.ID), invocationForPath("service stop --id "+exposure.ID))
	}
	return output.Bytes()
}

func runExposureRequest(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	port, ok := inputs.Integer("port")
	if !ok {
		return c.failUsage(ctx, "invalid_arguments", "Workspace service port is invalid", "help "+ExposureProgramName, "Choose an exact non-privileged port.")
	}
	if _, err := writeOnce(c.Err, []byte("Waiting for trusted-host review.\nRun on the host: "+invocationForPath("review services --watch")+"\n")); err != nil {
		return c.fail(ctx, fault.Wrap(fault.KindInternal, "service_instruction_write_failed", "The trusted-host review instruction could not be written.", false, err, fault.NextAction{Command: "status", Reason: "Retry with a writable terminal before another request."}))
	}
	result, err := c.serviceExposure.Request(ctx, serviceMutationIntent(command), int(port))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := marshalServiceJSON(ExposureProgramName, ExposureProgramName, "exposure", projectServiceExposure(result))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runExposureStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.AttachmentStatus(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := marshalServiceJSON(ExposureProgramName, "status", "service_status", projectAttachmentStatus(result))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func renderStopResult(program, path string, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		return marshalServiceJSON(program, path, "result", map[string]any{"stopped": true})
	}
	return []byte("Workspace service exposure stopped.\n"), nil
}

func runExposureStop(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	if err := c.serviceExposure.Stop(ctx, serviceMutationIntent(command), inputs.One("exposure-ref")); err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderStopResult(ExposureProgramName, "stop", successFormatJSON)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runServiceStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.Status(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	output := renderServiceStatus(result)
	if format == successFormatJSON {
		output, err = marshalServiceJSON(ProgramName, command.Path, "service_status", projectServiceStatus(result))
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	return c.emitResult(ctx, output)
}

func runServiceAllow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.Allow(ctx, serviceMutationIntent(command), inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output := renderServiceExposure(result, false)
	if format == successFormatJSON {
		output, err = marshalServiceJSON(ProgramName, command.Path, "exposure", projectServiceExposure(result))
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	return c.emitMutationResult(ctx, command, output)
}

func runServiceDeny(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	if err := c.serviceExposure.Deny(ctx, serviceMutationIntent(command), inputs.One("--id")); err != nil {
		return c.fail(ctx, err)
	}
	output := []byte("Workspace service request denied.\n")
	var err error
	if format == successFormatJSON {
		output, err = marshalServiceJSON(ProgramName, command.Path, "result", map[string]any{"denied": true})
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	return c.emitMutationResult(ctx, command, output)
}

func renderOpenResult(program, path string, result tobari.ServiceOpenResult, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		return marshalServiceJSON(program, path, "open", map[string]any{"id": result.ID, "url": result.URL, "outcome": result.Outcome})
	}
	switch result.Outcome {
	case tobari.ServiceOpenRequested:
		return []byte("Browser open requested for " + safeExternalText(result.URL) + "\n"), nil
	case tobari.ServiceOpenNotDispatched:
		return []byte("Browser open was not dispatched. The exposure remains active.\nRetry: " + invocationForPath("service open --id "+result.ID) + "\nURL: " + safeExternalText(result.URL) + "\n"), nil
	case tobari.ServiceOpenOutcomeUnknown:
		return []byte("Browser open dispatch outcome is unknown; do not retry automatically.\nURL: " + safeExternalText(result.URL) + "\n"), nil
	default:
		return nil, fmt.Errorf("unsupported Service open outcome")
	}
}

func runServiceOpen(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	result, err := c.serviceExposure.Open(ctx, serviceMutationIntent(command), inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderOpenResult(ProgramName, command.Path, result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runServiceStop(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	if err := c.serviceExposure.Stop(ctx, serviceMutationIntent(command), inputs.One("--id")); err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderStopResult(ProgramName, command.Path, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
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

func serviceEffectCard(request tobari.ServiceRequest) []string {
	return []string{
		"Tobari · Review Workspace service",
		"",
		"Context    " + safeExternalText(request.Context),
		"Project    " + safeExternalText(request.ProjectRoot),
		"Target     127.0.0.1:" + strconv.Itoa(request.TargetPort),
		"Access     one fresh generated .localhost origin on IPv4 host loopback",
		"Protocol   HTTP/1.1 and WebSocket Upgrade",
		"Boundary   exact generated Host and assigned port before Workspace I/O",
		"Forwarding accepted Host, headers, cookies, Origin, redirects, and content unchanged",
		"Lifetime   current owning attachment",
		"",
		"[a] Allow once  [o] Allow once then Open  [d] Deny  [b] Back",
	}
}

func serviceReviewListLines(snapshot tobari.ServiceReviewSnapshot, selected int) []string {
	lines := []string{"Tobari · Review Workspace services", "", fmt.Sprintf("%d pending · %s · %d unavailable", len(snapshot.Requests), snapshot.Observation, snapshot.UnavailableOwnerCount), ""}
	if len(snapshot.Requests) == 0 {
		return append(lines, "No pending requests.", "", "[b] Back")
	}
	for index, request := range snapshot.Requests {
		prefix := "  "
		if index == selected {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s · %s · 127.0.0.1:%d", prefix, index+1, safeExternalText(request.Context), safeExternalText(request.ProjectRoot), request.TargetPort))
	}
	return append(lines, "", "Use arrows or a number, Enter to review, [b] Back")
}

func serviceReviewSignature(snapshot tobari.ServiceReviewSnapshot) string {
	var value strings.Builder
	fmt.Fprintf(&value, "%s/%d/%d", snapshot.Observation, snapshot.ObservedOwnerCount, snapshot.UnavailableOwnerCount)
	for _, request := range snapshot.Requests {
		fmt.Fprintf(&value, "|%s/%s/%d", request.ID, request.AttachmentID, request.TargetPort)
	}
	return value.String()
}

func serviceRequestIDs(snapshot tobari.ServiceReviewSnapshot) map[string]struct{} {
	result := make(map[string]struct{}, len(snapshot.Requests))
	for _, request := range snapshot.Requests {
		result[request.ID] = struct{}{}
	}
	return result
}

type serviceReviewChoice struct {
	request tobari.ServiceRequest
	action  string
	back    bool
}

func selectServiceReviewRaw(ctx context.Context, c *CLI, initial tobari.ServiceReviewSnapshot, watch bool, notify string) (serviceReviewChoice, error) {
	mode := terminal.New()
	restore, err := mode.Enter(c.In)
	if err != nil {
		return serviceReviewChoice{}, err
	}
	defer restore()
	snapshot, selected, detail, lines, needsRender := initial, 0, len(initial.Requests) == 1, 0, true
	selectedID := ""
	if len(initial.Requests) == 1 {
		selectedID = initial.Requests[0].ID
	}
	seen, signature := serviceRequestIDs(initial), serviceReviewSignature(initial)
	refreshAt := time.Now().Add(500 * time.Millisecond)
	defer func() { finishSelectorScreen(c.Out, lines) }()
	for {
		if err := ctx.Err(); err != nil {
			return serviceReviewChoice{}, err
		}
		if needsRender {
			frame := serviceReviewListLines(snapshot, selected)
			if detail && len(snapshot.Requests) > 0 {
				frame = serviceEffectCard(snapshot.Requests[selected])
			}
			lines, err = renderSelectorScreen(c.Out, frame, lines)
			if err != nil {
				return serviceReviewChoice{}, err
			}
			needsRender = false
		}
		if !watch && len(snapshot.Requests) == 0 {
			return serviceReviewChoice{}, nil
		}
		key, keyErr := readSelectorKeyOnce(ctx, c.In)
		if errors.Is(keyErr, errSelectorTimeout) || key.kind == selectorKeyNone {
			if !watch || time.Now().Before(refreshAt) {
				continue
			}
			fresh, refreshErr := c.serviceExposure.Review(ctx)
			if refreshErr != nil {
				return serviceReviewChoice{}, refreshErr
			}
			freshSignature := serviceReviewSignature(fresh)
			for _, request := range fresh.Requests {
				if _, existed := seen[request.ID]; !existed && notify != "off" && c.serviceNotify != nil {
					_ = c.serviceNotify(c.Out, notify)
				}
				seen[request.ID] = struct{}{}
			}
			if freshSignature != signature {
				snapshot, signature, needsRender = fresh, freshSignature, true
				selected = 0
				if selectedID != "" {
					for index := range snapshot.Requests {
						if snapshot.Requests[index].ID == selectedID {
							selected = index
							break
						}
					}
				}
				if len(snapshot.Requests) == 1 {
					selected, detail, selectedID = 0, true, snapshot.Requests[0].ID
				} else if len(snapshot.Requests) == 0 {
					detail, selectedID = false, ""
				} else if selected >= len(snapshot.Requests) {
					selected, detail, selectedID = 0, false, snapshot.Requests[0].ID
				}
			}
			refreshAt = time.Now().Add(500 * time.Millisecond)
			continue
		}
		if keyErr != nil {
			return serviceReviewChoice{}, keyErr
		}
		if key.kind == selectorKeyCancel || key.kind == selectorKeyBack && !detail {
			return serviceReviewChoice{back: true}, nil
		}
		if len(snapshot.Requests) == 0 {
			if key.kind == selectorKeyBack {
				return serviceReviewChoice{back: true}, nil
			}
			continue
		}
		if detail {
			switch key.kind {
			case selectorKeyAllow:
				return serviceReviewChoice{request: snapshot.Requests[selected], action: "allow"}, nil
			case selectorKeyOpen:
				return serviceReviewChoice{request: snapshot.Requests[selected], action: "open"}, nil
			case selectorKeyDeny:
				return serviceReviewChoice{request: snapshot.Requests[selected], action: "deny"}, nil
			case selectorKeyBack:
				if len(snapshot.Requests) == 1 {
					return serviceReviewChoice{back: true}, nil
				}
				detail, needsRender = false, true
			}
			continue
		}
		switch key.kind {
		case selectorKeyUp:
			if selected > 0 {
				selected--
				needsRender = true
			}
		case selectorKeyDown:
			if selected+1 < len(snapshot.Requests) {
				selected++
				needsRender = true
			}
		case selectorKeyNumber:
			if key.index >= 0 && key.index < len(snapshot.Requests) {
				selected, detail, needsRender = key.index, true, true
			}
		case selectorKeyEnter:
			detail, needsRender = true, true
		}
		selectedID = snapshot.Requests[selected].ID
	}
}

func selectServiceReviewLine(ctx context.Context, c *CLI, snapshot tobari.ServiceReviewSnapshot) (serviceReviewChoice, error) {
	if len(snapshot.Requests) == 0 {
		_, err := c.Out.Write(renderServiceReview(snapshot))
		return serviceReviewChoice{}, err
	}
	selected := 0
	if len(snapshot.Requests) > 1 {
		if _, err := c.Out.Write(renderServiceReview(snapshot)); err != nil {
			return serviceReviewChoice{}, err
		}
		if _, err := fmt.Fprint(c.Out, "Select a request number, or back: "); err != nil {
			return serviceReviewChoice{}, err
		}
		value, err := readBoundedReviewLine(c.In)
		if err != nil {
			return serviceReviewChoice{}, err
		}
		if strings.EqualFold(value, "back") {
			return serviceReviewChoice{back: true}, nil
		}
		index, parseErr := strconv.Atoi(value)
		if parseErr != nil || index < 1 || index > len(snapshot.Requests) {
			return serviceReviewChoice{}, nil
		}
		selected = index - 1
	}
	for _, line := range serviceEffectCard(snapshot.Requests[selected]) {
		if _, err := fmt.Fprintln(c.Out, line); err != nil {
			return serviceReviewChoice{}, err
		}
	}
	if _, err := fmt.Fprint(c.Out, "> "); err != nil {
		return serviceReviewChoice{}, err
	}
	action, err := readBoundedReviewLine(c.In)
	if err != nil {
		return serviceReviewChoice{}, err
	}
	switch strings.ToLower(action) {
	case "allow", "open", "deny":
		return serviceReviewChoice{request: snapshot.Requests[selected], action: strings.ToLower(action)}, nil
	case "back":
		return serviceReviewChoice{back: true}, nil
	default:
		return serviceReviewChoice{}, nil
	}
}

func parsedServiceID(id string) ParsedInputs {
	return ParsedInputs{values: map[string][]string{"--id": {id}, "--format": {"text"}}, provided: map[string]bool{"--id": true}, defaults: map[string]bool{"--format": true}}
}

func applyServiceReviewChoice(ctx context.Context, c *CLI, choice serviceReviewChoice) int {
	path := "service deny"
	if choice.action == "allow" || choice.action == "open" {
		path = "service allow"
	}
	spec, found := c.catalog.lookupRegistered(path)
	if !found {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Service review action is missing.", false))
	}
	actionCtx := withCommandPath(ctx, path)
	if path == "service deny" {
		return runServiceDeny(actionCtx, c, spec, operation.Intent{}, parsedServiceID(choice.request.ID))
	}
	result, err := c.serviceExposure.Allow(actionCtx, serviceMutationIntent(spec), choice.request.ID)
	if err != nil {
		return c.fail(actionCtx, err)
	}
	if code := c.emitMutationResult(actionCtx, spec, renderServiceExposure(result, false)); code != ExitOK {
		return code
	}
	if choice.action != "open" {
		return ExitOK
	}
	openSpec, found := c.catalog.lookupRegistered("service open")
	if !found {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "Service open action is missing.", false))
	}
	openCtx := withCommandPath(ctx, openSpec.Path)
	opened, err := c.serviceExposure.Open(openCtx, serviceMutationIntent(openSpec), result.ID)
	if err != nil {
		return c.fail(openCtx, err)
	}
	output, err := renderOpenResult(ProgramName, openSpec.Path, opened, successFormatText)
	if err != nil {
		return c.fail(openCtx, err)
	}
	return c.emitMutationResult(openCtx, openSpec, output)
}

func runServiceReview(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requireServiceExposure(ctx); code != ExitOK {
		return code
	}
	format, code := serviceFormat(ctx, c, command, inputs)
	if code != ExitOK {
		return code
	}
	watch, _ := inputs.Boolean("--watch")
	notify := inputs.One("--notify")
	interactive := invocationErrorFormat(ctx) != errorFormatJSON && terminal.IsTerminal(c.In) && terminal.IsTerminal(c.Out)
	if inputs.Provided("--notify") && !watch {
		return c.failUsage(ctx, "invalid_arguments", "--notify requires --watch=true; usage: "+command.Usage(), "help review services", "Use notifications only with watch mode.")
	}
	if (watch || inputs.Provided("--notify")) && (format != successFormatText || !interactive) {
		return c.fail(ctx, fault.New(fault.KindInvalidInput, "service_review_requires_tty", "review services --watch and --notify require text output and trusted interactive terminal input and output", false, fault.NextAction{Command: "help review services", Reason: "Use redirected or JSON operation only for a read-only snapshot."}))
	}
	snapshot, err := c.serviceExposure.Review(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	if format == successFormatJSON {
		output, renderErr := marshalServiceJSON(ProgramName, command.Path, "service_review", projectServiceReview(snapshot))
		if renderErr != nil {
			return c.fail(ctx, renderErr)
		}
		return c.emitResult(ctx, output)
	}
	if !interactive {
		return c.emitResult(ctx, renderServiceReview(snapshot))
	}
	for {
		choice, selectErr := selectServiceReviewRaw(ctx, c, snapshot, watch, notify)
		if errors.Is(selectErr, terminal.ErrUnsupported) {
			choice, selectErr = selectServiceReviewLine(ctx, c, snapshot)
		}
		if selectErr != nil {
			return c.fail(ctx, selectErr)
		}
		if choice.back || choice.action == "" {
			if watch && !choice.back {
				fresh, refreshErr := c.serviceExposure.Review(ctx)
				if refreshErr != nil {
					return c.fail(ctx, refreshErr)
				}
				snapshot = fresh
				continue
			}
			return ExitOK
		}
		if actionCode := applyServiceReviewChoice(ctx, c, choice); actionCode != ExitOK {
			return actionCode
		}
		if !watch {
			return ExitOK
		}
		fresh, refreshErr := c.serviceExposure.Review(ctx)
		if refreshErr != nil {
			return c.fail(ctx, refreshErr)
		}
		snapshot = fresh
	}
}
