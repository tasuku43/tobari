// Command integrationfixture prepares repository-owned synthetic integration
// state that has no public product mutation workflow.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxFixtureDocumentBytes = 64 * 1024

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "integrationfixture:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "manifest-policy" {
		return fmt.Errorf("usage: integrationfixture manifest-policy --config-directory <path> --manifest <name> --graphql-endpoint <url>")
	}
	flags := flag.NewFlagSet("manifest-policy", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var configDirectory, manifestName, endpointURL string
	flags.StringVar(&configDirectory, "config-directory", "", "synthetic Tobari config directory")
	flags.StringVar(&manifestName, "manifest", "", "Workspace Manifest name")
	flags.StringVar(&endpointURL, "graphql-endpoint", "", "exact synthetic GraphQL endpoint")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(configDirectory) {
		return fmt.Errorf("config-directory must be absolute and no positional arguments are accepted")
	}
	if err := tobari.ValidateName(manifestName); err != nil {
		return err
	}
	endpoint, err := parseGraphQLEndpoint(endpointURL)
	if err != nil {
		return err
	}
	return publishManifestPolicyRevision(configDirectory, manifestName, endpoint)
}

func parseGraphQLEndpoint(raw string) (tobari.ManifestPolicyExactRule, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" || parsed.Port() == "" || parsed.EscapedPath() == "" {
		return tobari.ManifestPolicyExactRule{}, fmt.Errorf("GraphQL endpoint must be an exact https URL with an explicit port and path")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return tobari.ManifestPolicyExactRule{}, fmt.Errorf("GraphQL endpoint port is invalid")
	}
	endpoint := tobari.ManifestPolicyExactRule{
		Scheme: parsed.Scheme, Host: parsed.Hostname(), Port: port,
		Method: "POST", Path: parsed.EscapedPath(),
	}
	if err := endpoint.Validate(); err != nil {
		return tobari.ManifestPolicyExactRule{}, err
	}
	return endpoint, nil
}

func publishManifestPolicyRevision(
	configDirectory, manifestName string, endpoint tobari.ManifestPolicyExactRule,
) error {
	manifestDirectory := filepath.Join(configDirectory, "contexts", manifestName)
	manifestPath := filepath.Join(manifestDirectory, "context.json")
	policyPath := filepath.Join(manifestDirectory, "policy", "context.json")
	var previous tobari.WorkspaceManifest
	if err := readStrictJSON(manifestPath, &previous); err != nil {
		return fmt.Errorf("read published Workspace Manifest: %w", err)
	}
	if err := previous.ValidatePublished(); err != nil {
		return fmt.Errorf("validate published Workspace Manifest: %w", err)
	}
	var policy tobari.ManifestPolicy
	if err := readStrictJSON(policyPath, &policy); err != nil {
		return fmt.Errorf("read Workspace Manifest policy: %w", err)
	}
	_, currentPolicy, currentRevision, err := tobari.NormalizeContextPolicy(policy)
	if err != nil {
		return fmt.Errorf("validate Workspace Manifest policy: %w", err)
	}
	storedPolicy, err := os.ReadFile(policyPath) // #nosec G304 -- repository-only helper receives a synthetic test root.
	if err != nil {
		return err
	}
	if currentRevision != previous.PolicyRevision || !bytes.Equal(storedPolicy, currentPolicy) {
		return fmt.Errorf("current policy does not match the published Workspace Manifest")
	}
	policy.GraphQLEndpoints = append(policy.GraphQLEndpoints, endpoint)
	_, normalizedPolicy, policyRevision, err := tobari.NormalizeContextPolicy(policy)
	if err != nil {
		return fmt.Errorf("normalize fixture policy: %w", err)
	}
	draft := previous
	draft.PolicyRevision = policyRevision
	published, err := tobari.PublishWorkspaceManifest(draft, &previous)
	if err != nil {
		return fmt.Errorf("publish fixture Workspace Manifest revision: %w", err)
	}
	if published.Desired.Generation != previous.Desired.Generation+1 ||
		published.Desired.Revision == previous.Desired.Revision ||
		published.Desired.ClusterProjectionRevision == previous.Desired.ClusterProjectionRevision {
		return fmt.Errorf("fixture policy change did not produce a new complete desired revision")
	}
	receiptPath := filepath.Join(
		manifestDirectory, "revisions",
		fmt.Sprintf("%020d-%s.json", published.Desired.Generation, strings.TrimPrefix(published.Desired.Revision, "sha256:")),
	)
	if _, err := os.Lstat(receiptPath); err == nil {
		return fmt.Errorf("fixture retained revision already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeAtomicJSON(receiptPath, published, true); err != nil {
		return fmt.Errorf("retain fixture Workspace Manifest revision: %w", err)
	}
	// Readers fail closed between these writes: the new policy cannot match the
	// old current Manifest, and the new current Manifest is published last.
	if err := writeAtomicBytes(policyPath, normalizedPolicy, false); err != nil {
		return fmt.Errorf("publish fixture policy: %w", err)
	}
	if err := writeAtomicJSON(manifestPath, published, false); err != nil {
		return fmt.Errorf("publish fixture current Workspace Manifest: %w", err)
	}
	return nil
}

func readStrictJSON(path string, destination any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > maxFixtureDocumentBytes {
		return fmt.Errorf("fixture input is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- bounded owner-only synthetic fixture path.
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("fixture input contains trailing data")
	}
	return nil
}

func writeAtomicJSON(path string, value any, exclusive bool) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicBytes(path, append(data, '\n'), exclusive)
}

func writeAtomicBytes(path string, data []byte, exclusive bool) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".integrationfixture-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		return closeAfterFailure(temporary, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return closeAfterFailure(temporary, err)
	}
	if err := temporary.Sync(); err != nil {
		return closeAfterFailure(temporary, err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if exclusive {
		return os.Link(temporaryPath, path)
	}
	return os.Rename(temporaryPath, path)
}

func closeAfterFailure(temporary *os.File, failure error) error {
	return errors.Join(failure, temporary.Close())
}
