package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/serviceexposurecmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/systemdoctor"
)

type cliServiceExposurePort struct {
	requestPort int
	stopped     string
	allowed     string
	denied      string
	exposure    tobari.ServiceExposure
	requests    tobari.ServiceRequestList
}

func cliExposureFixture() tobari.ServiceExposure {
	return tobari.ServiceExposure{SchemaVersion: 1, ID: "exp_0123456789abcdef0123456789abcdef", RequestID: "srq_0123456789abcdef0123456789abcdef", AttachmentID: "att_0123456789abcdef0123456789abcdef", ProjectID: "01234567-89ab-7cde-8f01-23456789abcd", ContextID: "fedcba98-7654-7321-8abc-def012345678", Workspace: "/tmp/project", TargetPort: 3000, HostPort: 54321, URL: "http://127.0.0.1:54321", State: tobari.ServiceStateListening}
}

func (p *cliServiceExposurePort) RequestService(_ context.Context, port int) (tobari.ServiceExposure, error) {
	p.requestPort = port
	return p.exposure, nil
}
func (p *cliServiceExposurePort) ListServiceExposures(context.Context) (tobari.ServiceExposureList, error) {
	return tobari.ServiceExposureList{AttachmentID: p.exposure.AttachmentID, Exposures: []tobari.ServiceExposure{p.exposure}}, nil
}
func (p *cliServiceExposurePort) StopServiceExposure(_ context.Context, id string) error {
	p.stopped = id
	return nil
}
func (p *cliServiceExposurePort) ListServiceRequests(context.Context) (tobari.ServiceRequestList, error) {
	return p.requests, nil
}

func (p *cliServiceExposurePort) AllowServiceRequest(_ context.Context, id string) (tobari.ServiceExposure, error) {
	p.allowed = id
	return p.exposure, nil
}
func (p *cliServiceExposurePort) DenyServiceRequest(_ context.Context, id string) error {
	p.denied = id
	return nil
}

func TestServiceExposureCatalogSeparatesProgramsAndClosesReferenceFlows(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	helper := catalog.ForProgram(ExposureProgramName)
	for _, path := range []string{ExposureProgramName, "list", "stop", "help"} {
		if _, found := helper.Lookup(path); !found {
			t.Errorf("helper command %q missing", path)
		}
	}
	if _, found := helper.Lookup("review"); found {
		t.Fatal("host review leaked into Workspace helper")
	}
	if _, found := catalog.Lookup(ExposureProgramName); found {
		t.Fatal("Workspace helper leaked into host routing")
	}
	request, _ := helper.Lookup(ExposureProgramName)
	if request.Effect != operation.EffectCreate || request.Agent.FixedTarget == nil || request.Agent.FixedTarget.Kind != tobari.ServiceAttachmentServicesKind || len(request.ProducedRefs()) != 1 || request.ProducedRefs()[0].Kind != tobari.ServiceExposureKind {
		t.Fatalf("request contract = %+v refs=%+v", request.Agent, request.ProducedRefs())
	}
	stop, _ := helper.Lookup("stop")
	if stop.Agent.Mutation == nil || stop.Agent.Mutation.TargetIDInput != "exposure-ref" || len(stop.ConsumedRefs()) != 1 {
		t.Fatalf("stop contract = %+v", stop.Agent)
	}
	review, _ := catalog.lookupRegistered("review")
	if review.Agent.Interactive == nil || strings.Join(review.Agent.Interactive.ActionCommands, ",") != "service allow,service deny" {
		t.Fatalf("review workflow = %+v", review.Agent.Interactive)
	}
}

func TestExposureHelperPreservesOpaqueReferenceAndDoesNotRouteHostCLI(t *testing.T) {
	exposure := cliExposureFixture()
	port := &cliServiceExposurePort{exposure: exposure, requests: tobari.ServiceRequestList{Scope: "live_attachments", Requests: []tobari.ServiceRequest{}}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog().ForProgram(ExposureProgramName), systemdoctor.New())
	command.serviceExposure = serviceexposurecmd.New(port)
	if code := command.RunContext(context.Background(), []string{"3000"}); code != ExitOK {
		t.Fatalf("request code = %d stderr=%q", code, stderr.String())
	}
	if port.requestPort != 3000 || !strings.Contains(stdout.String(), exposure.ID) || !strings.Contains(stdout.String(), "tobari-expose stop "+exposure.ID) || !strings.Contains(stderr.String(), "tobari review") {
		t.Fatalf("request output=%q stderr=%q port=%d", stdout.String(), stderr.String(), port.requestPort)
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"stop", exposure.ID}); code != ExitOK || port.stopped != exposure.ID {
		t.Fatalf("stop code=%d id=%q stderr=%q", code, port.stopped, stderr.String())
	}
	if code := command.RunContext(context.Background(), []string{"review"}); code != ExitUsage {
		t.Fatalf("host command routed by helper: %d", code)
	}
}

func TestExposureHelperRejectsInvalidPortBeforeChannelCall(t *testing.T) {
	port := &cliServiceExposurePort{exposure: cliExposureFixture()}
	var stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &bytes.Buffer{}, &stderr, DefaultCatalog().ForProgram(ExposureProgramName), systemdoctor.New())
	command.serviceExposure = serviceexposurecmd.New(port)
	if code := command.RunContext(context.Background(), []string{"80"}); code != ExitUsage || port.requestPort != 0 {
		t.Fatalf("invalid port code=%d calls=%d", code, port.requestPort)
	}
	if !strings.Contains(stderr.String(), "tobari-expose help tobari-expose") || strings.Contains(stderr.String(), "tobari help tobari-expose") {
		t.Fatalf("helper recovery=%q", stderr.String())
	}
}

func TestUnifiedServiceReviewUsesFreshOpaqueSelectionAndImmediateDecision(t *testing.T) {
	exposure := cliExposureFixture()
	request := tobari.ServiceRequest{SchemaVersion: 1, ID: exposure.RequestID, AttachmentID: exposure.AttachmentID, ProjectID: exposure.ProjectID, ContextID: exposure.ContextID, Workspace: exposure.Workspace, TargetPort: exposure.TargetPort, State: tobari.ServiceStatePending}
	for _, test := range []struct {
		name, input string
		wantAllow   bool
		wantDeny    bool
	}{
		{name: "allow once", input: "1\na\ny\n", wantAllow: true},
		{name: "deny", input: "1\nd\ny\n", wantDeny: true},
		{name: "back", input: "b\n"},
		{name: "cancel confirmation", input: "1\na\nn\nb\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			port := &cliServiceExposurePort{exposure: exposure, requests: tobari.ServiceRequestList{Scope: "live_attachments", Requests: []tobari.ServiceRequest{request}}}
			var output bytes.Buffer
			command := newCLI(strings.NewReader(test.input), &output, &bytes.Buffer{}, DefaultCatalog(), systemdoctor.New())
			command.serviceExposure = serviceexposurecmd.New(port)
			if code := runServiceReviewLoop(context.Background(), command); code != ExitOK {
				t.Fatalf("review code=%d output=%q", code, output.String())
			}
			if (port.allowed == request.ID) != test.wantAllow || (port.denied == request.ID) != test.wantDeny {
				t.Fatalf("allow=%q deny=%q", port.allowed, port.denied)
			}
			if (test.wantAllow || test.wantDeny) && !strings.Contains(output.String(), request.ID) {
				t.Fatalf("opaque request missing from review: %q", output.String())
			}
		})
	}
}

func TestRedirectedUnifiedReviewIsReadOnlyAndExhaustive(t *testing.T) {
	exposure := cliExposureFixture()
	request := tobari.ServiceRequest{SchemaVersion: 1, ID: exposure.RequestID, AttachmentID: exposure.AttachmentID, ProjectID: exposure.ProjectID, ContextID: exposure.ContextID, Workspace: exposure.Workspace, TargetPort: exposure.TargetPort, State: tobari.ServiceStatePending}
	port := &cliServiceExposurePort{exposure: exposure, requests: tobari.ServiceRequestList{Scope: "live_attachments", Requests: []tobari.ServiceRequest{request}}}
	var output bytes.Buffer
	command := newCLI(strings.NewReader("1\na\ny\n"), &output, &bytes.Buffer{}, DefaultCatalog(), systemdoctor.New())
	command.serviceExposure = serviceexposurecmd.New(port)
	if code := command.RunContext(context.Background(), []string{"review"}); code != ExitOK {
		t.Fatalf("redirected review code=%d", code)
	}
	if port.allowed != "" || port.denied != "" || !strings.Contains(output.String(), request.ID) {
		t.Fatalf("redirected review mutated or omitted request: allow=%q deny=%q output=%q", port.allowed, port.denied, output.String())
	}
}
