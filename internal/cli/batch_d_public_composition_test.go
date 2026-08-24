package cli

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestBatchDRegisteredFinalHandlersAreExact(t *testing.T) {
	want := map[string]string{
		"tobari":                "runFinalDefaultPairEnter",
		"status":                "runFinalDefaultPairStatus",
		"cluster up":            "runFinalClusterUp",
		"cluster status":        "runFinalClusterStatus",
		"cluster down":          "runFinalClusterDown",
		"policy candidates":     "runFinalPolicyCandidates",
		"review permissions":    "runFinalPolicyReview",
		"policy apply-reviewed": "runFinalPolicyApplyReviewed",
		"policy rules":          "runFinalPolicyRules",
		"policy allow":          "runFinalPolicyAllow",
		"policy deny":           "runFinalPolicyDeny",
		"policy reset":          "runFinalPolicyReset",
	}
	if buildIdentityHasBroker() {
		want["auth login"] = "runAuthLogin"
		want["auth import"] = "runAuthImport"
		want["auth status"] = "runAuthStatus"
		want["auth logout"] = "runAuthLogout"
	}
	catalog := DefaultCatalog()
	for path, suffix := range want {
		spec, found := catalog.lookupRegistered(path)
		if !found {
			t.Errorf("%s is not registered", path)
			continue
		}
		name := runtime.FuncForPC(reflect.ValueOf(spec.handler).Pointer()).Name()
		if !strings.HasSuffix(name, "."+suffix) {
			t.Errorf("%s handler = %s, want %s", path, name, suffix)
		}
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
		"completion candidates", "completion zsh", "config bootstrap aws", "config bootstrap kubernetes eks", "config git", "config shell",
		"context create", "context delete", "context enter", "context list", "context show", "doctor", "help",
		"policy allow", "policy candidates", "policy deny", "policy reset", "policy rules", "review permissions", "review runtimes", "review services",
		"runtime build", "runtime create", "runtime delete", "runtime history", "runtime list", "runtime prune apply", "runtime prune dry-run", "runtime restore", "runtime show",
		"service allow", "service deny", "service open", "service status", "service stop", "status", "template copy", "template create", "template default set", "template delete", "template list", "template runtime set", "template show", "tobari", "version",
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
		"template list": 1, "template show": 1, "template create": 1, "template copy": 1, "template default set": 1, "template delete": 1,
		"context list": 1, "context show": 1, "context create": 1, "context enter": 1, "context delete": 1,
		"workspace list": 1, "workspace status": 1, "workspace delete": 1,
		"status": 3, "cluster status": 2, "cluster denials": 3,
		"policy candidates": 2, "review permissions": 2, "policy rules": 2, "policy apply-reviewed": 2,
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
	for _, path := range []string{"status", "cluster status", "policy candidates", "template create", "context enter", "workspace delete"} {
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
		"cli.go":        {"--manifest", "WorkspaceManifestName", "tobaricmd.New", "contextcmd.New(", "command.tobari =", "command.context ="},
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

func TestBatchDWholeCatalogRoleUtilityHasZeroReferenceEdges(t *testing.T) {
	for _, command := range DefaultCatalog().registeredCommands() {
		if command.Role == RoleUtility && (len(command.ProducedRefs()) != 0 || len(command.ConsumedRefs()) != 0) {
			t.Errorf("RoleUtility %s %q has produced=%+v consumed=%+v", command.programName(), command.Path, command.ProducedRefs(), command.ConsumedRefs())
		}
	}
	// The final kinds remain frozen by TestADR0084WholeCatalogReferenceGraphIsExact.
	_ = tobari.WorkspaceTemplateReferenceKind
}
