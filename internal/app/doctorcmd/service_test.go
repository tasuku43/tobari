package doctorcmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

type fakeInspector struct {
	err          error
	after        func()
	afterCheck   doctor.CheckID
	observations map[doctor.CheckID]doctor.Observation
	checkCalls   []doctor.CheckID
	roots        []string
}

func (f *fakeInspector) ObserveDoctorCheck(_ context.Context, root string, id doctor.CheckID) (doctor.Observation, error) {
	f.checkCalls = append(f.checkCalls, id)
	f.roots = append(f.roots, root)
	if f.after != nil && (f.afterCheck == "" || f.afterCheck == id) {
		f.after()
	}
	if f.err != nil {
		return doctor.Observation{}, f.err
	}
	if observation, exists := f.observations[id]; exists {
		return observation, nil
	}
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "observed"}, nil
}

func doctorReadIntent() operation.Intent {
	return operation.Intent{Command: "doctor", Effect: operation.EffectRead}
}

func TestRunSchedulesCompleteDependencyMatrix(t *testing.T) {
	tests := map[string]struct {
		observations map[doctor.CheckID]doctor.Observation
		want         map[doctor.CheckID]doctor.CheckStatus
		blockedBy    map[doctor.CheckID]doctor.CheckID
		healthy      bool
	}{
		"docker CLI missing": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDDockerCLI:   {Status: doctor.CheckStatusFail, Detail: "docker missing"},
				doctor.CheckIDRootSharing: {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
			},
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDDockerCLI:           doctor.CheckStatusFail,
				doctor.CheckIDDockerEngine:        doctor.CheckStatusBlocked,
				doctor.CheckIDDockerContext:       doctor.CheckStatusBlocked,
				doctor.CheckIDDockerCompose:       doctor.CheckStatusBlocked,
				doctor.CheckIDPolicy:              doctor.CheckStatusBlocked,
				doctor.CheckIDAuthBroker:          doctor.CheckStatusBlocked,
				doctor.CheckIDCredentialCompanion: doctor.CheckStatusBlocked,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckStatusBlocked,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckStatusBlocked,
				doctor.CheckIDOwnedResources:      doctor.CheckStatusBlocked,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDDockerEngine:        doctor.CheckIDDockerCLI,
				doctor.CheckIDDockerContext:       doctor.CheckIDDockerCLI,
				doctor.CheckIDDockerCompose:       doctor.CheckIDDockerCLI,
				doctor.CheckIDPolicy:              doctor.CheckIDDockerEngine,
				doctor.CheckIDAuthBroker:          doctor.CheckIDDockerEngine,
				doctor.CheckIDCredentialCompanion: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckIDAuthVaultIntegrity,
				doctor.CheckIDOwnedResources:      doctor.CheckIDDockerEngine,
			},
			healthy: false,
		},
		"engine down": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDDockerEngine: {Status: doctor.CheckStatusFail, Detail: "engine unavailable"},
				doctor.CheckIDRootSharing:  {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
			},
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDDockerEngine:        doctor.CheckStatusFail,
				doctor.CheckIDPolicy:              doctor.CheckStatusBlocked,
				doctor.CheckIDAuthBroker:          doctor.CheckStatusBlocked,
				doctor.CheckIDCredentialCompanion: doctor.CheckStatusBlocked,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckStatusBlocked,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckStatusBlocked,
				doctor.CheckIDOwnedResources:      doctor.CheckStatusBlocked,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDPolicy:              doctor.CheckIDDockerEngine,
				doctor.CheckIDAuthBroker:          doctor.CheckIDDockerEngine,
				doctor.CheckIDCredentialCompanion: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckIDAuthVaultIntegrity,
				doctor.CheckIDOwnedResources:      doctor.CheckIDDockerEngine,
			},
			healthy: false,
		},
		"cluster absent": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDRootSharing: {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
				doctor.CheckIDState:       {Status: doctor.CheckStatusWarn, Detail: "cluster is not configured"},
				doctor.CheckIDPolicy:      {Status: doctor.CheckStatusWarn, Detail: "policy is not initialized"},
				doctor.CheckIDPolicyData:  {Status: doctor.CheckStatusWarn, Detail: "policy data is not initialized"},
				doctor.CheckIDAuthRootKey: {Status: doctor.CheckStatusWarn, Detail: "root key is not initialized"},
			},
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDState:               doctor.CheckStatusWarn,
				doctor.CheckIDPolicy:              doctor.CheckStatusWarn,
				doctor.CheckIDPolicyData:          doctor.CheckStatusWarn,
				doctor.CheckIDAuthRootKey:         doctor.CheckStatusWarn,
				doctor.CheckIDAuthBroker:          doctor.CheckStatusBlocked,
				doctor.CheckIDCredentialCompanion: doctor.CheckStatusBlocked,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckStatusBlocked,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckStatusBlocked,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDAuthBroker:          doctor.CheckIDState,
				doctor.CheckIDCredentialCompanion: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckIDAuthVaultIntegrity,
			},
			healthy: true,
		},
		"invalid policy": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDRootSharing: {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
				doctor.CheckIDPolicy:      {Status: doctor.CheckStatusFail, Detail: "policy syntax is invalid"},
			},
			want:    map[doctor.CheckID]doctor.CheckStatus{doctor.CheckIDPolicy: doctor.CheckStatusFail},
			healthy: false,
		},
		"broker locked": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDRootSharing: {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
				doctor.CheckIDAuthBroker:  {Status: doctor.CheckStatusFail, Detail: "broker is locked"},
			},
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDAuthBroker:          doctor.CheckStatusFail,
				doctor.CheckIDCredentialCompanion: doctor.CheckStatusBlocked,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckStatusBlocked,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckStatusBlocked,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDCredentialCompanion: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthVaultIntegrity:  doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles:  doctor.CheckIDAuthVaultIntegrity,
			},
			healthy: false,
		},
		"healthy warnings": {
			observations: map[doctor.CheckID]doctor.Observation{
				doctor.CheckIDRootSharing:        {Status: doctor.CheckStatusWarn, Detail: "bind sharing is confirmed by attach"},
				doctor.CheckIDAuthProjectHandles: {Status: doctor.CheckStatusWarn, Detail: "re-entry required"},
				doctor.CheckIDOwnedResources:     {Status: doctor.CheckStatusWarn, Detail: "owned containers exist"},
			},
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDRootSharing:        doctor.CheckStatusWarn,
				doctor.CheckIDAuthProjectHandles: doctor.CheckStatusWarn,
				doctor.CheckIDOwnedResources:     doctor.CheckStatusWarn,
			},
			healthy: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			inspector := &fakeInspector{observations: test.observations}
			report, err := New(inspector).Run(context.Background(), doctorReadIntent(), ".")
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if len(report.Checks) != len(doctor.CheckInventory()) {
				t.Fatalf("checks = %d, want %d", len(report.Checks), len(doctor.CheckInventory()))
			}
			for _, check := range report.Checks {
				wantStatus := doctor.CheckStatusPass
				if observation, exists := test.observations[check.Name]; exists {
					wantStatus = observation.Status
				}
				if status, exists := test.want[check.Name]; exists {
					wantStatus = status
				}
				if check.Status != wantStatus {
					t.Errorf("%s status = %q, want %q", check.Name, check.Status, wantStatus)
				}
				if blocker, exists := test.blockedBy[check.Name]; exists {
					if check.BlockedBy == nil || *check.BlockedBy != blocker {
						t.Errorf("%s blocked_by = %v, want %q", check.Name, check.BlockedBy, blocker)
					}
					if check.Recovery != nil {
						t.Errorf("%s blocked check duplicated recovery: %+v", check.Name, check.Recovery)
					}
				} else if check.BlockedBy != nil {
					t.Errorf("%s unexpected blocked_by = %q", check.Name, *check.BlockedBy)
				}
			}
			if report.Healthy() != test.healthy {
				t.Errorf("Healthy() = %t, want %t", report.Healthy(), test.healthy)
			}
			if state := findDoctorCheck(t, report, doctor.CheckIDState); state.Status == doctor.CheckStatusWarn {
				if state.Recovery == nil || state.Recovery.NextCommand != "cluster up" {
					t.Errorf("state warning recovery = %+v, want cluster up", state.Recovery)
				}
			}
			if broker := findDoctorCheck(t, report, doctor.CheckIDAuthBroker); broker.Status == doctor.CheckStatusFail {
				if broker.Recovery == nil || broker.Recovery.NextCommand != "cluster up" {
					t.Errorf("auth_broker failure recovery = %+v, want cluster up", broker.Recovery)
				}
			}
			called := make(map[doctor.CheckID]bool, len(inspector.checkCalls))
			for index, id := range inspector.checkCalls {
				called[id] = true
				if inspector.roots[index] != "." {
					t.Errorf("%s root = %q, want .", id, inspector.roots[index])
				}
			}
			for blocked := range test.blockedBy {
				if called[blocked] {
					t.Errorf("blocked check %s called inspector", blocked)
				}
			}
		})
	}
}

func TestEveryObservedFailureReceivesTaskOwnedRecovery(t *testing.T) {
	for _, spec := range doctor.CheckInventory() {
		t.Run(string(spec.ID), func(t *testing.T) {
			inspector := &fakeInspector{observations: map[doctor.CheckID]doctor.Observation{
				spec.ID: {Status: doctor.CheckStatusFail, Detail: "synthetic observed failure"},
			}}
			report, err := New(inspector).Run(context.Background(), doctorReadIntent(), ".")
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			check := findDoctorCheck(t, report, spec.ID)
			if check.Status != doctor.CheckStatusFail || check.Recovery == nil {
				t.Fatalf("check = %+v, want failed with recovery", check)
			}
			if check.Recovery.Action == "" || check.Recovery.NextCommand == "" {
				t.Fatalf("recovery = %+v, want concrete action and exact command", check.Recovery)
			}
		})
	}
}

func findDoctorCheck(t *testing.T, report doctor.Report, id doctor.CheckID) doctor.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == id {
			return check
		}
	}
	t.Fatalf("report lacks %q", id)
	return doctor.Check{}
}

func TestRunReturnsValidatedReport(t *testing.T) {
	inspector := &fakeInspector{}

	report, err := New(inspector).Run(context.Background(), doctorReadIntent(), ".")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(inspector.checkCalls) != len(doctor.CheckInventory()) {
		t.Fatalf("ObserveDoctorCheck() calls = %d, want %d", len(inspector.checkCalls), len(doctor.CheckInventory()))
	}
	if len(report.Checks) != len(doctor.CheckInventory()) || report.Checks[0].Name != doctor.CheckIDDockerCLI {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunRejectsInvalidIntentBeforeInspection(t *testing.T) {
	tests := []operation.Intent{
		{},
		{Command: "doctor", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: "system", ID: "local"}},
		{Command: "version", Effect: operation.EffectRead},
	}
	for _, intent := range tests {
		inspector := &fakeInspector{}
		if _, err := New(inspector).Run(context.Background(), intent, "."); err == nil {
			t.Errorf("Run(%+v) succeeded", intent)
		}
		if len(inspector.checkCalls) != 0 {
			t.Errorf("Run(%+v) inspected %d times", intent, len(inspector.checkCalls))
		}
	}
}

func TestRunFailsClosedForMissingOrInvalidDependencies(t *testing.T) {
	if _, err := New(nil).Run(context.Background(), doctorReadIntent(), "."); err == nil {
		t.Fatal("Run() with nil inspector succeeded")
	}
	if _, err := New((*fakeInspector)(nil)).Run(context.Background(), doctorReadIntent(), "."); err == nil {
		t.Fatal("Run() with typed nil inspector succeeded")
	}

	inspector := &fakeInspector{observations: map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusBlocked, Detail: "invalid infrastructure status"},
	}}
	if _, err := New(inspector).Run(context.Background(), doctorReadIntent(), "."); err == nil {
		t.Fatal("Run() accepted an infrastructure blocked observation")
	}
	if len(inspector.checkCalls) != 1 {
		t.Fatalf("ObserveDoctorCheck() calls = %d, want 1", len(inspector.checkCalls))
	}
}

func TestRunPropagatesInspectorAndContextErrors(t *testing.T) {
	want := errors.New("offline probe failed")
	inspector := &fakeInspector{err: want}
	if _, err := New(inspector).Run(context.Background(), doctorReadIntent(), "."); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inspector = &fakeInspector{}
	if _, err := New(inspector).Run(ctx, doctorReadIntent(), "."); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if len(inspector.checkCalls) != 0 {
		t.Fatalf("canceled Run() inspected %d times", len(inspector.checkCalls))
	}
}

func TestRunSuppressesSuccessWhenCanceledDuringInspection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inspector := &fakeInspector{
		after: cancel,
	}
	report, err := New(inspector).Run(ctx, doctorReadIntent(), ".")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if report.Checks != nil {
		t.Fatalf("Run() report = %+v, want zero report", report)
	}
	if len(inspector.checkCalls) != 1 {
		t.Fatalf("ObserveDoctorCheck() calls = %d, want 1", len(inspector.checkCalls))
	}
}

func TestRunCancelsBeforeMaterializingRemainingBlockedRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inspector := &fakeInspector{
		after: cancel, afterCheck: doctor.CheckIDRoot,
		observations: map[doctor.CheckID]doctor.Observation{
			doctor.CheckIDRoot: {Status: doctor.CheckStatusFail, Detail: "unsafe root"},
		},
	}
	report, err := New(inspector).Run(ctx, doctorReadIntent(), ".")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if report.Checks != nil {
		t.Fatalf("Run() report = %+v, want zero report", report)
	}
	wantCalls := 0
	for _, spec := range doctor.CheckInventory() {
		wantCalls++
		if spec.ID == doctor.CheckIDRoot {
			break
		}
	}
	if len(inspector.checkCalls) != wantCalls {
		t.Fatalf("ObserveDoctorCheck() calls = %v, want through root only", inspector.checkCalls)
	}
}
