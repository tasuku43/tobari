package statuscmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type statusSnapshotPortFixture struct {
	observation tobari.StatusHomeObservation
	err         error
	calls       int
}

func (f *statusSnapshotPortFixture) ObserveStatusHome(context.Context) (tobari.StatusHomeObservation, error) {
	f.calls++
	return f.observation, f.err
}

func TestSnapshotOwnsOneAggregatePortCall(t *testing.T) {
	port := &statusSnapshotPortFixture{observation: tobari.StatusHomeObservation{ProjectRoot: "/workspace/example"}}
	result, err := New(port).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || result.Task != tobari.TaskStatusHome || result.ProjectRoot != "/workspace/example" {
		t.Fatalf("calls=%d snapshot=%+v", port.calls, result)
	}
}

func TestSnapshotFailsClosedWithoutReconstructingInvalidEvidence(t *testing.T) {
	for _, test := range []struct {
		name string
		port *statusSnapshotPortFixture
		code string
	}{
		{name: "observation", port: &statusSnapshotPortFixture{err: errors.New("changed")}, code: "status_observation_failed"},
		{name: "integrity", port: &statusSnapshotPortFixture{observation: tobari.StatusHomeObservation{ProjectRoot: "relative"}}, code: "invalid_status_snapshot"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.port).Snapshot(context.Background())
			var classified *fault.Error
			if err == nil || !errors.As(err, &classified) || classified.Code != test.code || test.port.calls != 1 {
				t.Fatalf("calls=%d fault=%+v err=%v", test.port.calls, classified, err)
			}
		})
	}
}

func TestSnapshotPreservesAuthorityRecoveryClassification(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		code      string
		next      string
		retryable bool
	}{
		{name: "supported migration", err: tobari.ErrFinalAuthorityMigrationRequired, code: "installation_migration_required", next: "installation migration plan"},
		{name: "unsupported legacy", err: tobari.ErrPreReleaseLegacyAuthority, code: "legacy_state_present", next: "doctor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			port := &statusSnapshotPortFixture{err: test.err}
			_, err := New(port).Snapshot(context.Background())
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || public.Kind != fault.KindRejected ||
				public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable ||
				public.Retryable != test.retryable || len(public.NextActions) != 1 ||
				public.NextActions[0].Command != test.next || port.calls != 1 {
				t.Fatalf("fault=%#v ok=%t calls=%d", public, ok, port.calls)
			}
		})
	}
}
