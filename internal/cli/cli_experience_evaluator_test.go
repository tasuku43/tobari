package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

type cliExperienceViolation struct {
	Rule   string
	Target string
	Detail string
}

func (v cliExperienceViolation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.Rule, v.Target, v.Detail)
}

// evaluateCLIExperience is the deterministic eligibility function for the
// shared human CLI. It deliberately evaluates both source ownership and a
// rendered semantic document: using a token is insufficient when a concrete
// palette, a local spinner, or a journey-only fallback can still regress the
// experience while focused render tests pass.
func evaluateCLIExperience() ([]cliExperienceViolation, error) {
	violations := make([]cliExperienceViolation, 0)

	wantStyles := map[styleToken]string{
		styleText: "", styleMuted: "\x1b[90m", styleAccent: "\x1b[1;36m",
		styleSuccess: "\x1b[32m", styleWarning: "\x1b[33m", styleDanger: "\x1b[31m",
	}
	for _, token := range semanticStyleTokens {
		got, found := ansiStyleTokens[token]
		if !found || got != wantStyles[token] {
			violations = append(violations, cliExperienceViolation{
				Rule: "terminal_owned_palette", Target: string(token),
				Detail: fmt.Sprintf("style %q, want %q", got, wantStyles[token]),
			})
		}
		if strings.Contains(got, "38;5;") || strings.Contains(got, "48;5;") ||
			strings.Contains(got, "38;2;") || strings.Contains(got, "48;2;") {
			violations = append(violations, cliExperienceViolation{
				Rule: "terminal_owned_palette", Target: string(token), Detail: "concrete 256/truecolor palette is forbidden",
			})
		}
	}
	wantSpinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if !reflect.DeepEqual(interactiveSpinnerFrames, wantSpinner) || interactiveSpinnerInterval != 100*time.Millisecond {
		violations = append(violations, cliExperienceViolation{
			Rule: "shared_spinner_contract", Target: "progress.go", Detail: "frames or 100 ms cadence changed",
		})
	}

	for _, command := range DefaultCatalog().Commands() {
		hasText := false
		for _, format := range command.Agent.Output.Formats {
			hasText = hasText || format == OutputFormatText
		}
		if hasText && command.Agent.Output.TextPresentation != TextPresentationSemanticTokens {
			violations = append(violations, cliExperienceViolation{
				Rule: "catalog_semantic_tokens", Target: command.Path, Detail: "human text bypasses shared semantic tokens",
			})
		}
	}
	mainHandlers := map[string]string{
		WorkspaceEntryCommandPath: "runFinalDefaultPairEnter",
		"status":                  "runFinalDefaultPairStatus",
		"cluster up":              "runFinalClusterUp",
		"cluster status":          "runFinalClusterStatus",
		"cluster down":            "runFinalClusterDown",
		"cluster denials":         "runFinalClusterDenials",
		"policy candidates":       "runFinalPolicyCandidates",
		"review permissions":      "runFinalPolicyReview",
		"policy rules":            "runFinalPolicyRules",
		"template list":           "runFinalTemplateList",
		"template show":           "runFinalTemplateShow",
		"template create":         "runFinalTemplateCreate",
		"template apply":          "runFinalTemplateApply",
		"context list":            "runFinalContextList",
		"context show":            "runFinalContextShow",
		"context create":          "runFinalContextCreate",
		"context apply":           "runFinalContextApply",
		"context enter":           "runFinalContextEnter",
		"workspace list":          "runFinalWorkspaceList",
		"workspace status":        "runFinalWorkspaceStatus",
		"workspace delete":        "runFinalWorkspaceDelete",
	}
	for path, expected := range mainHandlers {
		command, found := DefaultCatalog().Lookup(path)
		if !found {
			violations = append(violations, cliExperienceViolation{
				Rule: "public_handler_reachability", Target: path, Detail: "public command is absent",
			})
			continue
		}
		name := runtime.FuncForPC(reflect.ValueOf(command.handler).Pointer()).Name()
		if !strings.HasSuffix(name, "."+expected) {
			violations = append(violations, cliExperienceViolation{
				Rule: "public_handler_reachability", Target: path, Detail: "does not reach " + expected,
			})
		}
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	disallowedMarkers := []string{"❯", "○", "✗", "⛔", "◇"}
	spinnerRunes := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	rawSGR := regexp.MustCompile(`\\x1b\[[0-9;]*m`)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		source := string(data)
		for _, marker := range disallowedMarkers {
			if strings.Contains(source, marker) {
				violations = append(violations, cliExperienceViolation{
					Rule: "reviewed_marker_vocabulary", Target: path, Detail: fmt.Sprintf("contains %q", marker),
				})
			}
		}
		if path != "progress.go" {
			for _, frame := range spinnerRunes {
				if strings.Contains(source, frame) {
					violations = append(violations, cliExperienceViolation{
						Rule: "shared_spinner_ownership", Target: path, Detail: fmt.Sprintf("owns spinner frame %q", frame),
					})
				}
			}
		}
		if path != "styles.go" && rawSGR.MatchString(source) {
			violations = append(violations, cliExperienceViolation{
				Rule: "shared_style_ownership", Target: path, Detail: "owns an ANSI SGR literal outside styles.go",
			})
		}
	}
	for _, predecessor := range []string{"policy_review_selector.go", "runPolicyReview", "policyReviewSpec"} {
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			if strings.Contains(string(data), predecessor) {
				violations = append(violations, cliExperienceViolation{
					Rule: "public_experience_owner", Target: path, Detail: "retains predecessor owner " + predecessor,
				})
			}
		}
	}

	customizeSource, err := os.ReadFile("recommended_first_use_customize.go")
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(customizeSource), "mode: nil") ||
		!strings.Contains(string(customizeSource), "newContextConfigurationWizardWithStyle") {
		violations = append(violations, cliExperienceViolation{
			Rule: "first_use_interaction_continuity", Target: "recommended_first_use_customize.go",
			Detail: "interactive Customize may fall back to a locally disabled raw selector",
		})
	}

	plain := newHumanOutput(false)
	plain.heading("✓", "Workspace ready", styleSuccess)
	plain.row("State", "running", styleSuccess)
	plain.next("status", "Inspect this Workspace again.")
	styled := newHumanOutput(true)
	styled.heading("✓", "Workspace ready", styleSuccess)
	styled.row("State", "running", styleSuccess)
	styled.next("status", "Inspect this Workspace again.")
	if got := stripANSIStyles(styled.String()); got != plain.String() {
		violations = append(violations, cliExperienceViolation{
			Rule: "color_independent_meaning", Target: "representative human document",
			Detail: "stripping shared styles does not reproduce the no-color document",
		})
	}
	firstUse, err := os.ReadFile("testdata/recommended_first_use_line.txt")
	if err != nil {
		return nil, err
	}
	sectionOrder := []string{"Tobari · New Workspace", "Project", "Boundary", "Environment", "Action:"}
	previous := -1
	for _, section := range sectionOrder {
		index := strings.Index(string(firstUse), section)
		if index < 0 || index <= previous {
			violations = append(violations, cliExperienceViolation{
				Rule: "first_use_information_hierarchy", Target: section, Detail: "section is absent or out of order",
			})
		}
		previous = index
	}
	for index, line := range strings.Split(string(firstUse), "\n") {
		if len([]byte(line)) > 80 {
			violations = append(violations, cliExperienceViolation{
				Rule: "reviewed_static_density", Target: fmt.Sprintf("recommended first use line %d", index+1), Detail: "exceeds 80 UTF-8 bytes",
			})
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Rule != violations[j].Rule {
			return violations[i].Rule < violations[j].Rule
		}
		if violations[i].Target != violations[j].Target {
			return violations[i].Target < violations[j].Target
		}
		return violations[i].Detail < violations[j].Detail
	})
	return violations, nil
}

func TestCLIExperienceIsEligibleForPublicComposition(t *testing.T) {
	t.Parallel()
	violations, err := evaluateCLIExperience()
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation.String())
	}
}
