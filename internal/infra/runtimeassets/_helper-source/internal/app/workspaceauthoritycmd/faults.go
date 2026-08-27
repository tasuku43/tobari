package workspaceauthoritycmd

import (
	"context"
	"errors"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
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

func unclassifiedMutationFault(message string, err error) error {
	return fault.WithClassification(fault.Wrap(fault.KindContract, "unclassified_mutation_outcome", message, false, err), fault.PhaseMutation, fault.ChangeUnknown)
}

func readFault(err error, code, message string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	if errors.Is(err, tobari.ErrFinalAuthorityMigrationRequired) {
		return finalAuthorityMigrationRequiredFault(err, fault.PhaseObservation, fault.ChangeNotApplicable)
	}
	if isPreReleaseLegacyAuthority(err) {
		return preReleaseLegacyFault(err, fault.PhaseObservation, fault.ChangeNotApplicable)
	}
	return fault.WithClassification(fault.Wrap(fault.KindUnavailable, code, message, false, err), fault.PhaseObservation, fault.ChangeNotApplicable)
}

func preReleaseLegacyMutationFault(err error) (error, bool) {
	if errors.Is(err, tobari.ErrFinalAuthorityMigrationRequired) {
		return finalAuthorityMigrationRequiredFault(err, fault.PhasePrecondition, fault.ChangeNone), true
	}
	if !isPreReleaseLegacyAuthority(err) {
		return nil, false
	}
	return preReleaseLegacyFault(err, fault.PhasePrecondition, fault.ChangeNone), true
}

func finalAuthorityMigrationRequiredFault(err error, phase fault.Phase, change fault.ChangeState) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindRejected,
		"installation_migration_required",
		"The supported typed authority.json must be explicitly reviewed and migrated before active authority can be used.",
		false,
		err,
		fault.NextAction{Command: "installation migration plan", Reason: "Create a fresh read-only stale-bound migration plan."},
	), phase, change)
}

func finalAuthorityMutationRecoveryFault(err error) (error, bool) {
	if !errors.Is(err, tobari.ErrFinalAuthorityMutationRecoveryRequired) {
		return nil, false
	}
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"final_authority_mutation_recovery_required",
		"A preserved final-authority mutation must be recovered through its exact initiating command before another mutation; do not remove authority files manually.",
		false,
		err,
		fault.NextAction{Command: "status", Reason: "Read the preserved final-authority decision and follow the safe recovery command."},
	), fault.PhasePrecondition, fault.ChangeNone), true
}

func isPreReleaseLegacyAuthority(err error) bool {
	return errors.Is(err, tobari.ErrPreReleaseLegacyAuthority)
}

func preReleaseLegacyFault(err error, phase fault.Phase, change fault.ChangeState) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindRejected,
		"legacy_state_present",
		"Unsupported pre-release installation authority is present or unsafe; final authority was not used or changed.",
		false,
		err,
		fault.NextAction{Command: "help", Reason: "Follow the documented pre-release reset/recreate procedure; Tobari does not migrate or adopt development state."},
	), phase, change)
}
