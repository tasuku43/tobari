package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func (r *Runtime) addPolicyDataDiagnostic(
	ctx context.Context, add func(string, doctor.CheckStatus, string), policyDirectory string,
) {
	if strings.HasPrefix(policyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		contexts, err := r.readAggregateContexts(ctx)
		if err != nil || len(contexts) == 0 {
			add("policy_data", doctor.CheckStatusFail, "Context policy data is invalid or unsafe: "+fmt.Sprint(err))
			return
		}
		add("policy_data", doctor.CheckStatusPass, fmt.Sprintf("learned policy data is safe across %d Contexts", len(contexts)))
		return
	}
	if _, err := readPolicyData(policyDirectory); err != nil {
		add(
			"policy_data", doctor.CheckStatusFail,
			"learned policy data is invalid or unsafe; inspect the active Context policy data: "+err.Error(),
		)
		return
	}
	add("policy_data", doctor.CheckStatusPass, "learned policy data is safe for guided review")
}

func (r *Runtime) checkGatewayConfigAt(path string) (string, doctor.CheckStatus) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "Gateway projection will be initialized by cluster up", doctor.CheckStatusWarn
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "gateway.json must be a regular owner-only file", doctor.CheckStatusFail
	}
	data, err := os.ReadFile(path) // #nosec G304 -- state binds the generated aggregate Gateway projection.
	if err != nil || len(data) > 256*1024 {
		return "gateway.json is unreadable or exceeds 256 KiB", doctor.CheckStatusFail
	}
	var document struct {
		Version  string `json:"version"`
		Contexts map[string]struct {
			Name             string                   `json:"name"`
			GraphQLEndpoints []tobari.GraphQLEndpoint `json:"graphql_endpoints"`
		} `json:"contexts"`
	}
	if err := decodeStrictJSON(data, &document); err != nil || document.Version != "v1" || document.Contexts == nil {
		return "gateway.json does not match Gateway projection schema V1", doctor.CheckStatusFail
	}
	for contextID, projected := range document.Contexts {
		if err := tobari.ValidateContextID(contextID); err != nil || tobari.ValidateName(projected.Name) != nil {
			return "gateway.json contains an invalid Context projection", doctor.CheckStatusFail
		}
		seenEndpoints := make(map[tobari.GraphQLEndpoint]struct{}, len(projected.GraphQLEndpoints))
		for _, endpoint := range projected.GraphQLEndpoints {
			if err := endpoint.Validate(); err != nil {
				return "gateway.json contains an invalid GraphQL endpoint projection", doctor.CheckStatusFail
			}
			if _, duplicate := seenEndpoints[endpoint]; duplicate {
				return "gateway.json contains duplicate GraphQL endpoint projections", doctor.CheckStatusFail
			}
			seenEndpoints[endpoint] = struct{}{}
		}
	}
	return "Gateway routing metadata matches schema V1", doctor.CheckStatusPass
}
