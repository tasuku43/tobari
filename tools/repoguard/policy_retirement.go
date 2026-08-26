package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
)

// The fixed evaluator is allowed to mention Rego because these files either
// own the embedded evaluator or materialize it in a private, Docker-managed
// preflight/aggregate directory. They are not user Workspace state.
var fixedEvaluatorRepositoryFiles = map[string]bool{
	"internal/infra/runtimeassets/assets/opa/policy/tobari.rego":      true,
	"internal/infra/runtimeassets/assets/opa/policy/tobari_test.rego": true,
}

var fixedEvaluatorRegoLiterals = map[string]map[string]bool{
	"internal/infra/dockerruntime/aggregate.go": {
		".rego":                  true,
		"guided.rego":            true,
		"opa/policy/tobari.rego": true,
		"router.rego":            true,
	},
	"internal/infra/dockerruntime/cluster_observation.go": {
		"router.rego": true,
	},
	"internal/infra/dockerruntime/final_authority_projection.go": {
		"opa/policy/tobari_test.rego": true,
		"tobari.rego":                 true,
		"tobari_test.rego":            true,
	},
	"internal/infra/dockerruntime/policy.go": {
		"fence.rego":       true,
		"guided.rego":      true,
		"router.rego":      true,
		"tobari.rego":      true,
		"tobari_test.rego": true,
	},
	"internal/infra/dockerruntime/policydata.go": {
		".rego":                       true, // bounded suffix probe for a user-owned Context file.
		"opa/policy/tobari.rego":      true,
		"opa/policy/tobari_test.rego": true,
		"tobari.rego":                 true,
		"tobari_test.rego":            true,
	},
	"internal/infra/runtimeassets/assets.go": {
		".rego":                  true,
		"opa/policy/tobari.rego": true,
	},
}

// These files may inspect the raw JSON marker left by the retired v1
// executable-policy schema. They must not decode it into a domain policy or
// use it to construct an evaluator.
var legacyPolicyMarkerProductionFiles = map[string]bool{
	"internal/infra/dockerruntime/aggregate.go":       true,
	"internal/infra/dockerruntime/context_store.go":   true,
	"internal/infra/dockerruntime/migration.go":       true,
	"internal/infra/dockerruntime/policydata.go":      true,
	"internal/infra/workspaceauthoritystore/store.go": true,
}

var publicPolicySurfaceRoots = map[string]bool{
	"cmd":          true,
	"internal/app": true,
	"internal/cli": true,
}

var retiredPolicyIdentifiers = map[string]bool{
	"AdvancedPolicy":            true,
	"ErrLegacyExecutablePolicy": true,
	"LegacyPolicyMode":          true,
	"PolicyMode":                true,
}

var retiredPolicyIdentifierFragments = []string{
	"ManifestPolicyMode",
	"WorkspaceTemplateAdvancedPolicy",
}

var retiredPolicyMarkerStrings = []string{
	"policy_mode",
	"advanced_policy",
	"--mode guided",
	"--mode=guided",
	"--mode advanced",
	"--mode=advanced",
	"guided|advanced",
}

// Live work packets are agent guidance, not historical records. Keep their
// policy vocabulary aligned with the current fixed-evaluator boundary so a
// later packet cannot quietly revive the retired executable-policy authority.
var liveWorkPacketPolicyMarkers = []string{
	"advanced_policy",
	"advanced rego",
	"advanced owner rego",
	"owner-authored rego",
	"owner authored rego",
	"guided/advanced",
	"guided|advanced",
	"--mode guided",
	"--mode=guided",
	"--mode advanced",
	"--mode=advanced",
	"policy mode",
	"guided baseline",
	"guided learned",
	"guided allow",
	"advanced allow",
}

func checkLiveWorkPacketPolicyRetirement(root string, repositoryPaths []string) ([]issue, error) {
	available := make(map[string]bool, len(repositoryPaths))
	for _, relative := range repositoryPaths {
		available[relative] = true
	}
	var issues []issue
	for _, goalPath := range repositoryPaths {
		parts := strings.Split(goalPath, "/")
		if len(parts) != 4 || parts[0] != "docs" || parts[1] != "work" || parts[2] == "_template" || parts[3] != "goal.md" {
			continue
		}
		goalData, err := readRegularRepositoryFile(root, goalPath)
		if err != nil {
			return nil, err
		}
		statuses := workMetadata(string(goalData), "Status")
		if len(statuses) != 1 || statuses[0].Value == "Complete" || statuses[0].Value == "Superseded" {
			continue
		}
		packetPrefix := pathpkg.Dir(goalPath) + "/"
		for _, relative := range repositoryPaths {
			if !strings.HasPrefix(relative, packetPrefix) {
				continue
			}
			if !available[relative] {
				continue
			}
			data, readErr := readRegularRepositoryFile(root, relative)
			if readErr != nil {
				return nil, readErr
			}
			text := string(data)
			for lineNumber, lineText := range strings.Split(text, "\n") {
				lower := strings.ToLower(lineText)
				_, _, found := liveWorkPacketPolicyClaim(lower)
				if found {
					issues = append(issues, issue{
						Path: relative, Line: lineNumber + 1,
						Message: "live work packet contains a retired executable-policy claim; describe the fixed Tobari evaluator and canonical typed policy data",
					})
					break
				}
			}
		}
	}
	return issues, nil
}

// liveWorkPacketPolicyClaim distinguishes an affirmative current authority
// claim from a retirement/remediation statement. Active packets must not
// advertise the removed surface, but they still need to record the legacy
// threat and the deterministic rejection behavior that closed it.
func liveWorkPacketPolicyClaim(lower string) (string, int, bool) {
	for _, marker := range liveWorkPacketPolicyMarkers {
		index := strings.Index(lower, marker)
		if index < 0 || livePolicyMarkerIsNegated(lower, index, marker) {
			continue
		}
		return marker, index, true
	}
	return "", 0, false
}

func livePolicyMarkerIsNegated(line string, markerIndex int, marker string) bool {
	markerEnd := markerIndex + len(marker)
	before := line[localPolicyClauseStart(line, markerIndex):markerIndex]
	after := line[markerEnd:localPolicyClauseEnd(line, markerEnd)]
	return containsLocalPolicyRetirementPhrase(before) || containsLocalPolicyRetirementPhrase(after)
}

func containsLocalPolicyRetirementPhrase(clause string) bool {
	for _, phrase := range []string{
		"was removed", "were removed", "has been removed", "have been removed",
		"is removed", "are removed", "must not", "do not", "does not", "did not",
		"cannot", "can't", "is rejected", "are rejected", "was rejected", "were rejected",
		"is unsupported", "are unsupported", "was unsupported", "were unsupported",
		"is retired", "are retired", "was retired", "were retired", "no longer",
		"must be removed", "must be rejected", "must remain removed", "must not return",
	} {
		if strings.Contains(clause, phrase) {
			return true
		}
	}
	trimmed := strings.TrimSpace(clause)
	return strings.HasSuffix(trimmed, "no") || strings.HasSuffix(trimmed, "without") || strings.HasSuffix(trimmed, "not")
}

func localPolicyClauseStart(line string, markerIndex int) int {
	start := 0
	for _, separator := range []string{",", ";", ".", " but ", " and ", " or ", " however ", " while "} {
		if index := strings.LastIndex(line[:markerIndex], separator); index >= 0 {
			candidate := index + len(separator)
			if candidate > start {
				start = candidate
			}
		}
	}
	return start
}

func localPolicyClauseEnd(line string, markerEnd int) int {
	end := len(line)
	for _, separator := range []string{",", ";", ".", " but ", " and ", " or ", " however ", " while "} {
		if index := strings.Index(line[markerEnd:], separator); index >= 0 && markerEnd+index < end {
			end = markerEnd + index
		}
	}
	return end
}

// checkPolicyRetirement is intentionally an AST/source guard rather than a
// grep in a release script. It covers untracked production files too and
// makes the allowlist for the only legitimate Rego/legacy-marker paths
// explicit at the repository boundary.
func checkPolicyRetirement(root string) ([]issue, error) {
	var issues []issue
	// The executable-source allowlist is a repository-wide contract. Keep the
	// VCS metadata out of the walk, but inspect every other directory so an
	// untracked Rego file under a non-Go tree cannot bypass the guard.
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".rego") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !fixedEvaluatorRepositoryFiles[relative] {
			issues = append(issues, issue{Path: relative, Message: "Rego file is outside the fixed embedded evaluator allowlist"})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Keep the source/AST retirement scan deliberately scoped to the existing
	// Go production roots. The extension scan above is intentionally broader.
	for _, sourceRoot := range []string{"cmd", "internal"} {
		rootPath := filepath.Join(root, sourceRoot)
		if _, err := os.Stat(rootPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "_helper-source" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			publicSurface := false
			for root := range publicPolicySurfaceRoots {
				if relative == root || strings.HasPrefix(relative, root+"/") {
					publicSurface = true
					break
				}
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if node == nil {
					return false
				}
				line := fileSet.Position(node.Pos()).Line
				switch value := node.(type) {
				case *ast.Ident:
					if value.Name == "ErrLegacyExecutablePolicy" && relative == "internal/domain/tobari/workspace_authority.go" {
						return true
					}
					if retiredPolicyIdentifiers[value.Name] && !legacyPolicyMarkerProductionFiles[relative] {
						issues = append(issues, issue{Path: relative, Line: line, Message: "retired executable-policy identifier is outside the raw legacy-marker boundary"})
					}
					for _, fragment := range retiredPolicyIdentifierFragments {
						if strings.Contains(value.Name, fragment) {
							issues = append(issues, issue{Path: relative, Line: line, Message: "retired executable-policy type or mode identifier remains in production source"})
							break
						}
					}
				case *ast.BasicLit:
					if value.Kind != token.STRING {
						return true
					}
					text, err := strconv.Unquote(value.Value)
					if err != nil {
						return true
					}
					if strings.Contains(text, ".rego") && !fixedEvaluatorRegoLiterals[relative][text] {
						issues = append(issues, issue{Path: relative, Line: line, Message: "Rego path is outside the fixed embedded evaluator allowlist"})
					}
					if publicSurface && strings.Contains(strings.ToLower(text), "rego") {
						issues = append(issues, issue{Path: relative, Line: line, Message: "public policy surface mentions evaluator source; keep Rego internal"})
					}
					for _, marker := range retiredPolicyMarkerStrings {
						if strings.Contains(text, marker) && !legacyPolicyMarkerProductionFiles[relative] {
							issues = append(issues, issue{Path: relative, Line: line, Message: "retired policy-mode/Advanced marker is outside the raw legacy-marker boundary"})
							break
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return issues, nil
}
