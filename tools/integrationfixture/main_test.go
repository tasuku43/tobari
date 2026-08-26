package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestCloseAfterFailurePreservesOperationAndCloseErrors(t *testing.T) {
	t.Parallel()
	temporary, err := os.CreateTemp(t.TempDir(), "close-after-failure-")
	if err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	operationErr := errors.New("operation failed")
	combined := closeAfterFailure(temporary, operationErr)
	if !errors.Is(combined, operationErr) || !errors.Is(combined, os.ErrClosed) {
		t.Fatalf("combined error = %v", combined)
	}
}

func TestPublishManifestPolicyRevisionRetainsCompleteAuthority(t *testing.T) {
	t.Parallel()
	configDirectory, previous := fixtureManifestStore(t)
	endpoint, err := parseGraphQLEndpoint("https://graphql.tobari.dev:8080/graphql")
	if err != nil {
		t.Fatal(err)
	}
	if err := publishManifestPolicyRevision(configDirectory, previous.Name, endpoint); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(configDirectory, "contexts", previous.Name, "context.json")
	var current tobari.WorkspaceManifest
	if err := readStrictJSON(manifestPath, &current); err != nil {
		t.Fatal(err)
	}
	if err := current.ValidatePublished(); err != nil {
		t.Fatalf("current Manifest is not a complete published revision: %v", err)
	}
	if current.ID != previous.ID || current.Desired.Generation != previous.Desired.Generation+1 ||
		current.Desired.BoundaryRevision != previous.Desired.BoundaryRevision ||
		current.Desired.ClusterProjectionRevision == previous.Desired.ClusterProjectionRevision {
		t.Fatalf("published identity transition = before %+v, after %+v", previous.Desired, current.Desired)
	}
	policyPath := filepath.Join(configDirectory, "contexts", previous.Name, "policy", "context.json")
	var policy tobari.ManifestPolicy
	if err := readStrictJSON(policyPath, &policy); err != nil {
		t.Fatal(err)
	}
	_, normalized, revision, err := tobari.NormalizeContextPolicy(policy)
	if err != nil || revision != current.PolicyRevision {
		t.Fatalf("policy revision = %q, Manifest = %q, error = %v", revision, current.PolicyRevision, err)
	}
	stored, err := os.ReadFile(policyPath)
	if err != nil || string(stored) != string(normalized) || len(policy.GraphQLEndpoints) != 1 || policy.GraphQLEndpoints[0] != endpoint {
		t.Fatalf("normalized policy was not published exactly: endpoints=%+v error=%v", policy.GraphQLEndpoints, err)
	}
	receipt := filepath.Join(
		configDirectory, "contexts", previous.Name, "revisions",
		"00000000000000000002-"+strings.TrimPrefix(current.Desired.Revision, "sha256:")+".json",
	)
	var retained tobari.WorkspaceManifest
	if err := readStrictJSON(receipt, &retained); err != nil {
		t.Fatalf("read retained revision: %v", err)
	}
	if retained.ID != current.ID || retained.Desired != current.Desired {
		t.Fatalf("retained revision = %+v, current = %+v", retained.Desired, current.Desired)
	}
}

func TestPublishManifestPolicyRevisionRejectsExistingReceipt(t *testing.T) {
	t.Parallel()
	configDirectory, previous := fixtureManifestStore(t)
	endpoint, err := parseGraphQLEndpoint("https://graphql.tobari.dev:8080/graphql")
	if err != nil {
		t.Fatal(err)
	}
	draft := previous
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default policy unavailable")
	}
	policy.GraphQLEndpoints = append(policy.GraphQLEndpoints, endpoint)
	_, _, revision, err := tobari.NormalizeContextPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	draft.PolicyRevision = revision
	published, err := tobari.PublishWorkspaceManifest(draft, &previous)
	if err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(
		configDirectory, "contexts", previous.Name, "revisions",
		"00000000000000000002-"+strings.TrimPrefix(published.Desired.Revision, "sha256:")+".json",
	)
	if err := os.WriteFile(receipt, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishManifestPolicyRevision(configDirectory, previous.Name, endpoint); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing retained identity was not rejected: %v", err)
	}
}

func fixtureManifestStore(t *testing.T) (string, tobari.WorkspaceManifest) {
	t.Helper()
	configDirectory := filepath.Join(t.TempDir(), "config", "tobari")
	manifestDirectory := filepath.Join(configDirectory, "contexts", "default")
	for _, directory := range []string{configDirectory, filepath.Join(configDirectory, "contexts"), manifestDirectory, filepath.Join(manifestDirectory, "policy"), filepath.Join(manifestDirectory, "revisions")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default policy unavailable")
	}
	_, policyBytes, policyRevision, err := tobari.NormalizeContextPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	binding := tobari.RuntimeBinding{
		RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
		Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "tobari-runtime:test",
	}
	manifest, err := tobari.PublishWorkspaceManifest(tobari.WorkspaceManifest{
		SchemaVersion: tobari.WorkspaceManifestSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000", Name: "default",
		AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector,
		SourceAccess:   tobari.ManifestSourceAccessReadWrite,
		PolicyRevision: policyRevision, RuntimeBinding: &binding,
		ShellEnvironment: tobari.InitialContextShellEnvironment(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicBytes(filepath.Join(manifestDirectory, "policy", "context.json"), policyBytes, false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(filepath.Join(manifestDirectory, "context.json"), manifest, false); err != nil {
		t.Fatal(err)
	}
	return configDirectory, manifest
}
