package cli

import (
	"bytes"
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
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		if spec.Effect != operation.EffectWrite || spec.Role != RoleAct || spec.Agent.Mutation == nil {
			t.Fatalf("%s effect/role/mutation = %s/%s/%+v", path, spec.Effect.String(), spec.Role.String(), spec.Agent.Mutation)
		}
		if spec.Agent.FixedTarget == nil || spec.Agent.FixedTarget.Kind != authbroker.CredentialCatalogTargetKind ||
			spec.Agent.FixedTarget.ID != authbroker.CredentialCatalogTargetID || spec.Agent.FixedTarget.Scope != FixedTargetScopeToolLocal {
			t.Fatalf("%s fixed target = %+v", path, spec.Agent.FixedTarget)
		}
		if spec.Agent.Mutation.TargetKind != authbroker.CredentialCatalogTargetKind ||
			!reflect.DeepEqual(spec.Agent.Mutation.TargetInputs, []string{}) {
			t.Fatalf("%s mutation target = %+v", path, spec.Agent.Mutation)
		}
		if err := spec.Agent.Mutation.Impact.Validate(); err != nil {
			t.Fatalf("%s impact = %+v: %v", path, spec.Agent.Mutation.Impact, err)
		}
	}

	status, found := DefaultCatalog().Lookup("auth status")
	if !found || status.Effect != operation.EffectRead || status.Role != RoleUtility ||
		status.Agent.FixedTarget != nil || status.Agent.Mutation != nil {
		t.Fatalf("auth status = %+v, found=%t", status, found)
	}
}

func TestAuthCatalogDeclaresTerminalAndUnknownMutationOutcomeFaults(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		unknownFound := false
		for _, declared := range spec.Agent.Errors {
			if declared.Code != "auth_mutation_outcome_unknown" {
				continue
			}
			unknownFound = true
			if declared.Kind != fault.KindContract || declared.Retryable || len(declared.NextActions) != 1 ||
				declared.NextActions[0].Command != "auth status" {
				t.Fatalf("%s unknown mutation outcome = %+v", path, declared)
			}
		}
		if !unknownFound {
			t.Fatalf("%s lacks auth_mutation_outcome_unknown", path)
		}
	}

	login, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	for _, declared := range login.Agent.Errors {
		if declared.Code == "auth_login_tty_required" {
			if declared.Kind != fault.KindInvalidInput || declared.Retryable || len(declared.NextActions) != 1 ||
				declared.NextActions[0].Command != "help auth login" {
				t.Fatalf("auth login terminal fault = %+v", declared)
			}
			return
		}
	}
	t.Fatal("auth login lacks auth_login_tty_required")
}

func TestAuthLoginCatalogSeparatesProviderToolAndAcquisitionMethod(t *testing.T) {
	t.Parallel()
	login, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	if len(login.Agent.Inputs) == 0 || login.Agent.Inputs[0].Required ||
		login.Agent.Inputs[0].Name != "--provider" || login.Agent.Inputs[0].Source != InputSourceFlag ||
		login.Args != "[--provider <provider>] [--method identity-center|console] [--context <name>] [--format text|json]" ||
		!strings.Contains(login.Agent.Inputs[0].Description, "github") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "aws") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "interactive provider selector") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "selected automatically") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "GitHub CLI (gh)") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "AWS CLI (aws)") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "datadog uses pup") ||
		!strings.Contains(login.Agent.Inputs[0].Description, "not another tool") {
		t.Fatalf("auth login provider input = %+v", login.Agent.Inputs)
	}
	methodFound := false
	for _, input := range login.Agent.Inputs {
		if input.Name == "--method" {
			methodFound = reflect.DeepEqual(input.AllowedValues, []string{"identity-center", "console"}) &&
				reflect.DeepEqual(input.Requires, []string{"--provider"}) &&
				strings.Contains(input.Description, "backward compatibility")
		}
	}
	if !methodFound {
		t.Fatalf("auth login method input = %+v", login.Agent.Inputs)
	}
	joinedPrerequisites := strings.Join(login.Agent.Prerequisites, "\n")
	for _, want := range []string{"github", "aws", "trusted-host PATH", "access-portal start URL", "SSO region", "account ID", "role name", "request region"} {
		if !strings.Contains(joinedPrerequisites, want) {
			t.Fatalf("auth login prerequisites = %q, want %q", joinedPrerequisites, want)
		}
	}
	wantErrors := map[string]bool{
		"github_cli_unavailable":        false,
		"github_login_cancelled":        false,
		"github_login_failed":           false,
		"aws_cli_unavailable":           false,
		"aws_console_login_unsupported": false,
		"aws_console_config_invalid":    false,
		"aws_console_login_cancelled":   false,
		"aws_console_login_timeout":     false,
		"aws_console_login_failed":      false,
		"aws_sso_login_cancelled":       false,
		"aws_sso_config_invalid":        false,
		"aws_sso_login_timeout":         false,
		"aws_sso_login_failed":          false,
	}
	for _, declared := range login.Agent.Errors {
		if _, expected := wantErrors[declared.Code]; expected {
			wantErrors[declared.Code] = true
		}
	}
	for code, found := range wantErrors {
		if !found {
			t.Fatalf("auth login lacks %q", code)
		}
	}
}

func TestAuthLoginMethodRequiresExplicitProvider(t *testing.T) {
	t.Parallel()
	spec, found := DefaultCatalog().Lookup("auth login")
	if !found {
		t.Fatal("catalog lacks auth login")
	}
	if _, err := parseCommandInputs(spec, []string{"--method=console"}); err == nil ||
		!strings.Contains(err.Error(), "--method requires --provider") {
		t.Fatalf("parse auth login method without provider error = %v", err)
	}
	inputs, err := parseCommandInputs(spec, []string{})
	if err != nil {
		t.Fatalf("parse omitted provider: %v", err)
	}
	if inputs.Provided("--provider") || inputs.One("--provider") != "" {
		t.Fatalf("omitted provider = provided:%t value:%q", inputs.Provided("--provider"), inputs.One("--provider"))
	}
	if _, err := parseCommandInputs(spec, []string{"github"}); err == nil ||
		!strings.Contains(err.Error(), `unexpected argument "github"`) {
		t.Fatalf("legacy positional provider error = %v", err)
	}
}

func TestAuthScopedHelpPublishesProtectedStdinContract(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, []string{"help", "auth", "import", "--format=agent"}); code != ExitOK {
		t.Fatalf("help auth import code = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"path":"auth import"`, `"name":"credential"`, `"source":"stdin"`, `"required":true`,
		"interactive terminal stdin is rejected before any credential bytes are read",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("scoped help = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, `TOBARI_TOKEN`) || strings.Contains(output, `--credential`) {
		t.Fatalf("scoped help exposes an environment/argv credential channel: %q", output)
	}

	stdout.Reset()
	stderr.Reset()
	command.In = bytes.NewBufferString("")
	if code := runCLI(command, []string{"auth", "import", "--help"}); code != ExitOK {
		t.Fatalf("auth import --help code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "stdin") || strings.Contains(stdout.String(), "--credential") {
		t.Fatalf("human auth help = %q", stdout.String())
	}
}

func TestAuthImportDeclaresRequiredStdinWithoutArgvOrEnvironmentSecret(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("auth import")
	if !found {
		t.Fatal("catalog lacks auth import")
	}
	foundCredential := false
	for _, input := range spec.Agent.Inputs {
		if input.Name == "credential" {
			foundCredential = true
			if input.Source != InputSourceStdin || !input.Required || input.Cardinality != InputCardinalitySingle {
				t.Fatalf("credential input = %+v", input)
			}
		}
		if input.Source == InputSourceEnvironment ||
			(input.Source == InputSourceArgument && input.Name != "provider") {
			t.Fatalf("auth import exposes unexpected secret-capable input = %+v", input)
		}
	}
	if !foundCredential {
		t.Fatal("auth import lacks required stdin credential declaration")
	}
	if inputs, _, err := parseArgumentSyntaxInputs(spec.Args); err != nil {
		t.Fatal(err)
	} else if _, exposed := inputs["credential"]; exposed {
		t.Fatal("credential appears in public argv grammar")
	}
}

func TestAuthCommandsPublishOneSecretFreeSchema(t *testing.T) {
	wantMutationFields := []string{
		"provider", "context", "context_state", "context_id", "configured", "account_label",
		"storage_backend", "broker_state", "credential_revision", "workspace_activation",
	}
	for _, path := range []string{"auth login", "auth import", "auth logout"} {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		gotFields := make([]string, 0, len(spec.Agent.Output.Fields))
		for _, field := range spec.Agent.Output.Fields {
			gotFields = append(gotFields, field.Name)
		}
		if !reflect.DeepEqual(gotFields, wantMutationFields) || spec.Agent.Output.JSONEnvelope != "auth" ||
			spec.Agent.Output.JSONSchemaVersion != 3 || spec.Agent.Output.CollectionCoverage != CollectionCoverageNotApplicable {
			t.Fatalf("%s output = %+v", path, spec.Agent.Output)
		}
		for _, forbidden := range []string{"credential", "secret", "token", "handle", "vault", "root_key"} {
			for _, field := range gotFields {
				if field == forbidden {
					t.Fatalf("%s exposes forbidden field %q", path, field)
				}
			}
		}
	}

	status, found := DefaultCatalog().Lookup("auth status")
	if !found {
		t.Fatal("catalog lacks auth status")
	}
	gotStatusFields := make([]string, 0, len(status.Agent.Output.Fields))
	for _, field := range status.Agent.Output.Fields {
		gotStatusFields = append(gotStatusFields, field.Name)
	}
	wantStatusFields := []string{"context", "context_state", "context_id", "storage_backend", "broker_state", "providers", "workspace_activation"}
	if !reflect.DeepEqual(gotStatusFields, wantStatusFields) || status.Agent.Output.JSONEnvelope != "auth" ||
		status.Agent.Output.JSONSchemaVersion != 3 || status.Agent.Output.CollectionCoverage != CollectionCoverageExhaustive {
		t.Fatalf("auth status output = %+v", status.Agent.Output)
	}
}
