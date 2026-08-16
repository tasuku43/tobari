//go:build tobari_experimental

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
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

func TestAuthLoginCatalogAllowsInteractiveOmissionAndReviewedProviders(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	awsEnabled := authbroker.SupportsReviewedLoginProvider(authbroker.BuiltinAWSProviderID)
	wantArgs := "[--provider github|datadog|openai|anthropic] [--context <name>] [--format text|json]"
	wantProviders := []string{"github", "datadog", "openai", "anthropic"}
	if awsEnabled {
		wantArgs = "[--provider github|aws|datadog|openai|anthropic] [--method identity-center|console] [--context <name>] [--format text|json]"
		wantProviders = []string{"github", "aws", "datadog", "openai", "anthropic"}
	}
	if spec.Args != wantArgs || len(spec.Agent.Inputs) == 0 {
		t.Fatalf("auth login = %+v", spec)
	}
	provider := spec.Agent.Inputs[0]
	if provider.Name != "--provider" || provider.Required || !reflect.DeepEqual(provider.AllowedValues, wantProviders) ||
		!strings.Contains(provider.Description, "interactive selector") {
		t.Fatalf("provider input = %+v", provider)
	}
	inputs, err := parseCommandInputs(spec, []string{})
	if err != nil || inputs.Provided("--provider") || inputs.One("--provider") != "" {
		t.Fatalf("omitted provider parse = inputs:%+v error:%v", inputs, err)
	}
	if awsEnabled {
		method := spec.Agent.Inputs[1]
		if method.Name != "--method" || !reflect.DeepEqual(method.AllowedValues, []string{"identity-center", "console"}) ||
			!reflect.DeepEqual(method.Requires, []string{"--provider"}) {
			t.Fatalf("method input = %+v", method)
		}
		if _, err := parseCommandInputs(spec, []string{"--provider=aws"}); err != nil {
			t.Fatalf("AWS provider rejected: %v", err)
		}
		if _, err := parseCommandInputs(spec, []string{"--method=console"}); err == nil {
			t.Fatal("method without provider was accepted")
		}
	} else {
		if _, err := parseCommandInputs(spec, []string{"--provider=aws"}); err == nil {
			t.Fatal("standard profile accepted AWS provider")
		}
		if _, err := parseCommandInputs(spec, []string{"--method=console"}); err == nil {
			t.Fatal("standard profile accepted experimental AWS method")
		}
	}
	if _, err := parseCommandInputs(spec, []string{"--provider=github"}); err != nil {
		t.Fatalf("GitHub provider rejected: %v", err)
	}
	retired := []string{"managed", "credential_profile", "arbitrary helper", "tobari-toolbox", "toolbox:build"}
	encoded := spec.Args + spec.Summary + spec.Agent.Outcome + strings.Join(spec.Agent.Prerequisites, " ")
	for _, declared := range spec.Agent.Errors {
		encoded += declared.Code
	}
	for _, value := range retired {
		if strings.Contains(strings.ToLower(encoded), value) {
			t.Fatalf("unsupported auth surface %q remains in login contract", value)
		}
	}
	wantCodes := []string{"github_cli_unavailable", "datadog_cli_unavailable", "openai_cli_unavailable", "anthropic_cli_unavailable", "auth_login_selector_unavailable", "auth_login_tty_required"}
	if awsEnabled {
		wantCodes = append(wantCodes, "aws_cli_unavailable")
	}
	for _, code := range wantCodes {
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
	if !awsEnabled && strings.Contains(strings.ToLower(encoded), "aws") {
		t.Fatalf("standard auth login contract leaked AWS: %s", encoded)
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
