package workspaceauthoritystore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func storePolicyReadCollection(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	base := storeCollectionFixture(t)
	body := tobari.PolicyMemoryRuleBody{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/remembered",
		Segments: []string{}, Examples: []string{"/remembered"}, SourceCandidates: []string{"pcy_" + strings.Repeat("2", 32)},
	}
	rule, err := tobari.NewPolicyMemoryRule(storeContextID, tobari.PolicyMemoryAllow, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := tobari.PublishPolicyMemory(storeContextID, []tobari.PolicyMemoryRule{rule}, &base.Contexts[0].PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("publish memory: changed=%t err=%v", changed, err)
	}
	contexts := append([]tobari.WorkspaceAuthorityContextRecord{}, base.Contexts...)
	contexts[0] = base.Contexts[0].Clone()
	contexts[0].PolicyMemory = memory
	result, changed, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("publish collection: changed=%t err=%v", changed, err)
	}
	return result
}

func TestFinalStoreReadsExhaustivePolicyAuthorityFromOneEnvelope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	collection := storePolicyReadCollection(t)
	materializeCollection(t, root, collection)
	guard := &legacyGuardFake{}
	store, err := NewFinalOnly(root, guard)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := store.ListPendingPolicyCandidateAuthority(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rules, err := store.ListPolicyMemoryRuleAuthority(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.ValidateFor(collection, true); err != nil || len(candidates.Items) != 1 {
		t.Fatalf("candidates=%#v err=%v", candidates, err)
	}
	if err := rules.ValidateFor(collection, true); err != nil || len(rules.Items) != 1 {
		t.Fatalf("rules=%#v err=%v", rules, err)
	}
	if candidates.Items[0].ObservingWorkspaceRef == "" || rules.Items[0].ContextRef == "" || rules.Items[0].TemplateRef == "" {
		t.Fatalf("final dimensions are incomplete: candidates=%#v rules=%#v", candidates, rules)
	}
}

func TestBatchCB3StoreReadsOneCoherentPermissionInboxEnvelope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	collection := storePolicyReadCollection(t)
	materializeCollection(t, root, collection)
	store, err := NewFinalOnly(root, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.ReadPolicyMemoryReviewSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ValidateFor(collection, true); err != nil || len(snapshot.Items) != 1 || len(snapshot.Rules) != 1 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	if snapshot.Items[0].Candidates[0].ID != collection.PendingCandidates[0].ID || snapshot.Rules[0].Rule.ID != collection.Contexts[0].PolicyMemory.Rules[0].ID {
		t.Fatal("Permission Inbox mixed candidate or source-rule authority")
	}
}

func TestFinalStorePolicyReadsAreKnownEmptyWithoutCreatingFreshStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	store, err := NewFinalOnly(root, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := store.ListPendingPolicyCandidateAuthority(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rules, err := store.ListPolicyMemoryRuleAuthority(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if candidates.Items == nil || rules.Items == nil || candidates.CollectionPresent || rules.CollectionPresent {
		t.Fatalf("fresh candidates=%#v rules=%#v", candidates, rules)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh policy read created final store: %v", err)
	}
}

func TestFinalStorePolicyReadsPropagateLegacyGuardWithoutFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	store, err := NewFinalOnly(root, &legacyGuardFake{errors: []error{errors.New("legacy policy root is present")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListPendingPolicyCandidateAuthority(context.Background()); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("candidate legacy error=%v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy rejection created final store: %v", err)
	}
}
