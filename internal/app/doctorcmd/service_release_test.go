//go:build !tobari_research

package doctorcmd

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

type releaseDoctorInspector struct {
	calls []doctor.CheckID
}

func (i *releaseDoctorInspector) ObserveDoctorCheck(_ context.Context, _ string, id doctor.CheckID) (doctor.Observation, error) {
	i.calls = append(i.calls, id)
	if isResearchDoctorCheck(id) {
		return doctor.Observation{Status: doctor.CheckStatusFail, Detail: "research check must not be observed on release surface"}, nil
	}
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "observed"}, nil
}

func isResearchDoctorCheck(id doctor.CheckID) bool {
	switch id {
	case doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths, doctor.CheckIDAuthRootKey,
		doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity,
		doctor.CheckIDAuthProjectHandles:
		return true
	default:
		return false
	}
}

func TestReleaseDoctorUsesOnlyCommonInventoryBeforeInspection(t *testing.T) {
	inspector := &releaseDoctorInspector{}
	report, err := New(inspector).Run(context.Background(), operation.Intent{Command: "doctor", Effect: operation.EffectRead}, ".")
	if err != nil {
		t.Fatalf("release doctor: %v", err)
	}
	if len(report.Checks) != len(doctor.CheckInventory()) {
		t.Fatalf("report checks = %d, inventory = %d", len(report.Checks), len(doctor.CheckInventory()))
	}
	for _, id := range inspector.calls {
		if isResearchDoctorCheck(id) {
			t.Fatalf("release doctor observed research check %q", id)
		}
	}
	for _, id := range []doctor.CheckID{doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths, doctor.CheckIDAuthRootKey, doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity, doctor.CheckIDAuthProjectHandles} {
		for _, called := range inspector.calls {
			if called == id {
				t.Fatalf("release doctor called forbidden research observer %q", id)
			}
		}
	}
}
