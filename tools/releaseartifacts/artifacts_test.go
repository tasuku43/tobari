package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/tools/internal/projectconfig"
)

func TestCreateAndVerifyReleaseMetadata(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)

	if err := createReleaseMetadata(request); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseMetadata(request); err != nil {
		t.Fatal(err)
	}

	checksums := readTestArtifact(t, request.Directory, checksumsName)
	lines := strings.Split(strings.TrimSuffix(string(checksums), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("checksum records = %d, want 5", len(lines))
	}
	for index, name := range expectedArchiveNames(request.Project.BinaryName, request.Tag) {
		if !strings.HasSuffix(lines[index], "  "+name) {
			t.Fatalf("checksum line %d = %q, want subject %q", index, lines[index], name)
		}
	}

	var sbom spdxDocument
	if err := json.Unmarshal(readTestArtifact(t, request.Directory, sbomName), &sbom); err != nil {
		t.Fatal(err)
	}
	if sbom.SPDXVersion != "SPDX-2.3" || len(sbom.Packages) != 5 || len(sbom.Relationships) != 5 {
		t.Fatalf("SPDX shape = version %q, packages %d, relationships %d", sbom.SPDXVersion, len(sbom.Packages), len(sbom.Relationships))
	}
	for index, archive := range expectedArchiveNames(request.Project.BinaryName, request.Tag) {
		if sbom.Packages[index].Name != archive || len(sbom.Packages[index].Checksums) != 1 {
			t.Fatalf("SPDX package %d = %#v", index, sbom.Packages[index])
		}
	}

	var provenance provenanceStatement
	if err := json.Unmarshal(readTestArtifact(t, request.Directory, provenanceName), &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.Type != "https://in-toto.io/Statement/v1" || provenance.PredicateType != "https://slsa.dev/provenance/v1" || len(provenance.Subject) != 5 {
		t.Fatalf("provenance shape = %#v", provenance)
	}
	if provenance.Predicate.RunDetails.Builder.ID != request.BuilderID || provenance.Predicate.RunDetails.Metadata.InvocationID != request.InvocationID {
		t.Fatalf("provenance builder = %#v", provenance.Predicate.RunDetails)
	}
	formula := filepath.Join(request.Directory, request.Project.BinaryName+".rb")
	if err := os.WriteFile(formula, []byte("class Tobari < Formula\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyFinalReleaseAssets(request); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRefusesCollisionWithoutChangingExistingFile(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	sentinel := []byte("existing metadata\n")
	if err := os.WriteFile(filepath.Join(request.Directory, checksumsName), sentinel, 0o644); err != nil {
		t.Fatal(err)
	}

	err := createReleaseMetadata(request)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("createReleaseMetadata() error = %v, want collision rejection", err)
	}
	if actual := readTestArtifact(t, request.Directory, checksumsName); !bytes.Equal(actual, sentinel) {
		t.Fatalf("collision changed existing metadata: %q", actual)
	}
	for _, name := range []string{sbomName, provenanceName} {
		if _, err := os.Lstat(filepath.Join(request.Directory, name)); !os.IsNotExist(err) {
			t.Fatalf("collision created %s: %v", name, err)
		}
	}
}

func TestVerifyRejectsChangedArchiveAndMetadata(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	if err := createReleaseMetadata(request); err != nil {
		t.Fatal(err)
	}

	archive := expectedArchiveNames(request.Project.BinaryName, request.Tag)[0]
	if err := os.WriteFile(filepath.Join(request.Directory, archive), []byte("changed archive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseMetadata(request); err == nil || !strings.Contains(err.Error(), checksumsName) {
		t.Fatalf("verifyReleaseMetadata() after archive change = %v", err)
	}

	writeTestArchives(t, request)
	sbomPath := filepath.Join(request.Directory, sbomName)
	sbom := readTestArtifact(t, request.Directory, sbomName)
	sbom = bytes.Replace(sbom, []byte("SPDX-2.3"), []byte("SPDX-2.2"), 1)
	if err := os.WriteFile(sbomPath, sbom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseMetadata(request); err == nil || !strings.Contains(err.Error(), sbomName) {
		t.Fatalf("verifyReleaseMetadata() after SBOM change = %v", err)
	}
}

func TestVerifyRejectsMismatchedBuilderAndInvocation(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	if err := createReleaseMetadata(request); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*artifactRequest){
		"builder":    func(value *artifactRequest) { value.BuilderID = "https://example.com/builders/other/v1" },
		"invocation": func(value *artifactRequest) { value.InvocationID = "https://example.com/invocations/other" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := verifyReleaseMetadata(candidate); err == nil || !strings.Contains(err.Error(), provenanceName) {
				t.Fatalf("verifyReleaseMetadata() error = %v, want provenance identity mismatch", err)
			}
		})
	}
}

func TestVerifyRejectsUnexpectedReleaseAsset(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	if err := createReleaseMetadata(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.Directory, "unexpected.log"), []byte("not a release asset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseMetadata(request); err == nil || !strings.Contains(err.Error(), "inventory") {
		t.Fatalf("verifyReleaseMetadata() error = %v, want exact-inventory rejection", err)
	}
}

func TestReleaseMetadataRejectsSymlinkArchive(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	names := expectedArchiveNames(request.Project.BinaryName, request.Tag)
	target := filepath.Join(request.Directory, names[1])
	link := filepath.Join(request.Directory, names[0])
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := createReleaseMetadata(request); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("createReleaseMetadata() error = %v, want symbolic-link rejection", err)
	}
}

func TestReleaseMetadataRejectsUnsafeRequest(t *testing.T) {
	request := testArtifactRequest(t)
	writeTestArchives(t, request)
	for name, mutate := range map[string]func(*artifactRequest){
		"revision":   func(value *artifactRequest) { value.Revision = "main" },
		"builder":    func(value *artifactRequest) { value.BuilderID = "file:///tmp/builder" },
		"invocation": func(value *artifactRequest) { value.InvocationID = "file:///tmp/run" },
		"tag":        func(value *artifactRequest) { value.Tag = "v9.9.9" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if _, err := expectedMetadata(candidate); err == nil {
				t.Fatal("expectedMetadata() accepted unsafe request")
			}
		})
	}
}

func testArtifactRequest(t *testing.T) artifactRequest {
	t.Helper()
	return artifactRequest{
		Tag:          "v1.2.3",
		Version:      "1.2.3",
		Revision:     strings.Repeat("a", 40),
		BuilderID:    "https://example.com/builders/tobari-release/v1",
		InvocationID: "https://example.com/invocations/release-fixture-1",
		Directory:    t.TempDir(),
		LicenseSPDX:  "MIT",
		Stable:       true,
		Project: projectconfig.Project{
			Name:             "Tobari",
			BinaryName:       "tobari",
			GoModule:         "github.com/example/tobari",
			GitHubOwner:      "example",
			GitHubRepository: "tobari",
		},
	}
}

func writeTestArchives(t *testing.T, request artifactRequest) {
	t.Helper()
	for index, name := range expectedArchiveNames(request.Project.BinaryName, request.Tag) {
		contents := []byte(strings.Repeat(string(rune('a'+index)), index+1) + "\n")
		if err := os.WriteFile(filepath.Join(request.Directory, name), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readTestArtifact(t *testing.T, directory, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
