package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeDeleteCLI struct {
	*runtimeCatalogCLI
	result                 tobari.RuntimeDeleteResult
	deleteErr              error
	lifecycleErr           error
	deleteRecoveryOverride *tobari.RuntimeSummary
	deleteCalls            int
	deleteRefs             []string
}

type runtimeWithoutDeleteCLI struct{ runtimecmd.RuntimePort }

func (f *runtimeDeleteCLI) DeleteManagedRuntimeByReference(_ context.Context, runtimeRef string) (tobari.RuntimeDeleteResult, error) {
	f.deleteCalls++
	f.deleteRefs = append(f.deleteRefs, runtimeRef)
	return f.result, f.deleteErr
}

func (f *runtimeDeleteCLI) ReadRuntimeLifecycleSnapshot(ctx context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	if f.lifecycleErr != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, f.lifecycleErr
	}
	return f.runtimeCatalogCLI.ReadRuntimeLifecycleSnapshot(ctx)
}

func (f *runtimeDeleteCLI) ReadRuntimeDeleteRecovery(context.Context) (tobari.RuntimeSummary, bool, error) {
	if f.lifecycleErr != nil {
		return tobari.RuntimeSummary{}, false, f.lifecycleErr
	}
	if f.deleteRecoveryOverride != nil {
		return *f.deleteRecoveryOverride, true, nil
	}
	for _, activity := range f.lifecycleActivities {
		if activity.Kind == tobari.RuntimeLifecycleActivityDelete {
			return tobari.RuntimeSummaryFrom(f.runtimeManifest()), true, nil
		}
	}
	return tobari.RuntimeSummary{}, false, nil
}

func runtimeDeleteResult(manifest tobari.RuntimeManifest, state tobari.RuntimeDeleteState) tobari.RuntimeDeleteResult {
	revision := manifest.Revisions[0]
	virtualBytes := int64(8192)
	item := tobari.RuntimePruneItemResult{
		Kind: tobari.RuntimePruneCandidateRevision, RuntimeID: manifest.ID, Revision: revision.Revision,
		RuntimeRef: tobari.RuntimeRef(manifest.ID), RevisionRef: tobari.RuntimeRevisionRef(manifest.ID, revision.Revision),
		Name: manifest.Name, Ordinal: revision.Ordinal, LastUsed: tobari.RuntimeLastUsedUnknown,
		SourceLogicalBytes: 42, SnapshotLogicalBytes: 100, Disposition: tobari.RuntimePrunePreservedShared,
		RemovedTagCount: 1, ImageVirtualBytes: &virtualBytes,
	}
	return tobari.RuntimeDeleteResult{
		Task: tobari.TaskRuntimeDelete, RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID), Name: manifest.Name, State: state,
		SourceLogicalBytes: 42, SnapshotLogicalBytes: 100, SourceDisposition: tobari.RuntimeDeleteAuthorityRemoved,
		SnapshotsDisposition: tobari.RuntimeDeleteAuthorityRemoved, HistoryDisposition: tobari.RuntimeDeleteAuthorityRemoved,
		Items: []tobari.RuntimePruneItemResult{item}, RemovedTagCount: 1, ReceiptRevision: 1,
		WorkspaceManifestsPreserved: true, WorkspacesPreserved: true, WorkspaceIDsPreserved: true,
		WorkspaceHomesPreserved: true, AppliedReceiptsPreserved: true, ProjectRootsPreserved: true,
		CredentialsPreserved: true, SharedResourcesPreserved: true,
	}
}

func newRuntimeDeleteTestCLI(input string) (*CLI, *runtimeDeleteCLI, *bytes.Buffer, *bytes.Buffer) {
	manifest := readyRuntimeManifest()
	fake := &runtimeDeleteCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}, result: runtimeDeleteResult(manifest, tobari.RuntimeDeleted)}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(input), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	return command, fake, stdout, stderr
}

func TestRuntimeDeleteCatalogClosesExactRuntimeReferenceWorkflow(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	deleteCommand, found := catalog.Lookup("runtime delete")
	if !found {
		t.Fatal("runtime delete is absent")
	}
	wantConsumed := []ConsumedRef{{Kind: tobari.RuntimeReferenceKind, Argument: "--id"}}
	if deleteCommand.Role != RoleAct || deleteCommand.Effect != operation.EffectWrite || !reflect.DeepEqual(deleteCommand.ConsumedRefs(), wantConsumed) ||
		deleteCommand.Agent.Mutation == nil || deleteCommand.Agent.Mutation.TargetKind != tobari.RuntimeReferenceKind || deleteCommand.Agent.Mutation.TargetIDInput != "--id" ||
		!reflect.DeepEqual(deleteCommand.Agent.Mutation.TargetInputs, []string{"--id"}) || deleteCommand.Agent.Mutation.Impact != runtimecmd.DeleteImpact() {
		t.Fatalf("Runtime delete mutation/reference contract = role:%q effect:%q consumed:%+v mutation:%+v", deleteCommand.Role, deleteCommand.Effect, deleteCommand.ConsumedRefs(), deleteCommand.Agent.Mutation)
	}
	if got, want := deleteCommand.ProducedRefs(), []ProducedRef{{Kind: tobari.RuntimeReferenceKind, Field: "runtime_ref"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Runtime delete produced references = %+v, want %+v", got, want)
	}
	for _, produced := range deleteCommand.ProducedRefs() {
		if produced.Kind == tobari.RuntimeRevisionReferenceKind {
			t.Fatalf("Runtime delete advertises deterministically unrestorable revision reference: %+v", produced)
		}
	}
	for _, path := range []string{"runtime list", "runtime show", "runtime history", "runtime create", "runtime build", "runtime restore", "review runtimes"} {
		producer, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("Runtime producer %q is absent", path)
		}
		producesRuntime := false
		for _, produced := range producer.ProducedRefs() {
			if produced.Kind == tobari.RuntimeReferenceKind {
				producesRuntime = true
			}
		}
		if !producesRuntime {
			t.Errorf("Runtime producer %q lacks Runtime reference", path)
		}
	}
}

func TestRuntimeDeleteRequiresExactConfirmationBeforeAdapter(t *testing.T) {
	command, fake, stdout, stderr := newRuntimeDeleteTestCLI("")
	ref := fake.result.RuntimeRef
	for _, args := range [][]string{
		{"runtime", "delete", "--id", ref},
		{"runtime", "delete", "--id", ref, "--confirm=prune"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := command.RunContext(context.Background(), args); code != ExitUsage {
			t.Fatalf("args %q code = %d, stderr = %q", args, code, stderr.String())
		}
	}
	if fake.deleteCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("unconfirmed Runtime delete crossed adapter/output = calls:%d stdout:%q", fake.deleteCalls, stdout.String())
	}
}

func TestRuntimeDeleteJSONRoundTripsExactReferenceAndAllKeys(t *testing.T) {
	command, fake, stdout, stderr := newRuntimeDeleteTestCLI("")
	if code := command.RunContext(context.Background(), []string{"runtime", "delete", "--id", fake.result.RuntimeRef, "--confirm=delete", "--format=json"}); code != ExitOK {
		t.Fatalf("Runtime delete code = %d, stderr = %q", code, stderr.String())
	}
	if fake.deleteCalls != 1 || !reflect.DeepEqual(fake.deleteRefs, []string{fake.result.RuntimeRef}) {
		t.Fatalf("Runtime delete calls/refs = %d/%q", fake.deleteCalls, fake.deleteRefs)
	}
	var document runtimeDeleteDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || !reflect.DeepEqual(document.Delete, runtimeDeleteProjectionFrom(fake.result)) {
		t.Fatalf("Runtime delete JSON = %+v", document)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, raw, []string{"runtime_delete", "schema_version"})
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw["runtime_delete"], &result); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, result, []string{
		"applied_receipts_preserved", "credentials_preserved", "history_disposition", "items", "name", "project_roots_preserved",
		"receipt_revision", "reclaimed_bytes", "removed_tag_count", "runtime_id", "runtime_ref", "shared_resources_preserved",
		"snapshot_logical_bytes", "snapshots_disposition", "source_disposition", "source_logical_bytes", "state", "task",
		"workspace_homes_preserved", "workspace_ids_preserved", "workspace_manifests_preserved", "workspaces_preserved",
	})
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(result["items"], &items); err != nil || len(items) != 1 {
		t.Fatalf("Runtime delete items = %+v/%v", items, err)
	}
	assertExactJSONKeys(t, items[0], []string{
		"disposition", "image_virtual_bytes", "kind", "last_used", "name", "ordinal", "reclaimed_bytes", "removed_tag_count",
		"revision", "runtime_id", "snapshot_logical_bytes", "source_logical_bytes",
	})
	if _, exists := items[0]["runtime_ref"]; exists {
		t.Fatal("Runtime delete item exposed a retired Runtime opaque reference")
	}
	if _, exists := items[0]["revision_ref"]; exists {
		t.Fatal("Runtime delete item exposed a deterministically unrestorable revision reference")
	}
}

func assertExactJSONKeys(t *testing.T, values map[string]json.RawMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(values))
	for key := range values {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON keys = %q, want %q", got, want)
	}
}

func TestRuntimeDeleteHumanOutputStatesAuthorityLossAndPreservation(t *testing.T) {
	command, fake, stdout, stderr := newRuntimeDeleteTestCLI("")
	if code := command.RunContext(context.Background(), []string{"runtime", "delete", "--id", fake.result.RuntimeRef, "--confirm=delete"}); code != ExitOK {
		t.Fatalf("Runtime delete code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Delete Runtime " + fake.result.Name, fake.result.RuntimeRef, "Editable source", "removed", "Immutable snapshots", "Revision history", "Reclaimed bytes", "unknown", "Workspace Manifests", "Workspaces", "applied receipts", "Project roots"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("human Runtime delete omitted %q: %q", want, stdout.String())
		}
	}
	manifest := fake.runtimeManifest()
	for _, hidden := range []string{manifest.SourcePath, manifest.Revisions[0].Image, manifest.Revisions[0].ImageDigest, manifest.Revisions[0].SnapshotPath, "container_id", "docker_id"} {
		if hidden != "" && strings.Contains(stdout.String(), hidden) {
			t.Errorf("human Runtime delete exposed internal evidence %q: %q", hidden, stdout.String())
		}
	}
}

func TestRuntimeDeleteReviewUsesOneConfirmationAndSameExactReference(t *testing.T) {
	command, fake, stdout, stderr := newRuntimeDeleteTestCLI("\n")
	manifest := fake.runtimeManifest()
	fake.lifecycleActivities = []tobari.RuntimeLifecycleActivity{{Kind: tobari.RuntimeLifecycleActivityDelete, RuntimeID: manifest.ID, Revisions: []string{}}}
	fake.recovery = &tobari.RuntimeBuildRecovery{RuntimeID: manifest.ID, RuntimeRef: manifest.ID, Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryFailed}
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}
	if code := command.RunContext(context.Background(), []string{"review", "runtimes"}); code != ExitOK {
		t.Fatalf("Runtime delete recovery code = %d, stderr = %q", code, stderr.String())
	}
	if fake.recoveryReads != 0 || fake.deleteCalls != 1 || !reflect.DeepEqual(fake.deleteRefs, []string{manifest.ID}) || fake.recoveries != 0 || fake.buildCalls != 0 {
		t.Fatalf("Runtime delete recovery calls = recovery-read:%d delete:%d refs:%q build-recovery:%d build:%d", fake.recoveryReads, fake.deleteCalls, fake.deleteRefs, fake.recoveries, fake.buildCalls)
	}
	for _, want := range []string{"Tobari · Recover Runtime Delete", "Reference: " + manifest.ID, "Recover interrupted delete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("Runtime delete recovery review omitted %q: %q", want, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "Delete Runtime "+manifest.Name) || !strings.Contains(stdout.String(), string(tobari.RuntimeDeleted)) {
		t.Fatalf("Runtime delete recovery output = %q", stdout.String())
	}
}

func TestRuntimeDeleteReviewConfirmedOutputFailureUsesDeleteCatalogBoundary(t *testing.T) {
	manifest := readyRuntimeManifest()
	fake := &runtimeDeleteCLI{
		runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)},
		result:            runtimeDeleteResult(manifest, tobari.RuntimeDeleted),
	}
	fake.lifecycleActivities = []tobari.RuntimeLifecycleActivity{{Kind: tobari.RuntimeLifecycleActivityDelete, RuntimeID: manifest.ID, Revisions: []string{}}}
	var stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), shortWriter{}, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"review", "runtimes"}); code != ExitInternal {
		t.Fatalf("Runtime delete recovery output failure code = %d, stderr = %q", code, stderr.String())
	}
	if fake.deleteCalls != 1 || !reflect.DeepEqual(fake.deleteRefs, []string{manifest.ID}) {
		t.Fatalf("Runtime delete recovery output failure effects = calls:%d refs:%q", fake.deleteCalls, fake.deleteRefs)
	}
	if !humanOutputHasRow(stderr.String(), "Code", "mutation_output_write_failed") ||
		!humanOutputHasRow(stderr.String(), "Phase", "presentation") ||
		!humanOutputHasRow(stderr.String(), "Change state", "confirmed") ||
		!humanOutputHasRow(stderr.String(), "Retryable", "no") ||
		!humanOutputHasRow(stderr.String(), "Next", ProgramName+" review runtimes — Reconcile the confirmed mutation without repeating it.") ||
		strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("Runtime delete recovery output failure = %q", stderr.String())
	}
}

func TestRuntimeDeleteReviewRedirectedJSONRemainsReadOnly(t *testing.T) {
	command, fake, stdout, stderr := newRuntimeDeleteTestCLI("")
	manifest := fake.runtimeManifest()
	fake.lifecycleActivities = []tobari.RuntimeLifecycleActivity{{Kind: tobari.RuntimeLifecycleActivityDelete, RuntimeID: manifest.ID, Revisions: []string{}}}
	fake.recovery = &tobari.RuntimeBuildRecovery{RuntimeID: manifest.ID, RuntimeRef: manifest.ID, Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryFailed}
	if code := command.RunContext(context.Background(), []string{"review", "runtimes", "--format=json"}); code != ExitOK {
		t.Fatalf("redirected Runtime review code = %d, stderr = %q", code, stderr.String())
	}
	if fake.deleteCalls != 0 || fake.recoveryReads != 0 || fake.recoveries != 0 || fake.listCalls != 1 {
		t.Fatalf("redirected Runtime review effects = delete:%d recovery-read:%d recovery:%d list:%d", fake.deleteCalls, fake.recoveryReads, fake.recoveries, fake.listCalls)
	}
	if !strings.Contains(stdout.String(), `"runtimes"`) {
		t.Fatalf("redirected Runtime review output = %q", stdout.String())
	}
}

func TestRuntimeDeleteHelpAndCompletionDeriveFromCatalog(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	records, err := command.planCompletion(context.Background(), 4, []string{"tobari", "runtime", "delete", "--"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := completionRecordValues(records), []string{"candidate:--id", "candidate:--confirm", "candidate:--format"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Runtime delete completion = %v, want %v", got, want)
	}
	var stdout, stderr bytes.Buffer
	help := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	if code := help.RunContext(context.Background(), []string{"help", "runtime", "delete", "--format=agent"}); code != ExitOK {
		t.Fatalf("Runtime delete agent help code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{`"path":"runtime delete"`, `"effect":"write"`, `"role":"act"`, `"reference_kind":"runtime"`, `"target_id_input":"--id"`, `"destructive":"yes"`, `"command":"review runtimes"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("Runtime delete agent help omitted %q: %s", want, stdout.String())
		}
	}
}

func TestRuntimeDeleteCatalogMatchesDirectAndReviewRecoveryFaults(t *testing.T) {
	manifest := readyRuntimeManifest()
	valid := runtimeDeleteResult(manifest, tobari.RuntimeDeleted)
	intent := operation.Intent{Command: "runtime delete", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: valid.RuntimeRef}, Impact: runtimecmd.DeleteImpact()}
	catalog := DefaultCatalog()
	assertDeclared := func(t *testing.T, path string, err error) {
		t.Helper()
		public, ok := fault.PublicCopy(err)
		if !ok {
			t.Fatalf("%s returned no public structured fault: %v", path, err)
		}
		spec, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("Catalog lacks %q", path)
		}
		declared := commandErrorByCode(t, spec.Agent.Errors, public.Code)
		if declared.Kind != public.Kind || declared.Phase != public.Phase || declared.ChangeState != public.ChangeState || declared.Retryable != public.Retryable || !reflect.DeepEqual(declared.NextActions, public.NextActions) {
			t.Fatalf("%s fault %q Catalog=%+v PublicCopy=%+v", path, public.Code, declared, public)
		}
	}
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "not found", err: tobari.ErrRuntimeNotFound},
		{name: "protected", err: tobari.ErrRuntimeDeleteProtected},
		{name: "active", err: tobari.ErrRuntimeLifecycleActive},
		{name: "observation", err: tobari.ErrRuntimeRetirementObservationUnknown},
		{name: "interrupted", err: tobari.ErrRuntimeDeleteInterrupted},
		{name: "unknown", err: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &runtimeDeleteCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, result: valid, deleteErr: test.err}
			_, err := runtimecmd.New(fake).Delete(context.Background(), intent, valid.RuntimeRef)
			assertDeclared(t, "runtime delete", err)
		})
	}
	missingPort := runtimecmd.New(runtimeWithoutDeleteCLI{RuntimePort: &runtimeCatalogCLI{manifest: manifest}})
	_, err := missingPort.Delete(context.Background(), intent, valid.RuntimeRef)
	assertDeclared(t, "runtime delete", err)
	_, _, err = missingPort.ReviewDeleteRecovery(context.Background())
	assertDeclared(t, "review runtimes", err)
	observation := &runtimeDeleteCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, lifecycleErr: errors.New("synthetic observation failure")}
	_, _, err = runtimecmd.New(observation).ReviewDeleteRecovery(context.Background())
	assertDeclared(t, "review runtimes", err)
	invalid := tobari.RuntimeSummaryFrom(manifest)
	invalid.RuntimeRef = "018bcfe5-687b-7000-8000-000000000099"
	contract := &runtimeDeleteCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, deleteRecoveryOverride: &invalid}
	_, _, err = runtimecmd.New(contract).ReviewDeleteRecovery(context.Background())
	assertDeclared(t, "review runtimes", err)
}
