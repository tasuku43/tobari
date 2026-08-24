package workspaceauthoritycmd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

type workspaceReadinessFixture struct {
	observations map[doctor.CheckID]doctor.Observation
	errors       map[doctor.CheckID]error
	calls        []doctor.CheckID
}

func (f *workspaceReadinessFixture) ObserveDoctorCheck(_ context.Context, root string, id doctor.CheckID) (doctor.Observation, error) {
	if root != "" {
		return doctor.Observation{}, errors.New("readiness received a project selector")
	}
	f.calls = append(f.calls, id)
	if err := f.errors[id]; err != nil {
		return doctor.Observation{}, err
	}
	if observed, ok := f.observations[id]; ok {
		return observed, nil
	}
	value := ""
	if id == doctor.CheckIDDockerEngine {
		value = "24.0.0"
	}
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "ready", Value: value}, nil
}

func TestWorkspaceEntryReadinessUsesOnlyClosedGenericDockerProfile(t *testing.T) {
	fixture := &workspaceReadinessFixture{}
	if err := NewWorkspaceEntryReadinessService(fixture).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	want, _ := doctor.ReadinessChecks(doctor.ReadinessProfileWorkspaceStart)
	if !reflect.DeepEqual(fixture.calls, want) {
		t.Fatalf("readiness checks = %v, want %v", fixture.calls, want)
	}
}

func TestWorkspaceEntryReadinessFailsClosedWithDoctorRecovery(t *testing.T) {
	for _, test := range []struct {
		name  string
		id    doctor.CheckID
		value doctor.Observation
		err   error
		code  string
	}{
		{name: "engine stopped", id: doctor.CheckIDDockerEngine, value: doctor.Observation{Status: doctor.CheckStatusFail, Detail: "stopped"}, code: "docker_engine_unavailable"},
		{name: "engine incompatible", id: doctor.CheckIDDockerEngine, value: doctor.Observation{Status: doctor.CheckStatusPass, Detail: "old", Value: "23.0.0"}, code: "docker_engine_incompatible"},
		{name: "context observation failed", id: doctor.CheckIDDockerContext, err: errors.New("unavailable"), code: "docker_context_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := &workspaceReadinessFixture{observations: map[doctor.CheckID]doctor.Observation{}, errors: map[doctor.CheckID]error{}}
			fixture.observations[test.id] = test.value
			fixture.errors[test.id] = test.err
			err := NewWorkspaceEntryReadinessService(fixture).Check(context.Background())
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || public.ChangeState != fault.ChangeNone || len(public.NextActions) != 1 || public.NextActions[0].Command != "doctor" {
				t.Fatalf("readiness fault = %#v, ok=%v", public, ok)
			}
		})
	}
}
