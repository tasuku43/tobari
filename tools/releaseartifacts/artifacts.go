package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/tasuku43/tobari/tools/internal/projectconfig"
)

const (
	checksumsName     = "checksums.txt"
	sbomName          = "sbom.spdx.json"
	provenanceName    = "provenance.intoto.jsonl"
	componentLockName = "component-lock.json"

	canonicalCreated = "1980-01-01T00:00:00Z"
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type artifactRequest struct {
	Tag          string
	Version      string
	Revision     string
	BuilderID    string
	InvocationID string
	Directory    string
	Project      projectconfig.Project
	LicenseSPDX  string
	Stable       bool
}

type releaseSubject struct {
	Name   string
	Digest string
}

type generatedFile struct {
	Name string
	Data []byte
}

type spdxDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      spdxCreationInfo   `json:"creationInfo"`
	Packages          []spdxPackage      `json:"packages"`
	Relationships     []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
	Comment  string   `json:"comment"`
}

type spdxPackage struct {
	Name                  string         `json:"name"`
	SPDXID                string         `json:"SPDXID"`
	VersionInfo           string         `json:"versionInfo"`
	DownloadLocation      string         `json:"downloadLocation"`
	FilesAnalyzed         bool           `json:"filesAnalyzed"`
	Checksums             []spdxChecksum `json:"checksums"`
	LicenseConcluded      string         `json:"licenseConcluded"`
	LicenseDeclared       string         `json:"licenseDeclared"`
	CopyrightText         string         `json:"copyrightText"`
	PrimaryPackagePurpose string         `json:"primaryPackagePurpose"`
}

type spdxChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type spdxRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

type provenanceStatement struct {
	Type          string              `json:"_type"`
	Subject       []provenanceSubject `json:"subject"`
	PredicateType string              `json:"predicateType"`
	Predicate     provenancePredicate `json:"predicate"`
}

type provenanceSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type provenancePredicate struct {
	BuildDefinition provenanceBuildDefinition `json:"buildDefinition"`
	RunDetails      provenanceRunDetails      `json:"runDetails"`
}

type provenanceBuildDefinition struct {
	BuildType            string                       `json:"buildType"`
	ExternalParameters   provenanceExternalParameters `json:"externalParameters"`
	InternalParameters   map[string]any               `json:"internalParameters"`
	ResolvedDependencies []provenanceDependency       `json:"resolvedDependencies"`
}

type provenanceExternalParameters struct {
	Tag     string   `json:"tag"`
	Targets []string `json:"targets"`
}

type provenanceDependency struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest"`
}

type provenanceRunDetails struct {
	Builder  provenanceBuilder  `json:"builder"`
	Metadata provenanceMetadata `json:"metadata"`
}

type provenanceBuilder struct {
	ID string `json:"id"`
}

type provenanceMetadata struct {
	InvocationID string `json:"invocationId"`
}

func createReleaseMetadata(request artifactRequest) error {
	for _, name := range []string{checksumsName, provenanceName, sbomName} {
		path := filepath.Join(request.Directory, name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("release metadata already exists; refusing to overwrite it: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect release metadata %s: %w", name, err)
		}
	}
	if err := validateAssetInventory(request.Directory, expectedSubjectNames(request.Project.BinaryName, request.Tag)); err != nil {
		return err
	}
	files, err := expectedMetadata(request)
	if err != nil {
		return err
	}
	return createFilesWithoutOverwrite(request.Directory, files)
}

func verifyReleaseMetadata(request artifactRequest) error {
	expectedNames := expectedSubjectNames(request.Project.BinaryName, request.Tag)
	expectedNames = append(expectedNames, checksumsName, provenanceName, sbomName)
	if err := validateAssetInventory(request.Directory, expectedNames); err != nil {
		return err
	}
	return verifyReleaseMetadataFiles(request)
}

func verifyFinalReleaseAssets(request artifactRequest) error {
	if err := verifyReleaseMetadataFiles(request); err != nil {
		return err
	}
	expectedNames := expectedSubjectNames(request.Project.BinaryName, request.Tag)
	expectedNames = append(expectedNames, checksumsName, provenanceName, sbomName)
	if request.Stable {
		expectedNames = append(expectedNames, request.Project.BinaryName+".rb")
	}
	return validateAssetInventory(request.Directory, expectedNames)
}

func verifyReleaseMetadataFiles(request artifactRequest) error {
	files, err := expectedMetadata(request)
	if err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(request.Directory, file.Name)
		actual, err := readRegularFile(path, "release metadata "+file.Name)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, file.Data) {
			return fmt.Errorf("release metadata %s does not match the exact archive subjects and build parameters", file.Name)
		}
	}
	return nil
}

func expectedMetadata(request artifactRequest) ([]generatedFile, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	subjects, err := loadSubjects(request)
	if err != nil {
		return nil, err
	}
	checksums := make([]byte, 0, len(subjects)*128)
	for _, subject := range subjects {
		checksums = fmt.Appendf(checksums, "%s  %s\n", subject.Digest, subject.Name)
	}
	sbom, err := marshalSPDX(request, subjects)
	if err != nil {
		return nil, err
	}
	provenance, err := marshalProvenance(request, subjects)
	if err != nil {
		return nil, err
	}
	return []generatedFile{
		{Name: checksumsName, Data: checksums},
		{Name: provenanceName, Data: provenance},
		{Name: sbomName, Data: sbom},
	}, nil
}

func validateRequest(request artifactRequest) error {
	if request.Tag != "v"+request.Version || request.Version == "" {
		return errors.New("release tag and version are inconsistent")
	}
	if !revisionPattern.MatchString(request.Revision) {
		return errors.New("revision must be a full lowercase Git commit SHA")
	}
	for _, identity := range []struct {
		label string
		value string
	}{{label: "builder ID", value: request.BuilderID}, {label: "invocation ID", value: request.InvocationID}} {
		parsed, err := url.ParseRequestURI(identity.value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("%s must be an absolute credential-free HTTPS URI without a fragment", identity.label)
		}
	}
	if request.Project.BinaryName == "" || request.Project.GitHubOwner == "" || request.Project.GitHubRepository == "" || request.LicenseSPDX == "" {
		return errors.New("validated project metadata is incomplete")
	}
	info, err := os.Lstat(request.Directory)
	if err != nil {
		return fmt.Errorf("inspect artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("artifact directory must be a directory reached without a symbolic link")
	}
	return nil
}

func loadSubjects(request artifactRequest) ([]releaseSubject, error) {
	names := expectedSubjectNames(request.Project.BinaryName, request.Tag)
	subjects := make([]releaseSubject, 0, len(names))
	for _, name := range names {
		data, err := readRegularFile(filepath.Join(request.Directory, name), "release archive "+name)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		subjects = append(subjects, releaseSubject{Name: name, Digest: hex.EncodeToString(digest[:])})
	}
	return subjects, nil
}

func expectedSubjectNames(binary, tag string) []string {
	names := append(expectedArchiveNames(binary, tag), componentLockName)
	sort.Strings(names)
	return names
}

func expectedArchiveNames(binary, tag string) []string {
	names := []string{
		fmt.Sprintf("%s_%s_darwin_amd64.tar.gz", binary, tag),
		fmt.Sprintf("%s_%s_darwin_arm64.tar.gz", binary, tag),
		fmt.Sprintf("%s_%s_linux_amd64.tar.gz", binary, tag),
		fmt.Sprintf("%s_%s_linux_arm64.tar.gz", binary, tag),
		fmt.Sprintf("%s_%s_windows_amd64.zip", binary, tag),
	}
	sort.Strings(names)
	return names
}

func marshalSPDX(request artifactRequest, subjects []releaseSubject) ([]byte, error) {
	repositoryURL := projectRepositoryURL(request.Project)
	packages := make([]spdxPackage, 0, len(subjects))
	relationships := make([]spdxRelationship, 0, len(subjects))
	for index, subject := range subjects {
		identifier := fmt.Sprintf("SPDXRef-Archive-%d", index+1)
		packages = append(packages, spdxPackage{
			Name:                  subject.Name,
			SPDXID:                identifier,
			VersionInfo:           request.Version,
			DownloadLocation:      fmt.Sprintf("%s/releases/download/%s/%s", repositoryURL, request.Tag, subject.Name),
			FilesAnalyzed:         false,
			Checksums:             []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: subject.Digest}},
			LicenseConcluded:      "NOASSERTION",
			LicenseDeclared:       request.LicenseSPDX,
			CopyrightText:         "NOASSERTION",
			PrimaryPackagePurpose: "APPLICATION",
		})
		relationships = append(relationships, spdxRelationship{
			SPDXElementID:      "SPDXRef-DOCUMENT",
			RelationshipType:   "DESCRIBES",
			RelatedSPDXElement: identifier,
		})
	}
	document := spdxDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              fmt.Sprintf("%s %s CLI release archives", request.Project.Name, request.Tag),
		DocumentNamespace: fmt.Sprintf("%s/releases/tag/%s/sbom/%s", repositoryURL, request.Tag, request.Revision),
		CreationInfo: spdxCreationInfo{
			Created:  canonicalCreated,
			Creators: []string{"Tool: " + request.Project.GoModule + "/tools/releaseartifacts"},
			Comment:  "The creation timestamp is normalized to the release archive epoch for deterministic repository generation; it is not the publication time.",
		},
		Packages:      packages,
		Relationships: relationships,
	}
	return marshalIndented(document)
}

func marshalProvenance(request artifactRequest, subjects []releaseSubject) ([]byte, error) {
	provenanceSubjects := make([]provenanceSubject, 0, len(subjects))
	targets := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		provenanceSubjects = append(provenanceSubjects, provenanceSubject{
			Name:   subject.Name,
			Digest: map[string]string{"sha256": subject.Digest},
		})
		targets = append(targets, subject.Name)
	}
	repositoryURL := projectRepositoryURL(request.Project)
	statement := provenanceStatement{
		Type:          "https://in-toto.io/Statement/v1",
		Subject:       provenanceSubjects,
		PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: provenancePredicate{
			BuildDefinition: provenanceBuildDefinition{
				BuildType:          fmt.Sprintf("%s/blob/%s/tools/releaseartifacts#cli-archive-matrix-v1", repositoryURL, request.Revision),
				ExternalParameters: provenanceExternalParameters{Tag: request.Tag, Targets: targets},
				InternalParameters: map[string]any{},
				ResolvedDependencies: []provenanceDependency{{
					URI:    "git+" + repositoryURL + "@" + request.Revision,
					Digest: map[string]string{"gitCommit": request.Revision},
				}},
			},
			RunDetails: provenanceRunDetails{
				Builder:  provenanceBuilder{ID: request.BuilderID},
				Metadata: provenanceMetadata{InvocationID: request.InvocationID},
			},
		},
	}
	data, err := json.Marshal(statement)
	if err != nil {
		return nil, fmt.Errorf("marshal provenance: %w", err)
	}
	return append(data, '\n'), nil
}

func projectRepositoryURL(project projectconfig.Project) string {
	return "https://github.com/" + project.GitHubOwner + "/" + project.GitHubRepository
}

func marshalIndented(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func readRegularFile(path, description string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file reached without a symbolic link", description)
	}
	file, err := os.Open(path) // #nosec G304 G703 -- releaseartifacts intentionally reads one exact caller-selected artifact directory after entry-name validation.
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", description, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened %s: %w", description, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%s changed while it was being opened", description)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", description, err)
	}
	return data, nil
}

func validateAssetInventory(directory string, expected []string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read release asset inventory: %w", err)
	}
	want := append([]string(nil), expected...)
	sort.Strings(want)
	actual := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect release asset %s: %w", entry.Name(), err)
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("release asset %s must be a regular file reached without a symbolic link", entry.Name())
		}
		actual = append(actual, entry.Name())
	}
	sort.Strings(actual)
	if len(actual) != len(want) {
		return fmt.Errorf("release asset inventory = %v, want %v", actual, want)
	}
	for index := range want {
		if actual[index] != want[index] {
			return fmt.Errorf("release asset inventory = %v, want %v", actual, want)
		}
	}
	return nil
}

func createFilesWithoutOverwrite(directory string, files []generatedFile) (returnErr error) {
	type stagedFile struct {
		path string
		file *os.File
		info os.FileInfo
	}
	staged := make([]stagedFile, 0, len(files))
	created := make([]stagedFile, 0, len(files))
	defer func() {
		for _, file := range staged {
			_ = file.file.Close()
			_ = os.Remove(file.path)
		}
		if returnErr == nil {
			return
		}
		for _, file := range created {
			current, err := os.Lstat(file.path)
			if err == nil && current.Mode().IsRegular() && os.SameFile(current, file.info) {
				_ = os.Remove(file.path)
			}
		}
	}()

	for _, output := range files {
		file, err := os.CreateTemp(directory, ".release-artifacts.*")
		if err != nil {
			return fmt.Errorf("create staged %s: %w", output.Name, err)
		}
		stage := stagedFile{path: file.Name(), file: file}
		staged = append(staged, stage)
		if err := file.Chmod(0o644); err != nil {
			return fmt.Errorf("set staged %s mode: %w", output.Name, err)
		}
		if _, err := file.Write(output.Data); err != nil {
			return fmt.Errorf("write staged %s: %w", output.Name, err)
		}
		if err := file.Sync(); err != nil {
			return fmt.Errorf("sync staged %s: %w", output.Name, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close staged %s: %w", output.Name, err)
		}
		info, err := os.Lstat(file.Name())
		if err != nil {
			return fmt.Errorf("inspect staged %s: %w", output.Name, err)
		}
		staged[len(staged)-1].info = info
	}

	for index, output := range files {
		target := filepath.Join(directory, output.Name)
		if err := os.Link(staged[index].path, target); err != nil {
			return fmt.Errorf("create %s without overwrite: %w", output.Name, err)
		}
		info, err := os.Lstat(target)
		if err != nil {
			return fmt.Errorf("inspect created %s: %w", output.Name, err)
		}
		created = append(created, stagedFile{path: target, info: info})
	}
	return nil
}
