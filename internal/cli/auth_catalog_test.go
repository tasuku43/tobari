//go:build tobari_dev && tobari_research

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestAuthCatalogDeclaresFinalContextMutationAndReadOnlyStatus(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{"auth login", "auth import"} {
		spec, found := catalog.Lookup(path)
		if !found || spec.Effect != operation.EffectCreate || spec.Role != RoleAct || spec.Agent.FixedTarget != nil || spec.Agent.Mutation == nil || spec.Agent.Mutation.ParentInput != "--context" || spec.Agent.Mutation.TargetKind != authbroker.ContextCredentialTargetKind || !spec.Agent.Mutation.CurrentContextFallback {
			t.Fatalf("%s contract = %+v", path, spec)
		}
	}
	logout, found := catalog.Lookup("auth logout")
	if !found || logout.Effect != operation.EffectWrite || logout.Agent.Mutation.TargetIDInput != "--context" || logout.Agent.Mutation.TargetKind != tobari.ContextReferenceKind || !logout.Agent.Mutation.CurrentContextFallback {
		t.Fatalf("auth logout = %+v", logout)
	}
	status, found := catalog.Lookup("auth status")
	if !found || status.Effect != operation.EffectRead || status.Role != RoleDiscover || status.Agent.Output.JSONSchemaVersion != 2 {
		t.Fatalf("auth status = %+v", status)
	}
	if got := status.ProducedRefs(); !reflect.DeepEqual(got, []ProducedRef{{Kind: tobari.ContextReferenceKind, Field: "context_ref"}}) {
		t.Fatalf("auth status refs=%+v", got)
	}
	for _, spec := range []CommandSpec{logout, status} {
		encoded := spec.Args + spec.Summary + spec.Agent.Outcome
		if strings.Contains(encoded, "manifest") || strings.Contains(encoded, "name") || strings.Contains(encoded, "UUID") {
			t.Fatalf("%s retains predecessor selector vocabulary: %s", spec.Path, encoded)
		}
	}
}

func TestAuthLoginCatalogPreservesReviewedProviderAndMethodSemantics(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	var contextInput, providerInput, methodInput CommandInput
	for _, input := range spec.Agent.Inputs {
		switch input.Name {
		case "--context":
			contextInput = input
		case "--provider":
			providerInput = input
		case "--method":
			methodInput = input
		}
	}
	if contextInput.Required || contextInput.ReferenceKind != tobari.ContextReferenceKind || providerInput.Required || !reflect.DeepEqual(providerInput.AllowedValues, authbroker.ReviewedLoginProviderIDs()) {
		t.Fatalf("context/provider inputs=%+v/%+v", contextInput, providerInput)
	}
	if authbroker.SupportsReviewedLoginProvider(authbroker.BuiltinAWSProviderID) && (!reflect.DeepEqual(methodInput.AllowedValues, []string{"identity-center", "console"}) || !reflect.DeepEqual(methodInput.Requires, []string{"--provider"})) {
		t.Fatalf("method input=%+v", methodInput)
	}
	contextRef := "context:01912345-6789-7abc-8def-0123456789a2"
	inputs, err := parseCommandInputs(spec, []string{"--context", contextRef})
	if err != nil || inputs.Provided("--provider") || inputs.One("--context") != contextRef {
		t.Fatalf("omitted provider parse=%+v err=%v", inputs, err)
	}
	if inputs, err := parseCommandInputs(spec, []string{"--provider=github"}); err != nil || inputs.Provided("--context") {
		t.Fatalf("current-Context fallback parse=%+v err=%v", inputs, err)
	}
	if _, err := parseCommandInputs(spec, []string{"--context", contextRef, "--method=console"}); err == nil {
		t.Fatal("method without provider passed")
	}
}

func TestAuthImportPublishesProtectedStdinContract(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("auth import")
	if !found {
		t.Fatal("catalog lacks auth import")
	}
	stdin := false
	for _, input := range spec.Agent.Inputs {
		stdin = stdin || input.Name == "credential" && input.Source == InputSourceStdin && input.Required
		if input.Source == InputSourceArgument && input.Name != "provider" {
			t.Fatalf("unexpected argument input = %+v", input)
		}
	}
	if !stdin {
		t.Fatal("auth import lacks required stdin credential")
	}
}

func TestAuthCatalogDeclaresDurableUnknownOutcomeReconciliation(t *testing.T) {
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, _ := DefaultCatalog().Lookup(path)
		codes := map[string]bool{}
		for _, declared := range spec.Agent.Errors {
			codes[declared.Code] = true
		}
		for _, code := range []string{"research_auth_mutation_interrupted", "research_auth_result_delivery_interrupted", "unclassified_mutation_outcome"} {
			if !codes[code] {
				t.Fatalf("%s lacks %s", path, code)
			}
		}
	}
}

func TestAuthRecoveryStartsWithExecutableContextDiscovery(t *testing.T) {
	catalog := DefaultCatalog()
	contextList, found := catalog.Lookup(authContextRecoveryCommand)
	if !found || contextList.Effect != operation.EffectRead || contextList.Role != RoleDiscover {
		t.Fatalf("context recovery producer = found:%t spec:%+v", found, contextList)
	}
	for _, input := range contextList.Agent.Inputs {
		if input.Required {
			t.Fatalf("context recovery producer requires %q", input.Name)
		}
	}
	if got := contextList.ProducedRefs(); !reflect.DeepEqual(got, []ProducedRef{{Kind: tobari.ContextReferenceKind, Field: "items[].context_ref"}}) {
		t.Fatalf("context recovery producer refs=%+v", got)
	}
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %s", path)
		}
		for _, declared := range spec.Agent.Errors {
			for _, action := range declared.NextActions {
				if action.Command == "auth status" {
					t.Fatalf("%s error %s retains context-bound recovery", path, declared.Code)
				}
				if action.Command == authContextRecoveryCommand && !strings.Contains(action.Reason, "unchanged") {
					t.Fatalf("%s error %s context recovery reason=%q", path, declared.Code, action.Reason)
				}
			}
		}
	}
}
