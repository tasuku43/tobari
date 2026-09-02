package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthorityresources"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type templateDraftCLIResult struct {
	SchemaVersion int `json:"schema_version"`
	Template      struct {
		Lifecycle          string `json:"lifecycle"`
		TemplateRef        string `json:"template_ref"`
		CurrentRevisionRef string `json:"current_revision_ref"`
		Name               string `json:"name"`
		Generation         int    `json:"generation"`
		Changed            bool   `json:"changed"`
	} `json:"template"`
	Plan struct {
		PlanRef     string `json:"plan_ref"`
		TemplateRef string `json:"template_ref"`
	} `json:"template_change_plan"`
	Context struct {
		Lifecycle  string `json:"lifecycle"`
		ContextRef string `json:"context_ref"`
		ContextID  string `json:"context_id"`
	} `json:"context"`
	Result struct {
		ContextID string `json:"context_id"`
		Deleted   bool   `json:"deleted"`
	} `json:"result"`
}

type templateDraftContextDeleteReadiness struct{}

func (templateDraftContextDeleteReadiness) Check(context.Context) error { return nil }

func TestPublicTemplateCreateAndCopyDraftsPlanAndApplyAfterExistingAuthority(t *testing.T) {
	root := t.TempDir()
	configHome, stateHome, dataHome := filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	configRoot := filepath.Join(configHome, "tobari")
	stateRoot := filepath.Join(stateHome, "tobari")
	authorityRoot := filepath.Join(stateRoot, "authority")
	lifetime := context.Background()
	guard, err := dockerruntime.New(lifetime)
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspaceauthoritystore.NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &firstUseIntegrationLifecycle{parent: stateRoot}
	runtimeRevision := "sha256:" + strings.Repeat("f", 64)
	runtimeAuthority := &firstUseIntegrationRuntime{revision: runtimeRevision}
	activation := &firstUseIntegrationActivation{}
	settlement := &firstUseIntegrationSettlement{}
	mutator, err := workspaceauthoritystore.NewMutator(lifetime, store, lifecycle, runtimeAuthority, nil, activation, settlement)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := workspaceauthoritysource.New(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := workspaceauthorityresources.New(
		store, mutator, sources,
		func(context.Context, tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error) {
			return firstUseIntegrationDigest("r"), nil
		},
		func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
			return firstUseIntegrationMigrationStage{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.processLifetime = lifetime
	command.runtime = runtimecmd.New(finalTemplateCreateRuntime())
	finalAuthority := &finalWorkspaceAuthorityAdapter{Adapter: resources}
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(finalAuthority)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalAuthority, templateDraftContextDeleteReadiness{})
	run := func(args ...string) templateDraftCLIResult {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		if code := command.RunContext(context.Background(), args); code != ExitOK {
			t.Fatalf("%v exit=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		var result templateDraftCLIResult
		if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
			t.Fatalf("%v JSON=%q err=%v", args, stdout.String(), err)
		}
		return result
	}

	const absentContextRef = "ctx1_01912345-6789-7abc-8def-0123456789a1"
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "context", "use", "--id", absentContextRef, "--format=json"}); code != ExitNotFound || !strings.Contains(stderr.String(), `"code":"context_not_found"`) {
		t.Fatalf("fresh context use exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	const absentPlanRef = "ctxplan1_01912345-6789-7abc-8def-0123456789a1_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "context", "apply", "--plan", absentPlanRef, "--format=json"}); code != ExitNotFound || !strings.Contains(stderr.String(), `"code":"template_not_found"`) {
		t.Fatalf("fresh context apply exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	first := run("template", "create", "--name", "first", "--source-access", "read-only", "--format=json")
	firstPlan := run("template", "plan", "--id", first.Template.TemplateRef, "--format=json")
	firstApplied := run("template", "apply", "--plan", firstPlan.Plan.PlanRef, "--format=json")
	if firstApplied.Template.Generation != 1 || !firstApplied.Template.Changed || firstApplied.Template.TemplateRef != first.Template.TemplateRef {
		t.Fatalf("first publication=%+v", firstApplied.Template)
	}

	created := run("template", "create", "--name", "later", "--source-access", "read-write", "--format=json")
	createdPlan := run("template", "plan", "--id", created.Template.TemplateRef, "--format=json")
	createdApplied := run("template", "apply", "--plan", createdPlan.Plan.PlanRef, "--format=json")
	if createdApplied.Template.Generation != 1 || !createdApplied.Template.Changed || createdApplied.Template.TemplateRef != created.Template.TemplateRef {
		t.Fatalf("later create publication=%+v", createdApplied.Template)
	}

	copied := run("template", "copy", "--from", createdApplied.Template.CurrentRevisionRef, "--name", "copied", "--format=json")
	copiedPlan := run("template", "plan", "--id", copied.Template.TemplateRef, "--format=json")
	copiedApplied := run("template", "apply", "--plan", copiedPlan.Plan.PlanRef, "--format=json")
	if copiedApplied.Template.Generation != 1 || !copiedApplied.Template.Changed || copiedApplied.Template.TemplateRef != copied.Template.TemplateRef {
		t.Fatalf("copied publication=%+v", copiedApplied.Template)
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"template", "default", "set", "--id", createdApplied.Template.TemplateRef, "--format=json"}); code != ExitOK {
		t.Fatalf("default set exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"template", "show", "--name", "later", "--format=json"}); code != ExitOK || !strings.Contains(stdout.String(), `"generation":1`) {
		t.Fatalf("show later exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"template", "list", "--format=json"}); code != ExitOK || strings.Count(stdout.String(), `"lifecycle":"active"`) != 3 {
		t.Fatalf("list Templates exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	contextDraft := run("context", "create", "--template", createdApplied.Template.TemplateRef, "--format=json")
	if contextDraft.Context.Lifecycle != "draft" || contextDraft.Context.ContextRef == "" || contextDraft.Context.ContextID == "" {
		t.Fatalf("context create=%+v", contextDraft.Context)
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "context", "use", "--id", contextDraft.Context.ContextRef, "--format=json"}); code != ExitNotFound || !strings.Contains(stderr.String(), `"code":"context_not_found"`) {
		t.Fatalf("draft context use exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	deletedDraft := run("context", "delete", "--id", contextDraft.Context.ContextRef, "--confirm=delete", "--format=json")
	if !deletedDraft.Result.Deleted || deletedDraft.Result.ContextID != contextDraft.Context.ContextID {
		t.Fatalf("context draft delete=%+v", deletedDraft.Result)
	}
	if _, present, err := sources.ReadContext(context.Background(), tobari.ContextID(contextDraft.Context.ContextID)); err != nil || present {
		t.Fatalf("deleted Context draft source present=%t err=%v", present, err)
	}

	if _, err := os.Lstat(filepath.Join(authorityRoot, "active.json")); err != nil {
		t.Fatalf("final authority not published: %v", err)
	}
	if drafts, err := resources.ListWorkspaceTemplateDrafts(context.Background()); err != nil || len(drafts) != 0 {
		t.Fatalf("published Template drafts=%+v err=%v", drafts, err)
	}
	if _, err := resources.DiscoverWorkspaceTemplateDraft(context.Background(), "later"); !errors.Is(err, tobari.ErrWorkspaceTemplateNotFound) {
		t.Fatalf("active Template remained discoverable as draft: %v", err)
	}
}
