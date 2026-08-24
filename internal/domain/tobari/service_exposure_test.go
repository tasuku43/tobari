package tobari

import "testing"

func serviceRequestFixture() ServiceRequest {
	return ServiceRequest{SchemaVersion: 1, ID: "srq_0123456789abcdef0123456789abcdef", AttachmentID: "att_0123456789abcdef0123456789abcdef", ContextID: "01912345-6789-7abc-8def-0123456789a1", WorkspaceID: "01912345-6789-7abc-8def-0123456789a2", Context: "restricted", ProjectRoot: "/projects/app", TargetPort: 3000, State: ServiceStatePending}
}

func serviceExposureFixture() ServiceExposure {
	request := serviceRequestFixture()
	url, _ := ServiceExposureURL(54321, "0123456789abcdef0123456789abcdef")
	return ServiceExposure{SchemaVersion: 1, ID: "exp_0123456789abcdef0123456789abcdef", RequestID: request.ID, AttachmentID: request.AttachmentID, ContextID: request.ContextID, WorkspaceID: request.WorkspaceID, Context: request.Context, ProjectRoot: request.ProjectRoot, TargetPort: request.TargetPort, HostPort: 54321, URL: url, State: ServiceStateListening}
}

func TestServiceExposureBindsFinalIdentityOriginAndAttachment(t *testing.T) {
	fixture := serviceExposureFixture()
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ServiceExposure){
		"context":   func(value *ServiceExposure) { value.ContextID = "" },
		"workspace": func(value *ServiceExposure) { value.WorkspaceID = "" },
		"authority": func(value *ServiceExposure) { value.URL = "http://localhost:54321/" },
		"target":    func(value *ServiceExposure) { value.TargetPort = 80 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := fixture
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid exposure passed")
			}
		})
	}
}

func TestServiceExposureURLRequiresExactIndependentOrigin(t *testing.T) {
	valid, err := ServiceExposureURL(54321, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if label, port, err := ParseServiceExposureURL(valid); err != nil || label != "0123456789abcdef0123456789abcdef" || port != 54321 {
		t.Fatalf("parse = %q %d %v", label, port, err)
	}
	for _, invalid := range []string{"http://127.0.0.1:54321/", "http://localhost:54321/", "http://svc-0123456789abcdef0123456789abcdef.localhost:54321/path", "http://svc-0123456789abcdef0123456789abcdef.localhost:54321/?x=1", "http://svc-ABCDEF0123456789abcdef0123456789.localhost:54321/"} {
		if _, _, err := ParseServiceExposureURL(invalid); err == nil {
			t.Errorf("invalid URL passed: %s", invalid)
		}
	}
}

func TestServiceSnapshotsPreserveBoundedObservationAndDistinctScopes(t *testing.T) {
	request, exposure := serviceRequestFixture(), serviceExposureFixture()
	observation := ServiceOwnerObservation{Scope: ServiceHostScope, Anchor: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Coverage: ServiceBoundedWindow, Observation: ServiceObservationPartial, ObservedOwnerCount: 1, UnavailableOwnerCount: 1}
	review := ServiceReviewSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observation, Requests: []ServiceRequest{request}}
	status := ServiceStatusSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observation, Requests: []ServiceRequest{request}, Exposures: []ServiceExposure{exposure}}
	if err := review.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	helper := ServiceAttachmentStatus{SchemaVersion: 1, Scope: ServiceAttachmentScope, AttachmentID: request.AttachmentID, Pending: []ServicePendingStatus{{TargetPort: request.TargetPort, State: ServiceStatePending}}, Exposures: []ServiceExposure{exposure}}
	if err := helper.Validate(); err != nil {
		t.Fatal(err)
	}
	summary, err := status.SummaryFor(ContextID(request.ContextID), WorkspaceID(request.WorkspaceID))
	if err != nil || summary.PendingCount != 1 || summary.ActiveCount != 1 || !summary.Attention {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
}

func TestServiceObservationRejectsSilentUnavailableOwners(t *testing.T) {
	base := ServiceOwnerObservation{Scope: ServiceHostScope, Anchor: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Coverage: ServiceBoundedWindow, Observation: ServiceObservationComplete}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.UnavailableOwnerCount = 1
	if err := base.Validate(); err == nil {
		t.Fatal("complete observation hid unavailable owner")
	}
	base.Observation, base.ObservedOwnerCount = ServiceObservationPartial, 1
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceOpenAndCleanupResultsAreBounded(t *testing.T) {
	exposure := serviceExposureFixture()
	for _, outcome := range []ServiceOpenOutcome{ServiceOpenNotDispatched, ServiceOpenRequested, ServiceOpenOutcomeUnknown} {
		if err := (ServiceOpenResult{SchemaVersion: 1, ID: exposure.ID, URL: exposure.URL, Outcome: outcome}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := (ServiceCleanupReceipt{SchemaVersion: 1, PendingWithdrawnCount: 1, ExposureClosedCount: 1, StreamClosedCount: 2}).Validate(); err != nil {
		t.Fatal(err)
	}
}
