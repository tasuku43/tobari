package workspaceauthoritystore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type statusHomeRuntimeFixture struct {
	dockerCalls     int
	clusterCalls    int
	runtimeCalls    int
	workspaceCalls  int
	attachmentCalls int
	serviceCalls    int
}

func (f *statusHomeRuntimeFixture) ObserveFinalCluster(_ context.Context, collection tobari.WorkspaceAuthorityCollection, _ bool) (tobari.FinalClusterStatus, error) {
	f.clusterCalls++
	f.dockerCalls += 4
	return finalClusterReadStatus(collection), nil
}

func (f *statusHomeRuntimeFixture) ObserveStatusRuntime(context.Context, tobari.RuntimeBinding) (tobari.StatusRuntimeObservation, error) {
	f.runtimeCalls++
	f.dockerCalls++
	return tobari.StatusRuntimeObservation{Authority: tobari.StatusRuntimeAuthorityReady, Availability: tobari.RuntimeAvailabilityAvailable, Compatibility: tobari.StatusNativeCompatible}, nil
}

func (f *statusHomeRuntimeFixture) ObserveStatusWorkspace(context.Context, tobari.ContextAuthoritySnapshot) (tobari.StatusWorkspaceObservation, error) {
	f.workspaceCalls++
	f.dockerCalls++
	return tobari.StatusWorkspaceObservation{State: tobari.StatusWorkspaceRuntimeRunning}, nil
}

func (f *statusHomeRuntimeFixture) ObserveStatusAttachment(context.Context, tobari.WorkspaceSessionIdentity) (tobari.StatusAttachmentState, error) {
	f.attachmentCalls++
	return tobari.StatusAttachmentDetached, nil
}

func (f *statusHomeRuntimeFixture) ObserveStatusServices(context.Context, tobari.ContextID, tobari.WorkspaceID) (tobari.ServiceSummary, error) {
	f.serviceCalls++
	return tobari.ServiceSummary{SchemaVersion: 1, Observation: tobari.ServiceObservationComplete}, nil
}

func TestStatusHomeFrozenDockerBudgetIsSixNormalTwelveAfterOneRetry(t *testing.T) {
	if tobari.StatusHomeDockerCallsPerAttempt != 6 || tobari.StatusHomeDockerCallCeiling != 12 {
		t.Fatalf("frozen budget normal=%d ceiling=%d", tobari.StatusHomeDockerCallsPerAttempt, tobari.StatusHomeDockerCallCeiling)
	}
	for _, test := range []struct {
		name       string
		guard      *legacyGuardFake
		wantDocker int
		wantCalls  int
	}{
		{name: "normal", guard: &legacyGuardFake{}, wantDocker: 6, wantCalls: 1},
		{name: "one bounded retry", guard: &legacyGuardFake{errors: []error{nil, nil, errors.New("anchor changed"), nil, nil}}, wantDocker: 12, wantCalls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			collection := storeCollectionFixture(t)
			authorityRoot := filepath.Join(t.TempDir(), "authority")
			materializeCollection(t, authorityRoot, collection)
			store, err := NewFinalOnly(authorityRoot, test.guard)
			if err != nil {
				t.Fatal(err)
			}
			runtime := &statusHomeRuntimeFixture{}
			root := defaultPairRootFixture{cwd: collection.Workspaces[0].ProjectRoot, root: collection.Workspaces[0].ProjectRoot}
			adapter, err := NewStatusHomeAdapter(store, root, runtime)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := adapter.ObserveStatusHome(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !observation.Present || runtime.dockerCalls != test.wantDocker || runtime.clusterCalls != test.wantCalls || runtime.runtimeCalls != test.wantCalls || runtime.workspaceCalls != test.wantCalls {
				t.Fatalf("present=%t docker=%d calls=(%d,%d,%d)", observation.Present, runtime.dockerCalls, runtime.clusterCalls, runtime.runtimeCalls, runtime.workspaceCalls)
			}
			if runtime.attachmentCalls != test.wantCalls || runtime.serviceCalls != test.wantCalls {
				t.Fatalf("owner summary calls attachment=%d service=%d", runtime.attachmentCalls, runtime.serviceCalls)
			}
		})
	}
}

func TestStatusHomeFreshWithoutDefaultUsesZeroDockerAndCreatesNothing(t *testing.T) {
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	store, err := NewFinalOnly(authorityRoot, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &statusHomeRuntimeFixture{}
	adapter, err := NewStatusHomeAdapter(store, defaultPairRootFixture{cwd: "/workspace/fresh/subdir", root: "/workspace/fresh"}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := adapter.ObserveStatusHome(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Present || runtime.dockerCalls != 0 || runtime.attachmentCalls != 0 || runtime.serviceCalls != 0 {
		t.Fatalf("fresh observation=%+v runtime=%+v", observation, runtime)
	}
	if _, err := os.Lstat(authorityRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh status created state: %v", err)
	}
}

func TestStatusHomeIgnoresLocationFreeContextWithoutWorkspace(t *testing.T) {
	base := storeCollectionFixture(t)
	nestedContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b2")
	nestedContext := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: nestedContextID, TemplateID: base.Templates[0].ID}
	nestedMemory, _, err := tobari.PublishPolicyMemory(nestedContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	nestedRecord := tobari.WorkspaceAuthorityContextRecord{Context: nestedContext, PolicyMemory: nestedMemory}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, append(base.Contexts, nestedRecord), base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	materializeCollection(t, authorityRoot, collection)
	store, err := NewFinalOnly(authorityRoot, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &statusHomeRuntimeFixture{}
	resolver := defaultPairRootFixture{cwd: "/workspace/example/src/deep", root: "/workspace/example/src/deep"}
	adapter, err := NewStatusHomeAdapter(store, resolver, runtime)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := adapter.ObserveStatusHome(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.ProjectRoot != "/workspace/example" || runtime.dockerCalls != 6 {
		t.Fatalf("root=%q Docker calls=%d", observation.ProjectRoot, runtime.dockerCalls)
	}
}
