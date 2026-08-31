// Package completioncmd owns validated local candidate discovery for shell
// completion. It exposes no mutation, Docker, network, or arbitrary filesystem
// boundary.
package completioncmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxCandidates     = 8192
	maxCandidateBytes = 256 * 1024
)

type CandidateKind string

const (
	CandidateManifestName          CandidateKind = "manifest_name"
	CandidateRuntimeName           CandidateKind = "runtime_name"
	CandidateManagedRuntimeName    CandidateKind = "managed_runtime_name"
	CandidateReadyRuntimeReference CandidateKind = "ready_runtime_reference"
)

type RuntimePort interface {
	ListContexts(context.Context) (tobari.ManifestListResult, error)
	ListRuntimes(context.Context) (tobari.RuntimeListResult, error)
	RuntimeHistory(context.Context, string) (tobari.RuntimeReport, error)
}

type Service struct {
	runtime RuntimePort
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime}
}

func (s *Service) Candidates(ctx context.Context, kind CandidateKind) ([]string, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return nil, fault.New(fault.KindInternal, "missing_runtime", "Completion candidate discovery is not configured", false)
	}
	var values []string
	var err error
	switch kind {
	case CandidateManifestName:
		values, err = s.contextNames(ctx)
	case CandidateRuntimeName:
		values, err = s.runtimeNames(ctx, false)
	case CandidateManagedRuntimeName:
		values, err = s.runtimeNames(ctx, true)
	case CandidateReadyRuntimeReference:
		values, err = s.readyRuntimeReferences(ctx)
	default:
		return nil, fault.New(fault.KindContract, "invalid_completion_candidates", "Completion candidate kind is invalid", false)
	}
	if err != nil {
		return nil, err
	}
	if err := validateCandidates(values); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_completion_candidates", "Completion candidates are invalid", false, err)
	}
	return values, nil
}

func (s *Service) contextNames(ctx context.Context) ([]string, error) {
	result, err := s.runtime.ListContexts(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fault.Wrap(fault.KindInternal, "completion_template_read_failed", "Workspace Template completion candidates could not be read", false, err)
	}
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_completion_candidates", "Workspace Manifest completion candidates are invalid", false, err)
	}
	if result.ManifestState == tobari.ManifestObservationAbsent {
		return []string{}, nil
	}
	values := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		values = append(values, item.Name)
	}
	return values, nil
}

func (s *Service) runtimeNames(ctx context.Context, managedOnly bool) ([]string, error) {
	result, err := s.listRuntimes(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if managedOnly && item.Kind != tobari.RuntimeKindManaged {
			continue
		}
		values = append(values, item.Name)
	}
	return values, nil
}

func (s *Service) readyRuntimeReferences(ctx context.Context) ([]string, error) {
	result, err := s.listRuntimes(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if !item.Ready {
			continue
		}
		if item.Kind == tobari.RuntimeKindBuiltin {
			values = append(values, tobari.StandardRuntimeName)
			continue
		}
		history, historyErr := s.runtime.RuntimeHistory(ctx, item.Name)
		if historyErr != nil {
			if errors.Is(historyErr, context.Canceled) || errors.Is(historyErr, context.DeadlineExceeded) {
				return nil, historyErr
			}
			return nil, fault.Wrap(fault.KindInternal, "completion_runtime_read_failed", "Runtime completion history could not be read", false, historyErr)
		}
		if err := history.Validate(); err != nil || history.Task != tobari.TaskRuntimeHistory || history.Runtime.Name != item.Name {
			if err == nil {
				err = fmt.Errorf("Runtime history does not match its catalog item")
			}
			return nil, fault.Wrap(fault.KindContract, "invalid_completion_candidates", "Runtime completion history is invalid", false, err)
		}
		for index := len(history.Runtime.Revisions) - 1; index >= 0; index-- {
			values = append(values, fmt.Sprintf("%s@%d", item.Name, history.Runtime.Revisions[index].Ordinal))
		}
	}
	return values, nil
}

func (s *Service) listRuntimes(ctx context.Context) (tobari.RuntimeListResult, error) {
	result, err := s.runtime.ListRuntimes(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.RuntimeListResult{}, err
		}
		return tobari.RuntimeListResult{}, fault.Wrap(fault.KindInternal, "completion_runtime_read_failed", "Runtime completion candidates could not be read", false, err)
	}
	if err := result.Validate(); err != nil {
		return tobari.RuntimeListResult{}, fault.Wrap(fault.KindContract, "invalid_completion_candidates", "Runtime completion candidates are invalid", false, err)
	}
	return result, nil
}

func validateCandidates(values []string) error {
	if values == nil {
		return fmt.Errorf("candidate collection is unknown")
	}
	if len(values) > maxCandidates {
		return fmt.Errorf("candidate count exceeds %d", maxCandidates)
	}
	seen := make(map[string]struct{}, len(values))
	total := 0
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("candidate is empty")
		}
		for _, r := range value {
			if r == '\t' || r == '\n' || r == '\r' || r == 0 || r == '\u2028' || r == '\u2029' {
				return fmt.Errorf("candidate contains unsafe structure")
			}
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("candidate %q is duplicated", value)
		}
		seen[value] = struct{}{}
		total += len(value)
		if total > maxCandidateBytes {
			return fmt.Errorf("candidate bytes exceed %d", maxCandidateBytes)
		}
	}
	return nil
}
