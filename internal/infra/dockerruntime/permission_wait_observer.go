package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionPolicyObservation struct {
	Revision string `json:"revision"`
	Decision struct {
		Allow      bool   `json:"allow"`
		Reason     string `json:"reason"`
		StatusCode int    `json:"status_code"`
		Learnable  bool   `json:"learnable"`
	} `json:"decision"`
}

// ObservePermissionDisposition asks the canonical live OPA evaluator about the
// record's exact original effect. A false terminal flag is nonterminal; this
// adapter never recreates matching or policy precedence in Go.
func (r *Runtime) ObservePermissionDisposition(
	ctx context.Context, record tobari.PermissionWaitRecord,
) (tobari.PermissionWaitResult, bool, error) {
	if err := record.Validate(); err != nil {
		return "", false, fmt.Errorf("validate permission wait record: %w", err)
	}
	state, configured, err := r.LoadState(ctx)
	if err != nil {
		return "", false, fmt.Errorf("load active policy state: %w", err)
	}
	if !configured || state.AggregateRevision == "" {
		return "", false, nil
	}
	input, err := permissionPolicyInput(record)
	if err != nil {
		return "", false, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", false, fmt.Errorf("encode permission policy input: %w", err)
	}
	expression := `[result | observation := http.send({"method":"post","url":"http://127.0.0.1:8181/v1/data/tobari/http/permission_wait_observation","headers":{"content-type":"application/json"},"body":{"input":` + string(encoded) + `}}); observation.status_code == 200; object.get(observation.body, "result", null) != null; result := observation.body.result][0]`
	output, err := r.runner.Output(ctx, []string{
		"exec", opaContainer, "/opa", "eval", "--fail", "--format", "raw", expression,
	}, os.Environ())
	if err != nil {
		return "", false, fmt.Errorf("observe active permission policy: %w", err)
	}
	observation, err := parsePermissionPolicyObservation(output)
	if err != nil {
		return "", false, err
	}
	if observation.Revision != state.AggregateRevision {
		return "", false, nil
	}
	if observation.Decision.Allow {
		if observation.Decision.Learnable {
			return "", false, fmt.Errorf("active permission Allow is marked learnable")
		}
		return tobari.PermissionWaitResultAllow, true, nil
	}
	if observation.Decision.Reason == "denied by exact policy" && !observation.Decision.Learnable {
		return tobari.PermissionWaitResultDeny, true, nil
	}
	return "", false, nil
}

func permissionPolicyInput(record tobari.PermissionWaitRecord) (map[string]any, error) {
	if err := record.Validate(); err != nil {
		return nil, fmt.Errorf("validate permission policy input: %w", err)
	}
	return map[string]any{
		"schema_version": 1,
		"principal": map[string]any{
			"cluster": "default", "context_id": record.WorkspaceManifestID,
			"project_id": record.WorkspaceID,
		},
		"request": map[string]any{
			"authority": map[string]any{
				"scheme": record.Effect.Scheme, "host": record.Effect.Host, "port": record.Effect.Port,
			},
			"method":  record.Effect.Method,
			"path":    map[string]any{"raw": record.Effect.Path, "segments": append([]string{}, record.Effect.Segments...)},
			"query":   map[string]any{},
			"headers": map[string]any{},
		},
		"authorization": map[string]any{"broker_provider": nil},
	}, nil
}

func parsePermissionPolicyObservation(data []byte) (permissionPolicyObservation, error) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	decoder.DisallowUnknownFields()
	var result permissionPolicyObservation
	if err := decoder.Decode(&result); err != nil {
		return permissionPolicyObservation{}, fmt.Errorf("decode active permission policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return permissionPolicyObservation{}, fmt.Errorf("active permission policy contains trailing data")
	}
	if result.Revision == "" || result.Decision.Reason == "" || result.Decision.StatusCode < 100 || result.Decision.StatusCode > 599 {
		return permissionPolicyObservation{}, fmt.Errorf("active permission policy result is invalid")
	}
	return result, nil
}
