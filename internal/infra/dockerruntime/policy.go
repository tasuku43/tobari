package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type gatewayAuditRecord struct {
	Timestamp         string  `json:"timestamp"`
	RequestID         string  `json:"request_id"`
	Cluster           string  `json:"cluster"`
	Host              string  `json:"host"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Decision          string  `json:"decision"`
	Reason            string  `json:"reason"`
	CredentialProfile *string `json:"credential_profile"`
	UpstreamStatus    int     `json:"upstream_status"`
	DurationMS        int     `json:"duration_ms"`
}

// ClusterDenials projects only validated deny audit records from one bounded
// Gateway log window.
func (r *Runtime) ClusterDenials(
	ctx context.Context, state tobari.State, tail int,
) ([]tobari.PolicyDenial, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return nil, err
	}
	data, err := r.runner.Output(
		ctx, []string{"logs", "--tail", strconv.Itoa(tail), gatewayContainer}, os.Environ(),
	)
	if err != nil {
		return nil, fmt.Errorf("read Gateway logs: %w: %s", err, boundedDiagnostic(data))
	}
	if len(data) > maxLogBytes {
		return nil, fmt.Errorf("Gateway log output exceeds %d bytes", maxLogBytes)
	}
	return parseGatewayDenials(data)
}

func parseGatewayDenials(data []byte) ([]tobari.PolicyDenial, error) {
	items := make([]tobari.PolicyDenial, 0)
	for lineNumber, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
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
			return nil, fmt.Errorf("decode Gateway denial line %d: %w", lineNumber+1, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("Gateway denial line %d contains trailing data", lineNumber+1)
		}
		if record.Cluster != ownerValue || record.Decision != "deny" || record.DurationMS < 0 {
			return nil, fmt.Errorf("Gateway denial line %d violates the audit contract", lineNumber+1)
		}
		item := tobari.PolicyDenial{
			Timestamp: record.Timestamp, RequestID: record.RequestID,
			Host: record.Host, Method: record.Method, Path: record.Path,
			Reason: record.Reason, StatusCode: record.UpstreamStatus,
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("Gateway denial line %d: %w", lineNumber+1, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// ApplyPolicy validates the current host policy, then recreates only the exact
// owned OPA component and waits for it to become healthy. This is the portable
// activation path when Docker-host file watching does not propagate events.
func (r *Runtime) ApplyPolicy(ctx context.Context, state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := r.testPolicy(ctx, state); err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_test_failed", "OPA policy tests failed", false, err,
		)
	}
	label, err := r.runner.Output(
		ctx,
		[]string{
			"inspect", "--format", `{{index .Config.Labels "` + ownerLabel + `"}}`,
			opaContainer,
		},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("inspect owned OPA container: %w: %s", err, boundedDiagnostic(label))
	}
	if strings.TrimSpace(string(label)) != ownerValue {
		return fmt.Errorf("OPA container is not owned by Tobari")
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return err
	}
	output, err := r.runner.Output(
		ctx,
		[]string{
			"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"up", "-d", "--no-deps", "--force-recreate", "--wait", "opa",
		},
		environment,
	)
	if err != nil {
		_ = r.recordRecentError(state, "Policy activation did not complete; inspect OPA logs.")
		return fmt.Errorf("recreate OPA with tested policy: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}
