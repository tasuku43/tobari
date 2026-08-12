package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

func TestAuthCatalogDeclaresFixedTargetMutationAndReadOnlyStatus(t *testing.T) {
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, found := DefaultCatalog().Lookup(path)
		if !found || spec.Effect != operation.EffectWrite || spec.Role != RoleAct || spec.Agent.FixedTarget == nil || spec.Agent.Mutation == nil {
			t.Fatalf("%s contract = %+v", path, spec)
		}
	}
	status, found := DefaultCatalog().Lookup("auth status")
	if !found || status.Effect != operation.EffectRead || status.Role != RoleUtility {
		t.Fatalf("auth status = %+v", status)
	}
}

func TestAuthLoginCatalogIsExplicitGitHubOnly(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	if spec.Args != "--provider=github [--context <name>] [--format text|json]" || len(spec.Agent.Inputs) == 0 {
		t.Fatalf("auth login = %+v", spec)
	}
	provider := spec.Agent.Inputs[0]
	if provider.Name != "--provider" || !provider.Required || !reflect.DeepEqual(provider.AllowedValues, []string{"github"}) {
		t.Fatalf("provider input = %+v", provider)
	}
	for _, input := range spec.Agent.Inputs {
		if input.Name == "--method" {
			t.Fatal("retired method selector remains")
		}
	}
	if _, err := parseCommandInputs(spec, []string{}); err == nil {
		t.Fatal("omitted provider was accepted")
	}
	if _, err := parseCommandInputs(spec, []string{"--provider=aws"}); err == nil {
		t.Fatal("retired AWS provider was accepted")
	}
	if _, err := parseCommandInputs(spec, []string{"--method=console", "--provider=github"}); err == nil {
		t.Fatal("retired method was accepted")
	}
	if _, err := parseCommandInputs(spec, []string{"--provider=github"}); err != nil {
		t.Fatalf("GitHub provider rejected: %v", err)
	}
	retired := []string{"aws", "datadog", "openai", "anthropic", "chatwork", "console", "identity-center", "codex", "claude", "pup"}
	encoded := spec.Args + spec.Summary + spec.Agent.Outcome + strings.Join(spec.Agent.Prerequisites, " ")
	for _, declared := range spec.Agent.Errors {
		encoded += declared.Code
	}
	for _, value := range retired {
		if strings.Contains(strings.ToLower(encoded), value) {
			t.Fatalf("retired auth surface %q remains in login contract", value)
		}
	}
	for _, code := range []string{"github_cli_unavailable", "github_login_cancelled", "github_login_failed", "auth_login_tty_required"} {
		found := false
		for _, declared := range spec.Agent.Errors {
			if declared.Code == code {
				found = true
			}
		}
		if !found {
			t.Fatalf("auth login lacks %q", code)
		}
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

func TestAuthCatalogDeclaresTerminalAndUnknownMutationOutcomeFaults(t *testing.T) {
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, _ := DefaultCatalog().Lookup(path)
		unknown := false
		for _, declared := range spec.Agent.Errors {
			if declared.Code == "auth_mutation_outcome_unknown" && declared.Kind == fault.KindContract && !declared.Retryable {
				unknown = true
			}
		}
		if !unknown {
			t.Fatalf("%s lacks unknown mutation outcome", path)
		}
	}
}
