package workspaceauthoritycmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type defaultPairReceiptDriftFixture struct {
	*defaultPairFixture
	observations int
}

func (f *defaultPairReceiptDriftFixture) ObserveFinalDefaultPair(ctx context.Context, root string) (tobari.FinalDefaultPairObservation, error) {
	observation, err := f.defaultPairFixture.ObserveFinalDefaultPair(ctx, root)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	f.observations++
	// The first pair belongs to the pre-initialization stable observation. The
	// second pair is the required post-initialization fence immediately before
	// nested Context entry. Return another valid receipt on its second pass
	// without changing the fixture's authority, modelling concurrent drift.
	if f.observations == 4 {
		observation.CollectionGeneration++
		observation.CollectionRevision = digest("f")
	}
	return observation, observation.Validate()
}

func TestDefaultPairReceiptDriftBeforeNestedEntryMakesZeroEntryEffect(t *testing.T) {
	body := bodyFixture("/first-use")
	base := &defaultPairFixture{root: "/workspace/example", revisionDigit: 'a'}
	if _, err := base.InitializeFinalDefaultPair(context.Background(), base.root, body); err != nil {
		t.Fatal(err)
	}
	base.initializeCalls = 0
	base.templateCreates = 0
	base.defaultWrites = 0
	base.contextCreates = 0
	base.entries = 0
	beforeGeneration := base.generation
	fixture := &defaultPairReceiptDriftFixture{defaultPairFixture: base}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture))
	intent := operation.Intent{
		Command: TaskDefaultPairEnter, Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID},
		Impact: DefaultPairEnterImpact(),
	}
	_, err := service.Enter(context.Background(), intent, body, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "default_pair_changed" || public.ChangeState != fault.ChangeNone {
		t.Fatalf("default receipt drift classification=%+v err=%v", public, err)
	}
	if base.initializeCalls != 1 || base.templateCreates != 0 || base.defaultWrites != 0 || base.contextCreates != 0 ||
		base.entries != 0 || base.generation != beforeGeneration {
		t.Fatalf("receipt drift crossed nested entry or changed default authority: %+v", base)
	}
}
