package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/rootkey"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	maxDockerDoctorObservationBytes    = 4096
	maxDockerDoctorObservationDuration = 5 * time.Second
)

// ObserveDoctorCheck performs exactly one application-selected read-only
// diagnostic. Dependency scheduling and recovery remain in doctorcmd.
func (r *Runtime) ObserveDoctorCheck(
	ctx context.Context, root string, id doctor.CheckID,
) (doctor.Observation, error) {
	if ctx == nil {
		return doctor.Observation{}, fmt.Errorf("doctor observation context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Observation{}, err
	}
	switch id {
	case doctor.CheckIDDockerCLI:
		return r.observeDockerCLI(), nil
	case doctor.CheckIDDockerEngine:
		return r.observeDockerEngine(ctx), nil
	case doctor.CheckIDDockerContext:
		return r.observeDockerContext(ctx), nil
	case doctor.CheckIDDockerCompose:
		return r.observeDockerCompose(ctx), nil
	case doctor.CheckIDProxyPort:
		return observed(doctor.CheckStatusPass, "Gateway has no host-published port"), nil
	case doctor.CheckIDRoot:
		return r.observeDoctorRoot(ctx, root), nil
	case doctor.CheckIDRootSharing:
		return observed(doctor.CheckStatusWarn, "path is valid; Docker bind sharing is checked when a Workspace is created"), nil
	case doctor.CheckIDContext:
		return r.observeDoctorContext(ctx), nil
	case doctor.CheckIDState:
		return r.observeDoctorState(ctx), nil
	case doctor.CheckIDPolicy:
		return r.observeDoctorPolicy(ctx), nil
	case doctor.CheckIDPolicyData:
		return r.observeDoctorPolicyData(ctx), nil
	case doctor.CheckIDImageConfig:
		return r.observeDoctorImageConfig(ctx), nil
	case doctor.CheckIDAuthProviderManifests:
		return r.observeDoctorProviderManifests(), nil
	case doctor.CheckIDAuthVaultPaths:
		return r.observeDoctorVaultPaths(), nil
	case doctor.CheckIDAuthRootKey:
		return r.observeDoctorRootKey(ctx), nil
	case doctor.CheckIDAuthBroker:
		return r.observeDoctorBroker(ctx), nil
	case doctor.CheckIDCredentialCompanion:
		return r.observeDoctorCompanion(ctx), nil
	case doctor.CheckIDAuthVaultIntegrity:
		observation, _, _, _ := r.observeDoctorVaultIntegrity(ctx)
		return observation, nil
	case doctor.CheckIDAuthProjectHandles:
		return r.observeDoctorProjectHandles(ctx), nil
	case doctor.CheckIDOwnedResources:
		return r.observeDoctorOwnedResources(ctx), nil
	default:
		return doctor.Observation{}, fmt.Errorf("unsupported doctor check %q", id)
	}
}

func observed(status doctor.CheckStatus, detail string) doctor.Observation {
	return doctor.Observation{Status: status, Detail: detail}
}

func (r *Runtime) observeDockerCLI() doctor.Observation {
	if _, err := exec.LookPath("docker"); err != nil {
		return observed(doctor.CheckStatusFail, "docker was not found on PATH")
	}
	return observed(doctor.CheckStatusPass, "docker is available")
}

func (r *Runtime) observeDockerEngine(ctx context.Context) doctor.Observation {
	output, err := r.dockerDoctorOutput(ctx, []string{"version", "--format", "{{.Server.Version}}"})
	if err != nil {
		return observed(doctor.CheckStatusFail, "Docker Engine is unavailable")
	}
	value := strings.TrimSpace(string(output))
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: value, Value: value}
}

func (r *Runtime) observeDockerContext(ctx context.Context) doctor.Observation {
	output, err := r.dockerDoctorOutput(ctx, []string{"context", "show"})
	if err != nil {
		return observed(doctor.CheckStatusFail, "Docker context could not be read")
	}
	return observed(doctor.CheckStatusPass, strings.TrimSpace(string(output)))
}

func (r *Runtime) observeDockerCompose(ctx context.Context) doctor.Observation {
	output, err := r.dockerDoctorOutput(ctx, []string{"compose", "version", "--short"})
	if err != nil {
		return observed(doctor.CheckStatusFail, "Docker Compose v2 is unavailable")
	}
	return observed(doctor.CheckStatusPass, strings.TrimSpace(string(output)))
}

func (r *Runtime) dockerDoctorOutput(ctx context.Context, arguments []string) ([]byte, error) {
	if _, production := r.runner.(osCommandRunner); !production {
		output, err := r.runner.Output(ctx, arguments, os.Environ())
		if err != nil || len(output) > maxDockerDoctorObservationBytes {
			return nil, fmt.Errorf("bounded Docker readiness observation failed")
		}
		return output, nil
	}
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, err
	}
	observationContext, cancel := context.WithTimeout(ctx, maxDockerDoctorObservationDuration)
	defer cancel()
	command := exec.CommandContext(observationContext, path, arguments...) // #nosec G204 -- executable and argv are a closed generic Docker readiness set.
	command.Env = os.Environ()
	output := &boundedDoctorOutput{}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || output.exceeded {
		return nil, fmt.Errorf("bounded Docker readiness observation failed")
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

type boundedDoctorOutput struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (w *boundedDoctorOutput) Write(data []byte) (int, error) {
	remaining := maxDockerDoctorObservationBytes + 1 - w.buffer.Len()
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		_, _ = w.buffer.Write(data[:remaining])
	}
	if w.buffer.Len() > maxDockerDoctorObservationBytes || len(data) > remaining {
		w.exceeded = true
	}
	return len(data), nil
}

func (r *Runtime) observeDoctorRoot(ctx context.Context, root string) doctor.Observation {
	resolved, err := r.ResolveProjectRoot(ctx, root)
	if err != nil {
		return observed(doctor.CheckStatusFail, err.Error())
	}
	return observed(doctor.CheckStatusPass, resolved)
}

func (r *Runtime) observeDoctorContext(ctx context.Context) doctor.Observation {
	observation, err := r.ObserveContext(ctx, "")
	if err != nil {
		if r.installationMigrationRequired(ctx) {
			return doctor.Observation{
				Status: doctor.CheckStatusFail,
				Detail: "the supported unpublished Context snapshot requires migration",
				Cause:  doctor.ObservationCauseLegacyStatePresent,
			}
		}
		return observed(doctor.CheckStatusFail, "the current Context could not be inspected")
	}
	if _, err := r.diagnosticContextStores(); err != nil {
		return observed(doctor.CheckStatusFail, "the current Context store paths are invalid or unsafe")
	}
	switch observation.State {
	case tobari.ManifestObservationAbsent:
		return observed(doctor.CheckStatusPass, "the display-only synthetic default Context is observable")
	default:
		return observed(doctor.CheckStatusPass, "the persisted current Context is valid")
	}
}

func (r *Runtime) observeDoctorState(ctx context.Context) doctor.Observation {
	_, exists, err := r.LoadState(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "Tobari cluster state is invalid or unsafe")
	}
	if !exists {
		return observed(doctor.CheckStatusWarn, "cluster is not configured")
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "CWD-owned Tobari state is invalid")
	}
	return observed(doctor.CheckStatusPass, fmt.Sprintf("cluster has %d CWD-owned Tobari", len(projects)))
}

func (r *Runtime) doctorPolicyDirectory(ctx context.Context) (string, *tobari.State, error) {
	state, exists, err := r.LoadState(ctx)
	if err == nil && exists {
		return state.PolicyDirectory, &state, nil
	}
	paths, pathErr := r.diagnosticContextStores()
	if pathErr != nil {
		return "", nil, pathErr
	}
	return paths.PolicyDirectory, nil, nil
}

func (r *Runtime) observeDoctorPolicy(ctx context.Context) doctor.Observation {
	policyDirectory, _, err := r.doctorPolicyDirectory(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "the active policy directory could not be inspected")
	}
	sourceCount, err := r.validateDoctorPolicySources(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "a Context policy source structure is invalid or unsafe")
	}
	if _, err := os.Lstat(policyDirectory); errors.Is(err, os.ErrNotExist) {
		return observed(doctor.CheckStatusWarn, "policy will be initialized by cluster up")
	} else if err != nil {
		return observed(doctor.CheckStatusFail, "the XDG policy directory could not be inspected")
	}
	if err := validateOwnerPolicyDirectory(policyDirectory); err != nil {
		return observed(doctor.CheckStatusFail, "the XDG policy directory is unsafe")
	}
	return observed(
		doctor.CheckStatusPass,
		fmt.Sprintf("%d Context policy source structures are valid; executable OPA tests run during cluster up", sourceCount),
	)
}

func (r *Runtime) validateDoctorPolicySources(ctx context.Context) (int, error) {
	contexts, err := r.ListContexts(ctx)
	if err != nil {
		return 0, err
	}
	if len(contexts.Items) == 0 {
		source, err := runtimeassets.Read("opa/policy/tobari.rego")
		if err != nil {
			return 0, err
		}
		_, err = transformContextRego(aggregateContext{
			manifest: tobari.WorkspaceManifest{Name: tobari.DefaultManifestName, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite},
			rego:     source,
		})
		return 1, err
	}
	for _, summary := range contexts.Items {
		observed, err := r.observeContext(summary.Name)
		if err != nil {
			return 0, err
		}
		manifest := observed.manifest
		paths := r.contextPaths(manifest.Name)
		if err := paths.Validate(); err != nil {
			return 0, err
		}
		if err := validateOwnerPolicyDirectory(paths.PolicyDirectory); err != nil {
			return 0, err
		}
		var source []byte
		if manifest.PolicyMode == tobari.ManifestPolicyModeGuided {
			source, err = runtimeassets.Read("opa/policy/tobari.rego")
		} else {
			source, err = readOwnerPolicyFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), maxPolicyPreflight)
		}
		if err != nil {
			return 0, err
		}
		if _, err := transformContextRego(aggregateContext{manifest: manifest, paths: paths, rego: source}); err != nil {
			return 0, err
		}
	}
	return len(contexts.Items), nil
}

func (r *Runtime) observeDoctorPolicyData(ctx context.Context) doctor.Observation {
	policyDirectory, _, err := r.doctorPolicyDirectory(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "the active policy data path could not be inspected")
	}
	if _, err := os.Lstat(policyDirectory); errors.Is(err, os.ErrNotExist) {
		return observed(doctor.CheckStatusWarn, "learned policy data will be initialized by cluster up")
	} else if err != nil {
		return observed(doctor.CheckStatusFail, "the active policy data path could not be inspected")
	}
	var result doctor.Observation
	r.addPolicyDataDiagnostic(ctx, func(_ string, status doctor.CheckStatus, detail string) {
		result = observed(status, detail)
	}, policyDirectory)
	if result.Status == "" {
		return observed(doctor.CheckStatusFail, "learned policy data could not be inspected")
	}
	if result.Status == doctor.CheckStatusFail && r.installationMigrationRequired(ctx) {
		result.Cause = doctor.ObservationCauseLegacyStatePresent
		result.Detail = "the supported migration has residual predecessor policy state"
	}
	return result
}

func (r *Runtime) installationMigrationRequired(ctx context.Context) bool {
	plans, err := r.planInstallationMigration(ctx)
	if err != nil {
		return false
	}
	for _, plan := range plans {
		if migrationPlanChanges(plan) {
			return true
		}
	}
	return false
}

func (r *Runtime) observeDoctorImageConfig(ctx context.Context) doctor.Observation {
	if _, err := r.ResolveImageSelector(ctx, ""); err != nil {
		return observed(doctor.CheckStatusFail, err.Error())
	}
	return observed(doctor.CheckStatusPass, "default image configuration is valid")
}

func (r *Runtime) observeDoctorProviderManifests() doctor.Observation {
	projection, err := r.loadAuthProviders()
	if err != nil {
		return observed(doctor.CheckStatusFail, "credential-provider manifests are invalid or unsafe")
	}
	return observed(doctor.CheckStatusPass, fmt.Sprintf("%d credential-provider manifests normalize to projection schema v1", len(projection.Providers)))
}

func (r *Runtime) observeDoctorVaultPaths() doctor.Observation {
	exists, err := rootkey.EncryptedStateExists(r.stateDirectory)
	if err != nil {
		return observed(doctor.CheckStatusFail, "Auth Broker vault paths are unsafe")
	}
	if exists {
		return observed(doctor.CheckStatusPass, "encrypted Context vault paths are owner-only")
	}
	return observed(doctor.CheckStatusPass, "no encrypted Context vault is present")
}

func (r *Runtime) observeDoctorRootKey(ctx context.Context) doctor.Observation {
	vaultsExist, vaultErr := rootkey.EncryptedStateExists(r.stateDirectory)
	provider, rootErr := rootkey.New(r.stateDirectory)
	if vaultErr != nil || rootErr != nil {
		return observed(doctor.CheckStatusFail, "the installation root-key backend is unavailable")
	}
	backend, exists, err := provider.Inspect(ctx, vaultsExist)
	if err != nil {
		return observed(doctor.CheckStatusFail, "the "+string(backend)+" root-key backend is unavailable or inconsistent with encrypted state")
	}
	if exists {
		return observed(doctor.CheckStatusPass, "the "+string(backend)+" installation root key is available")
	}
	return observed(doctor.CheckStatusWarn, "the "+string(backend)+" installation root key will be created by cluster up")
}

func (r *Runtime) observeDoctorBroker(ctx context.Context) doctor.Observation {
	state, err := r.brokerState(ctx)
	if err != nil || state == authbroker.BrokerStateUnavailable {
		return observed(doctor.CheckStatusWarn, "Auth Broker is unavailable")
	}
	if state == authbroker.BrokerStateLocked {
		return observed(doctor.CheckStatusFail, "Auth Broker is locked")
	}
	return observed(doctor.CheckStatusPass, "Auth Broker is healthy and unlocked")
}

func (r *Runtime) observeDoctorCompanion(ctx context.Context) doctor.Observation {
	state, _, err := r.credentialCompanionStatus(ctx)
	if err != nil || state != "ready" {
		return observed(doctor.CheckStatusWarn, "trusted-host credential refresh is unavailable")
	}
	return observed(doctor.CheckStatusPass, "trusted-host credential companion is authenticated and ready")
}

func (r *Runtime) observeDoctorVaultIntegrity(
	ctx context.Context,
) (doctor.Observation, map[string]map[string]projectAuthProviderBinding, map[string][]byte, error) {
	projection, err := r.loadAuthProviders()
	if err != nil {
		return observed(doctor.CheckStatusFail, "credential-provider manifests changed during diagnostics"), nil, nil, err
	}
	contexts, err := r.ListContexts(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "Context identities could not be inspected"), nil, nil, err
	}
	configured := make(map[string]map[string]projectAuthProviderBinding, len(contexts.Items))
	encodedBindings := make(map[string][]byte, len(projection.Providers))
	for _, item := range contexts.Items {
		configured[item.ID] = make(map[string]projectAuthProviderBinding)
		for _, provider := range projection.Providers {
			response, statusErr := r.runBrokerControl(ctx, nil, "status", "--context-id", item.ID, "--provider", provider.ID)
			if statusErr != nil || response.Provider != provider.ID {
				return observed(doctor.CheckStatusFail, "an encrypted Context vault could not be authenticated"), nil, nil, statusErr
			}
			switch response.State {
			case "not_configured":
			case "ready":
				_, encoded, digest, bindingErr := brokerBindingsForProvider(projection, provider.ID)
				if bindingErr != nil || !validAuthRevision(response.Revision) {
					return observed(doctor.CheckStatusFail, "Context credential metadata is inconsistent with the provider projection"), nil, nil, bindingErr
				}
				encodedBindings[provider.ID] = encoded
				configured[item.ID][provider.ID] = projectAuthProviderBinding{Provider: provider.ID, Revision: response.Revision, BindingDigest: digest}
			default:
				return observed(doctor.CheckStatusFail, "Auth Broker returned an invalid Context credential state"), nil, nil, nil
			}
		}
	}
	return observed(doctor.CheckStatusPass, "encrypted Context vaults are readable without exposing contents"), configured, encodedBindings, nil
}

func (r *Runtime) observeDoctorProjectHandles(ctx context.Context) doctor.Observation {
	vault, configured, encodedBindings, _ := r.observeDoctorVaultIntegrity(ctx)
	if vault.Status != doctor.CheckStatusPass {
		return observed(doctor.CheckStatusFail, "project-bound authentication prerequisites changed during diagnostics")
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return observed(doctor.CheckStatusFail, "project-bound authentication state could not be inspected")
	}
	stale := 0
	for _, project := range projects {
		registry, registryErr := r.readProjectAuthRegistry(project.ID)
		if registryErr != nil {
			return observed(doctor.CheckStatusFail, "a project authentication ownership record is unsafe or invalid")
		}
		observedBindings := make(map[string]projectAuthProviderBinding, len(registry.Providers))
		for _, binding := range registry.Providers {
			observedBindings[binding.Provider] = binding
		}
		for providerID, expected := range configured[project.WorkspaceManifestID] {
			current, exists := observedBindings[providerID]
			if !exists || current.Revision != expected.Revision || current.BindingDigest != expected.BindingDigest {
				stale++
				delete(observedBindings, providerID)
				continue
			}
			response, statusErr := r.runBrokerControl(
				ctx, nil, "binding_status", "--context-id", project.WorkspaceManifestID, "--project-id", project.ID,
				"--provider", providerID, "--revision", current.Revision, "--bindings", string(encodedBindings[providerID]),
			)
			if statusErr != nil {
				return observed(doctor.CheckStatusFail, "project-bound authentication state could not be verified")
			}
			switch response.State {
			case "ready":
			case "missing", "stale":
				stale++
			default:
				return observed(doctor.CheckStatusFail, "project-bound authentication state could not be verified")
			}
			delete(observedBindings, providerID)
		}
		stale += len(observedBindings)
	}
	if stale != 0 {
		return observed(doctor.CheckStatusWarn, fmt.Sprintf("%d project authentication bindings require the next matching tobari entry", stale))
	}
	return observed(doctor.CheckStatusPass, "project authentication bindings match current Context credentials and provider manifests")
}

func (r *Runtime) observeDoctorOwnedResources(ctx context.Context) doctor.Observation {
	output, err := r.runner.Output(
		ctx, []string{"ps", "-a", "--filter", "label=" + ownerLabel + "=" + ownerValue, "--format", "{{.Names}}"}, os.Environ(),
	)
	if err != nil {
		return observed(doctor.CheckStatusFail, "owned Docker resources could not be listed")
	}
	if strings.TrimSpace(string(output)) == "" {
		return observed(doctor.CheckStatusPass, "no residual containers")
	}
	return observed(doctor.CheckStatusWarn, "owned containers exist: "+strings.Join(strings.Fields(string(output)), ","))
}
