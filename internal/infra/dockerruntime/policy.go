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
	"strconv"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

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
			if route.ProjectID == items[index].ProjectID && route.WorkspaceManifestID == items[index].WorkspaceManifestID {
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
	err := r.verifyOwned(ctx, "volume", policyBundleVolume)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errOwnedResourceMissing) {
		return err
	}
	output, createErr := r.runner.Output(ctx, []string{
		"volume", "create",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=opa-policy",
		policyBundleVolume,
	}, os.Environ())
	if createErr != nil {
		return fmt.Errorf("create policy bundle volume: %w: %s", createErr, boundedDiagnostic(output))
	}
	return r.verifyOwned(ctx, "volume", policyBundleVolume)
}

func (r *Runtime) publishPolicyBundle(ctx context.Context, state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return fmt.Errorf("read embedded runtime versions: %w", err)
	}
	source := "/bundle/.source-" + state.AggregateRevision
	candidate := "/bundle/.candidate-" + state.AggregateRevision + ".tar.gz"
	archive, cleanupArchive, err := r.policySourceArchive(state.PolicyDirectory)
	if err != nil {
		return fmt.Errorf("archive tested policy source: %w", err)
	}
	defer cleanupArchive()
	var stageOutput bytes.Buffer
	err = r.runner.Run(ctx, []string{
		"run", "--rm", "--interactive", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		"--entrypoint", "sh", versions["DEBIAN_IMAGE"], "-eu", "-c",
		`rm -rf -- "$1" && mkdir -m 0700 -- "$1" && tar --extract --file - --directory "$1" --no-same-owner && chmod -R u=rwX,go= -- "$1"`,
		"tobari-policy-stage", source,
	}, os.Environ(), archive, &stageOutput, &stageOutput)
	if err != nil {
		return fmt.Errorf("stage tested policy source: %w: %s", err, boundedDiagnostic(stageOutput.Bytes()))
	}
	output, err := r.runner.Output(ctx, []string{
		"run", "--rm", "--user", "0:0", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--tmpfs", "/tmp:size=16m,mode=1777",
		"--mount", "type=volume,src=" + policyBundleVolume + ",dst=/bundle",
		versions["OPA_IMAGE"], "build", "-b", source,
		"-o", candidate, "--revision", state.AggregateRevision,
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

func (r *Runtime) policySourceArchive(directory string) (*os.File, func(), error) {
	sourceRoot, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, err
	}
	defer sourceRoot.Close()

	archive, err := os.CreateTemp("", "tobari-policy-source-*.tar")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = archive.Close()
		_ = os.Remove(archive.Name())
	}
	if err := archive.Chmod(0o600); err != nil {
		cleanup()
		return nil, nil, err
	}
	writer := tar.NewWriter(archive)
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == directory {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
			return fmt.Errorf("invalid aggregate policy archive path %q", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("aggregate policy archive path %q must be a regular file or directory", relative)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("aggregate policy archive path %q must remain owner-only", relative)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Uid = 0
		header.Gid = 0
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := sourceRoot.Open(relative)
		if err != nil {
			return err
		}
		openedInfo, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		if !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0o077 != 0 || !os.SameFile(info, openedInfo) {
			_ = file.Close()
			return fmt.Errorf("aggregate policy archive path %q changed while being archived", relative)
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err == nil {
		err = writer.Close()
	} else {
		_ = writer.Close()
	}
	if err == nil {
		err = archive.Sync()
	}
	if err == nil {
		_, err = archive.Seek(0, io.SeekStart)
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return archive, cleanup, nil
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

func (r *Runtime) policyFenceState(state tobari.State) (tobari.State, func(), error) {
	digest := sha256.Sum256([]byte("tobari-policy-transition:" + state.AggregateRevision))
	revision := fmt.Sprintf("%x", digest[:])
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return tobari.State{}, nil, err
	}
	directory, err := os.MkdirTemp(r.stateDirectory, ".policy-transition-")
	if err != nil {
		return tobari.State{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	if err := os.Chmod(directory, 0o700); err != nil { // #nosec G302 -- transition policy is owner-only.
		cleanup()
		return tobari.State{}, nil, err
	}
	rego := []byte(`package tobari.http

default decision := {"allow": false, "reason": "policy transition in progress", "status_code": 503, "learnable": false}

permission_wait_observation := {"revision": data.tobari.aggregate_revision, "decision": decision}
`)
	data := []byte(fmt.Sprintf(`{"tobari":{"aggregate_schema_version":%d,"aggregate_revision":%q}}`+"\n", aggregateSchemaVersion, revision))
	for name, contents := range map[string][]byte{"fence.rego": rego, "data.json": data} {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil { // #nosec G306 -- transition policy is owner-only.
			cleanup()
			return tobari.State{}, nil, err
		}
	}
	fence := state
	fence.PolicyDirectory = directory
	fence.AggregateRevision = revision
	if fence.SchemaVersion == 2 {
		fence.Applied.AggregateRevision = revision
	}
	return fence, cleanup, nil
}

func (r *Runtime) applyPolicyRevision(ctx context.Context, state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := r.testPolicy(ctx, state); err != nil {
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
	if err := r.publishPolicyBundle(ctx, state); err != nil {
		return err
	}
	return r.waitForPolicyRevision(ctx, state.AggregateRevision)
}

// ApplyPolicy validates and hot-activates one complete revision in the stable
// owned OPA. The Docker-managed volume creates a Docker-host filesystem event
// without relying on host bind-mount notification behavior.
func (r *Runtime) ApplyPolicy(ctx context.Context, state tobari.State) error {
	fence, cleanup, err := r.policyFenceState(state)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := r.applyPolicyRevision(ctx, fence); err != nil {
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
	return r.testPolicyDirectory(ctx, state.PolicyDirectory)
}

func (r *Runtime) testPolicyDirectory(ctx context.Context, policyDirectory string) error {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	uid, gid := currentIDs()
	mount := "type=bind,src=" + policyDirectory + ",dst=/policy,readonly"
	output, err := r.runner.Output(
		ctx,
		[]string{
			"run", "--rm", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
			"--mount", mount, versions["OPA_IMAGE"], "test", "/policy",
		},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("%w: %s", err, boundedDiagnostic(output))
	}
	return nil
}
