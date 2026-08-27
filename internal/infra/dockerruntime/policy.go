package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

var aggregateRevisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const policyPreflightVolumeCleanupTimeout = 15 * time.Second

type gatewayAuditRecord struct {
	tobari.PolicyProtocolIdentity
	SchemaVersion int    `json:"schema_version"`
	Timestamp     string `json:"timestamp"`
	RequestID     string `json:"request_id"`
	Cluster       string `json:"cluster"`
	// Gateway audit schema v1 predates the public Workspace Manifest model.
	// Decode its exact compatibility tokens here, then project current domain
	// names in PolicyDenial and every public result.
	ProjectID             *string `json:"project_id"`
	WorkspaceManifestID   *string `json:"context_id"`
	WorkspaceManifestName *string `json:"context"`
	ProjectRoot           *string `json:"project_root"`
	Host                  string  `json:"host"`
	Port                  int     `json:"port"`
	Method                string  `json:"method"`
	Path                  string  `json:"path"`
	Decision              string  `json:"decision"`
	Reason                string  `json:"reason"`
	Learnable             bool    `json:"learnable"`
	UpstreamStatus        int     `json:"upstream_status"`
	DurationMS            int     `json:"duration_ms"`
}

// ClusterDenials projects only validated deny audit records from one bounded
// Gateway log window.
func (r *Runtime) ClusterDenials(
	ctx context.Context, state tobari.State, tail int,
) (tobari.DenialRead, error) {
	if err := state.Validate(); err != nil {
		return tobari.DenialRead{}, err
	}
	return r.ReadFinalClusterDenials(ctx, tail)
}

// ReadFinalClusterDenials reads the bounded Gateway audit window without
// loading predecessor installation state. The final Store adapter correlates
// every item to the selected complete authority envelope.
func (r *Runtime) ReadFinalClusterDenials(ctx context.Context, tail int) (tobari.DenialRead, error) {
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return tobari.DenialRead{}, err
	}
	data, err := r.runner.Output(
		ctx, []string{"logs", "--tail", strconv.Itoa(tail), gatewayContainer}, os.Environ(),
	)
	if err != nil {
		return tobari.DenialRead{}, fmt.Errorf("read Gateway logs: %w: %s", err, boundedDiagnostic(data))
	}
	if len(data) > maxLogBytes {
		return tobari.DenialRead{}, fmt.Errorf("Gateway log output exceeds %d bytes", maxLogBytes)
	}
	result := parseGatewayDenials(data)
	items, err := r.bindActiveHostLoopbackDenials(result.Items)
	if err != nil {
		return tobari.DenialRead{}, err
	}
	result.Items = items
	if err := result.Validate(); err != nil {
		return tobari.DenialRead{}, err
	}
	return result, nil
}

func (r *Runtime) bindActiveHostLoopbackDenials(items []tobari.PolicyDenial) ([]tobari.PolicyDenial, error) {
	needsRegistry := false
	for _, item := range items {
		needsRegistry = needsRegistry || item.Host == tobari.HostLoopbackHostname
	}
	if !needsRegistry {
		return items, nil
	}
	var registry tobari.HostLoopbackRegistry
	if err := readStrictJSON(r.hostLoopbackRegistryPath(), &registry); err != nil {
		return nil, fmt.Errorf("read active Host Loopback registry: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	bound := make([]tobari.PolicyDenial, 0, len(items))
	for index := range items {
		if items[index].Host != tobari.HostLoopbackHostname {
			bound = append(bound, items[index])
			continue
		}
		for _, route := range registry.Routes {
			if route.WorkspaceID == items[index].ProjectID && route.ContextID == items[index].WorkspaceManifestID {
				items[index].DestinationKind = tobari.PolicyDestinationHostLoopback
				items[index].AuthorityLifetime = tobari.AuthorityLifetimeAttachment
				items[index].AttachmentEpochID = route.EpochID
				items[index].Learnable = true
				break
			}
		}
		if items[index].AttachmentEpochID == "" {
			continue
		}
		if err := items[index].Validate(); err != nil {
			return nil, err
		}
		bound = append(bound, items[index])
	}
	return bound, nil
}

func parseGatewayDenials(data []byte) tobari.DenialRead {
	result := tobari.DenialRead{Items: make([]tobari.PolicyDenial, 0)}
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var probe struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(line, &probe); err != nil || probe.Decision != "deny" {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var record gatewayAuditRecord
		if err := decoder.Decode(&record); err != nil {
			result.UnparsedLines++
			continue
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			result.UnparsedLines++
			continue
		}
		if record.SchemaVersion != 1 || record.Cluster != ownerValue || record.Decision != "deny" || record.DurationMS < 0 {
			result.UnparsedLines++
			continue
		}
		item := tobari.PolicyDenial{
			PolicyProtocolIdentity: record.PolicyProtocolIdentity,
			Timestamp:              record.Timestamp, RequestID: record.RequestID,
			WorkspaceManifestID: nullableAuditString(record.WorkspaceManifestID), WorkspaceManifestName: nullableAuditString(record.WorkspaceManifestName),
			ProjectID: nullableAuditString(record.ProjectID), ProjectRoot: nullableAuditString(record.ProjectRoot),
			Host: record.Host, Port: record.Port, Method: record.Method, Path: record.Path,
			Reason: record.Reason, StatusCode: record.UpstreamStatus,
			Learnable: record.Learnable,
		}
		if err := item.Validate(); err != nil {
			result.UnparsedLines++
			continue
		}
		result.Items = append(result.Items, item)
	}
	return result
}

func nullableAuditString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *Runtime) ensurePolicyBundleVolume(ctx context.Context) error {
	_, err := r.ensurePolicyBundleVolumeWithCreation(ctx)
	return err
}

func (r *Runtime) ensurePolicyBundleVolumeWithCreation(ctx context.Context) (bool, error) {
	err := r.verifyOwned(ctx, "volume", policyBundleVolume)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, errOwnedResourceMissing) {
		return false, err
	}
	output, createErr := r.runner.Output(ctx, []string{
		"volume", "create",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=opa-policy",
		policyBundleVolume,
	}, os.Environ())
	if createErr != nil {
		return false, fmt.Errorf("create policy bundle volume: %w: %s", createErr, boundedDiagnostic(output))
	}
	if err := r.verifyOwned(ctx, "volume", policyBundleVolume); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) removeCreatedPolicyBundleVolume(ctx context.Context) error {
	if err := r.verifyOwned(ctx, "volume", policyBundleVolume); err != nil {
		return fmt.Errorf("verify preflight-created policy bundle volume: %w", err)
	}
	output, err := r.runner.Output(ctx, []string{"volume", "rm", policyBundleVolume}, os.Environ())
	if err != nil {
		return fmt.Errorf("remove preflight-created policy bundle volume: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) publishPolicyBundle(ctx context.Context, state tobari.State) error {
	if _, _, err := r.verifyPersistedAggregateState(ctx, state); err != nil {
		return err
	}
	return r.publishPolicyBundleTarget(ctx, state.PolicyDirectory, state.AggregateRevision, true)
}

func (r *Runtime) publishKnownGoodPolicyBundle(ctx context.Context, state tobari.State) error {
	if err := r.verifyKnownGoodAggregateState(ctx, state); err != nil {
		return err
	}
	return r.publishPolicyBundleTargetWithVerifier(
		ctx, state.PolicyDirectory, state.AggregateRevision,
		func() error { return r.verifyKnownGoodAggregateState(ctx, state) },
	)
}

func (r *Runtime) publishPolicyBundleTarget(
	ctx context.Context, policyDirectory, aggregateRevision string, verifyAggregate bool,
) error {
	var verify func() error
	if verifyAggregate {
		verify = func() error {
			return r.verifyAggregateTargetForRevision(ctx, policyDirectory, aggregateRevision)
		}
	}
	return r.publishPolicyBundleTargetWithVerifier(ctx, policyDirectory, aggregateRevision, verify)
}

func (r *Runtime) publishPolicyBundleTargetWithVerifier(
	ctx context.Context, policyDirectory, aggregateRevision string, verify func() error,
) error {
	if !aggregateRevisionPattern.MatchString(aggregateRevision) || !filepath.IsAbs(policyDirectory) || filepath.Clean(policyDirectory) != policyDirectory {
		return fmt.Errorf("policy bundle target is invalid")
	}
	if verify != nil {
		if err := verify(); err != nil {
			return fmt.Errorf("verify aggregate policy immediately before bundle assembly: %w", err)
		}
	}
	if r.policyBeforeBundleAssembly != nil {
		r.policyBeforeBundleAssembly()
	}
	archive, err := aggregatePolicyBundleArchive(policyDirectory)
	if err != nil {
		return fmt.Errorf("assemble fixed policy bundle: %w", err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return fmt.Errorf("verify aggregate policy immediately before staging: %w", err)
		}
		archivedData, err := policyBundleArchiveFile(archive, "data.json")
		if err != nil {
			return fmt.Errorf("read assembled aggregate data: %w", err)
		}
		currentData, err := readOwnerPolicyFile(filepath.Join(policyDirectory, "data.json"), maxPolicyPreflight)
		if err != nil {
			return fmt.Errorf("read aggregate data immediately before staging: %w", err)
		}
		if !bytes.Equal(archivedData, currentData) {
			return fmt.Errorf("aggregate data changed after verification and before staging")
		}
	}
	return r.publishPolicyArchiveTarget(ctx, archive, aggregateRevision)
}

func (r *Runtime) publishPolicyArchiveTarget(ctx context.Context, archive []byte, aggregateRevision string) error {
	if len(archive) == 0 || !aggregateRevisionPattern.MatchString(aggregateRevision) {
		return fmt.Errorf("policy bundle archive target is invalid")
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return fmt.Errorf("read embedded runtime versions: %w", err)
	}
	source := "/bundle/.source-" + aggregateRevision
	candidate := "/bundle/.candidate-" + aggregateRevision + ".tar.gz"
	var stageOutput bytes.Buffer
	err = r.stagePolicyArchive(ctx, versions["DEBIAN_IMAGE"], source, archive)
	if err != nil {
		return fmt.Errorf("stage tested policy source: %w: %s", err, boundedDiagnostic(stageOutput.Bytes()))
	}
	output, err := r.runner.Output(ctx, []string{
		"run", "--rm", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--tmpfs", "/tmp:size=16m,mode=1777",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		versions["OPA_IMAGE"], "build", "-b", source,
		"-o", candidate, "--revision", aggregateRevision,
	}, os.Environ())
	if err != nil {
		cleanupOutput, cleanupErr := r.removeStagedPolicySource(ctx, versions["DEBIAN_IMAGE"], source)
		if cleanupErr != nil {
			return fmt.Errorf("build tested policy bundle: %w: %s; clean staged source: %v: %s", err, boundedDiagnostic(output), cleanupErr, boundedDiagnostic(cleanupOutput))
		}
		return fmt.Errorf("build tested policy bundle: %w: %s", err, boundedDiagnostic(output))
	}
	output, err = r.runner.Output(ctx, []string{
		"run", "--rm", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		"--entrypoint", "sh", versions["DEBIAN_IMAGE"], "-eu", "-c",
		`rm -rf -- "$3" && mv -f -- "$1" "$2"`, "tobari-policy-publish", candidate, "/bundle/bundle.tar.gz", source,
	}, os.Environ())
	if err != nil {
		return fmt.Errorf("atomically publish tested policy bundle: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

type policyBundleFile struct {
	name    string
	content []byte
}

// policyBundleArchive creates the only archive form accepted by the Docker
// bundle boundary. It is assembled in memory, so executable evaluator bytes
// never acquire a host-XDG pathname. Callers provide a fixed, allowlisted file
// set; the archive order and headers are deterministic for audit/digest tests.
func policyBundleArchive(files []policyBundleFile) ([]byte, error) {
	if len(files) == 0 || len(files) > 8 {
		return nil, fmt.Errorf("policy bundle has an invalid file count")
	}
	seen := make(map[string]struct{}, len(files))
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, file := range files {
		if file.name == "" || filepath.Base(file.name) != file.name || strings.ContainsAny(file.name, `/\\`) {
			return nil, fmt.Errorf("policy bundle file name %q is unsafe", file.name)
		}
		if _, ok := seen[file.name]; ok {
			return nil, fmt.Errorf("policy bundle contains duplicate file %q", file.name)
		}
		seen[file.name] = struct{}{}
		if len(file.content) > maxPolicyPreflight {
			return nil, fmt.Errorf("policy bundle file %q is too large", file.name)
		}
		header := &tar.Header{
			Name: file.name, Mode: 0o600, Size: int64(len(file.content)), Typeflag: tar.TypeReg,
			Uid: 0, Gid: 0,
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write(file.content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

func policyBundleArchiveFile(archive []byte, name string) ([]byte, error) {
	reader := tar.NewReader(bytes.NewReader(archive))
	var found []byte
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		if header.Name == name {
			if found != nil {
				return nil, fmt.Errorf("policy bundle contains duplicate file %q", name)
			}
			found = contents
		}
	}
	if found == nil {
		return nil, fmt.Errorf("policy bundle omits %q", name)
	}
	return found, nil
}

func aggregatePolicyBundleArchive(policyDirectory string) ([]byte, error) {
	data, err := readOwnerPolicyFile(filepath.Join(policyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil {
		return nil, err
	}
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		return nil, err
	}
	return policyBundleArchive([]policyBundleFile{
		{name: "router.rego", content: router},
		{name: "guided.rego", content: module},
		{name: "data.json", content: data},
	})
}

func (r *Runtime) stagePolicyArchive(ctx context.Context, image, source string, archive []byte) error {
	if image == "" || source == "" {
		return fmt.Errorf("policy bundle staging target is invalid")
	}
	var stageOutput bytes.Buffer
	err := r.runner.Run(ctx, []string{
		"run", "--rm", "--interactive", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		"--entrypoint", "sh", image, "-eu", "-c",
		`rm -rf -- "$1" && mkdir -m 0700 -- "$1" && tar --extract --file - --directory "$1" --no-same-owner && chmod -R u=rwX,go= -- "$1"`,
		"tobari-policy-stage", source,
	}, os.Environ(), bytes.NewReader(archive), &stageOutput, &stageOutput)
	if err != nil {
		return fmt.Errorf("stage policy bundle: %w: %s", err, boundedDiagnostic(stageOutput.Bytes()))
	}
	return nil
}

func (r *Runtime) removeStagedPolicySource(ctx context.Context, image, source string) ([]byte, error) {
	return r.runner.Output(ctx, []string{
		"run", "--rm", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		"--entrypoint", "sh", image, "-eu", "-c", `rm -rf -- "$1"`,
		"tobari-policy-stage-cleanup", source,
	}, os.Environ())
}

func (r *Runtime) preparePolicyBundle(ctx context.Context, state tobari.State) error {
	if err := r.ensurePolicyBundleVolume(ctx); err != nil {
		return err
	}
	if ready, _ := r.policyRevisionReady(ctx, state.AggregateRevision); ready {
		return nil
	}
	return r.publishPolicyBundle(ctx, state)
}

// prepareFinalPolicyBundle publishes one final-authority projection without
// re-entering the predecessor Context store as an authority selector.
func (r *Runtime) prepareFinalPolicyBundle(ctx context.Context, projection FinalAggregateProjection) error {
	if err := r.verifyFinalAggregateTarget(projection); err != nil {
		return err
	}
	if err := r.ensurePolicyBundleVolume(ctx); err != nil {
		return err
	}
	if ready, _ := r.policyRevisionReady(ctx, projection.AggregateRevision); ready {
		return nil
	}
	return r.publishPolicyBundleTargetWithVerifier(
		ctx, projection.PolicyDirectory, projection.AggregateRevision,
		func() error { return r.verifyFinalAggregateTarget(projection) },
	)
}

func (r *Runtime) policyRevisionReady(ctx context.Context, revision string) (bool, []byte) {
	if revision == "" {
		return false, []byte("policy revision is required")
	}
	expression := `revision := http.send({"method":"get","url":"http://127.0.0.1:8181/v1/data/tobari/aggregate_revision"}); revision.status_code == 200; revision.body.result == ` + strconv.Quote(revision) + `; decision := http.send({"method":"post","url":"http://127.0.0.1:8181/v1/data/tobari/http/decision","headers":{"content-type":"application/json"},"body":{"input":{}}}); decision.status_code == 200; object.get(decision.body, "result", null) != null`
	output, err := r.runner.Output(ctx, []string{
		"exec", opaContainer, "/opa", "eval", "--fail", "--format", "raw", expression,
	}, os.Environ())
	if err != nil {
		return false, output
	}
	results := bytes.Fields(output)
	if len(results) == 0 {
		return false, output
	}
	for _, result := range results {
		if !bytes.Equal(result, []byte("true")) {
			return false, output
		}
	}
	return true, output
}

func (r *Runtime) waitForPolicyRevision(ctx context.Context, revision string) error {
	if revision == "" {
		return fmt.Errorf("policy revision is required")
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last []byte
	for {
		ready, output := r.policyRevisionReady(ctx, revision)
		last = output
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("OPA did not activate expected policy revision: %s", boundedDiagnostic(last))
		case <-ticker.C:
		}
	}
}

func policyFenceArchive(candidateRevision string) ([]byte, string, error) {
	digest := sha256.Sum256([]byte("tobari-policy-transition:" + candidateRevision))
	revision := fmt.Sprintf("%x", digest[:])
	rego := []byte(`package tobari.http

default decision := {"allow": false, "reason": "policy transition in progress", "status_code": 503, "learnable": false}

permission_wait_observation := {"revision": data.tobari.aggregate_revision, "decision": decision}
`)
	data := []byte(fmt.Sprintf(`{"tobari":{"aggregate_schema_version":%d,"aggregate_revision":%q}}`+"\n", aggregateSchemaVersion, revision))
	archive, err := policyBundleArchive([]policyBundleFile{
		{name: "fence.rego", content: rego}, {name: "data.json", content: data},
	})
	return archive, revision, err
}

func (r *Runtime) applyPolicyRevision(ctx context.Context, state tobari.State) error {
	if _, _, err := r.verifyPersistedAggregateState(ctx, state); err != nil {
		return err
	}
	return r.applyPolicyTarget(ctx, state.PolicyDirectory, state.AggregateRevision, true)
}

func (r *Runtime) applyPolicyTarget(
	ctx context.Context, policyDirectory, aggregateRevision string, verifyAggregate bool,
) error {
	var verify func() error
	if verifyAggregate {
		verify = func() error {
			return r.verifyAggregateTargetForRevision(ctx, policyDirectory, aggregateRevision)
		}
	}
	return r.applyPolicyTargetWithVerifier(ctx, policyDirectory, aggregateRevision, verify)
}

func (r *Runtime) applyPolicyTargetWithVerifier(
	ctx context.Context, policyDirectory, aggregateRevision string, verify func() error,
) error {
	if !aggregateRevisionPattern.MatchString(aggregateRevision) || !filepath.IsAbs(policyDirectory) || filepath.Clean(policyDirectory) != policyDirectory {
		return fmt.Errorf("policy activation target is invalid")
	}
	if err := r.testPolicyDirectory(ctx, policyDirectory); err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_test_failed", policyTestFailureMessage, false, err,
		)
	}
	if err := r.verifyOwned(ctx, "container", opaContainer); err != nil {
		return fmt.Errorf("inspect owned OPA container: %w", err)
	}
	if err := r.verifyOwned(ctx, "volume", policyBundleVolume); err != nil {
		return fmt.Errorf("inspect owned policy bundle volume: %w", err)
	}
	if err := r.publishPolicyBundleTargetWithVerifier(ctx, policyDirectory, aggregateRevision, verify); err != nil {
		return err
	}
	return r.waitForPolicyRevision(ctx, aggregateRevision)
}

func (r *Runtime) applyPolicyArchive(ctx context.Context, archive []byte, aggregateRevision string) error {
	if !aggregateRevisionPattern.MatchString(aggregateRevision) {
		return fmt.Errorf("policy activation revision is invalid")
	}
	if err := r.testPolicyArchive(ctx, archive); err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_test_failed", policyTestFailureMessage, false, err,
		)
	}
	if err := r.verifyOwned(ctx, "container", opaContainer); err != nil {
		return fmt.Errorf("inspect owned OPA container: %w", err)
	}
	if err := r.verifyOwned(ctx, "volume", policyBundleVolume); err != nil {
		return fmt.Errorf("inspect owned policy bundle volume: %w", err)
	}
	if err := r.publishPolicyArchiveTarget(ctx, archive, aggregateRevision); err != nil {
		return err
	}
	return r.waitForPolicyRevision(ctx, aggregateRevision)
}

// ApplyFinalAggregatePolicy hot-activates one already tested final-authority
// projection without reading legacy shared State as an authority selector. A
// deny-all fence makes every transition conservative; this dormant seam owns
// no public command or current composition route.
func (r *Runtime) ApplyFinalAggregatePolicy(ctx context.Context, projection FinalAggregateProjection) error {
	if !aggregateRevisionPattern.MatchString(projection.AggregateRevision) || !filepath.IsAbs(projection.PolicyDirectory) || filepath.Clean(projection.PolicyDirectory) != projection.PolicyDirectory {
		return fmt.Errorf("final aggregate policy target is invalid")
	}
	fenceArchive, fenceRevision, err := policyFenceArchive(projection.AggregateRevision)
	if err != nil {
		return err
	}
	if err := r.applyPolicyArchive(ctx, fenceArchive, fenceRevision); err != nil {
		return err
	}
	if err := r.verifyFinalAggregateTarget(projection); err != nil {
		return err
	}
	return r.applyPolicyTargetWithVerifier(
		ctx, projection.PolicyDirectory, projection.AggregateRevision,
		func() error { return r.verifyFinalAggregateTarget(projection) },
	)
}

func policyFinalFenceArchive(candidateRevision string) ([]byte, string, error) {
	digest := sha256.Sum256([]byte("tobari-final-policy-transition:" + candidateRevision))
	revision := fmt.Sprintf("%x", digest[:])
	rego := []byte("package tobari.http\n\ndefault decision := {\"allow\": false, \"reason\": \"final policy transition in progress\", \"status_code\": 503, \"learnable\": false}\n\npermission_wait_observation := {\"revision\": data.tobari.aggregate_revision, \"decision\": decision}\n")
	data := []byte(fmt.Sprintf(`{"tobari":{"aggregate_schema_version":%d,"aggregate_revision":%q}}`+"\n", aggregateSchemaVersion, revision))
	archive, err := policyBundleArchive([]policyBundleFile{
		{name: "fence.rego", content: rego}, {name: "data.json", content: data},
	})
	return archive, revision, err
}

// ApplyPolicy validates and hot-activates one complete revision in the stable
// owned OPA. The Docker-managed volume creates a Docker-host filesystem event
// without relying on host bind-mount notification behavior.
func (r *Runtime) ApplyPolicy(ctx context.Context, state tobari.State) error {
	fenceArchive, fenceRevision, err := policyFenceArchive(state.AggregateRevision)
	if err != nil {
		return err
	}
	if err := r.applyPolicyArchive(ctx, fenceArchive, fenceRevision); err != nil {
		_ = r.recordRecentError(state, "Policy activation did not complete; inspect OPA logs.")
		return err
	}
	if err := r.applyPolicyRevision(ctx, state); err != nil {
		_ = r.recordRecentError(state, "Policy activation did not complete; inspect OPA logs.")
		return err
	}
	return nil
}

func (r *Runtime) activatePolicyRevision(ctx context.Context, state tobari.State, reducing bool) error {
	if reducing {
		return r.ApplyPolicy(ctx, state)
	}
	if err := r.applyPolicyRevision(ctx, state); err != nil {
		_ = r.recordRecentError(state, "Policy activation did not complete; inspect OPA logs.")
		return err
	}
	return nil
}

func (r *Runtime) testPolicy(ctx context.Context, state tobari.State) error {
	if _, _, err := r.verifyPersistedAggregateState(ctx, state); err != nil {
		return err
	}
	return r.testPolicyDirectory(ctx, state.PolicyDirectory)
}

func (r *Runtime) testPolicyDirectory(ctx context.Context, policyDirectory string) error {
	archive, err := aggregatePolicyBundleArchive(policyDirectory)
	if err != nil {
		return err
	}
	return r.testPolicyArchive(ctx, archive)
}

func (r *Runtime) testPolicyPreflight(ctx context.Context, preflight policyPreflight) error {
	archive, err := policyBundleArchive([]policyBundleFile{
		{name: "tobari.rego", content: preflight.evaluator},
		{name: "tobari_test.rego", content: preflight.tests},
		{name: "data.json", content: preflight.data},
	})
	if err != nil {
		return err
	}
	return r.testPolicyArchive(ctx, archive)
}

func (r *Runtime) testPolicyArchive(ctx context.Context, archive []byte) (resultErr error) {
	if len(archive) == 0 || len(archive) > maxPolicyPreflight*2 {
		return fmt.Errorf("policy test bundle is invalid")
	}
	createdVolume, err := r.ensurePolicyBundleVolumeWithCreation(ctx)
	if err != nil {
		return err
	}
	if createdVolume {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(r.lifetimeParent(ctx), policyPreflightVolumeCleanupTimeout)
			defer cancel()
			if cleanupErr := r.removeCreatedPolicyBundleVolume(cleanupCtx); cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}()
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(archive)
	source := "/bundle/.test-" + fmt.Sprintf("%x", digest[:])
	if err := r.stagePolicyArchive(ctx, versions["DEBIAN_IMAGE"], source, archive); err != nil {
		return err
	}
	defer func() {
		_, _ = r.removeStagedPolicySource(ctx, versions["DEBIAN_IMAGE"], source)
	}()
	output, err := r.runner.Output(
		ctx,
		[]string{
			"run", "--rm", "--user", "0:0", "--network", "none", "--read-only",
			"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
			"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
			versions["OPA_IMAGE"], "test", source,
		},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("%w: %s", err, boundedDiagnostic(output))
	}
	return nil
}
