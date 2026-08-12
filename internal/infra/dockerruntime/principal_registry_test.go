package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const principalTestContextID = "01912345-6789-7abc-8def-0123456789ad"

func TestPolicyProjectionLockSerializesCrossContextMutations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtimeStore, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	var active atomic.Int32
	var maximum atomic.Int32
	var completed atomic.Int32
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := runtimeStore.withPolicyProjectionLock(context.Background(), func() error {
				current := active.Add(1)
				for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
				}
				for range 100 {
					runtime.Gosched()
				}
				active.Add(-1)
				completed.Add(1)
				return nil
			}); err != nil {
				t.Errorf("withPolicyProjectionLock() error = %v", err)
			}
		}()
	}
	wait.Wait()
	if completed.Load() != 16 || maximum.Load() != 1 {
		t.Fatalf("policy projection mutations completed=%d maximum_concurrent=%d", completed.Load(), maximum.Load())
	}
}

func principalTestProject(t *testing.T, root string) tobari.ProjectInstance {
	t.Helper()
	project, err := tobari.NewProjectInstance(
		time.Unix(0, 0).UTC(), strings.NewReader("0123456789abcdef"), tobari.ProjectInstanceRequest{
			Root:        root,
			ContextID:   principalTestContextID,
			ContextName: "default",
			Image:       tobari.BuiltinImageSelector,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func principalTestBinding(projectID, workspaceIP, gatewayIP, network string) projectPrincipalBinding {
	return projectPrincipalBinding{
		ProjectID: projectID, ContextID: principalTestContextID, ContextName: "default",
		ProjectRoot: "/workspace/project", WorkspaceIP: workspaceIP, GatewayIP: gatewayIP, Network: network,
	}
}

func TestProjectPrincipalRegistryRejectsAmbiguousBindings(t *testing.T) {
	registry := projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema,
		Bindings: []projectPrincipalBinding{
			principalTestBinding("01912345-6789-7abc-8def-0123456789ab", "172.29.0.3", "172.29.0.2", "tobari-a-net"),
			principalTestBinding("01912345-6789-7abc-8def-0123456789ac", "172.29.1.3", "172.29.0.2", "tobari-b-net"),
		},
	}
	if err := registry.Validate(); err == nil {
		t.Fatal("project principal registry accepted duplicate Gateway address")
	}
}

func TestProjectPrincipalRegistryRejectsCrossRoleOverlapAndIPv6(t *testing.T) {
	tests := []struct {
		name        string
		workspaceIP string
		gatewayIP   string
	}{
		{name: "Workspace matches another Gateway", workspaceIP: "172.29.1.3", gatewayIP: "172.29.0.3"},
		{name: "Gateway matches another Workspace", workspaceIP: "172.29.0.2", gatewayIP: "172.29.1.2"},
		{name: "Workspace IPv6", workspaceIP: "fd00::3", gatewayIP: "172.29.1.2"},
		{name: "Gateway IPv6", workspaceIP: "172.29.1.3", gatewayIP: "fd00::2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := projectPrincipalRegistry{
				SchemaVersion: projectPrincipalRegistrySchema,
				Bindings: []projectPrincipalBinding{
					principalTestBinding("01912345-6789-7abc-8def-0123456789ab", "172.29.0.3", "172.29.0.2", "tobari-a-net"),
					principalTestBinding("01912345-6789-7abc-8def-0123456789ac", test.workspaceIP, test.gatewayIP, "tobari-b-net"),
				},
			}
			if err := registry.Validate(); err == nil {
				t.Fatal("project principal registry accepted ambiguous or non-IPv4 endpoint")
			}
		})
	}
}

func TestProjectPrincipalRegistryUpdateIsAtomicAndProjectBound(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	projectID := "01912345-6789-7abc-8def-0123456789ab"
	project := principalTestProject(t, filepath.Join(root, "project"))
	project.ID = projectID
	if err := runtime.updateProjectPrincipal(context.Background(), project, "tobari-a-net", "172.29.0.3", "172.29.0.2"); err != nil {
		t.Fatalf("updateProjectPrincipal() error = %v", err)
	}
	data, err := os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	var registry projectPrincipalRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 1 || registry.Bindings[0].ProjectID != projectID {
		t.Fatalf("registry = %+v", registry)
	}
	if err := runtime.removeProjectPrincipal(context.Background(), projectID); err != nil {
		t.Fatalf("removeProjectPrincipal() error = %v", err)
	}
	registry = projectPrincipalRegistry{}
	data, err = os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 0 {
		t.Fatalf("removed project remains in registry: %+v", registry)
	}
}

func TestProjectPrincipalRegistryRejectsStaleOrMalformedState(t *testing.T) {
	tests := map[string]projectPrincipalRegistry{
		"wrong schema": {SchemaVersion: projectPrincipalRegistrySchema + 1},
		"invalid project": {
			SchemaVersion: projectPrincipalRegistrySchema,
			Bindings:      []projectPrincipalBinding{principalTestBinding("not-a-project", "172.29.0.3", "172.29.0.2", "tobari-net")},
		},
		"loopback": {
			SchemaVersion: projectPrincipalRegistrySchema,
			Bindings:      []projectPrincipalBinding{principalTestBinding("01912345-6789-7abc-8def-0123456789ab", "127.0.0.1", "172.29.0.2", "tobari-net")},
		},
	}
	for name, registry := range tests {
		t.Run(name, func(t *testing.T) {
			if err := registry.Validate(); err == nil {
				t.Fatal("registry unexpectedly validated")
			}
		})
	}
}

func TestProjectPrincipalRegistryMissingFileFailsClosed(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.readProjectPrincipalRegistry(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readProjectPrincipalRegistry() error = %v, want missing file", err)
	}
}

func TestProjectPrincipalRegistryUsesValidatedProjectIDs(t *testing.T) {
	if err := tobari.ValidateProjectID("01912345-6789-7abc-8def-0123456789ab"); err != nil {
		t.Fatal(err)
	}
}
