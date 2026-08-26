package workspaceauthoritycmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type legacyFinalClusterStatusPort struct {
	err   error
	calls int
}

func (p *legacyFinalClusterStatusPort) Observe(context.Context) (tobari.FinalClusterStatus, error) {
	p.calls++
	return tobari.FinalClusterStatus{}, p.err
}

type legacyFinalClusterReadPort struct {
	err   error
	calls int
}

func (p *legacyFinalClusterReadPort) ReadLogs(context.Context, tobari.LogRequest) ([]byte, error) {
	p.calls++
	return nil, p.err
}

func (p *legacyFinalClusterReadPort) ReadDenials(context.Context, int) (tobari.FinalClusterDenialWindow, error) {
	p.calls++
	return tobari.FinalClusterDenialWindow{}, p.err
}

func TestFinalClusterReadSurfacesClassifiedLegacyStateWithoutRecoveryMutation(t *testing.T) {
	for name, sentinel := range map[string]error{
		"pre-release envelope": tobari.ErrPreReleaseLegacyAuthority,
		"executable policy":    fmt.Errorf("%w: %w", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy),
	} {
		t.Run(name, func(t *testing.T) {
			statusPort := &legacyFinalClusterStatusPort{err: sentinel}
			_, err := NewFinalClusterLifecycleService(statusPort).Status(context.Background())
			assertLegacyObservationFault(t, err, statusPort.calls, 1)

			readPort := &legacyFinalClusterReadPort{err: sentinel}
			service := NewFinalClusterReadService(readPort)
			_, err = service.Logs(context.Background(), tobari.LogRequest{Component: "gateway", Tail: 1})
			assertLegacyObservationFault(t, err, readPort.calls, 1)
			_, err = service.Denials(context.Background(), 1)
			assertLegacyObservationFault(t, err, readPort.calls, 2)
		})
	}
}

func assertLegacyObservationFault(t *testing.T, err error, calls, wantCalls int) {
	t.Helper()
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhaseObservation ||
		public.ChangeState != fault.ChangeNotApplicable || len(public.NextActions) != 1 ||
		public.NextActions[0].Command != "help" || calls != wantCalls {
		t.Fatalf("legacy observation fault=%#v ok=%t calls=%d", public, ok, calls)
	}
}
