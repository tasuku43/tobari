package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func noOpHandler(context.Context, *CLI, CommandSpec, operation.Intent, ParsedInputs) int {
	return ExitOK
}

func utilitySpec(path string) CommandSpec {
	return CommandSpec{
		Path:    path,
		Summary: "Complete a test outcome",
		Effect:  operation.EffectRead,
		Role:    RoleUtility,
		Agent: AgentContract{
			CapabilityID: "test.complete",
			Outcome:      "Complete a bounded test outcome",
			Inputs:       []CommandInput{},
			Output: CommandOutput{
				Formats:            []OutputFormat{OutputFormatText},
				DefaultFormat:      OutputFormatText,
				TextPresentation:   TextPresentationSemanticTokens,
				Fields:             []OutputField{{Name: "result", Type: OutputFieldTypeString, Description: "Stable test result."}},
				Delivery:           OutputDeliveryComplete,
				CollectionCoverage: CollectionCoverageNotApplicable,
			},
			Prerequisites: []string{},
			Errors: []CommandError{
				declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, path, "Correct the command arguments."),
				{
					Code:        "test_failed",
					Kind:        fault.KindInternal,
					Retryable:   false,
					NextActions: []fault.NextAction{{Command: path, Reason: "Inspect the test fixture and correct it."}},
				},
				declaredCommandError(fault.KindInternal, "output_write_failed", true, path, "Retry with a writable output stream."),
				declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
			},
		},
		handler: noOpHandler,
	}
}

func mutationRuntimeErrors(path string) []CommandError {
	namespace := strings.Fields(path)[0]
	return []CommandError{
		declaredCommandError(fault.KindContract, "invalid_mutation_contract", false, path, "Repair the mutation declaration."),
		declaredCommandError(fault.KindContract, "missing_mutation_action", false, path, "Configure the mutation action."),
		declaredCommandError(fault.KindRejected, "missing_mutation_policy", false, path, "Configure the project mutation policy."),
		declaredCommandError(fault.KindRejected, "mutation_rejected", false, path, "Review the project mutation policy."),
		declaredCommandError(fault.KindContract, "unclassified_mutation_outcome", false, namespace+" list", "Reconcile the target before deciding whether another mutation is safe."),
		declaredCommandError(fault.KindInternal, "mutation_output_write_failed", false, namespace+" list", "Reconcile the confirmed mutation result without repeating the mutation."),
	}
}

func mutationErrors(base []CommandError, path string) []CommandError {
	errors := make([]CommandError, 0, len(base)+len(mutationRuntimeErrors(path)))
	for _, declared := range base {
		if declared.Code != "output_write_failed" {
			errors = append(errors, declared)
		}
	}
	return append(errors, mutationRuntimeErrors(path)...)
}

func discoverSpec(path, kind string) CommandSpec {
	spec := utilitySpec(path)
	spec.Summary = "Discover test items"
	spec.Role = RoleDiscover
	spec.Agent.Outcome = "Discover stable test item references"
	spec.Agent.Output.Formats = []OutputFormat{OutputFormatTSV, OutputFormatJSON}
	spec.Agent.Output.DefaultFormat = OutputFormatTSV
	spec.Agent.Output.Fields = []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque test item ID.", ReferenceKind: kind},
		{Name: "name", Type: OutputFieldTypeString, Description: "Test item name."},
	}
	spec.Agent.Output.JSONEnvelope = "items"
	spec.Agent.Output.JSONEnvelopeType = OutputFieldTypeArray
	spec.Agent.Output.JSONSchemaVersion = 1
	spec.Agent.Output.CollectionCoverage = CollectionCoverageExhaustive
	return spec
}

func actSpec(path, kind string, inputs ...string) CommandSpec {
	spec := utilitySpec(path)
	spec.Summary = "Read test items"
	spec.Role = RoleAct
	spec.Agent.Outcome = "Read the selected test items"
	spec.Agent.Inputs = make([]CommandInput, 0, len(inputs))
	parts := make([]string, 0, len(inputs)*2)
	for _, input := range inputs {
		spec.Agent.Inputs = append(spec.Agent.Inputs, CommandInput{
			Name: input, Source: InputSourceFlag, Required: true,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Opaque test item ID.", AllowedValues: []string{}, ReferenceKind: kind,
		})
		parts = append(parts, input, "<"+kind+"-id>")
	}
	spec.Args = strings.Join(parts, " ")
	return spec
}

func recursiveReferenceProducerSpec() CommandSpec {
	spec := discoverSpec("references list", "runtime")
	spec.Agent.Output.Formats = []OutputFormat{OutputFormatJSON}
	spec.Agent.Output.DefaultFormat = OutputFormatJSON
	spec.Agent.Output.Fields = []OutputField{
		{
			Name: "items", Type: OutputFieldTypeArray, Description: "Runtime selections.",
			Items: &OutputField{Type: OutputFieldTypeObject, Description: "One runtime selection.", Fields: []OutputField{
				{Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque Runtime reference.", ReferenceKind: "runtime"},
			}},
		},
		{
			Name: "metadata", Type: OutputFieldTypeObject, Description: "Reference metadata.", Fields: []OutputField{
				{Name: "owner_ref", Type: OutputFieldTypeString, Description: "Opaque owner reference.", ReferenceKind: "owner"},
			},
		},
		{
			Name: "ids", Type: OutputFieldTypeArray, Description: "Opaque identifier references.",
			Items: &OutputField{Type: OutputFieldTypeString, Description: "One opaque identifier reference.", ReferenceKind: "identifier"},
		},
		{
			Name: "revisions", Type: OutputFieldTypeArray, Description: "Runtime revision selections.",
			Items: &OutputField{Type: OutputFieldTypeObject, Description: "One runtime revision selection.", Fields: []OutputField{
				{Name: "revision_ref", Type: OutputFieldTypeString, Description: "Opaque Runtime revision reference.", Optional: true, ReferenceKind: "runtime-revision"},
			}},
		},
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque runtime prune plan reference.", ReferenceKind: "runtime-prune-plan"},
	}
	return spec
}

func fixedTargetActSpec(path string) CommandSpec {
	spec := utilitySpec(path)
	spec.Role = RoleAct
	spec.Agent.FixedTarget = &FixedTarget{
		Kind: "auth-config", ID: "selected", Description: "This CLI installation's selected authentication configuration.",
		Scope: FixedTargetScopeToolLocal,
	}
	return spec
}

func TestDefaultCatalogIsValidAndUnique(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	seen := map[string]bool{}
	for _, command := range catalog.Commands() {
		if seen[command.Path] {
			t.Fatalf("duplicate command path %q", command.Path)
		}
		seen[command.Path] = true
		if err := command.Effect.Validate(); err != nil {
			t.Errorf("%s effect: %v", command.Path, err)
		}
		if err := command.Role.validate(); err != nil {
			t.Errorf("%s role: %v", command.Path, err)
		}
	}
	for _, required := range []string{
		"doctor", "help", "version",
		"cluster up", "cluster status", "cluster denials", "cluster logs", "cluster down",
		"tobari", "status",
		"template list", "template show", "context list", "context show",
		"workspace list", "workspace status", "workspace delete",
	} {
		if !seen[required] {
			t.Errorf("catalog is missing %q", required)
		}
	}
	for _, removed := range []string{"list", "delete", "manifest show", "manifest list", "manifest create"} {
		if seen[removed] {
			t.Errorf("catalog retained predecessor path %q", removed)
		}
	}
	for _, path := range []string{WorkspaceEntryCommandPath, "status"} {
		command, found := catalog.Lookup(path)
		if !found {
			continue
		}
		for _, input := range command.Agent.Inputs {
			if input.Name == "--manifest" {
				t.Errorf("final root-owned path %q retained predecessor --manifest input", path)
			}
		}
	}
}

func TestCatalogSurfaceSetIsExact(t *testing.T) {
	const researchOnlyPrefix = "auth "
	wantResearchOnly := map[string]struct{}{
		"auth login":  {},
		"auth import": {},
		"auth status": {},
		"auth logout": {},
		"serve":       {},
	}

	paths := make([]string, 0, len(DefaultCatalog().Commands()))
	seen := make(map[string]struct{})
	for _, command := range DefaultCatalog().Commands() {
		if _, duplicate := seen[command.Path]; duplicate {
			t.Fatalf("catalog path %q is duplicated", command.Path)
		}
		seen[command.Path] = struct{}{}
		paths = append(paths, command.Path)
		_, researchOnly := wantResearchOnly[command.Path]
		if !researchOnly && strings.HasPrefix(command.Path, researchOnlyPrefix) {
			t.Fatalf("unexpected non-contract authentication path %q", command.Path)
		}
	}
	for path := range wantResearchOnly {
		_, found := seen[path]
		if found != buildIdentityHasBroker() {
			t.Fatalf("research-only command %q present=%t on research surface=%t", path, found, buildIdentityHasBroker())
		}
	}
	slices.Sort(paths)
	t.Logf("CATALOG_SURFACE_PATHS=%s", strings.Join(paths, "|"))
}

func TestProgramAwareCatalogFiltersRoutingWhileClosingGlobalReferenceGraph(t *testing.T) {
	producer := discoverSpec("requests", "service-request")
	producer.Program = ProgramName
	consumer := actSpec("stop", "service-request", "--id")
	consumer.Program = ExposureProgramName
	catalog := NewCatalog(producer, consumer)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("cross-program catalog: %v", err)
	}
	if _, found := catalog.Lookup("requests"); !found {
		t.Fatal("host producer is not routed")
	}
	if _, found := catalog.Lookup("stop"); found {
		t.Fatal("helper command leaked into host routing")
	}
	helper := catalog.ForProgram(ExposureProgramName)
	if _, found := helper.Lookup("stop"); !found {
		t.Fatal("helper consumer is not routed")
	}
	if _, found := helper.Lookup("requests"); found {
		t.Fatal("host command leaked into helper routing")
	}
	if got := helper.Commands()[0].Usage(); got != "tobari-expose stop --id <service-request-id>" {
		t.Fatalf("helper usage = %q", got)
	}

	closed := NewCatalog(consumer)
	if err := closed.Validate(); err == nil {
		t.Fatal("cross-program consumer without a producer passed")
	}
}

func TestCatalogRejectsInvalidCompletionMetadata(t *testing.T) {
	tests := []struct {
		name   string
		input  CommandInput
		needle string
	}{
		{name: "unknown source", input: CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Test value.", AllowedValues: []string{}, Completion: InputCompletion("unknown")}, needle: "completion source is invalid"},
		{name: "non-text", input: CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "Test value.", AllowedValues: []string{}, Completion: InputCompletionContextName}, needle: "completion requires text values"},
		{name: "finite values", input: CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Test value.", AllowedValues: []string{"one"}, Completion: InputCompletionContextName}, needle: "completion conflicts with finite allowed values"},
		{name: "opaque reference", input: CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Test value.", AllowedValues: []string{}, ReferenceKind: "test-item", Completion: InputCompletionContextName}, needle: "completion must not expose opaque references"},
		{name: "command flag", input: CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Test value.", AllowedValues: []string{}, Completion: InputCompletionCommand}, needle: "command completion requires a positional selector"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := utilitySpec("sample")
			spec.Args = "--value <value>"
			spec.Agent.Inputs = []CommandInput{test.input}
			err := NewCatalog(spec).Validate()
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("error = %v, want %q", err, test.needle)
			}
		})
	}
}

func TestDefaultCatalogSeparatesDeliveryFromCollectionCoverage(t *testing.T) {
	wantCoverage := map[string]CollectionCoverage{
		"doctor":           CollectionCoverageExhaustive,
		"help":             CollectionCoverageExhaustive,
		"version":          CollectionCoverageNotApplicable,
		"cluster up":       CollectionCoverageNotApplicable,
		"cluster status":   CollectionCoverageNotApplicable,
		"cluster denials":  CollectionCoverageBoundedWindow,
		"cluster logs":     CollectionCoverageBoundedWindow,
		"cluster down":     CollectionCoverageNotApplicable,
		"tobari":           CollectionCoverageNotApplicable,
		"status":           CollectionCoverageNotApplicable,
		"template list":    CollectionCoverageExhaustive,
		"template show":    CollectionCoverageNotApplicable,
		"context list":     CollectionCoverageExhaustive,
		"context show":     CollectionCoverageNotApplicable,
		"workspace list":   CollectionCoverageExhaustive,
		"workspace status": CollectionCoverageNotApplicable,
		"workspace delete": CollectionCoverageNotApplicable,
	}
	for path, coverage := range wantCoverage {
		command, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("default catalog lacks %q", path)
		}
		if command.Agent.Output.Delivery != OutputDeliveryComplete ||
			command.Agent.Output.CollectionCoverage != coverage {
			t.Errorf("%s output = %+v, want delivery complete and coverage %q", path, command.Agent.Output, coverage)
		}
	}
}

func TestSharedClusterCatalogDeclaresAuthBrokerLifecycle(t *testing.T) {
	if !buildIdentityHasBroker() {
		t.Skip("Auth Broker catalog exists only on the research surface")
	}
	t.Parallel()
	catalog := DefaultCatalog()
	up, found := catalog.Lookup("cluster up")
	if !found {
		t.Fatal("default catalog lacks cluster up")
	}
	if up.Agent.FixedTarget == nil || up.Agent.FixedTarget.Kind != tobari.ClusterTargetKind || up.Agent.FixedTarget.Scope != FixedTargetScopeToolLocal {
		t.Fatalf("cluster up final fixed target = %+v", up.Agent.FixedTarget)
	}

	logs, found := catalog.Lookup("cluster logs")
	if !found {
		t.Fatal("default catalog lacks cluster logs")
	}
	if logs.Args != "[--component auth-broker|gateway|opa|all] [--tail <lines>]" ||
		!reflect.DeepEqual(logs.Agent.Inputs[0].AllowedValues, []string{"auth-broker", "gateway", "opa", "all"}) {
		t.Fatalf("cluster logs component contract = usage %q inputs %+v", logs.Args, logs.Agent.Inputs[0])
	}

	status, found := catalog.Lookup("cluster status")
	if !found {
		t.Fatal("default catalog lacks cluster status")
	}
	if status.Agent.Output.JSONSchemaVersion != tobari.FinalClusterLifecycleSchemaVersion {
		t.Fatalf("cluster status schema version = %d, want %d", status.Agent.Output.JSONSchemaVersion, tobari.FinalClusterLifecycleSchemaVersion)
	}
	fieldNames := make([]string, 0, len(status.Agent.Output.Fields))
	var components *OutputField
	for _, field := range status.Agent.Output.Fields {
		fieldNames = append(fieldNames, field.Name)
		if field.Name == "components" {
			fieldCopy := field
			components = &fieldCopy
		}
	}
	for _, required := range []string{"authority", "contexts", "components"} {
		if !slices.Contains(fieldNames, required) {
			t.Errorf("cluster status output fields %v lack %q", fieldNames, required)
		}
	}
	if components == nil || components.Items == nil {
		t.Fatalf("cluster status components field = %+v", components)
	}
	var componentNames []string
	for _, field := range components.Items.Fields {
		if field.Name == "name" {
			componentNames = field.Enum
		}
	}
	for _, required := range []string{"gateway", "opa", "auth-broker", "credential-companion"} {
		if !slices.Contains(componentNames, required) {
			t.Errorf("cluster status component names %v lack %q", componentNames, required)
		}
	}
}

func TestStandardSharedClusterCatalogOmitsAuthBrokerLifecycle(t *testing.T) {
	if buildIdentityHasBroker() {
		t.Skip("standard-only catalog assertion")
	}
	catalog := DefaultCatalog()
	for _, spec := range catalog.Commands() {
		encoded, err := json.Marshal(spec.Agent)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "auth_broker") || strings.Contains(string(encoded), "auth-broker") {
			t.Fatalf("standard catalog exposed Auth Broker through %q: %s", spec.Path, encoded)
		}
	}
}

func TestReleaseCatalogDoesNotPublishResearchAuthorityVocabulary(t *testing.T) {
	if buildIdentityHasBroker() {
		t.Skip("release-surface catalog assertion")
	}
	for _, spec := range DefaultCatalog().Commands() {
		projection := struct {
			Path    string        `json:"path"`
			Summary string        `json:"summary"`
			Args    string        `json:"args"`
			Usage   string        `json:"usage"`
			Agent   AgentContract `json:"agent"`
		}{Path: spec.Path, Summary: spec.Summary, Args: spec.Args, Usage: spec.Usage(), Agent: spec.Agent}
		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatalf("marshal %q: %v", spec.Path, err)
		}
		agentHelp, err := (&CLI{catalog: DefaultCatalog()}).renderAgentHelp(spec.Path, true, []CommandSpec{spec})
		if err != nil {
			t.Fatalf("render scoped agent help %q: %v", spec.Path, err)
		}
		lower := strings.ToLower(string(encoded) + string(agentHelp) + string(renderCommandHelp(spec)))
		for _, forbidden := range []string{"broker", "provider", "vault", "root_key", "root key", "credential_companion", "companion"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("release catalog command %q publishes research vocabulary %q: %s", spec.Path, forbidden, encoded)
			}
		}
	}
}

func TestResearchCatalogPublishesOnlyFinalContextAuthenticationSurface(t *testing.T) {
	if !buildIdentityHasBroker() {
		t.Skip("research-surface catalog assertion")
	}
	catalog := DefaultCatalog()
	for _, path := range []string{"auth login", "auth import", "auth status", "auth logout"} {
		command, found := catalog.Lookup(path)
		if !found || command.Agent.Output.JSONSchemaVersion != 2 {
			t.Fatalf("final research auth command %q = found:%t schema:%d", path, found, command.Agent.Output.JSONSchemaVersion)
		}
		encoded, err := json.Marshal(command.Agent.Output)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"manifest", "workspace_manifest_id", "template_ref"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("final research auth command %q exposes predecessor owner %q: %s", path, forbidden, encoded)
			}
		}
	}
	if _, found := catalog.Lookup("manifest show"); found {
		t.Fatal("research Catalog retained predecessor manifest show")
	}
	status, _ := catalog.Lookup("auth status")
	produced := status.ProducedRefs()
	if len(produced) != 1 || produced[0].Kind != tobari.ContextReferenceKind {
		t.Fatalf("auth status produced refs = %v", produced)
	}
}

func TestDefaultCatalogRequiresSemanticTokensForEveryTextCommand(t *testing.T) {
	for _, command := range DefaultCatalog().Commands() {
		supportsText := false
		for _, format := range command.Agent.Output.Formats {
			if format == OutputFormatText {
				supportsText = true
				break
			}
		}
		if supportsText && command.Agent.Output.TextPresentation != TextPresentationSemanticTokens {
			t.Errorf("%s text presentation = %d, want semantic tokens", command.Path, command.Agent.Output.TextPresentation)
		}
	}
}

func TestDoctorCatalogDeclaresRecursiveSchemaV2Contract(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("doctor")
	if !found {
		t.Fatal("default catalog lacks doctor")
	}
	wantNames := []string{"check", "status", "detail", "blocked_by", "recovery"}
	gotNames := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		gotNames = append(gotNames, field.Name)
	}
	if spec.Agent.Output.JSONSchemaVersion != 1 || !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("doctor output = version %d fields %v", spec.Agent.Output.JSONSchemaVersion, gotNames)
	}
	check := spec.Agent.Output.Fields[0]
	status := spec.Agent.Output.Fields[1]
	blockedBy := spec.Agent.Output.Fields[3]
	recovery := spec.Agent.Output.Fields[4]
	if !reflect.DeepEqual(check.Enum, doctorCheckIDValues()) ||
		!reflect.DeepEqual(status.Enum, []string{"pass", "warn", "fail", "blocked"}) ||
		!blockedBy.Nullable || !reflect.DeepEqual(blockedBy.Enum, doctorCheckIDValues()) ||
		!recovery.Nullable || recovery.Type != OutputFieldTypeObject || len(recovery.Fields) != 2 ||
		recovery.Fields[0].Name != "action" || recovery.Fields[1].Name != "next_command" {
		t.Fatalf("doctor recursive fields = check:%+v status:%+v blocked:%+v recovery:%+v", check, status, blockedBy, recovery)
	}
	wantCheckCount := 0
	for _, item := range doctor.CheckInventory() {
		if buildIdentityHasBroker() || !isBrokerDoctorCheck(item.ID) {
			wantCheckCount++
		}
	}
	if len(check.Enum) != wantCheckCount {
		t.Fatalf("doctor check enum = %d, profile inventory = %d", len(check.Enum), wantCheckCount)
	}
	root := spec.Agent.Inputs[1]
	if root.DefaultValue == nil || *root.DefaultValue != "." || root.MinimumLength == nil || *root.MinimumLength != 1 {
		t.Fatalf("doctor root input = %+v", root)
	}
}

func TestCatalogRejectsMissingSemanticTokenPresentation(t *testing.T) {
	missing := utilitySpec("missing-semantic-presentation")
	missing.Agent.Output.TextPresentation = TextPresentationUnknown
	if err := NewCatalog(missing).Validate(); err == nil || !strings.Contains(err.Error(), "text output requires semantic-token presentation") {
		t.Fatalf("missing semantic presentation Validate() error = %v", err)
	}
}

func TestCatalogRejectsIncompleteDeclarations(t *testing.T) {
	valid := utilitySpec("valid")
	missingEffect := utilitySpec("missing-effect")
	missingEffect.Effect = operation.EffectUnknown
	missingRole := utilitySpec("missing-role")
	missingRole.Role = RoleUnknown
	badPath := utilitySpec("Bad Path")
	missingSummary := utilitySpec("missing-summary")
	missingSummary.Summary = ""
	missingHandler := utilitySpec("missing-handler")
	missingHandler.handler = nil

	tests := []Catalog{
		{},
		NewCatalog(missingEffect),
		NewCatalog(missingRole),
		NewCatalog(badPath),
		NewCatalog(missingSummary),
		NewCatalog(missingHandler),
		NewCatalog(valid, valid),
	}
	for index, catalog := range tests {
		if err := catalog.Validate(); err == nil {
			t.Errorf("invalid catalog %d passed validation", index)
		}
	}
}

func TestCatalogRejectsCommandPathNamespaceCollision(t *testing.T) {
	catalog := NewCatalog(utilitySpec("foo"), utilitySpec("foo bar"))
	if err := catalog.Validate(); err == nil || !strings.Contains(err.Error(), "command/namespace boundary") {
		t.Fatalf("Validate() error = %v, want command/namespace collision", err)
	}
}

func TestCatalogRejectsStructuralLineSeparators(t *testing.T) {
	for _, separator := range []rune{'\u2028', '\u2029'} {
		t.Run(strings.ToUpper(strconv.FormatInt(int64(separator), 16)), func(t *testing.T) {
			if err := validateContractText("test value", "before"+string(separator)+"after"); err == nil {
				t.Fatal("structural separator passed contract text validation")
			}

			spec := utilitySpec("test")
			spec.Args = "[label" + string(separator) + "]"
			if err := NewCatalog(spec).Validate(); err == nil || !strings.Contains(err.Error(), "invalid argument syntax") {
				t.Fatalf("Validate() error = %v, want invalid argument syntax", err)
			}
		})
	}
}

func TestArgumentSyntaxRequiredAndAllowedValuesMatchAgentInputs(t *testing.T) {
	valid := utilitySpec("configure")
	valid.Args = "[--mode fast|safe] <target> [label]"
	valid.Agent.Inputs = []CommandInput{
		{Name: "--mode", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select the operating mode.", AllowedValues: []string{"fast", "safe"}},
		{Name: "target", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Target value.", AllowedValues: []string{}},
		{Name: "label", Source: InputSourceArgument, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional display label.", AllowedValues: []string{}},
		{Name: "CLI_PROFILE", Source: InputSourceEnvironment, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional environment profile.", AllowedValues: []string{}},
	}
	if err := NewCatalog(valid).Validate(); err != nil {
		t.Fatalf("valid small argument grammar: %v", err)
	}

	tests := map[string]func(*CommandSpec){
		"optional flag declared required":       func(spec *CommandSpec) { spec.Agent.Inputs[0].Required = true },
		"required positional declared optional": func(spec *CommandSpec) { spec.Agent.Inputs[1].Required = false },
		"optional positional declared required": func(spec *CommandSpec) { spec.Agent.Inputs[2].Required = true },
		"enum order differs":                    func(spec *CommandSpec) { spec.Agent.Inputs[0].AllowedValues = []string{"safe", "fast"} },
		"enum set differs":                      func(spec *CommandSpec) { spec.Agent.Inputs[0].AllowedValues = []string{"fast"} },
		"free form claims enumeration":          func(spec *CommandSpec) { spec.Agent.Inputs[1].AllowedValues = []string{"fixed"} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := cloneCommandSpec(valid)
			mutate(&spec)
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatal("argument syntax mismatch passed validation")
			}
		})
	}
}

func TestCatalogValidatesTextMaximumLengthContract(t *testing.T) {
	minimum, maximum := int64(1), int64(36)
	valid := utilitySpec("probe")
	valid.Args = "--id <value>"
	valid.Agent.Inputs = []CommandInput{{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Bounded identifier.", AllowedValues: []string{},
		MinimumLength: &minimum, MaximumLength: &maximum,
	}}
	if err := NewCatalog(valid).Validate(); err != nil {
		t.Fatalf("valid text length bounds: %v", err)
	}

	for name, mutate := range map[string]func(*CommandInput){
		"non-text": func(input *CommandInput) { input.ValueKind = InputValueInteger },
		"negative maximum": func(input *CommandInput) {
			value := int64(-1)
			input.MaximumLength = &value
		},
		"minimum exceeds maximum": func(input *CommandInput) {
			value := int64(0)
			input.MaximumLength = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := cloneCommandSpec(valid)
			mutate(&spec.Agent.Inputs[0])
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatal("invalid text length bounds passed validation")
			}
		})
	}

	clone := cloneCommandSpec(valid)
	*clone.Agent.Inputs[0].MaximumLength = 10
	if *valid.Agent.Inputs[0].MaximumLength != 36 {
		t.Fatal("maximum length pointer shares cloned storage")
	}
}

func TestArgumentSyntaxAllowsOneExactFixedFlagValue(t *testing.T) {
	valid := utilitySpec("items apply")
	valid.Args = "--confirm=destructive"
	valid.Agent.Inputs = []CommandInput{{
		Name: "--confirm", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Confirm the exact mutation class.", AllowedValues: []string{"destructive"},
	}}
	if err := NewCatalog(valid).Validate(); err != nil {
		t.Fatalf("valid exact literal flag grammar: %v", err)
	}

	for name, allowed := range map[string][]string{
		"free form":     {},
		"wrong literal": {"access-change"},
		"extra literal": {"destructive", "access-change"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := cloneCommandSpec(valid)
			spec.Agent.Inputs[0].AllowedValues = allowed
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatal("exact literal syntax mismatch passed validation")
			}
		})
	}

	malformed := cloneCommandSpec(valid)
	malformed.Args = "--confirm=destructive=unexpected"
	if err := NewCatalog(malformed).Validate(); err == nil {
		t.Fatal("malformed exact literal syntax passed validation")
	}
}

func TestCatalogRequiresOneFaultSignatureAcrossCommands(t *testing.T) {
	first := utilitySpec("first")
	second := utilitySpec("second")
	if err := NewCatalog(first, second).Validate(); err != nil {
		t.Fatalf("matching fault signatures: %v", err)
	}

	for name, mutate := range map[string]func(*CommandError){
		"kind":         func(declared *CommandError) { declared.Kind = fault.KindUnavailable },
		"retryability": func(declared *CommandError) { declared.Retryable = true },
	} {
		t.Run(name, func(t *testing.T) {
			conflicting := cloneCommandSpec(second)
			for index := range conflicting.Agent.Errors {
				if conflicting.Agent.Errors[index].Code == "test_failed" {
					mutate(&conflicting.Agent.Errors[index])
				}
			}
			err := NewCatalog(first, conflicting).Validate()
			if err == nil || !strings.Contains(err.Error(), `fault code "test_failed" has conflicting signatures`) {
				t.Fatalf("Validate() error = %v, want conflicting fault signature", err)
			}
		})
	}
}

func TestCatalogFaultSignaturesIncludeAgentHelpGlobalErrors(t *testing.T) {
	matching := utilitySpec("matching")
	matching.Agent.Errors = append(matching.Agent.Errors, declaredCommandError(
		fault.KindContract,
		"invalid_catalog",
		false,
		matching.Path,
		"Repair the command-specific catalog observation.",
	))
	if err := NewCatalog(matching).Validate(); err != nil {
		t.Fatalf("matching global fault signature with command-local recovery: %v", err)
	}

	for name, declared := range map[string]CommandError{
		"kind": declaredCommandError(
			fault.KindUnavailable,
			"invalid_catalog",
			false,
			"test",
			"Retry after the unavailable dependency recovers.",
		),
		"retryability": declaredCommandError(
			fault.KindContract,
			"invalid_catalog",
			true,
			"test",
			"Retry after repairing the catalog.",
		),
	} {
		t.Run(name, func(t *testing.T) {
			conflicting := utilitySpec("test")
			conflicting.Agent.Errors = append(conflicting.Agent.Errors, declared)
			err := NewCatalog(conflicting).Validate()
			if err == nil || !strings.Contains(err.Error(), `fault code "invalid_catalog" has conflicting signatures`) {
				t.Fatalf("Validate() error = %v, want conflict with agent-help global error", err)
			}
		})
	}
}

func TestCatalogRequiresCommonRuntimeFailures(t *testing.T) {
	removeError := func(spec *CommandSpec, code string) {
		filtered := make([]CommandError, 0, len(spec.Agent.Errors))
		for _, declared := range spec.Agent.Errors {
			if declared.Code != code {
				filtered = append(filtered, declared)
			}
		}
		spec.Agent.Errors = filtered
	}
	for _, code := range []string{"operation_canceled", "output_write_failed"} {
		t.Run("missing_"+code, func(t *testing.T) {
			spec := utilitySpec("test")
			removeError(&spec, code)
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatalf("catalog without %q passed validation", code)
			}
		})
	}

	wrong := utilitySpec("test")
	for index := range wrong.Agent.Errors {
		if wrong.Agent.Errors[index].Code == "operation_canceled" {
			wrong.Agent.Errors[index].Retryable = false
		}
	}
	if err := NewCatalog(wrong).Validate(); err == nil {
		t.Fatal("catalog with inconsistent common runtime failure passed")
	}

	noOutput := utilitySpec("test")
	noOutput.Agent.Output.Formats = []OutputFormat{OutputFormatNone}
	noOutput.Agent.Output.DefaultFormat = OutputFormatNone
	noOutput.Agent.Output.Fields = []OutputField{}
	removeError(&noOutput, "output_write_failed")
	if err := NewCatalog(noOutput).Validate(); err != nil {
		t.Fatalf("no-output command unnecessarily requires output_write_failed: %v", err)
	}
	noOutput.Agent.Output.CollectionCoverage = CollectionCoverageExhaustive
	if err := NewCatalog(noOutput).Validate(); err == nil || !strings.Contains(err.Error(), "none output format requires collection coverage") {
		t.Fatalf("no-output command with collection coverage error = %v", err)
	}

	readWithMutationFailure := utilitySpec("read")
	readWithMutationFailure.Agent.Errors = append(readWithMutationFailure.Agent.Errors, declaredCommandError(
		fault.KindInternal, "mutation_output_write_failed", false, "read", "Reconcile without mutation replay.",
	))
	if err := NewCatalog(readWithMutationFailure).Validate(); err == nil || !strings.Contains(err.Error(), "must not declare mutation_output_write_failed") {
		t.Fatalf("read command with mutation output failure error = %v", err)
	}

	noOutputWithWriteFailure := cloneCommandSpec(noOutput)
	noOutputWithWriteFailure.Agent.Output.CollectionCoverage = CollectionCoverageNotApplicable
	noOutputWithWriteFailure.Agent.Errors = append(noOutputWithWriteFailure.Agent.Errors, declaredCommandError(
		fault.KindInternal, "output_write_failed", true, "read", "Retry with a writable stream.",
	))
	if err := NewCatalog(noOutputWithWriteFailure).Validate(); err == nil || !strings.Contains(err.Error(), "without output") {
		t.Fatalf("no-output command with write failure error = %v", err)
	}
}

func TestInputRelationsMustLeaveEveryDeclaredInputUsable(t *testing.T) {
	base := utilitySpec("inspect")
	base.Args = "[--a <value>] [--b <value>] [--c <value>]"
	base.Agent.Inputs = []CommandInput{
		{Name: "--a", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional A.", AllowedValues: []string{}},
		{Name: "--b", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional B.", AllowedValues: []string{}},
		{Name: "--c", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional C.", AllowedValues: []string{}},
	}
	if err := NewCatalog(base).Validate(); err != nil {
		t.Fatalf("valid independent optional inputs: %v", err)
	}

	optionalConflictsRequired := cloneCommandSpec(base)
	optionalConflictsRequired.Args = "--b <value> [--a <value>] [--c <value>]"
	optionalConflictsRequired.Agent.Inputs[1].Required = true
	optionalConflictsRequired.Agent.Inputs[0].ConflictsWith = []string{"--b"}
	if err := NewCatalog(optionalConflictsRequired).Validate(); err == nil || !strings.Contains(err.Error(), "unusable") {
		t.Fatalf("optional input conflicting with required input error = %v", err)
	}

	requiredRequiresOptional := cloneCommandSpec(base)
	requiredRequiresOptional.Args = "--a <value> [--b <value>] [--c <value>]"
	requiredRequiresOptional.Agent.Inputs[0].Required = true
	requiredRequiresOptional.Agent.Inputs[0].Requires = []string{"--b"}
	if err := NewCatalog(requiredRequiresOptional).Validate(); err == nil || !strings.Contains(err.Error(), "effectively mandatory") {
		t.Fatalf("required input requiring optional input error = %v", err)
	}

	transitiveConflict := cloneCommandSpec(base)
	transitiveConflict.Agent.Inputs[0].Requires = []string{"--b"}
	transitiveConflict.Agent.Inputs[1].Requires = []string{"--c"}
	transitiveConflict.Agent.Inputs[0].ConflictsWith = []string{"--c"}
	if err := NewCatalog(transitiveConflict).Validate(); err == nil || !strings.Contains(err.Error(), "unusable") {
		t.Fatalf("transitive dependency conflict error = %v", err)
	}
}

func TestAgentContractValidationFailsClosed(t *testing.T) {
	tests := map[string]func(*CommandSpec){
		"missing capability": func(spec *CommandSpec) { spec.Agent.CapabilityID = "" },
		"missing outcome":    func(spec *CommandSpec) { spec.Agent.Outcome = "" },
		"unknown inputs":     func(spec *CommandSpec) { spec.Agent.Inputs = nil },
		"unknown input source": func(spec *CommandSpec) {
			spec.Args = "--id <item-id>"
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Item ID.", AllowedValues: []string{}}}
		},
		"undocumented argument": func(spec *CommandSpec) {
			spec.Args = "--id <item-id>"
		},
		"input absent from syntax": func(spec *CommandSpec) {
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Item ID.", AllowedValues: []string{}}}
		},
		"missing input description": func(spec *CommandSpec) {
			spec.Args = "--id <item-id>"
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, AllowedValues: []string{}}}
		},
		"unknown allowed values": func(spec *CommandSpec) {
			spec.Args = "--id <item-id>"
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Item ID."}}
		},
		"unknown formats":        func(spec *CommandSpec) { spec.Agent.Output.Formats = nil },
		"unknown default format": func(spec *CommandSpec) { spec.Agent.Output.DefaultFormat = OutputFormatUnknown },
		"unknown fields":         func(spec *CommandSpec) { spec.Agent.Output.Fields = nil },
		"missing field description": func(spec *CommandSpec) {
			spec.Agent.Output.Fields[0].Description = ""
		},
		"unknown delivery": func(spec *CommandSpec) {
			spec.Agent.Output.Delivery = OutputDeliveryUnknown
		},
		"unknown collection coverage": func(spec *CommandSpec) {
			spec.Agent.Output.CollectionCoverage = CollectionCoverageUnknown
		},
		"unknown prerequisites": func(spec *CommandSpec) { spec.Agent.Prerequisites = nil },
		"unknown errors":        func(spec *CommandSpec) { spec.Agent.Errors = nil },
		"missing next action":   func(spec *CommandSpec) { spec.Agent.Errors[0].NextActions = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := utilitySpec("test")
			mutate(&spec)
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatal("incomplete agent contract passed validation")
			}
		})
	}
}

func TestCatalogMatchUsesLongestDeclarativePath(t *testing.T) {
	catalog := NewCatalog(utilitySpec("items"), utilitySpec("items list"))
	command, rest, found := catalog.Match([]string{"items", "list", "--limit", "2"})
	if !found {
		t.Fatal("Match() did not find a command")
	}
	if command.Path != "items list" {
		t.Fatalf("Match() path = %q, want items list", command.Path)
	}
	if got := strings.Join(rest, " "); got != "--limit 2" {
		t.Fatalf("Match() rest = %q", got)
	}
}

func TestCatalogEnforcesRoleAndReferenceFlowContracts(t *testing.T) {
	discover := discoverSpec("items list", "item")
	act := actSpec("items read", "item", "--id")
	if err := NewCatalog(discover, act).Validate(); err != nil {
		t.Fatalf("valid reference flow: %v", err)
	}

	utilityWithRef := discoverSpec("utility", "item")
	utilityWithRef.Role = RoleUtility
	mutatingDiscovery := discoverSpec("items list", "item")
	mutatingDiscovery.Effect = operation.EffectWrite
	emptyDiscovery := utilitySpec("items list")
	emptyDiscovery.Role = RoleDiscover
	emptyAct := utilitySpec("items read")
	emptyAct.Role = RoleAct
	optionalAct := actSpec("items inspect", "item", "--id")
	optionalAct.Args = "[--id <item-id>]"
	optionalAct.Agent.Inputs[0].Required = false
	invalidProducer := discoverSpec("items list", "Item")
	invalidConsumer := actSpec("items read", "Item", "--id")

	invalid := []Catalog{
		NewCatalog(utilityWithRef, act),
		NewCatalog(mutatingDiscovery, act),
		NewCatalog(emptyDiscovery),
		NewCatalog(emptyAct),
		NewCatalog(discover, optionalAct),
		NewCatalog(discover),
		NewCatalog(act),
		NewCatalog(invalidProducer, act),
		NewCatalog(discover, invalidConsumer),
	}
	for index, catalog := range invalid {
		if err := catalog.Validate(); err == nil {
			t.Errorf("invalid role/reference catalog %d passed validation", index)
		}
	}
}

func TestCatalogDeclaresInteractiveDiscoverToActionWorkflow(t *testing.T) {
	discover := discoverSpec("items list", "item")
	discover.Agent.Interactive = &InteractiveWorkflowContract{
		ActionCommand:          "items allow",
		SelectionReferenceKind: "item",
		SelectionOutputField:   "id",
		Confirmation:           "explicit_yes",
		NonInteractiveBehavior: "read_only",
	}
	action := actSpec("items allow", "item", "--id")
	action.Effect = operation.EffectWrite
	action.Agent.Errors = mutationErrors(action.Agent.Errors, action.Path)
	action.Agent.Mutation = &MutationContract{
		TargetKind: "item", TargetInputs: []string{"--id"}, TargetIDInput: "--id",
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
		},
	}
	if err := NewCatalog(discover, action).Validate(); err != nil {
		t.Fatalf("valid interactive workflow: %v", err)
	}

	invalid := cloneCommandSpec(discover)
	invalid.Agent.Interactive.ActionCommand = "items missing"
	if err := NewCatalog(invalid, action).Validate(); err == nil || !strings.Contains(err.Error(), "interactive action") {
		t.Fatalf("missing interactive action validation = %v", err)
	}

	invalid = cloneCommandSpec(discover)
	invalid.Agent.Interactive.Confirmation = "implicit"
	if err := NewCatalog(invalid, action).Validate(); err == nil || !strings.Contains(err.Error(), "explicit_yes") {
		t.Fatalf("confirmation validation = %v", err)
	}
}

func TestInteractiveWorkflowValidatesRecursiveSelectionPath(t *testing.T) {
	discover := discoverSpec("runtimes list", "runtime")
	discover.Agent.Output.Fields = []OutputField{{
		Name: "items", Type: OutputFieldTypeArray, Description: "Runtime choices.",
		Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Runtime choice.", Fields: []OutputField{{
			Name: "runtime_ref", Type: OutputFieldTypeString, Description: "Opaque Runtime reference.", ReferenceKind: "runtime",
		}}},
	}}
	discover.Agent.Interactive = &InteractiveWorkflowContract{
		ActionCommand: "runtimes build", SelectionReferenceKind: "runtime",
		SelectionOutputField: "items[].runtime_ref", Confirmation: "explicit_yes", NonInteractiveBehavior: "read_only",
	}
	action := actSpec("runtimes build", "runtime", "--id")
	action.Effect = operation.EffectWrite
	action.Agent.Errors = mutationErrors(action.Agent.Errors, action.Path)
	action.Agent.Mutation = &MutationContract{
		TargetKind: "runtime", TargetInputs: []string{"--id"}, TargetIDInput: "--id",
		Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo},
	}
	if err := NewCatalog(discover, action).Validate(); err != nil {
		t.Fatalf("nested interactive selection: %v", err)
	}

	wrongKind := cloneCommandSpec(discover)
	wrongKind.Agent.Interactive.SelectionReferenceKind = "other"
	if err := NewCatalog(wrongKind, action).Validate(); err == nil || !strings.Contains(err.Error(), "must produce reference kind") {
		t.Fatalf("nested wrong-kind selection validation = %v", err)
	}

	missing := cloneCommandSpec(discover)
	missing.Agent.Interactive.SelectionOutputField = "items[].missing_ref"
	if err := NewCatalog(missing, action).Validate(); err == nil || !strings.Contains(err.Error(), "is not declared") {
		t.Fatalf("nested missing selection validation = %v", err)
	}
}

func TestCatalogValidatesCommandBoundToolLocalFixedTargets(t *testing.T) {
	valid := fixedTargetActSpec("auth status")
	if err := NewCatalog(valid).Validate(); err != nil {
		t.Fatalf("valid fixed target: %v", err)
	}

	for name, mutate := range map[string]func(*CommandSpec){
		"missing kind":        func(spec *CommandSpec) { spec.Agent.FixedTarget.Kind = "" },
		"missing ID":          func(spec *CommandSpec) { spec.Agent.FixedTarget.ID = "" },
		"missing description": func(spec *CommandSpec) { spec.Agent.FixedTarget.Description = "" },
		"missing scope":       func(spec *CommandSpec) { spec.Agent.FixedTarget.Scope = FixedTargetScopeUnknown },
		"wrong scope":         func(spec *CommandSpec) { spec.Agent.FixedTarget.Scope = "provider" },
		"non-act role":        func(spec *CommandSpec) { spec.Role = RoleUtility },
		"consumed reference": func(spec *CommandSpec) {
			spec.Args = "--id <auth-config-id>"
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque config ID.", AllowedValues: []string{}, ReferenceKind: "auth-config"}}
		},
		"produced reference": func(spec *CommandSpec) {
			spec.Agent.Output.Fields[0].ReferenceKind = "auth-config"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneCommandSpec(valid)
			mutate(&candidate)
			if err := NewCatalog(candidate).Validate(); err == nil {
				t.Fatal("invalid fixed target passed validation")
			}
		})
	}

	clone := cloneCommandSpec(valid)
	clone.Agent.FixedTarget.ID = "changed"
	if valid.Agent.FixedTarget.ID != "selected" {
		t.Fatal("fixed target pointer was not deep-copied")
	}
	encoded, err := json.Marshal(valid.Agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind":"auth-config"`, `"id":"selected"`, `"scope":"tool_local"`, `"description":"This CLI installation's selected authentication configuration."`} {
		if !bytes.Contains(encoded, []byte(want)) {
			t.Errorf("fixed target JSON lacks %s: %s", want, encoded)
		}
	}
}

func TestFixedTargetMutationPreservesMutationSafetyContract(t *testing.T) {
	status := fixedTargetActSpec("auth status")
	write := fixedTargetActSpec("auth reset")
	write.Effect = operation.EffectWrite
	write.Agent.Errors = mutationErrors(write.Agent.Errors, write.Path)
	for index := range write.Agent.Errors {
		if write.Agent.Errors[index].Code == "unclassified_mutation_outcome" ||
			write.Agent.Errors[index].Code == "mutation_output_write_failed" {
			write.Agent.Errors[index].NextActions[0].Command = status.Path
		}
	}
	write.Agent.Mutation = &MutationContract{
		TargetKind: "auth-config", TargetInputs: []string{},
		Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes},
	}
	if err := NewCatalog(status, write).Validate(); err != nil {
		t.Fatalf("valid fixed-target mutation: %v", err)
	}

	for name, mutate := range map[string]func(*CommandSpec){
		"target kind mismatch": func(spec *CommandSpec) { spec.Agent.Mutation.TargetKind = "other" },
		"nil target inputs":    func(spec *CommandSpec) { spec.Agent.Mutation.TargetInputs = nil },
		"unknown target input": func(spec *CommandSpec) { spec.Agent.Mutation.TargetInputs = []string{"--missing"} },
		"nonempty target input": func(spec *CommandSpec) {
			spec.Agent.Mutation.TargetInputs = []string{"--id"}
		},
		"parent input":    func(spec *CommandSpec) { spec.Agent.Mutation.ParentInput = "--parent-id" },
		"target ID input": func(spec *CommandSpec) { spec.Agent.Mutation.TargetIDInput = "--id" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneCommandSpec(write)
			mutate(&candidate)
			if err := validateAgentContract(candidate); err == nil {
				t.Fatal("invalid fixed-target mutation passed validation")
			}
		})
	}

	create := cloneCommandSpec(write)
	create.Path = "auth initialize"
	create.Effect = operation.EffectCreate
	if err := validateAgentContract(create); err != nil {
		t.Fatalf("fixed target as create scope: %v", err)
	}
}

func TestFixedTargetCreateMayProduceOnlyDistinctConfirmedChildReferences(t *testing.T) {
	status := fixedTargetActSpec("attachments status")
	create := fixedTargetActSpec("attachments create-child")
	create.Effect = operation.EffectCreate
	create.Agent.Errors = mutationErrors(create.Agent.Errors, create.Path)
	for index := range create.Agent.Errors {
		if create.Agent.Errors[index].Code == "unclassified_mutation_outcome" ||
			create.Agent.Errors[index].Code == "mutation_output_write_failed" {
			create.Agent.Errors[index].NextActions[0].Command = status.Path
		}
	}
	create.Agent.Output.Fields[0].ReferenceKind = "attachment-child"
	create.Agent.Mutation = &MutationContract{
		TargetKind: "auth-config", TargetInputs: []string{},
		Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
	}
	consume := actSpec("attachments stop-child", "attachment-child", "--id")
	if err := NewCatalog(status, create, consume).Validate(); err != nil {
		t.Fatalf("fixed-target create with confirmed child reference: %v", err)
	}

	for name, mutate := range map[string]func(*CommandSpec){
		"scope kind escapes": func(spec *CommandSpec) { spec.Agent.Output.Fields[0].ReferenceKind = spec.Agent.FixedTarget.Kind },
		"write produces":     func(spec *CommandSpec) { spec.Effect = operation.EffectWrite },
		"read produces":      func(spec *CommandSpec) { spec.Effect = operation.EffectRead },
		"create consumes": func(spec *CommandSpec) {
			spec.Args = "--id <parent-id>"
			spec.Agent.Inputs = []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque parent.", AllowedValues: []string{}, ReferenceKind: "parent"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneCommandSpec(create)
			mutate(&candidate)
			if err := validateCommandReferenceRole(candidate); err == nil {
				t.Fatal("invalid fixed-target reference shape passed validation")
			}
		})
	}
}

func TestReferenceGraphRejectsClosedCyclesAndAcceptsReachableChains(t *testing.T) {
	selfCycle := actSpec("items rotate", "item", "--id")
	selfCycle.Agent.Output.Fields[0] = OutputField{
		Name: "id", Type: OutputFieldTypeString, Description: "Rotated item ID.", ReferenceKind: "item",
	}
	if err := NewCatalog(selfCycle).Validate(); err == nil || !strings.Contains(err.Error(), "closed required-reference cycle") {
		t.Fatalf("self-contained reference cycle error = %v", err)
	}

	alpha := actSpec("alpha derive", "beta", "--beta-id")
	alpha.Agent.Output.Fields[0] = OutputField{
		Name: "alpha_id", Type: OutputFieldTypeString, Description: "Derived alpha ID.", ReferenceKind: "alpha",
	}
	beta := actSpec("beta derive", "alpha", "--alpha-id")
	beta.Agent.Output.Fields[0] = OutputField{
		Name: "beta_id", Type: OutputFieldTypeString, Description: "Derived beta ID.", ReferenceKind: "beta",
	}
	if err := NewCatalog(alpha, beta).Validate(); err == nil || !strings.Contains(err.Error(), "closed required-reference cycle") {
		t.Fatalf("multi-kind reference cycle error = %v", err)
	}

	workspaces := discoverSpec("workspaces list", "workspace")
	items := discoverSpec("items list", "item")
	items.Args = "--workspace-id <workspace-id>"
	items.Agent.Inputs = []CommandInput{{
		Name: "--workspace-id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Opaque workspace ID.", AllowedValues: []string{}, ReferenceKind: "workspace",
	}}
	read := actSpec("items read", "item", "--id")
	if err := NewCatalog(workspaces, items, read).Validate(); err != nil {
		t.Fatalf("reachable reference chain failed validation: %v", err)
	}
}

func TestReferenceGraphAllowsMultipleInputsOfTheSameKind(t *testing.T) {
	discover := discoverSpec("items list", "item")
	act := actSpec("items compare", "item", "--left-id", "--right-id")
	catalog := NewCatalog(discover, act)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	consumed := act.ConsumedRefs()
	if len(consumed) != 2 || consumed[0].Argument != "--left-id" || consumed[1].Argument != "--right-id" {
		t.Fatalf("ConsumedRefs() = %+v", consumed)
	}
	workflows := catalog.referenceWorkflows()
	if len(workflows) != 1 || len(workflows[0].Producers) != 1 || len(workflows[0].Consumers) != 2 ||
		workflows[0].Consumers[0].Input != "--left-id" || workflows[0].Consumers[1].Input != "--right-id" {
		t.Fatalf("reference workflows = %+v, want one grouped kind with both inputs", workflows)
	}
}

func TestRecursiveOutputReferencesDriveCatalogGraphAndCanonicalPaths(t *testing.T) {
	producer := recursiveReferenceProducerSpec()
	want := []ProducedRef{
		{Kind: "runtime", Field: "items[].runtime_ref"},
		{Kind: "owner", Field: "metadata.owner_ref"},
		{Kind: "identifier", Field: "ids[]"},
		{Kind: "runtime-revision", Field: "revisions[].revision_ref"},
		{Kind: "runtime-prune-plan", Field: "plan_ref"},
	}
	if got := producer.ProducedRefs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ProducedRefs() = %+v, want %+v", got, want)
	}
	mutated := producer.ProducedRefs()
	mutated[0].Kind = "changed"
	if got := producer.ProducedRefs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("mutating returned references changed declaration: %+v", got)
	}

	commands := []CommandSpec{
		producer,
		actSpec("runtime inspect", "runtime", "--id"),
		actSpec("owners inspect", "owner", "--id"),
		actSpec("identifiers inspect", "identifier", "--id"),
		actSpec("runtime revision inspect", "runtime-revision", "--id"),
		actSpec("runtime prune inspect", "runtime-prune-plan", "--plan"),
	}
	catalog := NewCatalog(commands...)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("recursive produced-reference catalog failed validation: %v", err)
	}

	workflows := catalog.referenceWorkflows()
	if len(workflows) != len(want) {
		t.Fatalf("referenceWorkflows() = %+v", workflows)
	}
	for index, workflow := range workflows {
		if workflow.ReferenceKind != want[index].Kind || len(workflow.Producers) != 1 ||
			workflow.Producers[0].Field != want[index].Field || len(workflow.Consumers) != 1 {
			t.Fatalf("workflow %d = %+v, want kind=%q field=%q", index, workflow, want[index].Kind, want[index].Field)
		}
	}
}

func TestProducedReferenceTraversalIsBoundedIndependently(t *testing.T) {
	leaf := OutputField{Type: OutputFieldTypeString, Description: "Opaque reference.", ReferenceKind: "item"}
	for depth := 0; depth < maxOutputFieldDepth; depth++ {
		leaf = OutputField{Type: OutputFieldTypeArray, Description: "Nested references.", Items: &leaf}
	}
	spec := discoverSpec("items list", "item")
	spec.Agent.Output.Fields = []OutputField{{Name: "items", Type: OutputFieldTypeArray, Description: "Nested references.", Items: &leaf}}
	if _, err := spec.producedRefs(); err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("unbounded produced reference traversal error = %v", err)
	}
}

func TestInvalidCatalogFailsBeforeDispatch(t *testing.T) {
	called := false
	bad := utilitySpec("unsafe")
	bad.Effect = operation.EffectUnknown
	bad.handler = func(context.Context, *CLI, CommandSpec, operation.Intent, ParsedInputs) int {
		called = true
		return ExitOK
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, NewCatalog(bad), nil)
	if code := runCLI(command, []string{"unsafe"}); code != ExitContract {
		t.Fatalf("Run() code = %d, want %d", code, ExitContract)
	}
	if called {
		t.Fatal("handler ran for an invalid catalog")
	}
	if !humanOutputHasRow(stderr.String(), "Code", "invalid_catalog") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCatalogCommandsReturnsDeepCopy(t *testing.T) {
	catalog := DefaultCatalog()
	commands := catalog.Commands()
	commands[0].Path = "changed"
	commands[0].Agent.Outcome = "changed"
	commands[0].Agent.Output.Formats[0] = OutputFormatNone
	commands[0].Agent.Output.Fields[0].Name = "changed"
	commands[0].Agent.Inputs[0].AllowedValues[0] = "changed"
	commands[0].Agent.Prerequisites = append(commands[0].Agent.Prerequisites, "changed")
	commands[0].Agent.Errors[0].Code = "changed"
	commands[0].Agent.Errors[0].NextActions[0].Command = "changed"

	doctor, found := catalog.Lookup("doctor")
	if !found {
		t.Fatal("mutating Commands() changed the catalog")
	}
	want, _ := DefaultCatalog().Lookup("doctor")
	if !reflect.DeepEqual(doctor.Agent, want.Agent) {
		t.Fatalf("nested agent contract was mutated: %+v", doctor.Agent)
	}
}

func TestMutationContractFailsClosedAndDeepCopies(t *testing.T) {
	spec := utilitySpec("items update")
	spec.Effect = operation.EffectWrite
	spec.Role = RoleAct
	spec.Agent.Errors = mutationErrors(spec.Agent.Errors, spec.Path)
	spec.Args = "--id <item-id>"
	spec.Agent.Inputs = []CommandInput{{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Target item ID.", AllowedValues: []string{}, ReferenceKind: "item",
	}}
	spec.Agent.Mutation = &MutationContract{
		TargetKind: "item", TargetInputs: []string{"--id"}, TargetIDInput: "--id",
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatalf("valid mutation contract: %v", err)
	}
	if err := NewCatalog(discoverSpec("items list", "item"), spec).Validate(); err != nil {
		t.Fatalf("valid act mutation catalog: %v", err)
	}
	withReadOutputFailure := cloneCommandSpec(spec)
	withReadOutputFailure.Agent.Errors = append(withReadOutputFailure.Agent.Errors, declaredCommandError(
		fault.KindInternal,
		"output_write_failed",
		true,
		withReadOutputFailure.Path,
		"Retry the mutation with a writable output stream.",
	))
	if err := NewCatalog(discoverSpec("items list", "item"), withReadOutputFailure).Validate(); err == nil ||
		!strings.Contains(err.Error(), "must not declare retryable output_write_failed") {
		t.Fatalf("mutation with read output failure error = %v", err)
	}
	for _, code := range []string{"unclassified_mutation_outcome", "mutation_output_write_failed"} {
		unsafeRecovery := cloneCommandSpec(spec)
		for index := range unsafeRecovery.Agent.Errors {
			if unsafeRecovery.Agent.Errors[index].Code == code {
				unsafeRecovery.Agent.Errors[index].NextActions[0].Command = unsafeRecovery.Path
			}
		}
		if err := NewCatalog(discoverSpec("items list", "item"), unsafeRecovery).Validate(); err == nil ||
			!strings.Contains(err.Error(), "read-only reconciliation") {
			t.Fatalf("unsafe %s recovery error = %v", code, err)
		}
	}
	unsafePartial := cloneCommandSpec(spec)
	unsafePartial.Agent.Errors = append(unsafePartial.Agent.Errors, CommandError{
		Kind: fault.KindInternal, Code: "partial_test_failure",
		Phase: fault.PhaseVerification, ChangeState: fault.ChangePartial,
		NextActions: []fault.NextAction{{Command: unsafePartial.Path, Reason: "Unsafe mutation replay."}},
	})
	if err := NewCatalog(discoverSpec("items list", "item"), unsafePartial).Validate(); err == nil ||
		!strings.Contains(err.Error(), "read-only reconciliation") {
		t.Fatalf("unsafe partial recovery error = %v", err)
	}

	rateLimited := cloneCommandSpec(spec)
	rateLimited.Agent.Errors = append(rateLimited.Agent.Errors, declaredCommandError(
		fault.KindRateLimited,
		"mutation_rate_limited",
		false,
		rateLimited.Path,
		"Wait for the provider window, then reconcile before deciding on another mutation.",
	))
	if err := NewCatalog(discoverSpec("items list", "item"), rateLimited).Validate(); err == nil ||
		!strings.Contains(err.Error(), "read-only reconciliation") {
		t.Fatalf("unsafe non-retryable rate-limit recovery error = %v", err)
	}
	for index := range rateLimited.Agent.Errors {
		if rateLimited.Agent.Errors[index].Code == "mutation_rate_limited" {
			rateLimited.Agent.Errors[index].NextActions[0].Command = "items list"
		}
	}
	if err := NewCatalog(discoverSpec("items list", "item"), rateLimited).Validate(); err != nil {
		t.Fatalf("read-only rate-limit recovery rejected: %v", err)
	}

	missing := cloneCommandSpec(spec)
	missing.Agent.Mutation = nil
	if err := validateAgentContract(missing); err == nil {
		t.Fatal("mutation without declaration passed")
	}
	wrongInput := cloneCommandSpec(spec)
	wrongInput.Agent.Mutation.TargetInputs[0] = "--missing"
	if err := validateAgentContract(wrongInput); err == nil {
		t.Fatal("mutation with unknown target input passed")
	}
	clone := cloneCommandSpec(spec)
	clone.Agent.Mutation.TargetInputs[0] = "changed"
	if spec.Agent.Mutation.TargetInputs[0] != "--id" {
		t.Fatal("mutation target inputs share storage")
	}
	missingTargetBinding := cloneCommandSpec(spec)
	missingTargetBinding.Agent.Mutation.TargetIDInput = ""
	if err := validateAgentContract(missingTargetBinding); err == nil {
		t.Fatal("write mutation without target ID binding passed")
	}
	mismatchedTargetKind := cloneCommandSpec(spec)
	mismatchedTargetKind.Agent.Mutation.TargetKind = "other"
	if err := validateAgentContract(mismatchedTargetKind); err == nil {
		t.Fatal("write mutation with mismatched target reference kind passed")
	}
	optionalTarget := cloneCommandSpec(spec)
	optionalTarget.Args = "[--id <item-id>]"
	optionalTarget.Agent.Inputs[0].Required = false
	if err := validateAgentContract(optionalTarget); err == nil || !strings.Contains(err.Error(), "must be required") {
		t.Fatalf("optional mutation target error = %v", err)
	}
	configuredTarget := cloneCommandSpec(spec)
	configuredTarget.Args = ""
	configuredTarget.Agent.Inputs[0].Name = "id"
	configuredTarget.Agent.Inputs[0].Source = InputSourceConfiguration
	configuredTarget.Agent.Mutation.TargetInputs[0] = "id"
	configuredTarget.Agent.Mutation.TargetIDInput = "id"
	if err := validateAgentContract(configuredTarget); err == nil || !strings.Contains(err.Error(), "command argument or flag") {
		t.Fatalf("non-CLI mutation target error = %v", err)
	}
	withParent := cloneCommandSpec(spec)
	withParent.Args += " --collection-id <collection-id>"
	withParent.Agent.Inputs = append(withParent.Agent.Inputs, CommandInput{
		Name: "--collection-id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Parent collection ID.", AllowedValues: []string{}, ReferenceKind: "collection",
	})
	withParent.Agent.Mutation.ParentInput = "--collection-id"
	withParent.Agent.Mutation.TargetInputs = append(withParent.Agent.Mutation.TargetInputs, "--collection-id")
	if err := validateAgentContract(withParent); err != nil {
		t.Fatalf("write mutation with parent binding: %v", err)
	}
	ambiguousTargets := cloneCommandSpec(withParent)
	ambiguousTargets.Args += " --scope-id <scope-id>"
	ambiguousTargets.Agent.Inputs = append(ambiguousTargets.Agent.Inputs, CommandInput{
		Name: "--scope-id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Unbound scope ID.", AllowedValues: []string{}, ReferenceKind: "scope",
	})
	ambiguousTargets.Agent.Mutation.TargetInputs = append(ambiguousTargets.Agent.Mutation.TargetInputs, "--scope-id")
	if err := validateAgentContract(ambiguousTargets); err == nil {
		t.Fatal("write mutation with an unbound target input passed")
	}
	encoded, err := json.Marshal(spec.Agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"target_id_input":"--id"`, `"cardinality":"one"`, `"notification":"no"`, `"access_change":"no"`, `"destructive":"no"`} {
		if !bytes.Contains(encoded, []byte(want)) {
			t.Errorf("mutation JSON lacks %s: %s", want, encoded)
		}
	}
}

func TestMutationContractRequiresInvokerFailureSurface(t *testing.T) {
	spec := utilitySpec("items update")
	spec.Effect = operation.EffectWrite
	spec.Role = RoleAct
	spec.Agent.Errors = mutationErrors(spec.Agent.Errors, spec.Path)
	spec.Args = "--id <item-id>"
	spec.Agent.Inputs = []CommandInput{{
		Name: "--id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Target item ID.", AllowedValues: []string{}, ReferenceKind: "item",
	}}
	spec.Agent.Mutation = &MutationContract{
		TargetKind: "item", TargetInputs: []string{"--id"}, TargetIDInput: "--id",
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatalf("valid mutation failure surface: %v", err)
	}
	for _, missing := range []string{"invalid_mutation_contract", "missing_mutation_action", "missing_mutation_policy", "mutation_rejected", "unclassified_mutation_outcome", "mutation_output_write_failed"} {
		t.Run(missing, func(t *testing.T) {
			candidate := cloneCommandSpec(spec)
			filtered := make([]CommandError, 0, len(candidate.Agent.Errors)-1)
			for _, declared := range candidate.Agent.Errors {
				if declared.Code != missing {
					filtered = append(filtered, declared)
				}
			}
			candidate.Agent.Errors = filtered
			if err := validateAgentContract(candidate); err == nil {
				t.Fatalf("mutation without %q passed", missing)
			}
		})
	}
}

func TestCreateMutationBindsOpaqueParentOnly(t *testing.T) {
	spec := utilitySpec("items create")
	spec.Effect = operation.EffectCreate
	spec.Role = RoleAct
	spec.Agent.Errors = mutationErrors(spec.Agent.Errors, spec.Path)
	spec.Args = "--collection-id <collection-id>"
	spec.Agent.Inputs = []CommandInput{{
		Name: "--collection-id", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Parent collection ID.", AllowedValues: []string{}, ReferenceKind: "collection",
	}}
	spec.Agent.Mutation = &MutationContract{
		TargetKind: "item", TargetInputs: []string{"--collection-id"}, ParentInput: "--collection-id",
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatalf("valid create mutation: %v", err)
	}

	missingParent := cloneCommandSpec(spec)
	missingParent.Agent.Mutation.ParentInput = ""
	if err := validateAgentContract(missingParent); err == nil {
		t.Fatal("create mutation without parent binding passed")
	}
	withTargetID := cloneCommandSpec(spec)
	withTargetID.Agent.Mutation.TargetIDInput = "--collection-id"
	if err := validateAgentContract(withTargetID); err == nil {
		t.Fatal("create mutation with an existing target ID passed")
	}
	parentOutsideTargets := cloneCommandSpec(spec)
	parentOutsideTargets.Agent.Mutation.TargetInputs = []string{"--missing"}
	if err := validateAgentContract(parentOutsideTargets); err == nil {
		t.Fatal("create mutation with unbound parent passed")
	}
}

func TestCatalogValidatesExecutableRecoveryCommandGrammar(t *testing.T) {
	help, found := DefaultCatalog().Lookup("help")
	if !found {
		t.Fatal("default catalog lacks help")
	}
	version, found := DefaultCatalog().Lookup("version")
	if !found {
		t.Fatal("default catalog lacks version")
	}
	doctor, found := DefaultCatalog().Lookup("doctor")
	if !found {
		t.Fatal("default catalog lacks doctor")
	}
	itemsList := utilitySpec("items list")
	for _, action := range []string{"help", "items list", "help items", "help items list"} {
		t.Run("valid_"+strings.ReplaceAll(action, " ", "_"), func(t *testing.T) {
			spec := utilitySpec("test")
			spec.Agent.Errors[0].NextActions[0].Command = action
			if err := NewCatalog(help, version, doctor, itemsList, spec).Validate(); err != nil {
				t.Fatalf("valid recovery command %q: %v", action, err)
			}
		})
	}

	for _, action := range []string{
		"missing command",
		"items list --bogus",
		"help nonexistent",
		"help items list extra",
		"help items --format agent",
		"items  list",
	} {
		t.Run("invalid_"+strings.ReplaceAll(action, " ", "_"), func(t *testing.T) {
			spec := utilitySpec("test")
			spec.Agent.Errors[0].NextActions[0].Command = action
			if err := NewCatalog(help, version, doctor, itemsList, spec).Validate(); err == nil {
				t.Fatalf("invalid recovery command %q passed catalog validation", action)
			}
		})
	}
}
