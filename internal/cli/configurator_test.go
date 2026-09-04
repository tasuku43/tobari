package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func legacyConfiguratorTestCatalog(t *testing.T) Catalog {
	t.Helper()
	commands := append(DefaultCatalog().Commands(), configureSpec())
	catalog := NewCatalog(commands...)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestAggregateConfigureIsNotPublic(t *testing.T) {
	if _, found := DefaultCatalog().Lookup("configure"); found {
		t.Fatal("retired aggregate configure remains public")
	}
}

type failingConfiguratorEntryWriter struct{ bytes.Buffer }

func (w *failingConfiguratorEntryWriter) Write(value []byte) (int, error) {
	if strings.Contains(string(value), "Tobari · Entering isolated Configurator") {
		return 0, io.ErrClosedPipe
	}
	return w.Buffer.Write(value)
}

func (w *failingConfiguratorEntryWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

type configuratorStageFixture struct {
	order                  *[]string
	ref                    string
	fingerprint            string
	pending                *tobari.ConfiguratorPendingStage
	policyPublished        bool
	policyPendingPlanRef   string
	policyPublicationError error
}

func (f configuratorStageFixture) StageConfiguratorSubmission(_ context.Context, submission tobari.ConfiguratorSubmission) (tobari.ConfiguratorStage, error) {
	*f.order = append(*f.order, "stage")
	return tobari.ConfiguratorStage{SchemaVersion: tobari.ConfiguratorStageSchemaVersion, TemplateRef: f.ref, SourceRevision: submission.SourceRevision, SourceFingerprint: f.fingerprint}, nil
}
func (f configuratorStageFixture) DiscardConfiguratorStage(context.Context, tobari.ConfiguratorSubmission, tobari.ConfiguratorStage) error {
	return nil
}
func (f configuratorStageFixture) PendingConfiguratorStage(context.Context, tobari.WorkspaceTemplateID) (tobari.ConfiguratorPendingStage, bool, error) {
	if f.pending != nil {
		return *f.pending, true, nil
	}
	return tobari.ConfiguratorPendingStage{}, false, nil
}
func (f configuratorStageFixture) PendingConfiguratorStageForProject(context.Context, string) (tobari.ConfiguratorPendingStage, bool, error) {
	if f.pending != nil {
		return *f.pending, true, nil
	}
	return tobari.ConfiguratorPendingStage{}, false, nil
}
func (f configuratorStageFixture) BindConfiguratorStagePlan(_ context.Context, pending tobari.ConfiguratorPendingStage, planRef string) (tobari.ConfiguratorPendingStage, error) {
	pending.PlanRef = planRef
	return pending, pending.Validate()
}
func (f configuratorStageFixture) ConfirmConfiguratorStageApply(_ context.Context, pending tobari.ConfiguratorPendingStage) (tobari.ConfiguratorPendingStage, error) {
	pending.ApplyConfirmed = true
	return pending, pending.Validate()
}
func (f configuratorStageFixture) ConfirmConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission, tobari.ContextAuthoritySnapshot) error {
	return nil
}
func (f configuratorStageFixture) BeginConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error {
	return nil
}
func (f configuratorStageFixture) CompleteConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error {
	return nil
}
func (f configuratorStageFixture) PendingConfiguratorPublicationForProject(context.Context, string) (tobari.ConfiguratorSubmission, bool, error) {
	return tobari.ConfiguratorSubmission{}, false, nil
}
func (f configuratorStageFixture) ConfiguratorPolicyPublished(context.Context, tobari.ConfiguratorSubmission) (bool, string, error) {
	return f.policyPublished, f.policyPendingPlanRef, f.policyPublicationError
}

func TestRenderConfiguratorJSONProjectsHostileStructuralUnicode(t *testing.T) {
	rendered := renderConfiguratorJSON(map[string]string{"value": "safe\u202ebidi\u200b\u2028line\x1b[31m"})
	for _, hostile := range []string{"\u202e", "\u200b", "\u2028", "\x1b"} {
		if strings.Contains(rendered, hostile) {
			t.Fatalf("rendered review retained hostile rune %q: %q", hostile, rendered)
		}
	}
	for _, visible := range []string{`\u202E`, `\u200B`, `\\u2028`, `\\u001b`} {
		if !strings.Contains(strings.ToLower(rendered), strings.ToLower(visible)) {
			t.Fatalf("rendered review omitted visible projection %q: %q", visible, rendered)
		}
	}
}

func TestConfiguratorChooserExplainsAgentHomeEgressAndHostApply(t *testing.T) {
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	choice, err := newFirstUseSetupSelectorWithStyle(false).Choose(context.Background(), seed, strings.NewReader("\n\n"), &output)
	if err != nil || choice != firstUseSetupCodex {
		t.Fatalf("choice=%v err=%v output=%q", choice, err, output.String())
	}
	for _, phrase := range []string{"Tobari · Create this Project configuration", "Choose how to configure this Project", "Agent Home", "Internet", "Gateway policy is not active", "Activation", "host-reviewed Apply only", "Codex", "Claude Code", "Manual setup", "Tobari · Codex is ready", "isolated Configurator", "The next screen is Codex's native interface", "Open Codex", "Go back"} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("chooser omitted %q: %q", phrase, output.String())
		}
	}
}

func TestConfiguratorHandoffCanReturnToAgentChoiceBeforeMutation(t *testing.T) {
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	choice, err := newFirstUseSetupSelectorWithStyle(false).Choose(
		context.Background(), seed, strings.NewReader("\n2\n3\n"), &output,
	)
	if err != nil || choice != firstUseSetupManual {
		t.Fatalf("choice=%v err=%v output=%q", choice, err, output.String())
	}
	if count := strings.Count(output.String(), "Tobari · Create this Project configuration"); count != 2 {
		t.Fatalf("agent chooser rendered %d times after Go back: %q", count, output.String())
	}
}

func TestConfiguratorChooserUsesTwoStyledInlineScreensBeforeAgentHandoff(t *testing.T) {
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	mode := &selectorModeFake{}
	selector := &terminalFirstUseSetupSelector{chooser: &terminalContextConfigurationWizard{mode: mode, style: true}}
	output := &sizedSelectorBuffer{rows: 40, columns: 120}
	choice, err := selector.Choose(context.Background(), seed, strings.NewReader("\r\r"), output)
	if err != nil || choice != firstUseSetupCodex {
		t.Fatalf("choice=%v err=%v output=%q", choice, err, output.String())
	}
	if mode.entered != 2 || mode.restored != 2 {
		t.Fatalf("raw screens entered=%d restored=%d", mode.entered, mode.restored)
	}
	if count := strings.Count(output.String(), selectorCursorSave); count != 2 {
		t.Fatalf("inline-screen origins=%d output=%q", count, output.String())
	}
	if strings.Contains(output.String(), "\x1b[?1049h") || strings.Contains(output.String(), "\x1b[?1049l") {
		t.Fatalf("Configurator selector used the terminal alternate screen: %q", output.String())
	}
	for _, phrase := range []string{"Tobari · Create this Project configuration", "Tobari · Codex is ready", "Open Codex"} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("raw handoff omitted %q: %q", phrase, output.String())
		}
	}
}

func TestConfiguratorHandoffLinePresentationMatchesGolden(t *testing.T) {
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	open, err := confirmConfiguratorHandoff(
		context.Background(), newContextConfigurationWizardWithStyle(false), seed,
		tobari.ConfiguratorAgentCodex, strings.NewReader("\n"), &output,
	)
	if err != nil || !open {
		t.Fatalf("open=%v err=%v output=%q", open, err, output.String())
	}
	want, err := os.ReadFile("testdata/configurator_handoff_line.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, expected := output.String(), strings.TrimSuffix(string(want), "\n")+" "; got != expected {
		t.Fatalf("handoff presentation changed\n--- got ---\n%s\n--- want ---\n%s", got, expected)
	}
}

func TestPolicyAssistHandoffOmitsAmbientProjectLocation(t *testing.T) {
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(finalCurrentContextEntrySnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	open, err := confirmConfiguratorHandoff(
		context.Background(), newContextConfigurationWizardWithStyle(false), seed,
		tobari.ConfiguratorAgentCodex, strings.NewReader("\n"), &output,
	)
	if err != nil || !open {
		t.Fatalf("open=%v err=%v output=%q", open, err, output.String())
	}
	if strings.Contains(output.String(), "\n  Project             ") || (seed.ProjectRoot != "" && strings.Contains(output.String(), seed.ProjectRoot)) {
		t.Fatalf("Policy assistance exposed ambient Project location: %q", output.String())
	}
}

func TestRuntimeAssistReviewNamesInstallationOwnedHome(t *testing.T) {
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	base := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, "018bcfe5-687b-7000-8000-000000000077", base)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, seed.Initial)
	if err != nil {
		t.Fatal(err)
	}
	submission, err = submission.WithRuntimeSource(tobari.ConfiguratorRuntimeSource{SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: draft.TargetRuntimeID, BaseRevision: base, FrozenRevision: base})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	action, err := newConfiguratorSubmissionReviewerWithStyle(false).Review(context.Background(), seed, submission, strings.NewReader("2\n"), &output)
	if err != nil || action != configuratorSubmissionEdit {
		t.Fatalf("action=%v err=%v output=%q", action, err, output.String())
	}
	if !strings.Contains(output.String(), "installation-owned Home") || strings.Contains(output.String(), "same Context Home") {
		t.Fatalf("Runtime assistance used Context-owned wording: %q", output.String())
	}
}

type configuratorPlanReviewerFixture struct{ order *[]string }

func (f configuratorPlanReviewerFixture) Review(context.Context, tobari.ConfiguratorSubmission, tobari.WorkspaceTemplateChangePlan, io.Reader, io.Writer) (bool, error) {
	*f.order = append(*f.order, "plan review")
	return true, nil
}

func TestConfigureConsumesCatalogAgentAndRetainsOnlyDraft(t *testing.T) {
	order := []string{}
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &stdout, &stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	pair := &firstEntryPairFixture{order: &order, observation: tobari.FinalDefaultPairObservation{SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, ProjectRoot: "/workspace/example"}}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(draft.TemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: draft.TemplateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pair.resolution.Observation.Context = &tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: draft.TemplateID}, Template: template, PolicyMemory: memory}
	command.finalDefaultPair = pair
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) { return body, nil }
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: &order},
		runner,
	)
	command.configuratorReview = configuratorSubmissionReviewerFixture{order: &order, action: configuratorSubmissionApply}
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "codex"}); code != ExitOK {
		t.Fatalf("configure exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Configuration applied") || !strings.Contains(stderr.String(), "Gateway policy is not active") {
		t.Fatalf("configure streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, phrase := range []string{"Tobari · Entering isolated Configurator", "Agent: Codex", "Tobari · Returned from Codex", "exact configuration draft is frozen"} {
		if !strings.Contains(stderr.String(), phrase) {
			t.Fatalf("agent boundary omitted %q: %q", phrase, stderr.String())
		}
	}
}

func TestConfigurePreservesPartialConfiguratorCleanupOutcome(t *testing.T) {
	order := []string{}
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &stdout, &stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, observation: tobari.FinalDefaultPairObservation{
		SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, ProjectRoot: "/workspace/example",
	}}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) { return body, nil }
	runner := firstEntryConfiguratorRunnerFixture{order: &order, runErr: errors.Join(tobari.ErrNativeLoginBridgeUnavailable, tobari.ErrConfiguratorTransientCleanupUnknown)}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: &order},
		runner,
	)
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "codex"}); code != ExitUnavailable {
		t.Fatalf("configure cleanup outcome exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	if !strings.Contains(stderr.String(), "bounded cleanup could not confirm removal") || slices.Contains(order, "draft freeze") {
		t.Fatalf("configure cleanup outcome was not preserved: order=%v stderr=%q", order, stderr.String())
	}
}

func TestConfigureExplicitAgentCanBackOutBeforeRuntimeOrDraftMutation(t *testing.T) {
	order := []string{}
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("2\n"), &stdout, &stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalDefaultPair = &firstEntryPairFixture{
		order: &order,
		observation: tobari.FinalDefaultPairObservation{
			SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion,
			ProjectRoot:   "/workspace/example",
		},
	}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) { return body, nil }
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: &order},
		runner,
	)
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "codex"}); code != ExitCanceled {
		t.Fatalf("configure exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	if !reflect.DeepEqual(order, []string{"observe", "readiness"}) {
		t.Fatalf("handoff cancellation crossed mutation boundary: %v", order)
	}
	if !strings.Contains(stderr.String(), "Tobari · Codex is ready") || !strings.Contains(stderr.String(), "canceled before starting the selected agent") {
		t.Fatalf("handoff cancellation output=%q", stderr.String())
	}
}

func TestConfigureEntryBoundaryFailurePrecedesRuntimeAndDraftMutation(t *testing.T) {
	order := []string{}
	body := finalAxisTemplateBody("/configured")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	stderr := &failingConfiguratorEntryWriter{}
	command := newCLI(strings.NewReader("\n"), &stdout, stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, observation: tobari.FinalDefaultPairObservation{
		SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, ProjectRoot: "/workspace/example",
	}}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) { return body, nil }
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: body}, runner,
		configuratorStageFixture{order: &order}, runner,
	)
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "codex"}); code != ExitInternal {
		t.Fatalf("configure exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	if !reflect.DeepEqual(order, []string{"observe", "readiness"}) {
		t.Fatalf("entry boundary failure crossed Runtime/draft mutation: %v", order)
	}
}

func TestConfigureExistingContextUsesItsRuntimeThenStagesPlansAndApplies(t *testing.T) {
	order := []string{}
	templates := newFinalTemplateBatchPort(t)
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templates.template.ID}
	snapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: templates.template.Clone(), PolicyMemory: memory}
	snapshot.Workspace = &tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789ad"), ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: templates.template.Current.Slices.CreationDefaultsDigest}
	snapshot.ContextHome = snapshot.Workspace.Home
	snapshot.ContextCreationDefaults = snapshot.Workspace.CreationDefaults
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	selection := tobari.FinalDefaultPairSelection{
		SchemaVersion: tobari.FinalDefaultPairSelectionSchemaVersion, CollectionPresent: true, CollectionGeneration: 1,
		CollectionRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64)), CanonicalCWD: "/workspace/example",
		DefaultTemplate: func() *tobari.WorkspaceTemplate { value := templates.template.Clone(); return &value }(),
		Candidates:      []tobari.FinalDefaultPairCandidate{{Snapshot: snapshot.Clone()}},
	}
	selected := workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: contextID}}
	seed, err := tobari.NewEvolveConfiguratorSeed("/workspace/example", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, templates.template.ID)
	if err != nil {
		t.Fatal(err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(templates.template.ID)
	preview, err := templates.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &stdout, &stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, selected: &selected}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(templates)
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: snapshot.Template.Current.Body},
		runner,
		configuratorStageFixture{order: &order, ref: templateRef, fingerprint: preview.SourceFingerprint},
		runner,
	)
	command.configuratorReview = configuratorSubmissionReviewerFixture{order: &order, action: configuratorSubmissionApply}
	command.configuratorPlanReview = configuratorPlanReviewerFixture{order: &order}
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "claude"}); code != ExitOK {
		t.Fatalf("evolve configure exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	want := []string{"observe", "readiness", "draft reserve", "draft materialize", "runtime prepare", "agent", "draft freeze", "submission review", "stage", "plan review"}
	if !reflect.DeepEqual(order, want) || !strings.Contains(stderr.String(), "Update this Project configuration") || !strings.Contains(stderr.String(), "Configuration already current") {
		t.Fatalf("evolve order=%v want=%v stderr=%q", order, want, stderr.String())
	}
}

func TestConfigureResumesExactConfirmedStageWithoutRestartingAgent(t *testing.T) {
	order := []string{}
	templates := newFinalTemplateBatchPort(t)
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templates.template.ID}, Template: templates.template.Clone(), PolicyMemory: memory}
	snapshot.Workspace = &tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789ad"), ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: templates.template.Current.Slices.CreationDefaultsDigest}
	snapshot.ContextHome = snapshot.Workspace.Home
	snapshot.ContextCreationDefaults = snapshot.Workspace.CreationDefaults
	selection := tobari.FinalDefaultPairSelection{SchemaVersion: tobari.FinalDefaultPairSelectionSchemaVersion, CollectionPresent: true, CollectionGeneration: 1, CollectionRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64)), CanonicalCWD: snapshot.Workspace.ProjectRoot, DefaultTemplate: func() *tobari.WorkspaceTemplate { value := templates.template.Clone(); return &value }(), Candidates: []tobari.FinalDefaultPairCandidate{{Snapshot: snapshot.Clone()}}}
	selected := workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: contextID}}
	seed, err := tobari.NewEvolveConfiguratorSeed("/workspace/example", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templates.template.ID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, snapshot.Template.Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(templates.template.ID)
	plan, err := templates.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef)
	if err != nil {
		t.Fatal(err)
	}
	pending := tobari.ConfiguratorPendingStage{Submission: submission, Stage: tobari.ConfiguratorStage{SchemaVersion: tobari.ConfiguratorStageSchemaVersion, TemplateRef: templateRef, SourceRevision: submission.SourceRevision, SourceFingerprint: plan.SourceFingerprint}, PlanRef: plan.PlanRef, ApplyConfirmed: true}
	if err := pending.Validate(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &stdout, &stderr, legacyConfiguratorTestCatalog(t), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, selected: &selected}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(templates)
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: submission.Body}, runner, configuratorStageFixture{order: &order, pending: &pending}, runner)
	if code := command.RunContext(context.Background(), []string{"configure", "--agent", "codex"}); code != ExitOK {
		t.Fatalf("resume exit=%d order=%v stderr=%q", code, order, stderr.String())
	}
	if !reflect.DeepEqual(order, []string{"observe"}) || !strings.Contains(stderr.String(), "Configuration already current") {
		t.Fatalf("resume restarted authoring: order=%v stderr=%q", order, stderr.String())
	}
}
