package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimePruneCLI struct {
	*runtimeCatalogCLI
	snapshot    tobari.RuntimeLifecycleSnapshot
	observedAt  time.Time
	applyCalls  int
	appliedRefs []string
}

func (f *runtimePruneCLI) ReadRuntimeLifecycleSnapshot(context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	return f.snapshot, f.observedAt, nil
}

func (f *runtimePruneCLI) ApplyRuntimePrune(_ context.Context, planRef string) (tobari.RuntimePruneResult, error) {
	f.applyCalls++
	f.appliedRefs = append(f.appliedRefs, planRef)
	plan, err := tobari.PlanRuntimePrune(f.snapshot, f.observedAt)
	if err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	if len(plan.Candidates) == 0 {
		return tobari.RuntimePruneResult{
			Task: tobari.TaskRuntimePruneApply, PlanRef: planRef, State: tobari.RuntimePruneEmpty,
			Items: []tobari.RuntimePruneItemResult{}, SourcePreserved: true,
			SnapshotsPreserved: true, HistoryPreserved: true,
		}, nil
	}
	candidate := plan.Candidates[0]
	item := tobari.RuntimePruneItemResult{
		Kind: candidate.Kind, RuntimeID: candidate.RuntimeID, Revision: candidate.Revision,
		RuntimeRef: candidate.RuntimeRef, RevisionRef: candidate.RevisionRef, Name: candidate.Name,
		Ordinal: candidate.Ordinal, LastUsed: candidate.LastUsed, SourceLogicalBytes: candidate.SourceLogicalBytes,
		SnapshotLogicalBytes: candidate.SnapshotLogicalBytes, ImageVirtualBytes: candidate.ImageVirtualBytes,
		Disposition: tobari.RuntimePruneRemoved, RemovedTagCount: 1,
	}
	return tobari.RuntimePruneResult{
		Task: tobari.TaskRuntimePruneApply, PlanRef: planRef, State: tobari.RuntimePruneApplied,
		Items: []tobari.RuntimePruneItemResult{item}, RemovedTagCount: 1, ReceiptRevision: 1,
		SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true,
	}, nil
}

func runtimePruneAvailableSnapshot() (tobari.RuntimeLifecycleSnapshot, time.Time) {
	managed := readyRuntimeManifest()
	standard := tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: tobari.StandardRuntimeID,
		Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{
			Ordinal: 1, Revision: "sha256:" + strings.Repeat("f", 64), Image: tobari.OfficialRuntimeBase,
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
	revision := managed.Revisions[0]
	virtualBytes := int64(8192)
	return tobari.RuntimeLifecycleSnapshot{
		CatalogComplete: true,
		Runtimes:        []tobari.RuntimeManifest{standard, managed},
		Protection:      tobari.RuntimeProtectionInventory{Complete: true, Items: []tobari.RuntimeProtection{}},
		Materials: []tobari.RuntimeMaterialObservation{{
			RuntimeID: managed.ID, Revision: revision.Revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision,
			Availability: tobari.RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true,
			OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &virtualBytes,
		}},
		Storage: []tobari.RuntimeStorageObservation{{
			RuntimeID: managed.ID, Name: managed.Name, SourceLogicalBytes: 42,
			Snapshots: []tobari.RuntimeSnapshotStorage{{
				Kind: tobari.RuntimePruneCandidateRevision, Revision: revision.Revision,
				SemanticFingerprint: revision.Revision, LogicalBytes: 100,
			}},
		}},
		Journals: tobari.RuntimeLifecycleJournals{
			Complete: true, Active: []tobari.RuntimeLifecycleActivity{}, FailedBuilds: []tobari.RuntimeFailedBuildArtifact{},
		},
	}, time.Date(2026, 8, 24, 1, 2, 3, 4, time.UTC)
}

func newRuntimePruneTestCLI() (*CLI, *runtimePruneCLI, *bytes.Buffer, *bytes.Buffer) {
	snapshot, observedAt := runtimePruneAvailableSnapshot()
	fake := &runtimePruneCLI{runtimeCatalogCLI: &runtimeCatalogCLI{manifest: readyRuntimeManifest()}, snapshot: snapshot, observedAt: observedAt}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	return command, fake, stdout, stderr
}

func TestRuntimePruneCatalogClosesReviewedReferenceWorkflow(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("DefaultCatalog.Validate() error = %v", err)
	}
	dryRun, found := catalog.Lookup("runtime prune dry-run")
	if !found {
		t.Fatal("runtime prune dry-run is absent")
	}
	apply, found := catalog.Lookup("runtime prune apply")
	if !found {
		t.Fatal("runtime prune apply is absent")
	}
	if dryRun.Role != RoleDiscover || dryRun.Effect != operation.EffectRead ||
		!reflect.DeepEqual(dryRun.ProducedRefs(), []ProducedRef{{Kind: tobari.RuntimePrunePlanReferenceKind, Field: "plan_ref"}}) ||
		len(dryRun.ConsumedRefs()) != 0 {
		t.Fatalf("dry-run reference contract = role:%q effect:%q produced:%+v consumed:%+v", dryRun.Role, dryRun.Effect, dryRun.ProducedRefs(), dryRun.ConsumedRefs())
	}
	wantConsumed := []ConsumedRef{{Kind: tobari.RuntimePrunePlanReferenceKind, Argument: "--plan"}}
	if apply.Role != RoleAct || apply.Effect != operation.EffectWrite || !reflect.DeepEqual(apply.ConsumedRefs(), wantConsumed) || apply.Agent.Mutation == nil ||
		apply.Agent.Mutation.TargetKind != tobari.RuntimePrunePlanReferenceKind || apply.Agent.Mutation.TargetIDInput != "--plan" ||
		!reflect.DeepEqual(apply.Agent.Mutation.TargetInputs, []string{"--plan"}) || apply.Agent.Mutation.Impact != runtimecmd.PruneImpact() {
		t.Fatalf("apply mutation/reference contract = role:%q effect:%q consumed:%+v mutation:%+v", apply.Role, apply.Effect, apply.ConsumedRefs(), apply.Agent.Mutation)
	}
	confirm, found := commandInput(apply.Agent.Inputs, "--confirm")
	if !found || !confirm.Required || !reflect.DeepEqual(confirm.AllowedValues, []string{"prune"}) || confirm.ReferenceKind != "" {
		t.Fatalf("apply confirmation contract = %+v, found=%t", confirm, found)
	}
}

func TestRuntimePruneDryRunAndApplyRoundTripExactPlanReference(t *testing.T) {
	command, fake, stdout, stderr := newRuntimePruneTestCLI()
	fake.snapshot.RetirementGenerations = []tobari.RuntimeRetirementGeneration{{RuntimeID: fake.snapshot.Materials[0].RuntimeID, Revision: fake.snapshot.Materials[0].Revision, Generation: 1}}
	direct, err := tobari.PlanRuntimePrune(fake.snapshot, fake.observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := renderRuntimePrunePlan("runtime prune dry-run", direct, successFormatJSON, false); err != nil {
		t.Fatalf("direct dry-run rendering failed: %v", err)
	}
	directResult, err := fake.ApplyRuntimePrune(context.Background(), direct.PlanRef)
	if err != nil {
		t.Fatal(err)
	}
	if err := directResult.Validate(); err != nil {
		t.Fatalf("direct apply result failed validation: %v", err)
	}
	if _, err := renderRuntimePruneResult("runtime prune apply", directResult, successFormatJSON, false); err != nil {
		t.Fatalf("direct apply rendering failed: %v", err)
	}
	fake.applyCalls = 0
	fake.appliedRefs = nil
	if code := command.RunContext(context.Background(), []string{"runtime", "prune", "dry-run", "--format", "json"}); code != ExitOK {
		t.Fatalf("dry-run code = %d, stderr = %q", code, stderr.String())
	}
	for _, forbidden := range []string{"retirement_generations", "availability_supersessions", "through_receipt_revision"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("dry-run exposed internal retirement authority %q: %s", forbidden, stdout.String())
		}
		commandSpec, found := DefaultCatalog().Lookup("runtime prune dry-run")
		if !found {
			t.Fatal("runtime prune dry-run is absent from Catalog")
		}
		if outputDeclaresField(commandSpec.Agent.Output.Fields, forbidden) {
			t.Fatalf("Catalog declared internal retirement authority %q", forbidden)
		}
	}
	var planDoc runtimePrunePlanJSONDocument
	if err := json.Unmarshal(stdout.Bytes(), &planDoc); err != nil {
		t.Fatal(err)
	}
	plan := planDoc.Plan
	if planDoc.SchemaVersion != 1 || plan.Task != tobari.TaskRuntimePruneDryRun || plan.PlanRef == "" || plan.Empty || !plan.Applicable ||
		len(plan.Candidates) != 1 || plan.Candidates[0].LastUsed != tobari.RuntimeLastUsedUnknown || plan.Candidates[0].ReclaimableBytes != nil ||
		len(plan.Storage) != 1 || fake.applyCalls != 0 {
		t.Fatalf("dry-run projection/calls = %+v calls=%d", planDoc, fake.applyCalls)
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "prune", "apply", "--plan", plan.PlanRef, "--confirm=prune", "--format", "json"}); code != ExitOK {
		t.Fatalf("apply code = %d, stderr = %q", code, stderr.String())
	}
	if fake.applyCalls != 1 || !reflect.DeepEqual(fake.appliedRefs, []string{plan.PlanRef}) {
		t.Fatalf("apply calls/refs = %d/%q", fake.applyCalls, fake.appliedRefs)
	}
	var resultDoc runtimePruneResultJSONDocument
	if err := json.Unmarshal(stdout.Bytes(), &resultDoc); err != nil {
		t.Fatal(err)
	}
	result := resultDoc.Result
	if resultDoc.SchemaVersion != 1 || result.Task != tobari.TaskRuntimePruneApply || result.PlanRef != plan.PlanRef || result.State != tobari.RuntimePruneApplied ||
		len(result.Items) != 1 || result.Items[0].LastUsed != tobari.RuntimeLastUsedUnknown || result.Items[0].ReclaimedBytes != nil ||
		result.ReclaimedBytes != nil || result.RemovedTagCount != 1 || !result.SourcePreserved || !result.SnapshotsPreserved || !result.HistoryPreserved {
		t.Fatalf("apply projection = %+v", resultDoc)
	}
}

func TestRuntimePruneApplyRequiresExplicitConfirmationBeforeAdapter(t *testing.T) {
	command, fake, stdout, stderr := newRuntimePruneTestCLI()
	plan, err := tobari.PlanRuntimePrune(fake.snapshot, fake.observedAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"runtime", "prune", "apply", "--plan", plan.PlanRef},
		{"runtime", "prune", "apply", "--plan", plan.PlanRef, "--confirm=delete"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := command.RunContext(context.Background(), args); code != ExitUsage {
			t.Fatalf("args %q code = %d, stderr = %q", args, code, stderr.String())
		}
	}
	if fake.applyCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("unconfirmed prune crossed adapter/output = calls:%d stdout:%q", fake.applyCalls, stdout.String())
	}
}

func TestRuntimePruneHumanOutputPreservesUncertaintyAndHidesDockerMechanics(t *testing.T) {
	command, fake, stdout, stderr := newRuntimePruneTestCLI()
	if code := command.RunContext(context.Background(), []string{"runtime", "prune", "dry-run"}); code != ExitOK {
		t.Fatalf("dry-run code = %d, stderr = %q", code, stderr.String())
	}
	plan, err := tobari.PlanRuntimePrune(fake.snapshot, fake.observedAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Runtime prune plan", plan.PlanRef, "Last used", "unknown", "Reclaimable bytes", "preserved", "runtime prune apply --plan " + plan.PlanRef + " --confirm=prune"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("human dry-run omitted %q: %q", want, stdout.String())
		}
	}
	for _, hidden := range []string{fake.snapshot.Runtimes[1].Revisions[0].Image, fake.snapshot.Runtimes[1].Revisions[0].ImageDigest, "container_id", "docker_id"} {
		if hidden != "" && strings.Contains(stdout.String(), hidden) {
			t.Errorf("human dry-run exposed Docker mechanic %q: %q", hidden, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "prune", "apply", "--plan", plan.PlanRef, "--confirm=prune"}); code != ExitOK {
		t.Fatalf("apply code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Runtime prune applied", "Last used", "unknown", "Reclaimed bytes", "Runtime source · immutable snapshots · revision history"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("human apply omitted %q: %q", want, stdout.String())
		}
	}
}

func TestRuntimePruneRenderersRejectInvalidSemanticResults(t *testing.T) {
	snapshot, observedAt := runtimePruneAvailableSnapshot()
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	invalidPlan := plan
	invalidPlan.PlanRef = ""
	if _, err := renderRuntimePrunePlan("runtime prune dry-run", invalidPlan, successFormatJSON, false); err == nil {
		t.Fatal("plan renderer accepted missing semantic plan reference")
	}
	result := tobari.RuntimePruneResult{
		Task: tobari.TaskRuntimePruneApply, PlanRef: plan.PlanRef, State: tobari.RuntimePruneEmpty,
		Items: []tobari.RuntimePruneItemResult{}, SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true,
	}
	result.PlanRef = ""
	if _, err := renderRuntimePruneResult("runtime prune apply", result, successFormatJSON, false); err == nil {
		t.Fatal("result renderer accepted missing semantic plan reference")
	}
}

func TestRuntimePruneJSONPreservesExplicitEmptyAndOptionalBlockerAuthority(t *testing.T) {
	snapshot, observedAt := runtimePruneAvailableSnapshot()
	snapshot.Runtimes = snapshot.Runtimes[:1]
	snapshot.Materials = []tobari.RuntimeMaterialObservation{}
	snapshot.Storage = []tobari.RuntimeStorageObservation{}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := renderRuntimePrunePlan("runtime prune dry-run", plan, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"candidates":[]`, `"protected":[]`, `"blockers":[]`, `"storage":[]`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("empty plan omitted explicit %s: %s", want, encoded)
		}
	}

	projection := runtimePrunePlanProjectionFrom(tobari.RuntimePrunePlan{Blockers: []tobari.RuntimeMaterialBlocker{
		{RuntimeID: readyRuntimeManifest().ID, Reason: tobari.RuntimeBlockedByActiveBuild},
	}})
	blocker, err := json.Marshal(projection.Blockers[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blocker), `"revision"`) {
		t.Fatalf("whole-Runtime blocker invented an empty revision field: %s", blocker)
	}
}

func TestRuntimePruneHelpAndCompletionDeriveFromAtomicCatalogClosure(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	tests := []struct {
		current int
		words   []string
		want    []string
	}{
		{current: 4, words: []string{"tobari", "runtime", "prune", "d"}, want: []string{"candidate:dry-run"}},
		{current: 5, words: []string{"tobari", "runtime", "prune", "apply", "--"}, want: []string{"candidate:--plan", "candidate:--confirm", "candidate:--format"}},
		{current: 5, words: []string{"tobari", "runtime", "prune", "apply", "--confirm=p"}, want: []string{"candidate:--confirm=prune"}},
	}
	for _, test := range tests {
		records, err := command.planCompletion(context.Background(), test.current, test.words)
		if err != nil {
			t.Fatal(err)
		}
		if got := completionRecordValues(records); !reflect.DeepEqual(got, test.want) {
			t.Errorf("completion for %q = %v, want %v", test.words, got, test.want)
		}
	}

	var stdout, stderr bytes.Buffer
	help := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	if code := help.RunContext(context.Background(), []string{"help", "runtime", "prune", "--format=agent"}); code != ExitOK {
		t.Fatalf("agent help code = %d, stderr = %q", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{
		`"path":"runtime prune dry-run"`, `"path":"runtime prune apply"`,
		`"kind":"runtime-prune-plan"`, `"argument":"--plan"`,
		`"target_id_input":"--plan"`, `"destructive":"yes"`,
		`"command":"runtime prune dry-run"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("agent help omitted %q: %s", want, text)
		}
	}
}

func commandInput(inputs []CommandInput, name string) (CommandInput, bool) {
	for _, input := range inputs {
		if input.Name == name {
			return input, true
		}
	}
	return CommandInput{}, false
}
