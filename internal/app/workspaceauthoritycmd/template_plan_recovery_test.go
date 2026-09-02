package workspaceauthoritycmd

import (
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestTemplatePlanSourceFaultsRecoverThroughReferencePreservingList(t *testing.T) {
	for name, cause := range map[string]error{
		"missing":  tobari.ErrResourceSourceMissing,
		"invalid":  tobari.ErrResourceSourceInvalid,
		"modified": tobari.ErrResourceSourceModified,
	} {
		t.Run(name, func(t *testing.T) {
			public, ok := fault.PublicCopy(templatePlanFault(errors.Join(cause, errors.New("synthetic private detail"))))
			if !ok || len(public.NextActions) != 1 || public.NextActions[0].Command != "template list" {
				t.Fatalf("Template plan source fault = %#v, ok=%t", public, ok)
			}
		})
	}
}

func TestDeletedTemplateIdentityHasTypedPlanAndApplyFaults(t *testing.T) {
	planned, ok := fault.PublicCopy(templatePlanFault(tobari.ErrResourceIdentityDeleted))
	if !ok || planned.Code != "template_not_found" || planned.Phase != fault.PhaseObservation || planned.ChangeState != fault.ChangeNotApplicable ||
		len(planned.NextActions) != 1 || planned.NextActions[0].Command != "template list" {
		t.Fatalf("deleted Template plan fault = %#v, ok=%t", planned, ok)
	}
	applied, ok := fault.PublicCopy(templateMutationFault(tobari.ErrResourceIdentityDeleted))
	if !ok || applied.Code != "template_not_found" || applied.Phase != fault.PhasePrecondition || applied.ChangeState != fault.ChangeNone ||
		len(applied.NextActions) != 1 || applied.NextActions[0].Command != "template list" {
		t.Fatalf("deleted Template apply fault = %#v, ok=%t", applied, ok)
	}
}
