package policypresetcmd

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeRuntime struct {
	validateCalls int
	initCalls     int
}

func (f *fakeRuntime) ListPolicyPresets(context.Context) (tobari.PolicyPresetResult, error) {
	return tobari.PolicyPresetResult{Task: tobari.TaskPolicyPresetList, Items: []tobari.PolicyPresetSummary{}}, nil
}
func (f *fakeRuntime) ShowPolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error) {
	return tobari.PolicyPresetResult{Task: tobari.TaskPolicyPresetShow}, nil
}
func (f *fakeRuntime) ValidatePolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error) {
	f.validateCalls++
	return tobari.PolicyPresetResult{Task: tobari.TaskPolicyPresetValidate}, nil
}
func (f *fakeRuntime) InitPolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error) {
	f.initCalls++
	return tobari.PolicyPresetResult{Task: tobari.TaskPolicyPresetInit}, nil
}

func TestValidateRejectsBuiltinBeforeRuntime(t *testing.T) {
	fake := &fakeRuntime{}
	_, err := New(fake).Validate(context.Background(), tobari.DefaultPolicyPresetOrigin)
	if err == nil || fake.validateCalls != 0 {
		t.Fatalf("builtin validate error/calls=%v/%d", err, fake.validateCalls)
	}
}
func TestInitBindsFixedCatalogTarget(t *testing.T) {
	fake := &fakeRuntime{}
	intent := operation.Intent{Command: "policy preset init", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.PolicyPresetCatalogTargetKind, ParentID: tobari.PolicyPresetCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	if _, err := New(fake).Init(context.Background(), intent, "restricted"); err != nil || fake.initCalls != 1 {
		t.Fatalf("init error/calls=%v/%d", err, fake.initCalls)
	}
}
