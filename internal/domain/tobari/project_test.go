package tobari

import (
	"bytes"
	"testing"
	"time"
)

func TestNewProjectIDProducesUUIDv7(t *testing.T) {
	t.Parallel()
	id, err := NewProjectID(time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatalf("NewProjectID() error = %v", err)
	}
	if id != "018bcfe5-687b-7000-8000-000000000000" {
		t.Fatalf("NewProjectID() = %q", id)
	}
	if err := ValidateProjectID(id); err != nil {
		t.Fatalf("ValidateProjectID() error = %v", err)
	}
}

func TestProjectInstanceDoesNotRequireRuntimeResources(t *testing.T) {
	t.Parallel()
	instance, err := NewProjectInstance(
		time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)), "/workspace/project", BuiltinImageSelector,
	)
	if err != nil {
		t.Fatalf("NewProjectInstance() error = %v", err)
	}
	if instance.Runtime.ContainerID != "" || instance.Runtime.NetworkID != "" {
		t.Fatalf("new instance runtime = %+v, want absent diagnostic resources", instance.Runtime)
	}
	if err := instance.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestNearestRootSelectsNearestAncestor(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{
		{SchemaVersion: 1, Root: "/src/project", InstanceID: "018bcfe5-687b-7000-8000-000000000000"},
		{SchemaVersion: 1, Root: "/src/project/internal", InstanceID: "018bcfe5-687b-7000-8000-000000000001"},
	}
	index, found, err := NearestRoot("/src/project/internal/cli", indexes)
	if err != nil {
		t.Fatalf("NearestRoot() error = %v", err)
	}
	if !found || index.Root != "/src/project/internal" {
		t.Fatalf("NearestRoot() = (%+v, %t), want nearest internal root", index, found)
	}
}

func TestNearestRootRejectsPathPrefixConfusion(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{{SchemaVersion: 1, Root: "/src/project", InstanceID: "018bcfe5-687b-7000-8000-000000000000"}}
	_, found, err := NearestRoot("/src/project-other", indexes)
	if err != nil {
		t.Fatalf("NearestRoot() error = %v", err)
	}
	if found {
		t.Fatal("NearestRoot() matched a textual prefix outside the root")
	}
}

func TestProjectResourceNamesUseStableID(t *testing.T) {
	t.Parallel()
	container, network, err := ProjectResourceNames("018bcfe5-687b-7000-8000-000000000000")
	if err != nil {
		t.Fatalf("ProjectResourceNames() error = %v", err)
	}
	if container != "tobari-018bcfe5687b-work" || network != "tobari-018bcfe5687b-net" {
		t.Fatalf("ProjectResourceNames() = (%q, %q)", container, network)
	}
}
