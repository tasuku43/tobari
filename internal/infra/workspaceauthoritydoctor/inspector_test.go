package workspaceauthoritydoctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type readerFixture struct {
	collection  tobari.WorkspaceAuthorityCollection
	present     bool
	err         error
	calls       int
	recovery    tobari.FinalAuthorityMutationObservation
	recoveryErr error
}

func (r *readerFixture) ReadComplete(context.Context) (tobari.WorkspaceAuthorityCollection, bool, error) {
	r.calls++
	return r.collection.Clone(), r.present, r.err
}

func (r *readerFixture) ObserveMutationRecovery(context.Context) (tobari.FinalAuthorityMutationObservation, error) {
	return r.recovery, r.recoveryErr
}

type clusterFixture struct {
	status tobari.FinalClusterStatus
	calls  int
}

func (c *clusterFixture) Observe(context.Context) (tobari.FinalClusterStatus, error) {
	c.calls++
	return c.status, nil
}

type genericFixture struct {
	calls             []doctor.CheckID
	runtimeBindings   []tobari.RuntimeBinding
	runtimeErr        error
	runtimeCalls      int
	runtimeCollection tobari.WorkspaceAuthorityCollection
}

func (g *genericFixture) ObserveDoctorCheck(_ context.Context, _ string, id doctor.CheckID) (doctor.Observation, error) {
	g.calls = append(g.calls, id)
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "generic"}, nil
}

func (g *genericFixture) ObserveFinalRuntimeMaterials(_ context.Context, collection tobari.WorkspaceAuthorityCollection) ([]tobari.RuntimeBinding, error) {
	g.runtimeCalls++
	g.runtimeCollection = collection.Clone()
	return append([]tobari.RuntimeBinding{}, g.runtimeBindings...), g.runtimeErr
}

func emptyClusterStatus(runtime tobari.FinalClusterRuntimeState) tobari.FinalClusterStatus {
	return tobari.FinalClusterStatus{SchemaVersion: tobari.FinalClusterStatusSchemaVersion, Task: tobari.TaskClusterStatus,
		Authority: tobari.FinalClusterAuthorityAbsent, Runtime: runtime, Receipt: tobari.FinalClusterReceiptAbsent,
		Contexts: []tobari.FinalClusterContextReceiptObservation{}, Components: []tobari.FinalClusterComponentObservation{}}
}

func TestInspectorNeverDelegatesFinalOrLegacyAuthorityChecks(t *testing.T) {
	reader := &readerFixture{err: fmtLegacy()}
	cluster := &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeAbsent)}
	generic := &genericFixture{}
	inspector, err := New(reader, cluster, generic, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDContext)
	if err != nil || observation.Status != doctor.CheckStatusFail || observation.Cause != doctor.ObservationCauseLegacyStatePresent || !strings.Contains(observation.Detail, "pre-release") {
		t.Fatalf("legacy observation=%#v err=%v", observation, err)
	}
	if len(generic.calls) != 0 || cluster.calls != 0 {
		t.Fatalf("legacy bytes reached generic=%v cluster=%d", generic.calls, cluster.calls)
	}

	reader.err = nil
	reader.present = false
	observation, err = inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDPolicyData)
	if err != nil || observation.Status != doctor.CheckStatusPass || len(generic.calls) != 0 {
		t.Fatalf("final policy observation=%#v generic=%v err=%v", observation, generic.calls, err)
	}
	if _, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDDockerCLI); err != nil || len(generic.calls) != 1 || generic.calls[0] != doctor.CheckIDDockerCLI {
		t.Fatalf("generic calls=%v err=%v", generic.calls, err)
	}
}

func TestInspectorTreatsFreshFinalAuthorityAsExactEmptyAndUsesTypedClusterStatus(t *testing.T) {
	reader := &readerFixture{}
	cluster := &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeAbsent)}
	generic := &genericFixture{}
	inspector, err := New(reader, cluster, generic, nil)
	if err != nil {
		t.Fatal(err)
	}
	contextObservation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDContext)
	if err != nil || contextObservation.Status != doctor.CheckStatusPass || !strings.Contains(contextObservation.Detail, "clean and empty") {
		t.Fatalf("Context observation=%#v err=%v", contextObservation, err)
	}
	state, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDState)
	if err != nil || state.Status != doctor.CheckStatusWarn || cluster.calls != 1 {
		t.Fatalf("state=%#v calls=%d err=%v", state, cluster.calls, err)
	}
	image, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDImageConfig)
	if err != nil || image.Status != doctor.CheckStatusPass || generic.runtimeCalls != 0 {
		t.Fatalf("fresh image=%#v Runtime calls=%d err=%v", image, generic.runtimeCalls, err)
	}
}

func TestInspectorPolicyDataDoesNotMislabelDurableStateAsLiveCandidateCount(t *testing.T) {
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reader := &readerFixture{collection: collection, present: true}
	inspector, err := New(reader, &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeRunning)}, &genericFixture{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDPolicyData)
	if err != nil || observation.Status != doctor.CheckStatusPass || !strings.Contains(observation.Detail, "policy candidates") || strings.Contains(observation.Detail, "0 pending candidates") {
		t.Fatalf("policy_data observation=%#v err=%v", observation, err)
	}
}

func TestInspectorClassifiesPreservedMutationRecoveryBeforeFinalContextObservation(t *testing.T) {
	contextRef, err := tobari.ContextRef(tobari.ContextID("01912345-6789-7abc-8def-0123456789ad"))
	if err != nil {
		t.Fatal(err)
	}
	reader := &readerFixture{recovery: tobari.FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "context-entry", Target: contextRef}}
	inspector, err := New(reader, &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeAbsent)}, &genericFixture{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDContext)
	if err != nil || observation.Status != doctor.CheckStatusFail || observation.Cause != doctor.ObservationCauseMutationRecoveryRequired || !strings.Contains(observation.Detail, "exact initiating command") {
		t.Fatalf("recovery observation=%#v err=%v", observation, err)
	}
}

func TestInspectorRequiresExactFinalRuntimeMaterialObservation(t *testing.T) {
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reader := &readerFixture{collection: collection, present: true}
	cluster := &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeAbsent)}
	generic := &genericFixture{runtimeBindings: []tobari.RuntimeBinding{}}
	inspector, err := New(reader, cluster, generic, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDImageConfig)
	if err != nil || observation.Status != doctor.CheckStatusPass || generic.runtimeCalls != 1 || generic.runtimeCollection.Revision != collection.Revision {
		t.Fatalf("available observation=%#v Runtime calls=%d collection=%s err=%v", observation, generic.runtimeCalls, generic.runtimeCollection.Revision, err)
	}

	generic.runtimeErr = errors.New("synthetic missing or unknown Runtime material")
	observation, err = inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDImageConfig)
	if err != nil || observation.Status != doctor.CheckStatusFail || !strings.Contains(observation.Detail, "missing") {
		t.Fatalf("unavailable observation=%#v err=%v", observation, err)
	}
}

func TestInspectorRejectsGenericWithoutFinalRuntimeMaterialObserver(t *testing.T) {
	type doctorOnly struct{ GenericInspector }
	_, err := New(&readerFixture{}, &clusterFixture{}, &doctorOnly{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Runtime material observer") {
		t.Fatalf("New() error = %v", err)
	}
}

func TestInspectorNeverCallsRunningAuthorityHealthyWithoutExactActiveReceipt(t *testing.T) {
	reader := &readerFixture{}
	cluster := &clusterFixture{status: emptyClusterStatus(tobari.FinalClusterRuntimeRunning)}
	inspector, err := New(reader, cluster, &genericFixture{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inspector.ObserveDoctorCheck(context.Background(), "/workspace", doctor.CheckIDState)
	if err != nil || observation.Status != doctor.CheckStatusFail || !strings.Contains(observation.Detail, "interrupted") {
		t.Fatalf("state=%#v err=%v", observation, err)
	}
}

func fmtLegacy() error {
	return errors.Join(tobari.ErrPreReleaseLegacyAuthority, errors.New("hostile legacy bytes were not opened"))
}
