package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestObserveStatusRuntimeUsesOneExactReadWithoutCreatingLifecycleState(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	runner := &finalContextRuntimeRunner{observations: []string{finalStandardRuntimeObservation(
		"sha256:"+strings.Repeat("a", 64), tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand,
		"tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`,
	)}}
	runtime, err := newRuntime(filepath.Join(root, "config"), state, runner)
	if err != nil {
		t.Fatal(err)
	}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.ObserveStatusRuntime(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if result.Authority != tobari.StatusRuntimeAuthorityReady || result.Availability != tobari.RuntimeAvailabilityAvailable || result.Compatibility != tobari.StatusNativeCompatible {
		t.Fatalf("Runtime status=%+v", result)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "image" || runner.calls[0][1] != "inspect" || runner.calls[0][len(runner.calls[0])-1] != binding.Image {
		t.Fatalf("Docker calls=%v", runner.calls)
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Fatalf("Runtime status created lifecycle state: %v", err)
	}
}

func TestObserveStatusRuntimeDoesNotAcquireOrRequireMutationLock(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &finalContextRuntimeRunner{observations: []string{finalStandardRuntimeObservation(
		"sha256:"+strings.Repeat("b", 64), tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand,
		"tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`,
	)}}
	runtime, err := newRuntime(filepath.Join(root, "config"), state, runner)
	if err != nil {
		t.Fatal(err)
	}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ObserveStatusRuntime(context.Background(), binding); err != nil {
		t.Fatalf("pure Runtime observation required a mutation lock: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "lifecycle.lock")); !os.IsNotExist(err) {
		t.Fatalf("status created or required lifecycle.lock: %v", err)
	}
}

func TestObserveStatusRuntimeReportsExactHistoricalStandardMaterialReady(t *testing.T) {
	root := t.TempDir()
	runner := &finalContextRuntimeRunner{observations: []string{finalStandardRuntimeObservation(
		"sha256:"+strings.Repeat("c", 64), tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand,
		"tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`,
	)}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	binding := historicalStandardRuntimeBinding(strings.Repeat("4", 64))
	result, err := runtime.ObserveStatusRuntime(context.Background(), binding)
	if err != nil || result.Authority != tobari.StatusRuntimeAuthorityReady || result.Availability != tobari.RuntimeAvailabilityAvailable || result.Compatibility != tobari.StatusNativeCompatible {
		t.Fatalf("historical Runtime status=%+v err=%v", result, err)
	}
	if len(runner.calls) != 1 || runner.calls[0][len(runner.calls[0])-1] != binding.Image {
		t.Fatalf("historical Runtime Docker calls=%v", runner.calls)
	}
}

func TestObserveStatusRuntimeReportsWritableVolumeImageIncompatible(t *testing.T) {
	root := t.TempDir()
	observation := finalStandardRuntimeObservation(
		"sha256:"+strings.Repeat("d", 64), tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand,
		"tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`,
	)
	runner := &finalContextRuntimeRunner{observations: []string{strings.TrimSuffix(observation, "}") + `,"volumes":{"/data":{}}}`}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	binding := historicalStandardRuntimeBinding(strings.Repeat("6", 64))
	result, err := runtime.ObserveStatusRuntime(context.Background(), binding)
	if err != nil || result.Authority != tobari.StatusRuntimeAuthorityReady || result.Availability != tobari.RuntimeAvailabilityAvailable || result.Compatibility != tobari.StatusNativeIncompatible {
		t.Fatalf("writable-volume Runtime status=%+v err=%v", result, err)
	}
}

func TestObserveStatusServicesReturnsOnlyTypedEmptySummaryWithoutCreatingOwnerState(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	contextID, err := tobari.ParseContextID("01912345-6789-7abc-8def-0123456789a2")
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := tobari.ParseWorkspaceID("01912345-6789-7abc-8def-0123456789a3")
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.ObserveStatusServices(context.Background(), contextID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observation != tobari.ServiceObservationComplete || result.PendingCount != 0 || result.ActiveCount != 0 || result.UnavailableOwnerCount != 0 {
		t.Fatalf("Service summary=%+v", result)
	}
	if _, err := os.Lstat(runtime.serviceExposureLiveDirectory()); !os.IsNotExist(err) {
		t.Fatalf("Service summary read created owner state: %v", err)
	}
}

func TestObserveStatusAttachmentFreshReadCreatesNoRegistryOrLock(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &finalWorkspaceSessionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	identity, err := binding.Identity()
	if err != nil {
		t.Fatal(err)
	}
	state, err := runtime.ObserveStatusAttachment(context.Background(), identity)
	if err != nil || state != tobari.StatusAttachmentDetached {
		t.Fatalf("fresh attachment state=%q err=%v", state, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fresh status attachment created state: %v", entries)
	}
}
