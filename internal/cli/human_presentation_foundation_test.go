package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	humanPresentationFixtureSHA256 = "e4e786546630bb263a4afd8ae62356f7a29954bbfea2e1831ce697d1bdbd5714"
	humanPresentationAnswerSHA256  = "f2dbd3c1c819abf0da5ee05121b13178f9d2889afa9092563a4104de80e1fd32"
)

type humanPresentationFixture struct {
	SchemaVersion         int                          `json:"schema_version"`
	Lifecycle             tobari.ProjectStatus         `json:"lifecycle"`
	EmptyPolicyCandidates tobari.PolicyCandidateReport `json:"empty_policy_candidates"`
	Warning               doctor.Report                `json:"warning"`
	Failure               errorPayload                 `json:"failure"`
	Cancel                errorPayload                 `json:"cancel"`
}

type humanPresentationAnswer struct {
	SchemaVersion  int `json:"schema_version"`
	RoutineSuccess struct {
		TaskInvocations         int `json:"task_invocations"`
		ExternalProcessingSteps int `json:"external_processing_steps"`
	} `json:"routine_success"`
	Cases []struct {
		Name                  string   `json:"name"`
		Task                  string   `json:"task"`
		RequiredFacts         []string `json:"required_facts"`
		ExactNextArgv         []string `json:"exact_next_argv"`
		UnsupportedInferences []string `json:"unsupported_inferences"`
	} `json:"cases"`
}

func readPinnedHumanPresentationCorpus(t *testing.T) (humanPresentationFixture, humanPresentationAnswer) {
	t.Helper()
	read := func(path, wantSHA string, target any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != wantSHA {
			t.Fatalf("%s SHA-256 = %s, want %s", path, got, wantSHA)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	var fixture humanPresentationFixture
	var answer humanPresentationAnswer
	read("testdata/human-presentation-foundation-fixture.json", humanPresentationFixtureSHA256, &fixture)
	read("testdata/human-presentation-foundation-answer-key.json", humanPresentationAnswerSHA256, &answer)
	return fixture, answer
}

func humanOutputHasRow(value, label, want string) bool {
	for _, line := range strings.Split(stripANSIStyles(value), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, label) {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, label)) == want {
			return true
		}
	}
	return false
}

func TestPinnedHumanPresentationCorpusDrivesEveryTerminalMode(t *testing.T) {
	fixture, answer := readPinnedHumanPresentationCorpus(t)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 {
		t.Fatalf("presentation corpus schema = fixture:%d answer:%d", fixture.SchemaVersion, answer.SchemaVersion)
	}
	if err := fixture.Lifecycle.Validate(); err != nil {
		t.Fatalf("lifecycle fixture: %v", err)
	}
	if err := fixture.EmptyPolicyCandidates.Validate(); err != nil {
		t.Fatalf("empty collection fixture: %v", err)
	}
	if err := fixture.Warning.Validate(); err != nil {
		t.Fatalf("warning fixture: %v", err)
	}
	if answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalProcessingSteps != 0 {
		t.Fatalf("routine-success answer = %+v", answer.RoutineSuccess)
	}

	type render func(color bool) ([]byte, error)
	renders := map[string]render{
		"lifecycle": func(color bool) ([]byte, error) {
			return renderProjectStatusWithColor(fixture.Lifecycle, successFormatText, color)
		},
		"scoped_empty_collection": func(color bool) ([]byte, error) {
			return renderPolicyCandidatesWithColor(fixture.EmptyPolicyCandidates, "tobari policy allow", successFormatText, color)
		},
		"warning": func(color bool) ([]byte, error) {
			return renderDoctorReportWithColor(fixture.Warning, successFormatText, color)
		},
		"failure": func(color bool) ([]byte, error) {
			return renderTextErrorWithColor(fixture.Failure, color), nil
		},
		"cancel": func(color bool) ([]byte, error) {
			return renderTextErrorWithColor(fixture.Cancel, color), nil
		},
	}
	if len(answer.Cases) != len(renders) {
		t.Fatalf("answer cases = %d, want %d", len(answer.Cases), len(renders))
	}
	for _, answerCase := range answer.Cases {
		renderCase, found := renders[answerCase.Name]
		if !found {
			t.Fatalf("answer names unknown presentation case %q", answerCase.Name)
		}
		t.Run(answerCase.Name, func(t *testing.T) {
			colored, err := renderCase(true)
			if err != nil {
				t.Fatal(err)
			}
			noColor, err := renderCase(false)
			if err != nil {
				t.Fatal(err)
			}
			redirected, err := renderCase(false)
			if err != nil {
				t.Fatal(err)
			}
			plain := string(noColor)
			if strings.Contains(plain, "\x1b[") || plain != string(redirected) || plain != stripANSIStyles(string(colored)) {
				t.Fatalf("terminal mode changed structure\n--- colored ---\n%q\n--- NO_COLOR ---\n%q\n--- redirected ---\n%q", colored, noColor, redirected)
			}
			for _, fact := range answerCase.RequiredFacts {
				if !strings.Contains(plain, fact) {
					t.Errorf("output lacks required fact %q: %q", fact, plain)
				}
			}
			if len(answerCase.ExactNextArgv) > 0 && !strings.Contains(plain, strings.Join(answerCase.ExactNextArgv, " ")) {
				t.Errorf("output lacks exact next argv %q: %q", answerCase.ExactNextArgv, plain)
			}
			if len(answerCase.UnsupportedInferences) == 0 {
				t.Error("answer key must state at least one negative-inference canary")
			}
			for _, unsupported := range answerCase.UnsupportedInferences {
				if strings.Contains(plain, unsupported) {
					t.Errorf("output invented unsupported inference %q: %q", unsupported, plain)
				}
			}
			if answerCase.Name == "cancel" && (strings.Contains(string(colored), ansiStyleTokens[styleDanger]) || strings.Contains(string(colored), ansiStyleTokens[styleSuccess])) {
				t.Errorf("cancel uses failure/success styling: %q", colored)
			}
		})
	}
}

type humanPresentationFixtureRuntime struct {
	policyReviewRuntimeFake
}

func TestPinnedCorpusCrossesCLITerminalStyleSelectionBoundary(t *testing.T) {
	fixture, _ := readPinnedHumanPresentationCorpus(t)
	run := func(terminal, noColor bool) string {
		t.Helper()
		runtime := &humanPresentationFixtureRuntime{
			policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: terminal},
		}
		inspector := passingInspector("ready")
		inspector.observations = make(map[doctor.CheckID]doctor.Observation, len(fixture.Warning.Checks))
		for _, check := range fixture.Warning.Checks {
			inspector.observations[check.Name] = doctor.Observation{Status: check.Status, Detail: check.Detail}
		}
		command, stdout, stderr := newTestCLI(inspector)
		command.tobari = tobaricmd.New(runtime)
		command.noColor = noColor
		if code := command.RunContext(context.Background(), []string{"doctor"}); code != ExitOK {
			t.Fatalf("doctor exit = %d, stderr = %q", code, stderr.String())
		}
		return stdout.String()
	}
	coloredTTY := run(true, false)
	noColorTTY := run(true, true)
	redirected := run(false, false)
	if !strings.Contains(coloredTTY, ansiStyleTokens[styleWarning]) {
		t.Fatalf("colored TTY did not cross style projection: %q", coloredTTY)
	}
	if strings.Contains(noColorTTY, "\x1b[") || strings.Contains(redirected, "\x1b[") {
		t.Fatalf("ANSI crossed disabled style boundary: NO_COLOR=%q redirected=%q", noColorTTY, redirected)
	}
	if plain := stripANSIStyles(coloredTTY); plain != noColorTTY || plain != redirected {
		t.Fatalf("CLI style selection changed semantic document\ncolored=%q\nNO_COLOR=%q\nredirected=%q", coloredTTY, noColorTTY, redirected)
	}
}

type idlePollThenInput struct {
	polls int
	input string
}

func (r *idlePollThenInput) Read(buffer []byte) (int, error) {
	if r.polls > 0 {
		r.polls--
		return 0, nil
	}
	if r.input == "" {
		return 0, errors.New("test input exhausted")
	}
	buffer[0] = r.input[0]
	r.input = r.input[1:]
	return 1, nil
}

func TestHumanTextStructureDoesNotDependOnANSIStyle(t *testing.T) {
	t.Parallel()
	status := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: true, Root: "/workspace/example-project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/state/example/home",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "toolbox",
		Runtime: tobari.RuntimeDiagnosticReady, Attachment: tobari.AttachmentDetached,
	}
	emptyCandidates := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: "/config/example/policy",
		WindowLines: 200, Items: []tobari.PolicyCandidate{},
	}
	deleted := tobari.ProjectDeleteResult{
		Task: tobari.TaskDelete, Deleted: true, Root: status.Root, ID: status.ID,
		Home: status.Home, ContextID: status.ContextID, ContextName: status.ContextName,
	}
	errorCase := errorPayload{
		Kind: fault.KindUnavailable, Code: "cluster_not_running", Message: "The cluster is not running.",
		NextActions: []fault.NextAction{{Command: "cluster up", Reason: "Start the shared cluster."}},
	}
	canceled := errorPayload{
		Kind: fault.KindCanceled, Code: "operation_canceled", Message: "The operation was canceled.", Retryable: true,
		NextActions: []fault.NextAction{{Command: "status", Reason: "Retry when the caller is ready."}},
	}

	statusPlain, err := renderProjectStatusWithColor(status, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	statusStyled, err := renderProjectStatusWithColor(status, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	emptyPlain, err := renderPolicyCandidatesWithColor(emptyCandidates, "tobari policy allow", successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	emptyStyled, err := renderPolicyCandidatesWithColor(emptyCandidates, "tobari policy allow", successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		plain  []byte
		styled []byte
	}{
		{name: "lifecycle scalar", plain: statusPlain, styled: statusStyled},
		{name: "scoped empty collection", plain: emptyPlain, styled: emptyStyled},
		{name: "lifecycle mutation", plain: renderProjectDeleteWithColor(deleted, false), styled: renderProjectDeleteWithColor(deleted, true)},
		{name: "failure", plain: renderTextErrorWithColor(errorCase, false), styled: renderTextErrorWithColor(errorCase, true)},
		{name: "cancel", plain: renderTextErrorWithColor(canceled, false), styled: renderTextErrorWithColor(canceled, true)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plain := string(test.plain)
			styled := stripANSIStyles(string(test.styled))
			if strings.Contains(plain, "\x1b[") || plain != styled {
				t.Fatalf("style changed semantic structure\n--- plain ---\n%s--- styled (stripped) ---\n%s", plain, styled)
			}
		})
	}
	if got := string(emptyPlain); !strings.Contains(got, "No policy candidates") || !strings.Contains(got, "200") {
		t.Fatalf("scoped empty state lost meaning or bounds: %q", got)
	}
	if got := string(renderTextErrorWithColor(canceled, true)); !strings.Contains(stripANSIStyles(got), "· Canceled") || strings.Contains(got, ansiStyleTokens[styleDanger]) || strings.Contains(got, ansiStyleTokens[styleSuccess]) {
		t.Fatalf("cancellation is not neutral: %q", got)
	}
}

func TestEveryTextCollectionHasAnExplicitScopedEmptyState(t *testing.T) {
	policyDirectory := "/config/example/policy"
	type emptyCase struct {
		name     string
		plain    []byte
		styled   []byte
		required []string
	}
	candidates := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: policyDirectory, WindowLines: 200, Items: []tobari.PolicyCandidate{},
	}
	review := candidates
	review.Task = tobari.TaskPolicyReview
	rules := tobari.PolicyRuleReport{Task: tobari.TaskPolicyRules, PolicyDirectory: policyDirectory, Items: []tobari.PolicyRule{}}
	denials := tobari.DenialReport{Task: tobari.TaskClusterDenials, PolicyDirectory: policyDirectory, WindowLines: 200, Items: []tobari.PolicyDenial{}}
	projectPlain, err := renderProjectListWithColor(tobari.ProjectListResult{Task: tobari.TaskProjectList, Items: []tobari.ProjectListItem{}}, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	projectStyled, err := renderProjectListWithColor(tobari.ProjectListResult{Task: tobari.TaskProjectList, Items: []tobari.ProjectListItem{}}, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	candidatePlain, _ := renderPolicyCandidatesWithColor(candidates, "tobari policy allow", successFormatText, false)
	candidateStyled, _ := renderPolicyCandidatesWithColor(candidates, "tobari policy allow", successFormatText, true)
	denialPlain, _ := renderClusterDenialsWithColor(denials, "tobari policy review", successFormatText, false)
	denialStyled, _ := renderClusterDenialsWithColor(denials, "tobari policy review", successFormatText, true)
	cases := []emptyCase{
		{name: "policy candidates", plain: candidatePlain, styled: candidateStyled, required: []string{"No policy candidates", policyDirectory, "200 Gateway lines"}},
		{name: "policy review", plain: renderPolicyReviewHuman(review, "tobari policy allow", "tobari policy deny", false), styled: renderPolicyReviewHuman(review, "tobari policy allow", "tobari policy deny", true), required: []string{"No pending network permissions", policyDirectory, "200 Gateway lines"}},
		{name: "policy rules", plain: renderPolicyRulesHuman(rules, "tobari policy reset", false), styled: renderPolicyRulesHuman(rules, "tobari policy reset", true), required: []string{"No learned policy decisions", policyDirectory}},
		{name: "cluster denials", plain: denialPlain, styled: denialStyled, required: []string{"No policy denials", policyDirectory, "200 Gateway lines"}},
		{name: "Workspaces", plain: projectPlain, styled: projectStyled, required: []string{"No Workspaces", "No Workspace state is configured"}},
		{name: "auth providers", plain: renderAuthStatusText(authStatusProjection{Context: "toolbox", ContextID: stringPointer("018bcfe5-687b-7000-8000-000000000099"), Providers: []authProviderStatusProjection{}}, false), styled: renderAuthStatusText(authStatusProjection{Context: "toolbox", ContextID: stringPointer("018bcfe5-687b-7000-8000-000000000099"), Providers: []authProviderStatusProjection{}}, true), required: []string{"No authentication providers installed", "toolbox", "explicitly empty"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plain := string(test.plain)
			if plain == "" || plain != stripANSIStyles(string(test.styled)) {
				t.Fatalf("empty state differs by style: plain=%q styled=%q", test.plain, test.styled)
			}
			for _, required := range test.required {
				if !strings.Contains(plain, required) {
					t.Errorf("empty state lacks scope/bound %q: %q", required, plain)
				}
			}
		})
	}
}

func TestCatalogWideHumanPresentationIsDeclared(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	commands := catalog.Commands()
	if got, want := len(commands), 34; got != want {
		t.Fatalf("catalog command count = %d, want %d; update the human presentation inventory", got, want)
	}
	for _, command := range commands {
		text := false
		for _, format := range command.Agent.Output.Formats {
			text = text || format == OutputFormatText
		}
		if text && command.Agent.Output.TextPresentation != TextPresentationSemanticTokens {
			t.Errorf("text command %q does not use shared semantic presentation", command.Path)
		}
	}
	root := (&CLI{catalog: catalog}).renderRootHelpWithColor(false)
	if !strings.Contains(string(root), "Tobari") || strings.Contains(string(root), "\x1b[") {
		t.Fatalf("catalog-owned root help is not semantic ANSI-free text: %q", root)
	}
}

func TestProductionPresentationNeverBranchesOnStyleCapability(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || path == "styles.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			usesStyleCapability := false
			ast.Inspect(statement.Cond, func(condition ast.Node) bool {
				identifier, ok := condition.(*ast.Ident)
				if ok && (identifier.Name == "color" || identifier.Name == "style") {
					usesStyleCapability = true
				}
				return true
			})
			if usesStyleCapability {
				t.Errorf("%s:%d branches semantic structure on ANSI style capability", path, fileSet.Position(statement.Pos()).Line)
			}
			return true
		})
	}
}

func TestRawSelectorsDoNotRedrawDuringIdlePollsAndRestoreTerminal(t *testing.T) {
	tests := []struct {
		name  string
		title string
		run   func(*idlePollThenInput, *bytes.Buffer) error
	}{
		{
			name: "workspace", title: "Select a Workspace for",
			run: func(input *idlePollThenInput, output *bytes.Buffer) error {
				_, err := selectWorkspaceRaw(context.Background(), testWorkspaceSelection(), input, output, false)
				return err
			},
		},
		{
			name: "permission review", title: "Tobari · Permission Inbox",
			run: func(input *idlePollThenInput, output *bytes.Buffer) error {
				decision, err := selectPolicyReviewRaw(context.Background(), testPolicyReviewReport(), input, output, false, nil, nil, "")
				if err == nil && !decision.Canceled {
					return errors.New("permission review did not cancel")
				}
				return err
			},
		},
		{
			name: "policy rules", title: "Tobari · Policy decisions",
			run: func(input *idlePollThenInput, output *bytes.Buffer) error {
				report := tobari.PolicyRuleReport{Task: tobari.TaskPolicyRules, PolicyDirectory: "/config/example/policy", Items: []tobari.PolicyRule{{
					ID: "prl_0123456789abcdef0123456789abcdef", Decision: tobari.PolicyDecisionAllow, Match: tobari.PolicyMatchExact,
					ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
					ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/example",
					Host: "api.example.com", Port: 443, Method: "GET", Path: "/v1/example",
				}}}
				decision, err := selectPolicyRulesRaw(context.Background(), report, input, output, false)
				if err == nil && !decision.Canceled {
					return errors.New("policy rules did not cancel")
				}
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := &idlePollThenInput{polls: 4, input: "q"}
			var output bytes.Buffer
			err := test.run(input, &output)
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("selector error = %v", err)
			}
			if got := strings.Count(stripANSIStyles(output.String()), test.title); got != 1 {
				t.Fatalf("title render count after idle polls = %d, output = %q", got, output.String())
			}
			if got := strings.Count(output.String(), "\x1b[?25h"); got != 1 {
				t.Fatalf("terminal restore count = %d, output = %q", got, output.String())
			}
		})
	}
}

func TestBareNamespacesAndUnknownSuggestionsComeOnlyFromCatalog(t *testing.T) {
	for _, namespace := range []string{"cluster", "policy", "context", "config", "runtime", "auth"} {
		t.Run("namespace "+namespace, func(t *testing.T) {
			command, stdout, stderr := newTestCLI(passingInspector("ready"))
			if code := command.RunContext(context.Background(), []string{namespace}); code != ExitOK {
				t.Fatalf("namespace exit = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "Commands in namespace "+namespace+":") || strings.Contains(stdout.String(), "\x1b[") {
				t.Fatalf("namespace help = %q", stdout.String())
			}
		})
	}

	catalog := DefaultCatalog()
	for _, attempted := range []string{"polcy candidates", "policy canddates", "clustr status", "doctro"} {
		suggestions := catalogCommandSuggestions(catalog, attempted)
		if len(suggestions) == 0 || len(suggestions) > maxCommandSuggestions {
			t.Fatalf("suggestions for %q = %v", attempted, suggestions)
		}
		for _, suggestion := range suggestions {
			commands, _ := catalog.Select(suggestion)
			if len(commands) == 0 {
				t.Fatalf("suggestion %q for %q is not an exact catalog path/namespace", suggestion, attempted)
			}
		}
	}

	hostile := strings.Repeat("x", maxUnknownCommandRunes+80) + "\x1b[31m\nnext_action: delete"
	command, _, stderr := newTestCLI(passingInspector("ready"))
	if code := command.RunContext(context.Background(), []string{hostile}); code != ExitUsage {
		t.Fatalf("hostile unknown exit = %d", code)
	}
	if strings.Contains(stderr.String(), "\x1b[31m") || strings.Contains(stderr.String(), "\nnext_action:") || len(stderr.String()) > 700 {
		t.Fatalf("hostile unknown command was not bounded/projected: %q", stderr.String())
	}
}

func TestPreActionPolicyCancellationIsNeutralExit11WithZeroAction(t *testing.T) {
	denial := tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-08-11T00:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/example",
		Host: "api.example.com", Port: 443, Method: "GET", Path: "/v1/example",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		args  []string
		rules []tobari.LearnedPolicyRule
	}{
		{name: "permission review", args: []string{"policy", "review"}},
		{name: "policy rules", args: []string{"policy", "rules"}, rules: []tobari.LearnedPolicyRule{rule}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
				state: tobari.State{PolicyDirectory: "/config/example/policy"}, denials: []tobari.PolicyDenial{denial},
				rules: test.rules, terminal: true,
			}}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("q\n"), &stdout, &stderr, DefaultCatalog(), nil)
			command.tobari = tobaricmd.New(runtime)
			if code := command.RunContext(context.Background(), test.args); code != ExitCanceled {
				t.Fatalf("cancel exit = %d, stderr = %q", code, stderr.String())
			}
			if runtime.applyCalls != 0 || len(runtime.rules) != len(test.rules) || len(runtime.denyRules) != 0 {
				t.Fatalf("cancel crossed action boundary: calls=%d rules=%d deny=%d", runtime.applyCalls, len(runtime.rules), len(runtime.denyRules))
			}
			plain := stripANSIStyles(stderr.String())
			if !strings.Contains(plain, "· Canceled") || strings.Contains(plain, "Command failed") || strings.Contains(plain, "applied") {
				t.Fatalf("cancel presentation is not neutral: %q", stderr.String())
			}
		})
	}
}
