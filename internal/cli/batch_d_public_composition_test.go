package cli

import (
	"bytes"
	"context"
	"maps"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestBatchDRegisteredFinalHandlersAreExact(t *testing.T) {
	// public-production-handler-evidence:start
	want := map[string]string{
		ReleaseProgramName + "\x00cluster denials":              "runFinalClusterDenials",
		ReleaseProgramName + "\x00cluster down":                 "runFinalClusterDown",
		ReleaseProgramName + "\x00cluster logs":                 "runFinalClusterLogs",
		ReleaseProgramName + "\x00cluster status":               "runFinalClusterStatus",
		ReleaseProgramName + "\x00cluster up":                   "runFinalClusterUp",
		ReleaseProgramName + "\x00completion candidates":        "runCompletionCandidates",
		ReleaseProgramName + "\x00completion zsh":               "runCompletionZsh",
		ReleaseProgramName + "\x00context apply":                "runFinalContextApply",
		ReleaseProgramName + "\x00context create":               "runFinalContextCreate",
		ReleaseProgramName + "\x00context delete":               "runFinalContextDelete",
		ReleaseProgramName + "\x00context use":                  "runFinalContextUse",
		ReleaseProgramName + "\x00context list":                 "runFinalContextList",
		ReleaseProgramName + "\x00context plan":                 "runFinalContextPlan",
		ReleaseProgramName + "\x00context show":                 "runFinalContextShow",
		ReleaseProgramName + "\x00doctor":                       "runDoctor",
		ReleaseProgramName + "\x00help":                         "runHelp",
		ReleaseProgramName + "\x00installation migration apply": "runInstallationMigrationApply",
		ReleaseProgramName + "\x00installation migration plan":  "runInstallationMigrationPlan",
		ReleaseProgramName + "\x00policy allow":                 "runFinalPolicyAllow",
		ReleaseProgramName + "\x00policy assist":                "runPolicyAssist",
		ReleaseProgramName + "\x00policy candidates":            "runFinalPolicyCandidates",
		ReleaseProgramName + "\x00policy deny":                  "runFinalPolicyDeny",
		ReleaseProgramName + "\x00policy reset":                 "runFinalPolicyReset",
		ReleaseProgramName + "\x00policy rules":                 "runFinalPolicyRules",
		ReleaseProgramName + "\x00review permissions":           "runFinalPolicyReview",
		ReleaseProgramName + "\x00review runtimes":              "runRuntimeReview",
		ReleaseProgramName + "\x00review services":              "runServiceReview",
		ReleaseProgramName + "\x00runtime build":                "runRuntimeBuild",
		ReleaseProgramName + "\x00runtime assist":               "runRuntimeAssist",
		ReleaseProgramName + "\x00runtime create":               "runRuntimeCreate",
		ReleaseProgramName + "\x00runtime delete":               "runRuntimeDelete",
		ReleaseProgramName + "\x00runtime history":              "runRuntimeHistory",
		ReleaseProgramName + "\x00runtime list":                 "runRuntimeList",
		ReleaseProgramName + "\x00runtime prune apply":          "runRuntimePruneApply",
		ReleaseProgramName + "\x00runtime prune dry-run":        "runRuntimePruneDryRun",
		ReleaseProgramName + "\x00runtime restore":              "runRuntimeRestore",
		ReleaseProgramName + "\x00runtime show":                 "runRuntimeShow",
		ReleaseProgramName + "\x00service allow":                "runServiceAllow",
		ReleaseProgramName + "\x00service deny":                 "runServiceDeny",
		ReleaseProgramName + "\x00service open":                 "runServiceOpen",
		ReleaseProgramName + "\x00service status":               "runServiceStatus",
		ReleaseProgramName + "\x00service stop":                 "runServiceStop",
		ReleaseProgramName + "\x00status":                       "runFinalDefaultPairStatus",
		ReleaseProgramName + "\x00template apply":               "runFinalTemplateApply",
		ReleaseProgramName + "\x00template copy":                "runFinalTemplateCopy",
		ReleaseProgramName + "\x00template create":              "runFinalTemplateCreate",
		ReleaseProgramName + "\x00template default set":         "runFinalTemplateDefaultSet",
		ReleaseProgramName + "\x00template delete":              "runFinalTemplateDelete",
		ReleaseProgramName + "\x00template list":                "runFinalTemplateList",
		ReleaseProgramName + "\x00template migration apply":     "runFinalTemplateMigrationApply",
		ReleaseProgramName + "\x00template migration plan":      "runFinalTemplateMigrationPlan",
		ReleaseProgramName + "\x00template plan":                "runFinalTemplatePlan",
		ReleaseProgramName + "\x00template show":                "runFinalTemplateShow",
		ReleaseProgramName + "\x00tobari":                       "runFinalDefaultPairEnter",
		ReleaseProgramName + "\x00version":                      "runVersion",
		ReleaseProgramName + "\x00workspace delete":             "runFinalWorkspaceDelete",
		ReleaseProgramName + "\x00workspace list":               "runFinalWorkspaceList",
		ReleaseProgramName + "\x00workspace status":             "runFinalWorkspaceStatus",
		ExposureProgramName + "\x00help":                        "runHelp",
		ExposureProgramName + "\x00status":                      "runExposureStatus",
		ExposureProgramName + "\x00stop":                        "runExposureStop",
		ExposureProgramName + "\x00" + ExposureProgramName:      "runExposureRequest",
		PermissionProgramName + "\x00help":                      "runHelp",
		PermissionProgramName + "\x00wait":                      "runPermissionWait",
	}
	// public-production-handler-evidence:end
	if buildIdentityHasBroker() {
		delete(want, ReleaseProgramName+"\x00"+WorkspaceEntryCommandPath)
		for key, handler := range maps.Clone(want) {
			if strings.HasPrefix(key, ReleaseProgramName+"\x00") {
				delete(want, key)
				want[ResearchProgramName+strings.TrimPrefix(key, ReleaseProgramName)] = handler
			}
		}
		want[ResearchProgramName+"\x00auth login"] = "runAuthLogin"
		want[ResearchProgramName+"\x00auth import"] = "runAuthImport"
		want[ResearchProgramName+"\x00auth status"] = "runAuthStatus"
		want[ResearchProgramName+"\x00auth logout"] = "runAuthLogout"
		want[ResearchProgramName+"\x00serve"] = "runServe"
		want[ResearchProgramName+"\x00"+WorkspaceEntryCommandPath] = "runFinalDefaultPairEnter"
	}
	catalog := DefaultCatalog()
	got := make(map[string]string)
	for _, spec := range catalog.PublicCommands() {
		name := runtime.FuncForPC(reflect.ValueOf(spec.handler).Pointer()).Name()
		got[spec.programName()+"\x00"+spec.Path] = name[strings.LastIndex(name, ".")+1:]
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public production handler composition differs\ngot=%v\nwant=%v", got, want)
	}
	if !buildIdentityHasBroker() {
		for _, path := range []string{"auth login", "auth import", "auth status", "auth logout", "serve"} {
			if _, found := catalog.lookupRegistered(path); found {
				t.Errorf("release surface exposes research path %q", path)
			}
		}
	}
}

func TestBatchDPublicPathSetAndResearchDeltaAreExact(t *testing.T) {
	want := []string{
		"cluster denials", "cluster down", "cluster logs", "cluster status", "cluster up",
		"completion candidates", "completion zsh",
		"context apply", "context create", "context delete", "context list", "context plan", "context show", "context use", "doctor", "help",
		"installation migration apply", "installation migration plan",
		"policy allow", "policy assist", "policy candidates", "policy deny", "policy reset", "policy rules", "review permissions", "review runtimes", "review services",
		"runtime assist", "runtime build", "runtime create", "runtime delete", "runtime history", "runtime list", "runtime prune apply", "runtime prune dry-run", "runtime restore", "runtime show",
		"service allow", "service deny", "service open", "service status", "service stop", "status", "template apply", "template copy", "template create", "template default set", "template delete", "template list", "template migration apply", "template migration plan", "template plan", "template show", "tobari", "version",
		"workspace delete", "workspace list", "workspace status",
	}
	if buildIdentityHasBroker() {
		want = append(want, "auth import", "auth login", "auth logout", "auth status", "serve")
	}
	slices.Sort(want)
	got := make([]string, 0, len(DefaultCatalog().Commands()))
	for _, command := range DefaultCatalog().Commands() {
		got = append(got, command.Path)
	}
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public paths =\n%q\nwant\n%q", got, want)
	}
}

func TestBatchDPublicContractsRejectPredecessorSelectorsKeysAndMigration(t *testing.T) {
	for _, command := range DefaultCatalog().registeredCommands() {
		if strings.HasPrefix(command.Path, "manifest") || command.Path == "migrate" || strings.HasPrefix(command.Path, "migrate ") {
			t.Errorf("retired public/internal path remains registered: %q", command.Path)
		}
		if strings.Contains(command.Args, "--manifest") || strings.HasPrefix(command.Agent.CapabilityID, "manifest.") {
			t.Errorf("%s retains predecessor selector/capability: args=%q capability=%q", command.Path, command.Args, command.Agent.CapabilityID)
		}
		for _, input := range command.Agent.Inputs {
			if input.Name == "--manifest" || input.Completion == InputCompletion("manifest_name") {
				t.Errorf("%s retains predecessor input/completion %+v", command.Path, input)
			}
		}
		assertBatchDOutputFields(t, command.Path, command.Agent.Output.Fields)
		for _, declared := range command.Agent.Errors {
			for _, action := range declared.NextActions {
				if action.Command == "migrate apply" || strings.HasPrefix(action.Command, "manifest") || strings.Contains(action.Command, " --manifest") {
					t.Errorf("%s fault %s advertises retired recovery %q", command.Path, declared.Code, action.Command)
				}
			}
		}
	}
	if _, _, err := parseRootOptions([]string{"--manifest", "legacy", "status"}); err == nil {
		t.Error("global --manifest selector remains accepted")
	}
}

func assertBatchDOutputFields(t *testing.T, path string, fields []OutputField) {
	t.Helper()
	for _, field := range fields {
		switch field.Name {
		case "manifest_id", "workspace_manifest_id", "workspace_manifest", "manifest_count", "workspace_manifest_state", "default_manifest_id", "default_manifest":
			t.Errorf("%s exposes retired output field %q", path, field.Name)
		}
		assertBatchDOutputFields(t, path, field.Fields)
		if field.Items != nil {
			assertBatchDOutputFields(t, path, []OutputField{*field.Items})
		}
	}
}

func TestBatchDChangedPublicSchemasAreExact(t *testing.T) {
	want := map[string]int{
		"template list": 1, "template show": 1, "template create": 1, "template copy": 1, "template plan": 2, "template apply": 1, "template migration plan": 1, "template migration apply": 1, "template default set": 1, "template delete": 1,
		"context list": 2, "context show": 2, "context apply": 2, "context create": 1, "context use": 1, "context delete": 1,
		"workspace list": 1, "workspace status": 1, "workspace delete": 1,
		"status": 3, "cluster status": 3, "cluster denials": 5,
		"policy candidates": 3, "review permissions": 3, "policy rules": 3, "policy apply-reviewed": 3,
		"runtime prune dry-run": 2, "runtime prune apply": 2,
	}
	if buildIdentityHasBroker() {
		want["auth login"] = 2
		want["auth import"] = 2
		want["auth status"] = 2
		want["auth logout"] = 2
	}
	for path, version := range want {
		spec, found := DefaultCatalog().lookupRegistered(path)
		if !found {
			t.Errorf("schema owner %s is not registered", path)
			continue
		}
		if got := spec.Agent.Output.JSONSchemaVersion; got != version {
			t.Errorf("%s schema = %d, want %d", path, got, version)
		}
	}
}

func TestBatchDHelpCompletionAndFaultSurfaceRejectsLegacyVocabulary(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	for _, request := range []struct {
		current int
		words   []string
	}{
		{2, []string{"tobari", "mani"}},
		{2, []string{"tobari", "mig"}},
		{2, []string{"tobari", "--m"}},
	} {
		records, err := command.planCompletion(context.Background(), request.current, request.words)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 0 {
			t.Errorf("completion %q = %+v, want no retired candidate", request.words, records)
		}
	}
	for _, path := range []string{"status", "cluster status", "policy candidates", "template create", "context use", "workspace delete"} {
		for _, format := range []string{"text", "agent"} {
			var out, errOut bytes.Buffer
			cli := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), passingInspector("unused"))
			args := append([]string{"help"}, strings.Fields(path)...)
			args = append(args, "--format", format)
			if code := runCLI(cli, args); code != ExitOK {
				t.Fatalf("help %s %s code=%d stderr=%q", path, format, code, errOut.String())
			}
			visible := strings.ToLower(out.String())
			for _, forbidden := range []string{"--manifest", "workspace_manifest_id", "manifest_id", "migrate apply"} {
				if strings.Contains(visible, forbidden) {
					t.Errorf("help %s %s exposes %q: %s", path, format, forbidden, out.String())
				}
			}
		}
	}
}

func TestBatchDReachableCLISourcesContainNoPredecessorComposition(t *testing.T) {
	checks := map[string][]string{
		"cli.go":        {"--manifest", "WorkspaceManifestName", "tobaricmd.New", "contextcmd.New(", "command.tobari =", "command.context =", "MigrateInstallation(", "planInstallationMigration(", "prepareLegacyMigrationRuntime(", "convertLegacyContextPolicy(", "CreateContext(", "ListContexts(", "runtime.ClusterUp(", "runtime.ClusterDown("},
		"completion.go": {"--manifest", "rootContextCompletionInput", "completion_manifest_read_failed"},
	}
	for file, forbidden := range checks {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Errorf("reachable CLI source %s retains %q", file, token)
			}
		}
	}

	forbiddenHandlers := map[string]struct{}{
		"runClusterUp": {}, "runClusterStatus": {}, "runClusterDown": {},
		"runProjectEnter": {}, "runProjectStatus": {}, "runProjectList": {}, "runProjectDelete": {},
		"runPolicyCandidates": {}, "runPolicyReview": {}, "runPolicyRules": {},
		"runManifestList": {}, "runManifestShow": {}, "runManifestCreate": {},
		"runManifestDefaultSet": {}, "runManifestDelete": {}, "runManifestRuntimeSet": {},
	}
	for _, command := range DefaultCatalog().registeredCommands() {
		name := runtime.FuncForPC(reflect.ValueOf(command.handler).Pointer()).Name()
		short := name[strings.LastIndex(name, ".")+1:]
		if _, forbidden := forbiddenHandlers[short]; forbidden {
			t.Errorf("public command %s reaches predecessor handler %s", command.Path, short)
		}
	}
}

func TestBatchDReachableFinalAuthoritySourcesContainNoLegacyRuntimeHelpers(t *testing.T) {
	for _, file := range []string{
		"../infra/workspaceauthoritystore/store.go",
		"../infra/workspaceauthoritystore/mutator.go",
		"../infra/workspaceauthoritystore/entry.go",
		"../infra/workspaceauthoritystore/default_pair.go",
		"../infra/workspaceauthoritystore/cluster.go",
		"../infra/workspaceauthoritystore/cluster_lifecycle.go",
		"../infra/workspaceauthoritystore/cluster_read.go",
		"../infra/workspaceauthoritystore/status_home.go",
		"../infra/workspaceauthoritystore/policy_read.go",
		"../infra/workspaceauthoritystore/final_policy_candidates.go",
		"../infra/workspaceauthoritystore/host_loopback_policy.go",
		"../infra/workspaceauthoritystore/auth.go",
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, token := range []string{
			"MigrateInstallation(", "planInstallationMigration(", "prepareLegacyMigrationRuntime(",
			"convertLegacyContextPolicy(", "migratePrePlatformSharedClusterState(",
			"CreateContext(", "ListContexts(", "ResolveContext(", "ObserveContext(",
			"ReadWorkspaceManifestByID(", "r.ClusterUp(", "r.ClusterDown(",
		} {
			if strings.Contains(text, token) {
				t.Errorf("reachable final authority source %s retains legacy runtime helper %q", file, token)
			}
		}
	}
}

func TestBatchDWholeCatalogRoleUtilityHasZeroReferenceEdges(t *testing.T) {
	for _, command := range DefaultCatalog().registeredCommands() {
		if command.Role == RoleUtility && (len(command.ProducedRefs()) != 0 || len(command.ConsumedRefs()) != 0) {
			t.Errorf("RoleUtility %s %q has produced=%+v consumed=%+v", command.programName(), command.Path, command.ProducedRefs(), command.ConsumedRefs())
		}
	}
	// The final kinds remain frozen by TestADR0084WholeCatalogReferenceGraphIsExact.
	_ = tobari.WorkspaceTemplateReferenceKind
}
