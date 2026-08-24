//go:build tobari_dev && tobari_research

package workspaceauthoritystore

import (
	"context"
	"fmt"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// finalAuthDoctorRuntime owns only fixed-root host and Broker observation. The
// adapter keeps complete final-envelope selection and coherent all-Context
// inventory reads on the Store side of the boundary.
type finalAuthDoctorRuntime interface {
	ObserveFinalAuthDoctorRuntimeCheck(context.Context, tobari.WorkspaceAuthorityCollection, bool, []authbroker.ContextStatusObservation, doctor.CheckID) (doctor.Observation, error)
}

// ObserveFinalAuthDoctorCheck observes every final Context twice while holding
// the existing non-creating lifecycle observation. It never decodes a
// predecessor Manifest, selects a default, or reconstructs an authority from a
// name or path.
func (a *FinalContextAuthAdapter) ObserveFinalAuthDoctorCheck(ctx context.Context, selected tobari.WorkspaceAuthorityCollection, id doctor.CheckID) (result doctor.Observation, resultErr error) {
	if a == nil || a.mutator == nil || a.mutator.store == nil || a.broker == nil {
		return result, fmt.Errorf("final Context research authentication doctor adapter is unavailable")
	}
	runtime, ok := a.broker.(finalAuthDoctorRuntime)
	if !ok {
		return result, fmt.Errorf("final Context research authentication doctor observer is unavailable")
	}
	resultErr = a.broker.WithFinalContextAuthObservation(ctx, func(observationContext context.Context) error {
		before, beforePresent, err := a.mutator.store.ReadComplete(observationContext)
		if err != nil {
			return fmt.Errorf("read final authentication authority before doctor observation: %w", err)
		}
		if err := confirmSelectedFinalAuthCollection(selected, before, beforePresent); err != nil {
			return err
		}
		first, err := a.observeCompleteFinalContextInventories(observationContext, before, beforePresent)
		if err != nil {
			return err
		}
		observed, err := runtime.ObserveFinalAuthDoctorRuntimeCheck(observationContext, before, beforePresent, first, id)
		if err != nil {
			return err
		}
		after, afterPresent, err := a.mutator.store.ReadComplete(observationContext)
		if err != nil {
			return fmt.Errorf("read final authentication authority after doctor observation: %w", err)
		}
		if beforePresent != afterPresent || !reflect.DeepEqual(before, after) {
			return fmt.Errorf("final authentication authority changed during doctor observation")
		}
		second, err := a.observeCompleteFinalContextInventories(observationContext, after, afterPresent)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(first, second) {
			return fmt.Errorf("final Context credential inventory changed during doctor observation")
		}
		result = observed
		return nil
	})
	return result, resultErr
}

func confirmSelectedFinalAuthCollection(selected, observed tobari.WorkspaceAuthorityCollection, present bool) error {
	if !present {
		if !reflect.DeepEqual(selected, tobari.WorkspaceAuthorityCollection{}) || !reflect.DeepEqual(observed, tobari.WorkspaceAuthorityCollection{}) {
			return fmt.Errorf("clean final authentication authority differs from the selected collection")
		}
		return nil
	}
	if err := selected.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(selected, observed) {
		return fmt.Errorf("final authentication authority differs from the selected complete collection")
	}
	return nil
}

func (a *FinalContextAuthAdapter) observeCompleteFinalContextInventories(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool) ([]authbroker.ContextStatusObservation, error) {
	if !present {
		return []authbroker.ContextStatusObservation{}, nil
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return nil, err
	}
	result := make([]authbroker.ContextStatusObservation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		contextRef, err := tobari.ContextRef(snapshot.Context.ID)
		if err != nil {
			return nil, err
		}
		authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, contextRef)
		if err != nil {
			return nil, err
		}
		observed, err := a.broker.ObserveFinalContextInventory(ctx, authority)
		if err != nil {
			return nil, err
		}
		if err := observed.ValidateFor(contextRef); err != nil {
			return nil, err
		}
		result = append(result, observed)
	}
	return result, nil
}
