package tobari

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func validState(root string) State {
	return State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ManifestCount: 1,
		PolicyDirectory: filepath.Join(root, "policy"),
		GatewayConfig:   filepath.Join(root, "gateway.json"), AssetVersion: "asset",
	}
}

func TestValidateImageSelectorRejectsOptionAndTransportSyntax(t *testing.T) {
	t.Parallel()
	for _, image := range []string{
		BuiltinImageSelector,
		"workbench:dev",
		"ghcr.io/example/workbench:1.2.3",
		"localhost:5000/example/workbench@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		if err := ValidateImageSelector(image); err != nil {
			t.Errorf("ValidateImageSelector(%q) = %v", image, err)
		}
	}
	for _, image := range []string{"", "--pull=always", "https://example.com/image", "UPPER/name", "name:bad tag"} {
		if err := ValidateImageSelector(image); err == nil {
			t.Errorf("ValidateImageSelector(%q) accepted invalid input", image)
		}
	}
}

func TestStateSchemasKeepOneExactAppliedIdentityShape(t *testing.T) {
	t.Parallel()
	legacy := validState(t.TempDir())
	if err := legacy.Validate(); err != nil {
		t.Fatal(err)
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(legacyJSON, []byte(`"applied"`)) {
		t.Fatalf("schema-1 write exposed successor applied key: %s", legacyJSON)
	}
	current := legacy
	current.SchemaVersion = 2
	current.Applied = SharedClusterAppliedEntry{
		AggregateRevision: current.AggregateRevision, AssetVersion: current.AssetVersion,
		GatewayImageID: "sha256:" + strings.Repeat("a", 64),
		OPAImageID:     "sha256:" + strings.Repeat("b", 64), PermissionProfile: SharedClusterProfileUnix,
	}
	if err := current.Validate(); err != nil {
		t.Fatal(err)
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(currentJSON, []byte(`"applied"`)) != 1 {
		t.Fatalf("schema-2 applied key count is not exact: %s", currentJSON)
	}
	var currentShape map[string]json.RawMessage
	if err := json.Unmarshal(currentJSON, &currentShape); err != nil {
		t.Fatal(err)
	}
	var appliedShape map[string]json.RawMessage
	if err := json.Unmarshal(currentShape["applied"], &appliedShape); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"aggregate_revision", "asset_version", "gateway_image_id", "opa_image_id", "permission_profile"} {
		if _, ok := appliedShape[key]; !ok {
			t.Fatalf("schema-2 applied object omits %q: %s", key, currentJSON)
		}
	}
	for _, invalid := range []State{
		func() State { value := legacy; value.Applied = current.Applied; return value }(),
		func() State { value := current; value.Applied.GatewayImageID = ""; return value }(),
		func() State { value := current; value.Applied.OPAImageID = ""; return value }(),
		func() State { value := current; value.Applied.PermissionProfile = "tcp"; return value }(),
		func() State { value := current; value.Applied.AssetVersion = "other"; return value }(),
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid state was accepted: %+v", invalid)
		}
	}
}

func TestSchemaOneReaderRejectsSchemaTwoAppliedIdentity(t *testing.T) {
	t.Parallel()
	state := validState(t.TempDir())
	state.SchemaVersion = 2
	state.Applied = SharedClusterAppliedEntry{
		AggregateRevision: state.AggregateRevision, AssetVersion: state.AssetVersion,
		GatewayImageID: "sha256:" + strings.Repeat("b", 64),
		OPAImageID:     "sha256:" + strings.Repeat("c", 64), PermissionProfile: SharedClusterProfileLoopbackTCP,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var predecessor struct {
		SchemaVersion         int    `json:"schema_version"`
		RuntimeDirectory      string `json:"runtime_directory"`
		WorkspaceManifestName string `json:"manifest_name,omitempty"`
		AgentProfile          string `json:"agent_profile,omitempty"`
		AggregateRevision     string `json:"aggregate_revision,omitempty"`
		ManifestCount         int    `json:"manifest_count,omitempty"`
		PolicyDirectory       string `json:"policy_directory"`
		GatewayConfig         string `json:"gateway_config"`
		AssetVersion          string `json:"asset_version"`
		RecentError           string `json:"recent_error"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&predecessor); err == nil {
		t.Fatal("strict schema-1 reader accepted schema-2 applied identity")
	}
}
