package workspaceauthoritycmd

import (
	"context"
	"errors"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

func missingPort(owner string) error {
	return fault.WithClassification(
		fault.New(fault.KindInternal, "missing_runtime", owner+" runtime is not configured", false),
		fault.PhasePrecondition, fault.ChangeNone,
	)
}

func invalidFault(code, message string, err error, recovery string) error {
	next := []fault.NextAction{}
	if recovery != "" {
		next = append(next, fault.NextAction{Command: recovery, Reason: "Discover a current exact reference or review valid input."})
	}
	return fault.WithClassification(fault.Wrap(fault.KindInvalidInput, code, message, false, err, next...), fault.PhasePrecondition, fault.ChangeNone)
}

func notFoundFault(code, message, recovery string) error {
	return fault.WithClassification(
		fault.New(fault.KindNotFound, code, message, false, fault.NextAction{Command: recovery, Reason: "Discover current exact authority."}),
		fault.PhaseObservation, fault.ChangeNotApplicable,
	)
}

func contractFault(code, message string, err error) error {
	return fault.WithClassification(fault.Wrap(fault.KindContract, code, message, false, err), fault.PhaseVerification, fault.ChangeUnknown)
}

func readFault(err error, code, message string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	return fault.WithClassification(fault.Wrap(fault.KindUnavailable, code, message, false, err), fault.PhaseObservation, fault.ChangeNotApplicable)
}
