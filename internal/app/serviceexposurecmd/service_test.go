package serviceexposurecmd

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type servicePortStub struct {
	requestCalls int
	allowID      string
	denyID       string
	stopID       string
	exposure     tobari.ServiceExposure
}

func (p *servicePortStub) RequestService(context.Context, int) (tobari.ServiceExposure, error) {
	p.requestCalls++
	return p.exposure, nil
}
func (p *servicePortStub) ListServiceExposures(context.Context) (tobari.ServiceExposureList, error) {
	return tobari.ServiceExposureList{AttachmentID: p.exposure.AttachmentID, Exposures: []tobari.ServiceExposure{p.exposure}}, nil
}
func (p *servicePortStub) StopServiceExposure(_ context.Context, id string) error {
	p.stopID = id
	return nil
}
func (p *servicePortStub) ListServiceRequests(context.Context) (tobari.ServiceRequestList, error) {
	return tobari.ServiceRequestList{Scope: "live_attachments", Requests: []tobari.ServiceRequest{}}, nil
}
func (p *servicePortStub) AllowServiceRequest(_ context.Context, id string) (tobari.ServiceExposure, error) {
	p.allowID = id
	return p.exposure, nil
}
func (p *servicePortStub) DenyServiceRequest(_ context.Context, id string) error {
	p.denyID = id
	return nil
}

func serviceExposureFixture() tobari.ServiceExposure {
	return tobari.ServiceExposure{SchemaVersion: 1, ID: "exp_0123456789abcdef0123456789abcdef", RequestID: "srq_0123456789abcdef0123456789abcdef", AttachmentID: "att_0123456789abcdef0123456789abcdef", ProjectID: "01234567-89ab-7cde-8f01-23456789abcd", ContextID: "fedcba98-7654-7321-8abc-def012345678", Workspace: "/tmp/project", TargetPort: 3000, HostPort: 54321, URL: "http://127.0.0.1:54321", State: tobari.ServiceStateListening}
}

func serviceIntent(command string, effect operation.Effect, access operation.Declaration) operation.Intent {
	return operation.Intent{Command: command, Effect: effect, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: access, Destructive: operation.DeclarationNo}}
}

func TestServiceUsesFixedCreateAndOpaqueReferenceMutationBindings(t *testing.T) {
	port := &servicePortStub{exposure: serviceExposureFixture()}
	service := New(port)
	if _, err := service.Request(context.Background(), serviceIntent("tobari-expose", operation.EffectCreate, operation.DeclarationNo), 3000); err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if port.requestCalls != 1 {
		t.Fatalf("request calls = %d", port.requestCalls)
	}
	id := port.exposure.RequestID
	if _, err := service.Allow(context.Background(), serviceIntent("service allow", operation.EffectCreate, operation.DeclarationYes), id); err != nil || port.allowID != id {
		t.Fatalf("Allow() error = %v, id = %q", err, port.allowID)
	}
	if err := service.Deny(context.Background(), serviceIntent("service deny", operation.EffectWrite, operation.DeclarationNo), id); err != nil || port.denyID != id {
		t.Fatalf("Deny() error = %v, id = %q", err, port.denyID)
	}
	if err := service.Stop(context.Background(), serviceIntent("stop", operation.EffectWrite, operation.DeclarationYes), port.exposure.ID); err != nil || port.stopID != port.exposure.ID {
		t.Fatalf("Stop() error = %v, id = %q", err, port.stopID)
	}
}

func TestServiceRejectsInvalidPortAndReferenceBeforeAdapter(t *testing.T) {
	port := &servicePortStub{exposure: serviceExposureFixture()}
	service := New(port)
	if _, err := service.Request(context.Background(), serviceIntent("tobari-expose", operation.EffectCreate, operation.DeclarationNo), 80); err == nil || port.requestCalls != 0 {
		t.Fatalf("invalid request error = %v, calls = %d", err, port.requestCalls)
	}
	if err := service.Stop(context.Background(), serviceIntent("stop", operation.EffectWrite, operation.DeclarationYes), "exp_invalid"); err == nil || port.stopID != "" {
		t.Fatalf("invalid stop error = %v, id = %q", err, port.stopID)
	}
}
