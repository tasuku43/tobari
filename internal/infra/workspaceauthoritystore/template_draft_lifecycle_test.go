package workspaceauthoritystore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestTemplateDraftPlansAndPublishesGenerationOneAfterExistingAuthority(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	mutator.runtimeRevision.(*templateRuntimeRevisionFixture).binding = existing.Templates[0].Current.Body.EntryDefaults.Runtime
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789f7")
	source, err := tobari.NewWorkspaceTemplateDraftSource(id, "later", existing.Templates[0].Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.Repeat("7", 64)
	templateRef, err := tobari.WorkspaceTemplateRef(id)
	if err != nil {
		t.Fatal(err)
	}
	load := func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	}

	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, load)
	if err != nil {
		t.Fatalf("plan later draft: %v", err)
	}
	if plan.TemplateRef != templateRef || plan.ActiveRevision != nil || plan.ActiveMetadataRevision != nil || plan.BaseRevision != nil || plan.AffectedContextCount != 0 || plan.RunningWorkspaceCount != 0 {
		t.Fatalf("generation-one plan=%+v", plan)
	}

	publication, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, load)
	if err != nil {
		t.Fatalf("apply later draft: %v", err)
	}
	if !publication.Changed || publication.Template.ID != id || publication.Current.Generation != 1 || publication.Current.TemplateID != id {
		t.Fatalf("generation-one publication=%+v", publication)
	}
	if err := mutator.CompleteWorkspaceTemplateApplySettlement(plan.PlanRef); err != nil {
		t.Fatal(err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(after.Templates) != len(existing.Templates)+1 {
		t.Fatalf("authority after draft apply: present=%t templates=%d err=%v", present, len(after.Templates), err)
	}
	found := false
	for _, template := range after.Templates {
		if template.ID == id {
			found = template.Name == "later" && template.Current.Generation == 1
		}
	}
	if !found {
		t.Fatalf("published draft missing from authority: %+v", after.Templates)
	}
}

func TestTemplateDraftApplyRetainsFinalSourceAndConcurrentPublicationFences(t *testing.T) {
	newFixture := func(t *testing.T) (*Store, *Mutator, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceTemplateID, string) {
		t.Helper()
		existing := storeCollectionFixture(t)
		store, mutator, _, _, _ := newMutationFixture(t, &existing)
		mutator.runtimeRevision.(*templateRuntimeRevisionFixture).binding = existing.Templates[0].Current.Body.EntryDefaults.Runtime
		id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789f9")
		ref, err := tobari.WorkspaceTemplateRef(id)
		if err != nil {
			t.Fatal(err)
		}
		return store, mutator, existing, id, ref
	}

	t.Run("source changes at final fingerprint fence", func(t *testing.T) {
		store, mutator, existing, id, ref := newFixture(t)
		source, err := tobari.NewWorkspaceTemplateDraftSource(id, "later", existing.Templates[0].Current.Body)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint := strings.Repeat("a", 64)
		plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), ref, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return source.Clone(), fingerprint, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		loads := 0
		_, err = mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			loads++
			if loads == 1 {
				return source.Clone(), fingerprint, nil
			}
			return source.Clone(), strings.Repeat("b", 64), nil
		})
		if !errors.Is(err, tobari.ErrResourceSourceChanged) || loads != 2 {
			t.Fatalf("final draft source fence err=%v loads=%d", err, loads)
		}
		after, present, err := store.ReadComplete(context.Background())
		if err != nil || !present || !reflect.DeepEqual(after, existing) {
			t.Fatalf("source drift published draft: present=%t after=%+v err=%v", present, after, err)
		}
	})

	t.Run("another generation one publication wins", func(t *testing.T) {
		store, mutator, existing, id, ref := newFixture(t)
		first, err := tobari.NewWorkspaceTemplateDraftSource(id, "first-choice", existing.Templates[0].Current.Body)
		if err != nil {
			t.Fatal(err)
		}
		second := first.Clone()
		second.Template.Name = "second-choice"
		firstFingerprint, secondFingerprint := strings.Repeat("c", 64), strings.Repeat("d", 64)
		firstPlan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), ref, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return first.Clone(), firstFingerprint, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		secondPlan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), ref, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return second.Clone(), secondFingerprint, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), secondPlan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return second.Clone(), secondFingerprint, nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := mutator.CompleteWorkspaceTemplateApplySettlement(secondPlan.PlanRef); err != nil {
			t.Fatal(err)
		}
		if _, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), firstPlan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return first.Clone(), firstFingerprint, nil
		}); !errors.Is(err, tobari.ErrResourceSourceModified) {
			t.Fatalf("concurrent generation-one publication err=%v", err)
		}
		after, present, err := store.ReadComplete(context.Background())
		if err != nil || !present {
			t.Fatal(err)
		}
		found := false
		for _, template := range after.Templates {
			if template.ID == id {
				found = true
				if template.Name != "second-choice" || template.Current.Generation != 1 {
					t.Fatalf("winning publication changed: %+v", template)
				}
			}
		}
		if !found {
			t.Fatal("winning generation-one publication disappeared")
		}
	})
}

func TestTemplateDraftPlanRequiresExactValidUnpublishedSource(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	mutator.runtimeRevision.(*templateRuntimeRevisionFixture).binding = existing.Templates[0].Current.Body.EntryDefaults.Runtime
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789f8")
	templateRef, err := tobari.WorkspaceTemplateRef(id)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing source", func(t *testing.T) {
		calls := 0
		_, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			calls++
			return tobari.WorkspaceTemplateSource{}, "", tobari.ErrResourceSourceMissing
		})
		if !errors.Is(err, tobari.ErrResourceSourceMissing) || calls != 1 {
			t.Fatalf("missing draft source err=%v calls=%d", err, calls)
		}
	})

	t.Run("active base without active identity", func(t *testing.T) {
		source, sourceErr := tobari.NewWorkspaceTemplateDraftSource(id, "later", existing.Templates[0].Current.Body)
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		base := existing.Templates[0].Current.Revision
		source.Template.BaseRevision = &base
		_, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return source, strings.Repeat("8", 64), nil
		})
		if !errors.Is(err, tobari.ErrResourceSourceModified) {
			t.Fatalf("non-null draft base err=%v", err)
		}
	})

	t.Run("active name collision", func(t *testing.T) {
		source, sourceErr := tobari.NewWorkspaceTemplateDraftSource(id, existing.Templates[0].Name, existing.Templates[0].Current.Body)
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		_, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
			return source, strings.Repeat("9", 64), nil
		})
		if !errors.Is(err, tobari.ErrWorkspaceTemplateExists) {
			t.Fatalf("draft name collision err=%v", err)
		}
	})
}
