package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

type runtimeRestoreCLI struct {
	*runtimeCatalogCLI
	result              tobari.RuntimeRestoreResult
	restoreErr          error
	restoreCalls        int
	restoreRefs         []string
	recoverRestoreCalls int
	recoverRestoreRefs  []string
	recoverRestoreKinds []tobari.RuntimeBuildRecoveryKind
}

func (f *runtimeRestoreCLI) RestoreManagedRuntimeByRevisionReference(_ context.Context, revisionRef string, diagnostics io.Writer) (tobari.RuntimeRestoreResult, error) {
	f.restoreCalls++
	f.restoreRefs = append(f.restoreRefs, revisionRef)
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "restoring exact retained source\n")
	}
	return f.result, f.restoreErr
}

func (f *runtimeRestoreCLI) RecoverRuntimeRestoreByRevisionReference(_ context.Context, revisionRef string, kind tobari.RuntimeBuildRecoveryKind, diagnostics io.Writer) (tobari.RuntimeRestoreResult, error) {
	f.recoverRestoreCalls++
	f.recoverRestoreRefs = append(f.recoverRestoreRefs, revisionRef)
	f.recoverRestoreKinds = append(f.recoverRestoreKinds, kind)
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "resuming exact retained restore\n")
	}
	return f.result, f.restoreErr
}

func runtimeRestoreResult(manifest tobari.RuntimeManifest, revisionIndex int, state tobari.RuntimeRestoreState) tobari.RuntimeRestoreResult {
	revision := manifest.Revisions[revisionIndex]
	result := tobari.RuntimeRestoreResult{
		Task: tobari.TaskRuntimeRestore, RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID),
		Revision: revision.Revision, RevisionRef: tobari.RuntimeRevisionRef(manifest.ID, revision.Revision),
		Name: manifest.Name, Ordinal: revision.Ordinal, State: state, DigestMatch: true,
		ArtifactDisposition: tobari.RuntimeRestoreArtifactRemoved,
	}
	if state == tobari.RuntimeAlreadyAvailable {
		result.ArtifactDisposition = tobari.RuntimeRestoreArtifactNotCreated
	}
	return result
}

func newRuntimeRestoreTestCLI(manifest tobari.RuntimeManifest, result tobari.RuntimeRestoreResult, input string) (*CLI, *runtimeRestoreCLI, *bytes.Buffer, *bytes.Buffer) {
	fake := &runtimeRestoreCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}, result: result}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(input), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	return command, fake, stdout, stderr
}

func TestRuntimeRestoreCatalogClosesManagedRevisionReferenceWorkflow(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("DefaultCatalog.Validate() error = %v", err)
	}
	restore, found := catalog.Lookup("runtime restore")
	if !found {
		t.Fatal("runtime restore is absent")
	}
	wantConsumed := []ConsumedRef{{Kind: tobari.RuntimeRevisionReferenceKind, Argument: "--id"}}
	if restore.Role != RoleAct || restore.Effect != operation.EffectWrite || !reflect.DeepEqual(restore.ConsumedRefs(), wantConsumed) || restore.Agent.Mutation == nil ||
		restore.Agent.Mutation.TargetKind != tobari.RuntimeRevisionReferenceKind || restore.Agent.Mutation.TargetIDInput != "--id" ||
		!reflect.DeepEqual(restore.Agent.Mutation.TargetInputs, []string{"--id"}) || restore.Agent.Mutation.Impact != runtimecmd.RestoreImpact() {
		t.Fatalf("restore mutation/reference contract = role:%q effect:%q consumed:%+v mutation:%+v", restore.Role, restore.Effect, restore.ConsumedRefs(), restore.Agent.Mutation)
	}
	for _, path := range []string{"runtime list", "runtime show", "runtime history", "runtime build", "review runtimes"} {
		producer, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("producer %q is absent", path)
		}
		producesRevision := false
		for _, produced := range producer.ProducedRefs() {
			if produced.Kind == tobari.RuntimeRevisionReferenceKind {
				producesRevision = true
				break
			}
		}
		if !producesRevision {
			t.Errorf("producer %q lacks managed revision reference", path)
		}
	}
	create, found := catalog.Lookup("runtime create")
	if !found {
		t.Fatal("runtime create is absent")
	}
	for _, produced := range create.ProducedRefs() {
		if produced.Kind == tobari.RuntimeRevisionReferenceKind {
			t.Fatalf("fresh empty-history create advertises impossible revision producer: %+v", create.ProducedRefs())
		}
	}
	var runtimeReportRevisionProducers []string
	for _, command := range catalog.Commands() {
		if command.Agent.Output.JSONEnvelope != "runtime" && command.Agent.Output.JSONEnvelope != "runtimes" {
			continue
		}
		for _, produced := range command.ProducedRefs() {
			if produced.Kind == tobari.RuntimeRevisionReferenceKind {
				runtimeReportRevisionProducers = append(runtimeReportRevisionProducers, command.Path)
				break
			}
		}
	}
	sort.Strings(runtimeReportRevisionProducers)
	wantProducers := []string{"review runtimes", "runtime build", "runtime history", "runtime list", "runtime show"}
	if !reflect.DeepEqual(runtimeReportRevisionProducers, wantProducers) {
		t.Fatalf("Runtime report revision producers = %v, want %v", runtimeReportRevisionProducers, wantProducers)
	}
}

func TestRuntimeCreateOutputCannotAdvertiseOrEmitRevisionReference(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{})
	if code := command.RunContext(context.Background(), []string{"runtime", "create", "--name", "mobile", "--format=json"}); code != ExitOK {
		t.Fatalf("create code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), `"revision_ref"`) || !strings.Contains(stdout.String(), `"revisions":[]`) {
		t.Fatalf("fresh create output invented revision authority: %s", stdout.String())
	}
}

func TestRuntimeListAndRedirectedReviewRenderManagedRevisionReference(t *testing.T) {
	manifest := readyRuntimeManifest()
	wantRef := tobari.RuntimeRevisionRef(manifest.ID, manifest.Revisions[0].Revision)
	for _, args := range [][]string{{"runtime", "list"}, {"review", "runtimes"}} {
		fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
		command.runtime = runtimecmd.New(fake)
		if code := command.RunContext(context.Background(), args); code != ExitOK {
			t.Fatalf("%v code = %d, stderr = %q", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Revision reference") || !strings.Contains(stdout.String(), wantRef) || strings.Contains(stdout.String(), tobari.StandardRuntimeID+"/") {
			t.Fatalf("%v Runtime revision reference output = %q", args, stdout.String())
		}
	}

	draft := testRuntimeManifest()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{list: runtimeReviewList(draft)})
	if code := command.RunContext(context.Background(), []string{"runtime", "list"}); code != ExitOK {
		t.Fatalf("draft list code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Revision reference") {
		t.Fatalf("standard/draft list invented revision reference: %q", stdout.String())
	}
}

func TestRuntimeHistoryAndRestoreRoundTripExactRevisionReference(t *testing.T) {
	manifest := readyRuntimeManifestWithHistory()
	result := runtimeRestoreResult(manifest, 0, tobari.RuntimeRestored)
	command, fake, stdout, stderr := newRuntimeRestoreTestCLI(manifest, result, "")
	if code := command.RunContext(context.Background(), []string{"runtime", "history", "--name", manifest.Name, "--format=json"}); code != ExitOK {
		t.Fatalf("history code = %d, stderr = %q", code, stderr.String())
	}
	var report runtimeReportDocument
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Runtime.Runtime.Revisions) != 2 {
		t.Fatalf("history revisions = %+v", report.Runtime.Runtime.Revisions)
	}
	ref := report.Runtime.Runtime.Revisions[0].RevisionRef
	if ref != result.RevisionRef {
		t.Fatalf("history revision_ref = %q, want %q", ref, result.RevisionRef)
	}
	parsedID, parsedRevision, err := tobari.ParseRuntimeRevisionRef(ref)
	if err != nil || parsedID != result.RuntimeID || parsedRevision != result.Revision {
		t.Fatalf("history reference round trip = %q/%q/%v", parsedID, parsedRevision, err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "restore", "--id", ref, "--format=json"}); code != ExitOK {
		t.Fatalf("restore code = %d, stderr = %q", code, stderr.String())
	}
	if fake.restoreCalls != 1 || !reflect.DeepEqual(fake.restoreRefs, []string{ref}) || fake.recoverRestoreCalls != 0 {
		t.Fatalf("restore calls/refs/recovery = %d/%q/%d", fake.restoreCalls, fake.restoreRefs, fake.recoverRestoreCalls)
	}
	var document runtimeRestoreDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"runtime_restore":`) || strings.Contains(stdout.String(), `"restore":`) {
		t.Fatalf("restore JSON envelope = %s", stdout.String())
	}
	if document.SchemaVersion != 1 || document.Restore != result || document.Restore.RevisionAppended || document.Restore.ManifestChanged || document.Restore.WorkspaceChanged {
		t.Fatalf("restore JSON = %+v", document)
	}
}

func TestRuntimeRestoreHumanOutputDistinguishesNoOpWithoutInventingMutation(t *testing.T) {
	manifest := readyRuntimeManifest()
	result := runtimeRestoreResult(manifest, 0, tobari.RuntimeAlreadyAvailable)
	command, fake, stdout, stderr := newRuntimeRestoreTestCLI(manifest, result, "")
	if code := command.RunContext(context.Background(), []string{"runtime", "restore", "--id", result.RevisionRef}); code != ExitOK {
		t.Fatalf("restore code = %d, stderr = %q", code, stderr.String())
	}
	if fake.restoreCalls != 1 {
		t.Fatalf("restore calls = %d", fake.restoreCalls)
	}
	for _, want := range []string{result.Name, result.RevisionRef, "already available", "no durable state changed", "History", "unchanged", "Workspace Templates", "Contexts", "Workspaces"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("human restore omitted %q: %q", want, stdout.String())
		}
	}
	for _, forbidden := range []string{manifest.Revisions[0].Image, manifest.Revisions[0].ImageDigest, manifest.Revisions[0].SnapshotPath} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Errorf("human restore exposed infrastructure evidence %q: %q", forbidden, stdout.String())
		}
	}
}

func TestRuntimeRestoreRejectsInvalidReferenceAndSemanticResultBeforeSuccess(t *testing.T) {
	manifest := readyRuntimeManifest()
	valid := runtimeRestoreResult(manifest, 0, tobari.RuntimeRestored)
	command, fake, stdout, stderr := newRuntimeRestoreTestCLI(manifest, valid, "")
	for _, invalidRef := range []string{"frontend@1"} {
		stderr.Reset()
		if code := command.RunContext(context.Background(), []string{"runtime", "restore", "--id", invalidRef}); code != ExitUsage {
			t.Fatalf("invalid ref %q code = %d, stderr = %q", invalidRef, code, stderr.String())
		}
	}
	if fake.restoreCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("invalid ref crossed restore/output = %d/%q", fake.restoreCalls, stdout.String())
	}

	invalid := valid
	invalid.RevisionRef = ""
	if _, err := renderRuntimeRestore("runtime restore", invalid, successFormatJSON, false); err == nil {
		t.Fatal("renderer accepted missing semantic revision reference")
	}
	invalid = valid
	invalid.ManifestChanged = true
	if _, err := renderRuntimeRestore("runtime restore", invalid, successFormatText, false); err == nil {
		t.Fatal("renderer accepted invented Manifest mutation")
	}
}

func TestRuntimeStandardOmitsRestoreReferenceWhileManagedHistoryRemainsExact(t *testing.T) {
	standard := tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: "sha256:" + strings.Repeat("f", 64), Image: "tobari-runtime:test", CreatedAt: time.Unix(1, 0).UTC()}},
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{manifest: standard, list: runtimeReviewList(readyRuntimeManifest())[:1]})
	for _, args := range [][]string{{"runtime", "show", "--name", "standard", "--format=json"}, {"runtime", "history", "--name", "standard", "--format=json"}, {"runtime", "list", "--format=json"}} {
		stdout.Reset()
		stderr.Reset()
		if code := command.RunContext(context.Background(), args); code != ExitOK {
			t.Fatalf("%v code = %d, stderr = %q", args, code, stderr.String())
		}
		if strings.Contains(stdout.String(), `"revision_ref"`) {
			t.Fatalf("standard producer %v exposed restore reference: %s", args, stdout.String())
		}
	}
}

func TestRuntimeRestoreReviewUsesOneConfirmationAndExactRecoveryReference(t *testing.T) {
	manifest := readyRuntimeManifest()
	result := runtimeRestoreResult(manifest, 0, tobari.RuntimeRestored)
	recovery := tobari.RuntimeBuildRecovery{
		RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID), RevisionRef: result.RevisionRef,
		Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryCleanup,
	}
	command, fake, stdout, stderr := newRuntimeRestoreTestCLI(manifest, result, "\n")
	fake.recovery = &recovery
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}
	if code := command.RunContext(context.Background(), []string{"review", "runtimes"}); code != ExitOK {
		t.Fatalf("restore recovery code = %d, stderr = %q", code, stderr.String())
	}
	if fake.recoveryReads != 1 || fake.recoverRestoreCalls != 1 || !reflect.DeepEqual(fake.recoverRestoreRefs, []string{result.RevisionRef}) ||
		!reflect.DeepEqual(fake.recoverRestoreKinds, []tobari.RuntimeBuildRecoveryKind{recovery.Kind}) || fake.recoveries != 0 || fake.restoreCalls != 0 {
		t.Fatalf("restore recovery calls = reads:%d restore:%d refs:%q kinds:%q build:%d direct:%d", fake.recoveryReads, fake.recoverRestoreCalls, fake.recoverRestoreRefs, fake.recoverRestoreKinds, fake.recoveries, fake.restoreCalls)
	}
	for _, want := range []string{"Tobari · Recover Runtime Restore", "Reference: " + result.RevisionRef, "Recover interrupted restore"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("restore recovery review omitted %q: %q", want, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), result.RevisionRef) || !strings.Contains(stdout.String(), "restored") {
		t.Fatalf("restore recovery output = %q", stdout.String())
	}
}

func TestRuntimeRestoreHelpAndCompletionDeriveFromCatalog(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	records, err := command.planCompletion(context.Background(), 4, []string{"tobari", "runtime", "restore", "--"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := completionRecordValues(records), []string{"candidate:--id", "candidate:--format"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restore completion = %v, want %v", got, want)
	}
	var stdout, stderr bytes.Buffer
	help := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	if code := help.RunContext(context.Background(), []string{"help", "runtime", "restore", "--format=agent"}); code != ExitOK {
		t.Fatalf("agent help code = %d, stderr = %q", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{
		`"path":"runtime restore"`, `"effect":"write"`, `"role":"act"`,
		`"reference_kind":"runtime-revision"`, `"argument":"--id"`,
		`"target_kind":"runtime-revision"`, `"target_id_input":"--id"`,
		`"destructive":"no"`, `"command":"runtime list"`, `"command":"review runtimes"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("restore agent help omitted %q: %s", want, text)
		}
	}
}

func TestRuntimeRestoreCatalogMatchesDirectAndReviewedPublicFaults(t *testing.T) {
	manifest := readyRuntimeManifest()
	valid := runtimeRestoreResult(manifest, 0, tobari.RuntimeRestored)
	intent := operation.Intent{
		Command: "runtime restore", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.RuntimeRevisionReferenceKind, ID: valid.RevisionRef}, Impact: runtimecmd.RestoreImpact(),
	}
	recovery := tobari.RuntimeBuildRecovery{
		RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID), RevisionRef: valid.RevisionRef,
		Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryCleanup,
	}
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
		if declared.Kind != public.Kind || declared.Phase != public.Phase || declared.ChangeState != public.ChangeState ||
			declared.Retryable != public.Retryable || !reflect.DeepEqual(declared.NextActions, public.NextActions) {
			t.Fatalf("%s fault %q Catalog=%+v PublicCopy=%+v", path, public.Code, declared, public)
		}
	}

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "observation unknown", err: tobari.ErrRuntimeRetirementObservationUnknown},
		{name: "digest unrestorable", err: tobari.ErrRuntimeRevisionUnrestorable},
		{name: "known partial interruption", err: tobari.ErrRuntimeRestoreInterrupted},
		{name: "unknown interruption outcome", err: context.Canceled},
	} {
		t.Run("direct "+test.name, func(t *testing.T) {
			fake := &runtimeRestoreCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, result: valid, restoreErr: test.err}
			_, err := runtimecmd.New(fake).Restore(context.Background(), intent, valid.RevisionRef, nil)
			assertDeclared(t, "runtime restore", err)
		})
		t.Run("reviewed "+test.name, func(t *testing.T) {
			fake := &runtimeRestoreCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, result: valid, restoreErr: test.err}
			_, err := runtimecmd.New(fake).RecoverRestore(context.Background(), intent, recovery, nil)
			assertDeclared(t, "review runtimes", err)
		})
	}

	for _, test := range []struct {
		name   string
		result tobari.RuntimeRestoreResult
	}{
		{name: "partial", result: func() tobari.RuntimeRestoreResult {
			result := valid
			result.Revision = "sha256:" + strings.Repeat("e", 64)
			result.RevisionRef = tobari.RuntimeRevisionRef(result.RuntimeID, result.Revision)
			return result
		}()},
		{name: "confirmed", result: func() tobari.RuntimeRestoreResult {
			result := valid
			result.ManifestChanged = true
			return result
		}()},
	} {
		t.Run("direct invalid result "+test.name, func(t *testing.T) {
			fake := &runtimeRestoreCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, result: test.result}
			_, err := runtimecmd.New(fake).Restore(context.Background(), intent, valid.RevisionRef, nil)
			assertDeclared(t, "runtime restore", err)
		})
		t.Run("reviewed invalid result "+test.name, func(t *testing.T) {
			fake := &runtimeRestoreCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: manifest}, result: test.result}
			_, err := runtimecmd.New(fake).RecoverRestore(context.Background(), intent, recovery, nil)
			assertDeclared(t, "review runtimes", err)
		})
	}
}
