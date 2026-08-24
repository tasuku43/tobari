package workspaceauthoritystore

import (
	"context"
	"fmt"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterReadRuntime interface {
	ObserveFinalCluster(context.Context, tobari.WorkspaceAuthorityCollection, bool) (tobari.FinalClusterStatus, error)
	ReadFinalClusterLogs(context.Context, tobari.LogRequest) ([]byte, error)
	ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error)
}

// ClusterReadAdapter owns the final-envelope and selected-component fences for
// bounded cluster observations. The runtime never receives a Store path or
// predecessor State and cannot rediscover one.
type ClusterReadAdapter struct {
	store   *Store
	runtime finalClusterReadRuntime
}

func NewClusterReadAdapter(store *Store, runtime any) (*ClusterReadAdapter, error) {
	reader, ok := runtime.(finalClusterReadRuntime)
	if store == nil || !ok {
		return nil, fmt.Errorf("final cluster read authority is unavailable")
	}
	return &ClusterReadAdapter{store: store, runtime: reader}, nil
}

func (a *ClusterReadAdapter) ReadLogs(ctx context.Context, request tobari.LogRequest) ([]byte, error) {
	if err := request.ValidateCluster(); err != nil {
		return nil, err
	}
	collection, present, before, err := a.begin(ctx)
	if err != nil {
		return nil, err
	}
	output, err := a.runtime.ReadFinalClusterLogs(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := a.finish(ctx, collection, present, before); err != nil {
		return nil, err
	}
	return append([]byte{}, output...), nil
}

func (a *ClusterReadAdapter) ReadDenials(ctx context.Context, tail int) (tobari.FinalClusterDenialWindow, error) {
	collection, present, before, err := a.begin(ctx)
	if err != nil {
		return tobari.FinalClusterDenialWindow{}, err
	}
	read, err := a.runtime.ReadFinalClusterDenials(ctx, tail)
	if err != nil {
		return tobari.FinalClusterDenialWindow{}, err
	}
	if err := a.finish(ctx, collection, present, before); err != nil {
		return tobari.FinalClusterDenialWindow{}, err
	}
	return tobari.NewFinalClusterDenialWindow(collection, tail, read)
}

func (a *ClusterReadAdapter) begin(ctx context.Context) (tobari.WorkspaceAuthorityCollection, bool, tobari.FinalClusterStatus, error) {
	if a == nil || a.store == nil || a.runtime == nil {
		return tobari.WorkspaceAuthorityCollection{}, false, tobari.FinalClusterStatus{}, fmt.Errorf("final cluster read adapter is unavailable")
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, tobari.FinalClusterStatus{}, err
	}
	status, err := a.runtime.ObserveFinalCluster(ctx, collection, present)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, tobari.FinalClusterStatus{}, err
	}
	if err := status.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, tobari.FinalClusterStatus{}, err
	}
	if !present || status.Runtime != tobari.FinalClusterRuntimeRunning || status.Receipt != tobari.FinalClusterReceiptActive {
		return tobari.WorkspaceAuthorityCollection{}, false, tobari.FinalClusterStatus{}, tobari.ErrFinalClusterNotRunning
	}
	return collection, present, status, nil
}

func (a *ClusterReadAdapter) finish(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool, before tobari.FinalClusterStatus) error {
	if err := a.store.ConfirmSelected(ctx, collection, present); err != nil {
		return fmt.Errorf("%w: %v", tobari.ErrFinalClusterObservationChanged, err)
	}
	after, err := a.runtime.ObserveFinalCluster(ctx, collection, present)
	if err != nil {
		return err
	}
	if err := after.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return tobari.ErrFinalClusterObservationChanged
	}
	return nil
}
