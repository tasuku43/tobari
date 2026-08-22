package tobaricmd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

type readinessRuntime struct {
	fakeRuntime
	observations map[doctor.CheckID]doctor.Observation
	observeErr   map[doctor.CheckID]error
	checks       []doctor.CheckID
}

func (r *readinessRuntime) ObserveDoctorCheck(
	_ context.Context, _ string, id doctor.CheckID,
) (doctor.Observation, error) {
	r.checks = append(r.checks, id)
	if err := r.observeErr[id]; err != nil {
		return doctor.Observation{}, err
	}
	if observation, ok := r.observations[id]; ok {
		return observation, nil
	}
	result := doctor.Observation{Status: doctor.CheckStatusPass, Detail: "available"}
	if id == doctor.CheckIDDockerEngine {
		result.Detail, result.Value = "24.0.0", "24.0.0"
	}
	return result, nil
}

func TestWorkspaceStartReadinessIsClosedAndItsReceiptAvoidsDuplicatePreflight(t *testing.T) {
	runtime := &readinessRuntime{fakeRuntime: fakeRuntime{state: testState(t.TempDir())}}
	service := New(runtime)
	readyContext, err := service.CheckWorkspaceStartPrerequisites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantChecks, _ := doctor.ReadinessChecks(doctor.ReadinessProfileWorkspaceStart)
	if !reflect.DeepEqual(runtime.checks, wantChecks) {
		t.Fatalf("readiness checks = %v, want %v", runtime.checks, wantChecks)
	}
	if _, err := service.ClusterUp(readyContext, createIntent("cluster up")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runtime.checks, wantChecks) || runtime.clusterCalls != 1 {
		t.Fatalf("composed readiness/cluster calls = %v/%d", runtime.checks, runtime.clusterCalls)
	}
}

func TestWorkspaceStartReadinessEnforcesEngine24BeforeMutation(t *testing.T) {
	for name, version := range map[string]string{"below": "23.0.6", "invalid": "provider-output", "boundary": "24.0.0"} {
		t.Run(name, func(t *testing.T) {
			runtime := &readinessRuntime{
				fakeRuntime: fakeRuntime{state: testState(t.TempDir())},
				observations: map[doctor.CheckID]doctor.Observation{
					doctor.CheckIDDockerEngine: {Status: doctor.CheckStatusPass, Detail: version, Value: version},
				},
			}
			_, err := New(runtime).CheckWorkspaceStartPrerequisites(context.Background())
			if version == "24.0.0" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Code != "docker_engine_incompatible" ||
				structured.Phase != fault.PhasePrecondition || structured.ChangeState != fault.ChangeNone || runtime.clusterCalls != 0 {
				t.Fatalf("version fault=%#v cluster_calls=%d", structured, runtime.clusterCalls)
			}
		})
	}
}

func TestWorkspaceStartReadinessStripsObserverCauseAndStopsAtFirstFailure(t *testing.T) {
	canary := "provider-secret-canary"
	runtime := &readinessRuntime{
		fakeRuntime: fakeRuntime{state: testState(t.TempDir())},
		observeErr:  map[doctor.CheckID]error{doctor.CheckIDDockerEngine: errors.New(canary)},
	}
	_, err := New(runtime).CheckWorkspaceStartPrerequisites(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "docker_engine_unavailable" || public.Message == canary ||
		public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("readiness fault = %#v", public)
	}
	if want := []doctor.CheckID{doctor.CheckIDDockerCLI, doctor.CheckIDDockerEngine}; !reflect.DeepEqual(runtime.checks, want) {
		t.Fatalf("checks after failure = %v, want %v", runtime.checks, want)
	}
}
