package tobari

import "testing"

func serviceExposureFixture() ServiceExposure {
	return ServiceExposure{
		SchemaVersion: ServiceExposureSchema,
		ID:            "exp_0123456789abcdef0123456789abcdef", RequestID: "srq_0123456789abcdef0123456789abcdef",
		AttachmentID: "att_0123456789abcdef0123456789abcdef",
		ProjectID:    "01912345-6789-7abc-8def-0123456789ab", ContextID: "01912345-6789-7abc-8def-0123456789ab",
		Workspace: "/projects/app", TargetPort: 3000, HostPort: 54321,
		URL: "http://127.0.0.1:54321", State: ServiceStateListening,
	}
}

func TestServiceExposureBindsOpaqueIdentityExactAuthorityAndAttachment(t *testing.T) {
	fixture := serviceExposureFixture()
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ServiceExposure){
		"reference":  func(value *ServiceExposure) { value.ID = "exp_ffffffffffffffffffffffffffffffff" },
		"attachment": func(value *ServiceExposure) { value.AttachmentID = "att_ffffffffffffffffffffffffffffffff" },
		"authority":  func(value *ServiceExposure) { value.URL = "http://localhost:54321" },
		"target":     func(value *ServiceExposure) { value.TargetPort = 80 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := fixture
			mutate(&candidate)
			if name == "reference" || name == "attachment" {
				// Fresh opaque identities remain structurally valid; collection and
				// action tests bind their exact bytes and owner relationship.
				if err := candidate.Validate(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid exposure passed")
			}
		})
	}
}

func TestServiceExposureListIsKnownExhaustiveAttachmentScope(t *testing.T) {
	fixture := serviceExposureFixture()
	list := ServiceExposureList{AttachmentID: fixture.AttachmentID, Exposures: []ServiceExposure{fixture}}
	if err := list.Validate(); err != nil {
		t.Fatal(err)
	}
	foreign := fixture
	foreign.AttachmentID = "att_ffffffffffffffffffffffffffffffff"
	list.Exposures = append(list.Exposures, foreign)
	if err := list.Validate(); err == nil {
		t.Fatal("cross-attachment exposure passed")
	}
}
