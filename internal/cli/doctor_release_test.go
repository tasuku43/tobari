//go:build !tobari_research

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
)

func TestReleaseDoctorCompositionNeverInvokesResearchObservers(t *testing.T) {
	inspector := passingInspector("observed")
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format", "json"}); code != ExitOK {
		t.Fatalf("release doctor code = %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !jsonHasCommonDoctorReport(stdout.Bytes()) {
		t.Fatalf("release doctor JSON is not a complete common report: %s", stdout.Bytes())
	}
	for _, id := range inspector.ids {
		if releaseDoctorResearchID(id) {
			t.Fatalf("release doctor reached research observer %q", id)
		}
	}
	if strings.Contains(strings.ToLower(stdout.String()), "broker") || strings.Contains(strings.ToLower(stdout.String()), "vault") {
		t.Fatalf("release doctor published research state: %q", stdout.String())
	}
}

func releaseDoctorResearchID(id doctor.CheckID) bool {
	switch id {
	case doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths, doctor.CheckIDAuthRootKey,
		doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity,
		doctor.CheckIDAuthProjectHandles:
		return true
	default:
		return false
	}
}

func jsonHasCommonDoctorReport(raw []byte) bool {
	return bytes.Contains(raw, []byte(`"schema_version":1`)) && bytes.Contains(raw, []byte(`"report"`))
}
