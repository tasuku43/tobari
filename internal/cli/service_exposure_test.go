package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/serviceexposurecmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type cliServiceExposurePort struct {
	exposure    tobari.ServiceExposure
	review      tobari.ServiceReviewSnapshot
	status      tobari.ServiceStatusSnapshot
	attachment  tobari.ServiceAttachmentStatus
	reviewCalls int
	events      []string
}

func (p *cliServiceExposurePort) RequestService(context.Context, int) (tobari.ServiceExposure, error) {
	p.events = append(p.events, "request")
	return p.exposure, nil
}
func (p *cliServiceExposurePort) AttachmentServiceStatus(context.Context) (tobari.ServiceAttachmentStatus, error) {
	return p.attachment, nil
}
func (p *cliServiceExposurePort) StopServiceExposure(_ context.Context, id string) error {
	p.events = append(p.events, "stop:"+id)
	return nil
}
func (p *cliServiceExposurePort) ReviewServiceRequests(context.Context) (tobari.ServiceReviewSnapshot, error) {
	p.reviewCalls++
	return p.review, nil
}
func (p *cliServiceExposurePort) ServiceStatus(context.Context) (tobari.ServiceStatusSnapshot, error) {
	return p.status, nil
}
func (p *cliServiceExposurePort) AllowServiceRequest(_ context.Context, id string) (tobari.ServiceExposure, error) {
	p.events = append(p.events, "allow:"+id)
	return p.exposure, nil
}
func (p *cliServiceExposurePort) DenyServiceRequest(_ context.Context, id string) error {
	p.events = append(p.events, "deny:"+id)
	return nil
}
func (p *cliServiceExposurePort) OpenServiceExposure(_ context.Context, id string) (tobari.ServiceOpenResult, error) {
	p.events = append(p.events, "open:"+id)
	return tobari.ServiceOpenResult{SchemaVersion: 1, ID: id, URL: p.exposure.URL, Outcome: tobari.ServiceOpenRequested}, nil
}

func cliServiceFixture() (*cliServiceExposurePort, tobari.ServiceRequest) {
	request := tobari.ServiceRequest{SchemaVersion: 1, ID: "srq_0123456789abcdef0123456789abcdef", AttachmentID: "att_0123456789abcdef0123456789abcdef", ContextID: "01912345-6789-7abc-8def-0123456789a1", WorkspaceID: "01912345-6789-7abc-8def-0123456789a2", Context: "restricted", ProjectRoot: "/projects/app", TargetPort: 3000, State: tobari.ServiceStatePending}
	url, _ := tobari.ServiceExposureURL(54321, "0123456789abcdef0123456789abcdef")
	exposure := tobari.ServiceExposure{SchemaVersion: 1, ID: "exp_0123456789abcdef0123456789abcdef", RequestID: request.ID, AttachmentID: request.AttachmentID, ContextID: request.ContextID, WorkspaceID: request.WorkspaceID, Context: request.Context, ProjectRoot: request.ProjectRoot, TargetPort: request.TargetPort, HostPort: 54321, URL: url, State: tobari.ServiceStateListening}
	observation := tobari.ServiceOwnerObservation{Scope: tobari.ServiceHostScope, Anchor: strings.Repeat("a", 64), Coverage: tobari.ServiceBoundedWindow, Observation: tobari.ServiceObservationComplete, ObservedOwnerCount: 1}
	return &cliServiceExposurePort{
		exposure:   exposure,
		review:     tobari.ServiceReviewSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observation, Requests: []tobari.ServiceRequest{request}},
		status:     tobari.ServiceStatusSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observation, Requests: []tobari.ServiceRequest{request}, Exposures: []tobari.ServiceExposure{exposure}},
		attachment: tobari.ServiceAttachmentStatus{SchemaVersion: 1, Scope: tobari.ServiceAttachmentScope, AttachmentID: request.AttachmentID, Pending: []tobari.ServicePendingStatus{{TargetPort: request.TargetPort, State: tobari.ServiceStatePending}}, Exposures: []tobari.ServiceExposure{exposure}},
	}, request
}

func serviceCLIForTest(input string, port *cliServiceExposurePort) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(input), out, errOut, DefaultCatalog(), nil)
	command.serviceExposure = serviceexposurecmd.New(port)
	return command, out, errOut
}

func TestServiceCatalogFixesTaskPartitionAndRecursiveReferenceGraph(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{"review services", "service status", "service allow", "service deny", "service open", "service stop"} {
		if _, ok := catalog.Lookup(path); !ok {
			t.Errorf("missing %s", path)
		}
	}
	for _, retired := range []string{"service requests", "list"} {
		if _, ok := catalog.Lookup(retired); ok {
			t.Errorf("retired host path remains: %s", retired)
		}
	}
	helper := catalog.ForProgram(ExposureProgramName)
	for _, path := range []string{ExposureProgramName, "status", "stop", "help"} {
		if _, ok := helper.Lookup(path); !ok {
			t.Errorf("missing helper %s", path)
		}
	}
	if _, ok := helper.Lookup("list"); ok {
		t.Fatal("retired helper list alias remains")
	}
	review, _ := catalog.Lookup("review services")
	if review.Agent.Interactive == nil || review.Agent.Interactive.Confirmation != "explicit_action" || review.Agent.Interactive.SelectionReferenceKind != tobari.ServiceRequestKind {
		t.Fatalf("review workflow = %#v", review.Agent.Interactive)
	}
	status, _ := catalog.Lookup("service status")
	got := status.ProducedRefs()
	want := []ProducedRef{{Kind: tobari.ServiceRequestKind, Field: "requests[].id"}, {Kind: tobari.ServiceExposureKind, Field: "exposures[].id"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status refs = %#v, want %#v", got, want)
	}
}

func TestServiceStatusAndHelperStatusHaveDistinctSchemaOneJSON(t *testing.T) {
	port, _ := cliServiceFixture()
	command, out, _ := serviceCLIForTest("", port)
	if code := runCLI(command, []string{"service", "status", "--format=json"}); code != ExitOK {
		t.Fatalf("host status code=%d output=%s", code, out.String())
	}
	var host map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &host); err != nil || host["service_status"] == nil || host["schema_version"] == nil {
		t.Fatalf("host JSON = %s, %v", out.String(), err)
	}
	helper, helperOut, helperErr := serviceCLIForTest("", port)
	helper.catalog = DefaultCatalog().ForProgram(ExposureProgramName)
	if code := runCLI(helper, []string{"status"}); code != ExitOK {
		t.Fatalf("helper status code=%d output=%s error=%s", code, helperOut.String(), helperErr.String())
	}
	if strings.Contains(helperOut.String(), port.review.Requests[0].ID) {
		t.Fatal("helper pending state exposed host mutation reference")
	}
	if !strings.Contains(helperOut.String(), port.exposure.ID) {
		t.Fatal("helper status omitted exact stop reference")
	}
}

func TestExposureHelperWritesOnlyOneFinalSuccessJSONToStdout(t *testing.T) {
	port, _ := cliServiceFixture()
	helper, out, errOut := serviceCLIForTest("", port)
	helper.catalog = DefaultCatalog().ForProgram(ExposureProgramName)
	if code := runCLI(helper, []string{"3000"}); code != ExitOK {
		t.Fatalf("helper create code=%d output=%s error=%s", code, out.String(), errOut.String())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &document); err != nil || document["schema_version"] == nil || document["exposure"] == nil {
		t.Fatalf("helper stdout=%q err=%v", out.String(), err)
	}
	if strings.Contains(out.String(), "Waiting") || !strings.Contains(errOut.String(), "Waiting for trusted-host review") || strings.Count(strings.TrimSpace(out.String()), "\n") != 0 {
		t.Fatalf("helper stdout=%q stderr=%q", out.String(), errOut.String())
	}
	helper, _, _ = serviceCLIForTest("", port)
	helper.catalog = DefaultCatalog().ForProgram(ExposureProgramName)
	if code := runCLI(helper, []string{"3000", "--format=json"}); code != ExitUsage {
		t.Fatalf("helper accepted undeclared format flag: code=%d", code)
	}
}

func TestServiceReviewJSONIsReadOnlyAndWatchFailsBeforeRead(t *testing.T) {
	port, _ := cliServiceFixture()
	command, out, _ := serviceCLIForTest("", port)
	if code := runCLI(command, []string{"review", "services", "--format=json"}); code != ExitOK {
		t.Fatalf("review JSON code=%d output=%s", code, out.String())
	}
	if port.reviewCalls != 1 || len(port.events) != 0 {
		t.Fatalf("review calls=%d events=%v", port.reviewCalls, port.events)
	}
	port.reviewCalls = 0
	command, _, _ = serviceCLIForTest("", port)
	if code := runCLI(command, []string{"review", "services", "--watch"}); code == ExitOK {
		t.Fatal("redirected watch passed")
	}
	if port.reviewCalls != 0 {
		t.Fatalf("redirected watch read owners %d times", port.reviewCalls)
	}
}

func TestServiceLineFallbackUsesOneFullTokenAsConfirmation(t *testing.T) {
	port, request := cliServiceFixture()
	command, out, _ := serviceCLIForTest("allow\n", port)
	choice, err := selectServiceReviewLine(context.Background(), command, port.review)
	if err != nil || choice.action != "allow" || choice.request.ID != request.ID {
		t.Fatalf("choice=%#v err=%v", choice, err)
	}
	if strings.Contains(out.String(), "Confirm") || strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("redundant confirmation remains: %s", out.String())
	}
	key, err := readSelectorKeyOnce(context.Background(), strings.NewReader("o"))
	if err != nil || key.kind != selectorKeyOpen {
		t.Fatalf("raw open key = %#v %v", key, err)
	}
}

func TestServiceAllowThenOpenSettlesAllowFirstAndNeverReplaysIt(t *testing.T) {
	port, request := cliServiceFixture()
	command, out, _ := serviceCLIForTest("", port)
	code := applyServiceReviewChoice(context.Background(), command, serviceReviewChoice{request: request, action: "open"})
	if code != ExitOK || !reflect.DeepEqual(port.events, []string{"allow:" + request.ID, "open:" + port.exposure.ID}) {
		t.Fatalf("code=%d events=%v output=%s", code, port.events, out.String())
	}
	if !strings.Contains(out.String(), "Workspace service ready") || !strings.Contains(out.String(), "Browser open requested") {
		t.Fatalf("combined output=%s", out.String())
	}
}

func TestServiceDirectJSONActionsConsumeExactRefsWithoutConfirmFlags(t *testing.T) {
	port, request := cliServiceFixture()
	command, out, _ := serviceCLIForTest("", port)
	if code := runCLI(command, []string{"service", "allow", "--id", request.ID, "--format=json"}); code != ExitOK {
		t.Fatalf("allow code=%d output=%s", code, out.String())
	}
	if got := port.events; !reflect.DeepEqual(got, []string{"allow:" + request.ID}) {
		t.Fatalf("events=%v", got)
	}
	if strings.Contains(out.String(), "confirm") {
		t.Fatalf("redundant confirmation output=%s", out.String())
	}
}

func TestEveryServiceMutationHandlerConsumesItsExactPublicReference(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      func(tobari.ServiceRequest, tobari.ServiceExposure) []string
		wantEvent func(tobari.ServiceRequest, tobari.ServiceExposure) string
		wantField string
	}{
		{
			name: "deny",
			args: func(request tobari.ServiceRequest, _ tobari.ServiceExposure) []string {
				return []string{"service", "deny", "--id", request.ID, "--format=json"}
			},
			wantEvent: func(request tobari.ServiceRequest, _ tobari.ServiceExposure) string { return "deny:" + request.ID },
			wantField: "denied",
		},
		{
			name: "open",
			args: func(_ tobari.ServiceRequest, exposure tobari.ServiceExposure) []string {
				return []string{"service", "open", "--id", exposure.ID, "--format=json"}
			},
			wantEvent: func(_ tobari.ServiceRequest, exposure tobari.ServiceExposure) string { return "open:" + exposure.ID },
			wantField: "outcome",
		},
		{
			name: "stop",
			args: func(_ tobari.ServiceRequest, exposure tobari.ServiceExposure) []string {
				return []string{"service", "stop", "--id", exposure.ID, "--format=json"}
			},
			wantEvent: func(_ tobari.ServiceRequest, exposure tobari.ServiceExposure) string { return "stop:" + exposure.ID },
			wantField: "stopped",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			port, request := cliServiceFixture()
			command, out, errOut := serviceCLIForTest("", port)
			if code := runCLI(command, test.args(request, port.exposure)); code != ExitOK {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			if !reflect.DeepEqual(port.events, []string{test.wantEvent(request, port.exposure)}) {
				t.Fatalf("events=%v", port.events)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			var payload map[string]json.RawMessage
			for _, envelope := range []string{"result", "open"} {
				if document[envelope] != nil {
					if err := json.Unmarshal(document[envelope], &payload); err != nil {
						t.Fatal(err)
					}
				}
			}
			if payload[test.wantField] == nil {
				t.Fatalf("%s JSON omitted %q: %s", test.name, test.wantField, out.String())
			}
		})
	}
}

func TestExposureHelperStopConsumesOnlyTheAttachmentLocalReference(t *testing.T) {
	port, _ := cliServiceFixture()
	helper, out, errOut := serviceCLIForTest("", port)
	helper.catalog = DefaultCatalog().ForProgram(ExposureProgramName)
	if code := runCLI(helper, []string{"stop", port.exposure.ID}); code != ExitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !reflect.DeepEqual(port.events, []string{"stop:" + port.exposure.ID}) || !strings.Contains(out.String(), `"stopped":true`) {
		t.Fatalf("events=%v output=%s", port.events, out.String())
	}
}
