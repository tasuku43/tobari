package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const finalWorkspaceRuntimeSchema = 1

const (
	finalWorkspaceRetirementPrepared         = "prepared"
	finalWorkspaceRetirementContainerRetired = "container_retired"
)

type finalWorkspaceRuntimeSpec struct {
	WorkspaceID      tobari.WorkspaceID                       `json:"workspace_id"`
	ContextID        tobari.ContextID                         `json:"context_id"`
	TemplateID       tobari.WorkspaceTemplateID               `json:"workspace_template_id"`
	TemplateRevision tobari.SemanticDigest                    `json:"workspace_template_revision"`
	EntrySlice       tobari.SemanticDigest                    `json:"entry_slice_digest"`
	RuntimeID        string                                   `json:"runtime_id"`
	RuntimeRevision  tobari.SemanticDigest                    `json:"runtime_revision"`
	ImageSelector    string                                   `json:"image_selector"`
	ImageID          string                                   `json:"image_id"`
	AssetVersion     string                                   `json:"asset_version"`
	RuntimeDirectory string                                   `json:"runtime_directory"`
	ProjectRoot      string                                   `json:"project_root"`
	WorkspaceRoot    string                                   `json:"workspace_root"`
	WorkspaceHome    string                                   `json:"workspace_home"`
	GitDirectory     string                                   `json:"git_directory"`
	GitConfigDigest  tobari.SemanticDigest                    `json:"git_config_digest"`
	SourceAccess     tobari.ManifestSourceAccess              `json:"source_access"`
	AgentProfile     string                                   `json:"agent_profile"`
	ProfileDirectory string                                   `json:"profile_directory"`
	ProfileDigest    tobari.SemanticDigest                    `json:"profile_digest"`
	CreationDefaults tobari.WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
	Network          tobari.WorkspaceRuntimeNetworkAuthority  `json:"network_authority"`
	Resources        []string                                 `json:"resources"`
	LifetimeCommand  []string                                 `json:"lifetime_command"`
	// AuthEnvironment is an effect-time research projection. It is deliberately
	// absent from the final runtime authority digest: the Broker issues a fresh
	// opaque handle only after the reviewed entry plan is durable and before the
	// container effect. Release builds leave this field empty.
	AuthEnvironment []string `json:"-"`
}

type finalWorkspaceEntryRecord struct {
	SchemaVersion int                                        `json:"schema_version"`
	DecisionRef   string                                     `json:"decision_ref"`
	Plan          tobari.WorkspaceEntryReconciliationPlan    `json:"plan"`
	Receipt       tobari.WorkspaceEntryReconciliationReceipt `json:"receipt"`
}

type finalWorkspaceRetirementRecord struct {
	SchemaVersion int                     `json:"schema_version"`
	DecisionRef   string                  `json:"decision_ref"`
	Workspace     tobari.WorkspaceBinding `json:"workspace"`
}

type finalWorkspaceRetirementDecision struct {
	SchemaVersion int                     `json:"schema_version"`
	DecisionRef   string                  `json:"decision_ref"`
	Workspace     tobari.WorkspaceBinding `json:"workspace"`
	Force         bool                    `json:"force"`
	Phase         string                  `json:"phase"`
}

func (r *Runtime) finalWorkspaceRuntimeRoot() string {
	return filepath.Join(r.stateDirectory, "workspace-authority-runtime")
}

func (r *Runtime) finalWorkspaceDirectory(id tobari.WorkspaceID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(r.finalWorkspaceRuntimeRoot(), "workspaces", string(id)), nil
}

func (r *Runtime) finalWorkspaceHome(id tobari.WorkspaceID) (string, error) {
	directory, err := r.finalWorkspaceDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "home"), nil
}

func (r *Runtime) finalWorkspaceGitDirectory(id tobari.WorkspaceID) (string, error) {
	directory, err := r.finalWorkspaceDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "git"), nil
}

func (r *Runtime) finalWorkspaceEntryRecordPath(id tobari.WorkspaceID) (string, error) {
	directory, err := r.finalWorkspaceDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "entry.json"), nil
}

func (r *Runtime) finalWorkspaceRetirementRecordPath() string {
	return filepath.Join(r.finalWorkspaceRuntimeRoot(), "retirement.json")
}

func (r *Runtime) finalWorkspaceRetirementDecisionPath() string {
	return filepath.Join(r.finalWorkspaceRuntimeRoot(), "retirement-active.json")
}

// WorkspaceHomeForID derives the final owner-only Workspace home solely from
// the stable Workspace ID. It never consults predecessor instance/root state.
func (r *Runtime) WorkspaceHomeForID(ctx context.Context, id tobari.WorkspaceID) (string, error) {
	if r == nil {
		return "", fmt.Errorf("Docker runtime is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return r.finalWorkspaceHome(id)
}

// ResolveWorkspaceTemplateRuntimeRevision resolves one unchanged Runtime
// revision reference through the coherent WP03 lifecycle snapshot. The caller
// already owns the installation lifecycle lock, so this method uses the
// lock-held observation and does not acquire another lifecycle lock.
func (r *Runtime) ResolveWorkspaceTemplateRuntimeRevision(ctx context.Context, reference string) (tobari.RuntimeBinding, error) {
	if r == nil {
		return tobari.RuntimeBinding{}, fmt.Errorf("Docker runtime is unavailable")
	}
	runtimeID, revision, err := tobari.ParseRuntimeRevisionRef(reference)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	snapshot, _, err := r.readRuntimeLifecycleSnapshotLocked(ctx)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	return runtimeBindingFromLifecycle(snapshot, runtimeID, revision)
}

func runtimeBindingFromLifecycle(snapshot tobari.RuntimeLifecycleSnapshot, runtimeID, revision string) (tobari.RuntimeBinding, error) {
	if err := snapshot.Validate(); err != nil {
		return tobari.RuntimeBinding{}, err
	}
	for _, activity := range snapshot.Journals.Active {
		if activity.RuntimeID == runtimeID {
			return tobari.RuntimeBinding{}, tobari.ErrRuntimeLifecycleActive
		}
	}
	for _, manifest := range snapshot.Runtimes {
		if manifest.ID != runtimeID {
			continue
		}
		for _, candidate := range manifest.Revisions {
			if candidate.Revision != revision {
				continue
			}
			if manifest.Kind == tobari.RuntimeKindManaged {
				available := false
				for _, material := range snapshot.Materials {
					if material.RuntimeID == runtimeID && material.Revision == revision {
						available = material.Availability == tobari.RuntimeAvailabilityAvailable && material.OwnershipVerified && material.ObservationComplete
						break
					}
				}
				if !available {
					return tobari.RuntimeBinding{}, tobari.ErrRuntimeNotReady
				}
			}
			return manifest.Binding(candidate.Ordinal)
		}
		return tobari.RuntimeBinding{}, tobari.ErrRuntimeRevisionNotFound
	}
	return tobari.RuntimeBinding{}, tobari.ErrRuntimeNotFound
}

// PlanWorkspaceEntry is read-only. It binds every value needed by crash
// recovery into the returned final plan, including the exact Runtime image
// identity, profile and Git projections, source-access ceiling, and final-only
// filesystem paths.
func (r *Runtime) PlanWorkspaceEntry(
	ctx context.Context,
	snapshot tobari.ContextAuthoritySnapshot,
	authority tobari.WorkspaceTemplateEntryAuthority,
	workspaceID tobari.WorkspaceID,
	reconciledAt time.Time,
) (tobari.WorkspaceEntryReconciliationPlan, error) {
	if r == nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, fmt.Errorf("Docker runtime is unavailable")
	}
	if err := snapshot.Validate(); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	if err := authority.ValidateFor(snapshot.Template.Current); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	if err := workspaceID.Validate(); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	if reconciledAt.IsZero() || reconciledAt.Location() != time.UTC {
		return tobari.WorkspaceEntryReconciliationPlan{}, fmt.Errorf("Workspace reconciliation time must be non-zero UTC")
	}
	resolvedRoot, err := r.ResolveProjectRoot(ctx, snapshot.Context.ProjectRoot)
	if err != nil || resolvedRoot != snapshot.Context.ProjectRoot {
		return tobari.WorkspaceEntryReconciliationPlan{}, fmt.Errorf("final Context project root is not exact: %w", err)
	}
	binding, image, imageID, err := r.resolveFinalWorkspaceRuntimeMaterial(ctx, authority.Runtime)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	if !reflect.DeepEqual(binding, authority.Runtime) {
		return tobari.WorkspaceEntryReconciliationPlan{}, fmt.Errorf("Template Runtime binding is not the exact coherent WP03 revision")
	}
	gitConfig, err := r.finalWorkspaceGitConfig(ctx, authority.SessionDefaults.GitIdentity, snapshot.Context.ProjectRoot)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	networkAuthority, err := r.selectFinalWorkspaceNetworkAuthority(ctx, workspaceID)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	creationDefaults, err := retainedWorkspaceCreationDefaults(snapshot, authority)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	spec, err := r.finalWorkspaceSpec(authority, creationDefaults, networkAuthority, snapshot.Context, workspaceID, image, imageID, gitConfig)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	resolvedSpec, err := finalWorkspaceSpecDigest(spec)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	home, err := r.finalWorkspaceHome(workspaceID)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID,
		ContextID: snapshot.Context.ID, ProjectRoot: snapshot.Context.ProjectRoot,
		Home: home, CreationDefaults: snapshot.Template.Current.Slices.CreationDefaultsDigest,
	}
	if snapshot.Workspace != nil {
		workspace = *snapshot.Workspace
		if workspace.ID != workspaceID || workspace.Home != home {
			return tobari.WorkspaceEntryReconciliationPlan{}, fmt.Errorf("existing Workspace identity crosses final runtime paths")
		}
	}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: authority.TemplateID,
		TemplateRevision: authority.TemplateRevision, EntrySliceDigest: authority.EntrySliceDigest,
		RuntimeID: authority.Runtime.RuntimeID, RuntimeRevision: tobari.SemanticDigest(authority.Runtime.Revision),
		ResolvedSpec: resolvedSpec, ReconciledAt: reconciledAt,
	}
	if workspace.LastSuccessfulEntry != nil {
		previous := *workspace.LastSuccessfulEntry
		expected := applied
		expected.ReconciledAt = previous.ReconciledAt
		if previous == expected {
			applied = previous
		}
	}
	workspace.LastSuccessfulEntry = &applied
	plan := tobari.WorkspaceEntryReconciliationPlan{
		Workspace: workspace, Applied: applied, Authority: authority,
		CreationDefaults: creationDefaults, Network: networkAuthority,
	}
	return plan, plan.ValidateFor(snapshot)
}

func retainedWorkspaceCreationDefaults(snapshot tobari.ContextAuthoritySnapshot, current tobari.WorkspaceTemplateEntryAuthority) (tobari.WorkspaceTemplateCreationDefaults, error) {
	if snapshot.Workspace == nil {
		return current.CreationDefaults.Clone(), nil
	}
	for _, revision := range snapshot.Template.Retained {
		if revision.Slices.CreationDefaultsDigest == snapshot.Workspace.CreationDefaults {
			return revision.Body.CreationDefaults.Clone(), nil
		}
	}
	return tobari.WorkspaceTemplateCreationDefaults{}, fmt.Errorf("retained Workspace creation authority is unavailable")
}

type finalWorkspaceNetworkObservation struct {
	IPAM []struct {
		Subnet  string `json:"Subnet"`
		Gateway string `json:"Gateway"`
	} `json:"ipam"`
	Containers map[string]struct {
		Name        string `json:"Name"`
		EndpointID  string `json:"EndpointID"`
		MacAddress  string `json:"MacAddress"`
		IPv4Address string `json:"IPv4Address"`
		IPv6Address string `json:"IPv6Address"`
	} `json:"containers"`
}

func (r *Runtime) selectFinalWorkspaceNetworkAuthority(ctx context.Context, id tobari.WorkspaceID) (tobari.WorkspaceRuntimeNetworkAuthority, error) {
	_, network, err := tobari.ProjectResourceNames(string(id))
	if err != nil {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, err
	}
	exists, err := r.projectResourceExists(ctx, "network", network)
	if err != nil {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, err
	}
	if exists {
		if err := r.verifyOwnedProjectResource(ctx, "network", network, string(id), projectNetRole); err != nil {
			return tobari.WorkspaceRuntimeNetworkAuthority{}, err
		}
		observed, err := r.observeFinalWorkspaceNetwork(ctx, network)
		if err != nil {
			return tobari.WorkspaceRuntimeNetworkAuthority{}, err
		}
		return finalWorkspaceNetworkAuthorityFromObservation(id, network, observed)
	}
	used, err := r.observeBoundedDockerIPv4Subnets(ctx)
	if err != nil {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, err
	}
	digest := sha256.Sum256([]byte(id))
	const subnetCount uint16 = 1 << 14 // 10.64.0.0/10 split into /24 networks.
	start := binary.BigEndian.Uint16(digest[:2]) % subnetCount
	for offset := uint16(0); offset < subnetCount; offset++ {
		index := (start + offset) % subnetCount
		var octets [2]byte
		binary.BigEndian.PutUint16(octets[:], index)
		candidate := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 64 + octets[0], octets[1], 0}), 24)
		collision := false
		for _, occupied := range used {
			if prefixesOverlap(candidate, occupied) {
				collision = true
				break
			}
		}
		if collision {
			continue
		}
		base := candidate.Addr()
		authority := tobari.WorkspaceRuntimeNetworkAuthority{
			Network: network, Subnet: candidate.String(), DockerGateway: base.Next().String(),
			GatewayIP: base.Next().Next().String(), WorkspaceIP: base.Next().Next().Next().String(),
		}
		return authority, authority.ValidateFor(id)
	}
	return tobari.WorkspaceRuntimeNetworkAuthority{}, fmt.Errorf("no bounded private subnet is available for the final Workspace")
}

func prefixesOverlap(left, right netip.Prefix) bool {
	return left.Contains(right.Addr()) || right.Contains(left.Addr())
}

func (r *Runtime) observeBoundedDockerIPv4Subnets(ctx context.Context) ([]netip.Prefix, error) {
	output, err := r.runner.Output(ctx, []string{"network", "ls", "--quiet", "--no-trunc"}, os.Environ())
	if err != nil {
		return nil, fmt.Errorf("list Docker network authority: %w: %s", err, boundedDiagnostic(output))
	}
	if len(output) > 512*1024 {
		return nil, fmt.Errorf("Docker network inventory exceeds the bounded observation")
	}
	ids := strings.Fields(string(output))
	if len(ids) > 4096 {
		return nil, fmt.Errorf("Docker network inventory exceeds the bounded observation")
	}
	result := make([]netip.Prefix, 0, len(ids))
	for _, id := range ids {
		if !runtimeLifecycleContainerID.MatchString(id) {
			return nil, fmt.Errorf("Docker network inventory identity is invalid")
		}
		config, err := r.runner.Output(ctx, []string{"network", "inspect", "--format", `{{json .IPAM.Config}}`, id}, os.Environ())
		if err != nil {
			return nil, fmt.Errorf("inspect Docker network allocation: %w: %s", err, boundedDiagnostic(config))
		}
		var values []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		}
		if err := decodeStrictJSON(config, &values); err != nil {
			return nil, err
		}
		for _, value := range values {
			prefix, parseErr := netip.ParsePrefix(value.Subnet)
			if parseErr == nil && prefix.Addr().Is4() {
				result = append(result, prefix.Masked())
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result, nil
}

func (r *Runtime) observeFinalWorkspaceNetwork(ctx context.Context, network string) (finalWorkspaceNetworkObservation, error) {
	output, err := r.runner.Output(ctx, []string{"network", "inspect", "--format", `{"ipam":{{json .IPAM.Config}},"containers":{{json .Containers}}}`, network}, os.Environ())
	if err != nil {
		return finalWorkspaceNetworkObservation{}, fmt.Errorf("observe final Workspace network: %w: %s", err, boundedDiagnostic(output))
	}
	var observed finalWorkspaceNetworkObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return observed, err
	}
	return observed, nil
}

func finalWorkspaceNetworkAuthorityFromObservation(id tobari.WorkspaceID, network string, observed finalWorkspaceNetworkObservation) (tobari.WorkspaceRuntimeNetworkAuthority, error) {
	if len(observed.IPAM) != 1 {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, fmt.Errorf("final Workspace network IPAM authority is ambiguous")
	}
	prefix, err := netip.ParsePrefix(observed.IPAM[0].Subnet)
	if err != nil || !prefix.Addr().Is4() {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, fmt.Errorf("final Workspace network subnet is invalid")
	}
	base := prefix.Masked().Addr()
	authority := tobari.WorkspaceRuntimeNetworkAuthority{
		Network: network, Subnet: prefix.Masked().String(), DockerGateway: base.Next().String(),
		GatewayIP: base.Next().Next().String(), WorkspaceIP: base.Next().Next().Next().String(),
	}
	if observed.IPAM[0].Gateway != authority.DockerGateway {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, fmt.Errorf("final Workspace network Docker gateway drifted")
	}
	if err := authority.ValidateFor(id); err != nil {
		return tobari.WorkspaceRuntimeNetworkAuthority{}, err
	}
	container, _, _ := tobari.ProjectResourceNames(string(id))
	for _, endpoint := range observed.Containers {
		address := strings.Split(endpoint.IPv4Address, "/")[0]
		if address == authority.WorkspaceIP && endpoint.Name != container || address == authority.GatewayIP && endpoint.Name != gatewayContainer {
			return tobari.WorkspaceRuntimeNetworkAuthority{}, fmt.Errorf("final Workspace reserved network endpoint is occupied")
		}
	}
	return authority, nil
}

func (r *Runtime) resolveFinalWorkspaceRuntimeMaterial(ctx context.Context, expected tobari.RuntimeBinding) (tobari.RuntimeBinding, string, string, error) {
	if err := expected.Validate(); err != nil {
		return tobari.RuntimeBinding{}, "", "", err
	}
	if r.finalWorkspaceRuntimeMaterial != nil {
		return r.finalWorkspaceRuntimeMaterial(ctx, expected)
	}
	snapshot, _, err := r.readRuntimeLifecycleSnapshotLocked(ctx)
	if err != nil {
		return tobari.RuntimeBinding{}, "", "", err
	}
	binding, err := runtimeBindingFromLifecycle(snapshot, expected.RuntimeID, expected.Revision)
	if err != nil {
		return tobari.RuntimeBinding{}, "", "", err
	}
	if !reflect.DeepEqual(binding, expected) {
		return tobari.RuntimeBinding{}, "", "", fmt.Errorf("Runtime binding changed or is not canonical")
	}
	image := r.resolveBuiltinImageSelector(binding.Image)
	if err := r.validateCompatibleImage(ctx, image); err != nil {
		return tobari.RuntimeBinding{}, "", "", err
	}
	imageID, err := r.compatibleImageID(ctx, image)
	if err != nil {
		return tobari.RuntimeBinding{}, "", "", err
	}
	if !imageIDPattern.MatchString(imageID) {
		return tobari.RuntimeBinding{}, "", "", fmt.Errorf("Runtime image identity is not immutable")
	}
	return binding, image, imageID, nil
}

func (r *Runtime) finalWorkspaceSpec(
	authority tobari.WorkspaceTemplateEntryAuthority,
	creationDefaults tobari.WorkspaceTemplateCreationDefaults,
	networkAuthority tobari.WorkspaceRuntimeNetworkAuthority,
	contextBinding tobari.ContextBinding,
	workspaceID tobari.WorkspaceID,
	image, imageID string,
	gitConfig []byte,
) (finalWorkspaceRuntimeSpec, error) {
	version, err := runtimeassets.Version()
	if err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	home, err := r.finalWorkspaceHome(workspaceID)
	if err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	gitDirectory, err := r.finalWorkspaceGitDirectory(workspaceID)
	if err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	workspaceRoot, err := r.projectContainerRoot(contextBinding.ProjectRoot)
	if err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	profile := filepath.Join(r.dataDirectory, "profiles", authority.AgentProfile)
	profileDigest, err := r.finalWorkspaceProfileDigest(profile)
	if err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	gitDigest := sha256.Sum256(gitConfig)
	if err := networkAuthority.ValidateFor(workspaceID); err != nil {
		return finalWorkspaceRuntimeSpec{}, err
	}
	return finalWorkspaceRuntimeSpec{
		WorkspaceID: workspaceID, ContextID: contextBinding.ID, TemplateID: authority.TemplateID,
		TemplateRevision: authority.TemplateRevision, EntrySlice: authority.EntrySliceDigest,
		RuntimeID: authority.Runtime.RuntimeID, RuntimeRevision: tobari.SemanticDigest(authority.Runtime.Revision),
		ImageSelector: image, ImageID: imageID, AssetVersion: version, RuntimeDirectory: runtimeDirectory,
		ProjectRoot: contextBinding.ProjectRoot, WorkspaceRoot: workspaceRoot, WorkspaceHome: home,
		GitDirectory: gitDirectory, GitConfigDigest: tobari.SemanticDigest("sha256:" + hex.EncodeToString(gitDigest[:])),
		SourceAccess: authority.SourceAccess, AgentProfile: authority.AgentProfile,
		ProfileDirectory: profile, ProfileDigest: profileDigest,
		CreationDefaults: creationDefaults.Clone(), Network: networkAuthority,
		Resources: projectResourceHashFields(), LifetimeCommand: projectLifetimeCommand(),
	}, nil
}

func finalWorkspaceSpecDigest(spec finalWorkspaceRuntimeSpec) (tobari.SemanticDigest, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])), nil
}

func (r *Runtime) finalWorkspaceProfileDigest(profile string) (tobari.SemanticDigest, error) {
	if _, err := os.Lstat(profile); errors.Is(err, os.ErrNotExist) {
		hash := sha256.New()
		_, _ = hash.Write([]byte("claude"))
		_, _ = hash.Write([]byte("{}\n"))
		_, _ = hash.Write([]byte("common"))
		_, _ = hash.Write([]byte("{}\n"))
		return tobari.SemanticDigest(fmt.Sprintf("sha256:%x", hash.Sum(nil))), nil
	} else if err != nil {
		return "", err
	}
	digest, err := r.projectProfileDigest(profile)
	return tobari.SemanticDigest(digest), err
}

func (r *Runtime) reconcileFinalWorkspaceAuth(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, spec finalWorkspaceRuntimeSpec) (projectAuthProjection, error) {
	if _, err := os.Lstat(r.authProviderProjectionPath()); errors.Is(err, os.ErrNotExist) {
		return projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}, JSONMerges: []projectAuthJSONMerge{}, Providers: []projectAuthProviderBinding{}}, nil
	} else if err != nil {
		return projectAuthProjection{}, fmt.Errorf("inspect final Workspace authentication projection: %w", err)
	}
	instance := tobari.Workspace{
		SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
		ID:                    string(plan.Workspace.ID),
		Root:                  plan.Workspace.ProjectRoot,
		WorkspaceManifestID:   string(plan.Workspace.ContextID),
		WorkspaceManifestName: tobari.DefaultManifestName,
		Profile:               tobari.DefaultProfile,
		Image:                 spec.ImageSelector,
		CreationApplied: tobari.WorkspaceCreationApplied{
			CreationDefaultsRevision: string(plan.Workspace.CreationDefaults),
			AppliedAt:                time.Unix(1, 0).UTC(),
		},
	}
	if err := instance.Validate(); err != nil {
		return projectAuthProjection{}, fmt.Errorf("final Workspace authentication owner: %w", err)
	}
	return r.reconcileProjectAuthAtHome(ctx, instance, spec.WorkspaceHome)
}

func (r *Runtime) finalWorkspaceGitConfig(ctx context.Context, setting *tobari.ManifestGitIdentitySetting, root string) ([]byte, error) {
	var identity *projectGitIdentity
	if setting != nil && setting.Source != tobari.ManifestGitIdentityDefault {
		switch setting.Source {
		case tobari.ManifestGitIdentityInherit:
			if r.gitIdentity == nil {
				return nil, gitIdentityResolutionFailed()
			}
			resolved, err := r.gitIdentity.Resolve(ctx, root)
			if err != nil || validateProjectGitIdentity(resolved) != nil {
				return nil, gitIdentityResolutionFailed()
			}
			identity = resolved
		case tobari.ManifestGitIdentityLiteral:
			if setting.Name == nil || setting.Email == nil {
				return nil, fmt.Errorf("literal Git identity is incomplete")
			}
			identity = &projectGitIdentity{Name: *setting.Name, Email: *setting.Email}
		default:
			return nil, fmt.Errorf("Template Git identity source is invalid")
		}
	}
	return encodeProjectGitConfig(identity)
}

func validateFinalWorkspaceDecisionRef(prefix string, id tobari.WorkspaceID, value string) error {
	want := prefix + ":" + string(id) + ":sha256:"
	if !strings.HasPrefix(value, want) || len(value) != len(want)+64 {
		return fmt.Errorf("final Workspace decision reference is invalid")
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, want))
	if err != nil {
		return fmt.Errorf("final Workspace decision reference is invalid")
	}
	return nil
}

func (r *Runtime) ReconcileWorkspaceEntry(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, decisionRef string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	if err := validateFinalWorkspacePlan(plan); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	if err := validateFinalWorkspaceDecisionRef("workspace-entry", plan.Workspace.ID, decisionRef); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	var result tobari.WorkspaceEntryReconciliationReceipt
	err := r.withProjectLock(ctx, func() error {
		if err := r.requireNoPredecessorProjectJournal(); err != nil {
			return err
		}
		if existing, present, err := r.readFinalWorkspaceEntryRecord(plan.Workspace.ID); err != nil {
			return err
		} else if present && reflect.DeepEqual(existing.Plan, plan) {
			if err := r.confirmFinalWorkspaceEntryRecord(ctx, existing); err != nil {
				return err
			}
			if existing.DecisionRef != decisionRef {
				existing.DecisionRef = decisionRef
				path, _ := r.finalWorkspaceEntryRecordPath(plan.Workspace.ID)
				if err := writeAtomicJSON(path, existing); err != nil {
					return err
				}
			}
			result = existing.Receipt
			return nil
		}
		sessionState, _, err := r.observeFinalWorkspaceSessionOwner(ctx, plan.Workspace.ContextID, plan.Workspace.ID)
		if err != nil {
			return err
		}
		if sessionState == FinalWorkspaceSessionLive {
			return fmt.Errorf("final Workspace runtime cannot change while its canonical attachment is live")
		}
		binding, image, imageID, err := r.resolveFinalWorkspaceRuntimeMaterial(ctx, plan.Authority.Runtime)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(binding, plan.Authority.Runtime) {
			return fmt.Errorf("Workspace entry Runtime authority changed")
		}
		gitConfig, err := r.finalWorkspaceGitConfig(ctx, plan.Authority.SessionDefaults.GitIdentity, plan.Workspace.ProjectRoot)
		if err != nil {
			return err
		}
		contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: plan.Workspace.ContextID, ProjectRoot: plan.Workspace.ProjectRoot, TemplateID: plan.Authority.TemplateID}
		spec, err := r.finalWorkspaceSpec(plan.Authority, plan.CreationDefaults, plan.Network, contextBinding, plan.Workspace.ID, image, imageID, gitConfig)
		if err != nil {
			return err
		}
		digest, err := finalWorkspaceSpecDigest(spec)
		if err != nil || digest != plan.Applied.ResolvedSpec {
			return fmt.Errorf("Workspace entry plan no longer matches exact runtime material: %w", err)
		}
		if err := r.ensureFinalWorkspaceRuntimeRoot(); err != nil {
			return err
		}
		if err := r.ensureFinalWorkspaceHelpers(ctx, image); err != nil {
			return err
		}
		if err := r.prepareFinalWorkspaceFiles(plan, spec, gitConfig); err != nil {
			return err
		}
		authProjection, err := r.reconcileFinalWorkspaceAuth(ctx, plan, spec)
		if err != nil {
			return err
		}
		spec.AuthEnvironment = append([]string(nil), authProjection.Environment...)
		containerID, err := r.reconcileFinalWorkspaceDocker(ctx, plan, spec)
		if err != nil {
			return err
		}
		result = tobari.WorkspaceEntryReconciliationReceipt{WorkspaceID: plan.Workspace.ID, ContextID: plan.Workspace.ContextID, Applied: plan.Applied, ContainerID: containerID}
		if err := result.ValidateFor(plan); err != nil {
			return err
		}
		record := finalWorkspaceEntryRecord{SchemaVersion: finalWorkspaceRuntimeSchema, DecisionRef: decisionRef, Plan: plan.Clone(), Receipt: result}
		path, _ := r.finalWorkspaceEntryRecordPath(plan.Workspace.ID)
		if err := writeAtomicJSON(path, record); err != nil {
			return err
		}
		return r.confirmFinalWorkspaceEntryRecord(ctx, record)
	})
	return result, err
}

func validateFinalWorkspacePlan(plan tobari.WorkspaceEntryReconciliationPlan) error {
	if err := plan.Workspace.ID.Validate(); err != nil {
		return err
	}
	if plan.Workspace.SchemaVersion != tobari.WorkspaceBindingSchemaVersion || plan.Workspace.ContextID != plan.Applied.ContextID || plan.Workspace.LastSuccessfulEntry == nil || *plan.Workspace.LastSuccessfulEntry != plan.Applied ||
		plan.Authority.TemplateID != plan.Applied.TemplateID || plan.Authority.TemplateRevision != plan.Applied.TemplateRevision || plan.Authority.EntrySliceDigest != plan.Applied.EntrySliceDigest ||
		plan.Authority.Runtime.RuntimeID != plan.Applied.RuntimeID || tobari.SemanticDigest(plan.Authority.Runtime.Revision) != plan.Applied.RuntimeRevision {
		return fmt.Errorf("final Workspace entry plan is incomplete or inconsistent")
	}
	if err := plan.Applied.Validate(); err != nil {
		return err
	}
	if err := plan.Authority.Runtime.Validate(); err != nil {
		return err
	}
	if err := plan.Authority.SourceAccess.Validate(); err != nil {
		return err
	}
	if err := plan.CreationDefaults.Validate(); err != nil {
		return err
	}
	if err := plan.Network.ValidateFor(plan.Workspace.ID); err != nil {
		return err
	}
	if err := tobari.ValidateName(plan.Authority.AgentProfile); err != nil {
		return err
	}
	if err := plan.Authority.SessionDefaults.Validate(); err != nil {
		return err
	}
	if err := plan.Authority.CreationDefaults.Validate(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) ensureFinalWorkspaceRuntimeRoot() error {
	root := r.finalWorkspaceRuntimeRoot()
	if err := r.ensurePrivateDirectory(root); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(root)); err != nil {
		return fmt.Errorf("publish final Workspace runtime root durably: %w", err)
	}
	if err := r.ensurePrivateDirectory(filepath.Join(root, "workspaces")); err != nil {
		return err
	}
	return syncDirectory(root)
}

func (r *Runtime) requireNoPredecessorProjectJournal() error {
	if _, err := os.Lstat(r.projectJournalPath()); err == nil {
		return fmt.Errorf("unsupported predecessor project recovery authority is present; reset and recreate the pre-release installation")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("predecessor project recovery authority is ambiguous: %w", err)
	}
	return nil
}

func (r *Runtime) prepareFinalWorkspaceFiles(plan tobari.WorkspaceEntryReconciliationPlan, spec finalWorkspaceRuntimeSpec, gitConfig []byte) error {
	directory, _ := r.finalWorkspaceDirectory(plan.Workspace.ID)
	if err := r.ensurePrivateDirectory(directory); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(directory)); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(spec.WorkspaceHome); err != nil {
		return fmt.Errorf("prepare final Workspace home: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	if err := reconcileFinalWorkspaceBootstrap(spec.WorkspaceHome, plan.Authority.CreationDefaults.Bootstrap); err != nil {
		return err
	}
	if err := ensureProjectHomeMountTarget(spec.WorkspaceHome, spec.WorkspaceRoot); err != nil {
		return err
	}
	profile, err := r.ensureSharedProfile(plan.Authority.AgentProfile)
	if err != nil || profile != spec.ProfileDirectory {
		return fmt.Errorf("prepare exact final Workspace profile: %w", err)
	}
	profileDigest, err := r.finalWorkspaceProfileDigest(profile)
	if err != nil || profileDigest != spec.ProfileDigest {
		return fmt.Errorf("final Workspace profile authority changed: %w", err)
	}
	if err := r.ensureFinalWorkspaceAgentState(spec.WorkspaceHome, profile); err != nil {
		return err
	}
	if err := writeFinalWorkspaceGitConfig(spec.GitDirectory, gitConfig); err != nil {
		return err
	}
	return r.confirmFinalWorkspaceRuntimeAssets(spec)
}

func (r *Runtime) confirmFinalWorkspaceRuntimeAssets(spec finalWorkspaceRuntimeSpec) error {
	if version, err := runtimeassets.Version(); err != nil || version != spec.AssetVersion || spec.RuntimeDirectory != filepath.Join(r.stateDirectory, "runtime", version) {
		return fmt.Errorf("final Workspace runtime asset authority changed: %w", err)
	}
	if err := r.confirmFinalWorkspaceHelperAssets(spec.RuntimeDirectory); err != nil {
		return err
	}
	for _, relative := range []string{"browser/tobari-open"} {
		path := filepath.Join(spec.RuntimeDirectory, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("final Workspace runtime asset %s is unavailable or unsafe: %w", relative, err)
		}
	}
	return nil
}

func (r *Runtime) ensureFinalWorkspaceHelpers(ctx context.Context, image string) error {
	version, err := runtimeassets.Version()
	if err != nil {
		return err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := r.confirmFinalWorkspaceHelperAssets(runtimeDirectory); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := r.materializeWorkspaceHelpers(ctx, image); err != nil {
		return fmt.Errorf("materialize final Workspace helpers: %w", err)
	}
	return r.confirmFinalWorkspaceHelperAssets(runtimeDirectory)
}

func (r *Runtime) confirmFinalWorkspaceHelperAssets(runtimeDirectory string) error {
	for _, relative := range []string{"helpers/tobari-expose", "helpers/tobari-permission"} {
		path := filepath.Join(runtimeDirectory, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("final Workspace runtime asset %s is unavailable or unsafe: %w", relative, err)
		}
	}
	return nil
}

func reconcileFinalWorkspaceBootstrap(home string, snapshot *tobari.ManifestBootstrapSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	aws, err := encodeProjectAWSConfig(snapshot.AWS)
	if err != nil {
		return err
	}
	if err := ensureExactFinalBootstrapFile(filepath.Join(home, ".aws"), "config", aws); err != nil {
		return err
	}
	if snapshot.EKS == nil {
		return nil
	}
	kube, err := encodeProjectEKSConfig(snapshot.AWS.Profile, *snapshot.EKS)
	if err != nil {
		return err
	}
	return ensureExactFinalBootstrapFile(filepath.Join(home, ".kube"), "config", kube)
}

func ensureExactFinalBootstrapFile(directory, name string, content []byte) error {
	if err := ensureNewOrPrivateDirectory(directory); err != nil {
		return err
	}
	path := filepath.Join(directory, name)
	if info, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := initializeBytes(path, content, 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() > maxProjectStateBytes {
		return fmt.Errorf("final Workspace bootstrap target is unsafe")
	} else if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, content) { // #nosec G304 -- fixed bootstrap filename below the validated owner-only Workspace home child.
		return fmt.Errorf("final Workspace bootstrap target differs from reviewed creation authority: %w", err)
	}
	return syncDirectory(directory)
}

func (r *Runtime) ensureFinalWorkspaceAgentState(home, profile string) error {
	claude := filepath.Join(home, ".claude")
	for _, directory := range []string{claude, filepath.Join(claude, "skills"), filepath.Join(claude, "agents"), filepath.Join(claude, "commands")} {
		if err := r.ensurePrivateDirectory(directory); err != nil {
			return err
		}
	}
	baseSettings, err := os.ReadFile(filepath.Join(profile, "common", "settings.json")) // #nosec G304 -- profile is the exact runtime-owned path selected from a validated name and revalidated digest.
	if err != nil {
		return err
	}
	base, err := decodeSettingsObject(baseSettings)
	if err != nil {
		return err
	}
	local, found, err := readProjectSettings(filepath.Join(claude, projectLocalSettingsFile), "per-Workspace local agent settings")
	if err != nil {
		return err
	}
	if found {
		for key, value := range local {
			base[key] = value
		}
	}
	return writeAtomicJSON(filepath.Join(claude, "settings.json"), base)
}

func writeFinalWorkspaceGitConfig(directory string, data []byte) error {
	if len(data) == 0 || len(data) > maxProjectGitConfigBytes {
		return fmt.Errorf("final Workspace Git projection is empty or oversized")
	}
	if err := ensureNewOrPrivateDirectory(directory); err != nil {
		return err
	}
	path := filepath.Join(directory, "config")
	if info, present, err := inspectProjectGitConfig(path); err != nil {
		return err
	} else if present {
		if info.Mode().Perm() != 0o600 {
			return fmt.Errorf("final Workspace Git projection is not owner-only")
		}
		current, err := os.ReadFile(path) // #nosec G304 -- fixed config child after owner-only directory and regular-file validation.
		if err != nil {
			return err
		}
		if bytes.Equal(current, data) {
			return nil
		}
	}
	return writeFinalWorkspaceAtomicBytes(path, data, 0o600)
}

func writeFinalWorkspaceAtomicBytes(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tobari-final-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (r *Runtime) reconcileFinalWorkspaceDocker(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, spec finalWorkspaceRuntimeSpec) (string, error) {
	if r.finalWorkspaceDockerReconcile != nil {
		return r.finalWorkspaceDockerReconcile(ctx, plan.Clone(), spec)
	}
	container, network, err := tobari.ProjectResourceNames(string(plan.Workspace.ID))
	if err != nil || network != spec.Network.Network || !reflect.DeepEqual(plan.Network, spec.Network) {
		return "", fmt.Errorf("derive final Workspace Docker identity: %w", err)
	}
	if err := r.ensureExactFinalWorkspaceNetwork(ctx, plan.Workspace.ID, plan.Network); err != nil {
		return "", err
	}
	if err := r.ensureFinalWorkspaceContainer(ctx, plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP); err != nil {
		return "", err
	}
	subnet, err := r.projectNetworkSubnet(ctx, network)
	if err != nil || subnet != plan.Network.Subnet {
		return "", err
	}
	if err := r.ensureFinalWorkspaceNetworkGuard(ctx, plan.Workspace.ID, container, network, subnet, plan.Network.GatewayIP); err != nil {
		return "", err
	}
	if err := r.waitProjectReady(ctx, container); err != nil {
		return "", err
	}
	observed, err := r.observeFinalWorkspaceContainerForPlan(ctx, plan)
	if err != nil {
		return "", err
	}
	return observed.ID, nil
}

func (r *Runtime) ensureExactFinalWorkspaceNetwork(ctx context.Context, id tobari.WorkspaceID, authority tobari.WorkspaceRuntimeNetworkAuthority) error {
	if err := authority.ValidateFor(id); err != nil {
		return err
	}
	exists, err := r.projectResourceExists(ctx, "network", authority.Network)
	if err != nil {
		return err
	}
	if !exists {
		args := []string{
			"network", "create", "--internal", "--subnet", authority.Subnet, "--gateway", authority.DockerGateway,
			"--label", ownerLabel + "=" + ownerValue, "--label", componentLabel + "=tobari",
			"--label", projectIDLabel + "=" + string(id), "--label", projectRoleLabel + "=" + projectNetRole,
			authority.Network,
		}
		if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
			return fmt.Errorf("create exact final Workspace network: %w: %s", err, boundedDiagnostic(output))
		}
	}
	if err := r.verifyOwnedProjectResource(ctx, "network", authority.Network, string(id), projectNetRole); err != nil {
		return err
	}
	observed, err := r.observeFinalWorkspaceNetwork(ctx, authority.Network)
	if err != nil {
		return err
	}
	exact, err := finalWorkspaceNetworkAuthorityFromObservation(id, authority.Network, observed)
	if err != nil || !reflect.DeepEqual(exact, authority) {
		return fmt.Errorf("final Workspace network does not match its durable topology: %w", err)
	}
	return nil
}

func (r *Runtime) ensureFinalWorkspaceContainer(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, spec finalWorkspaceRuntimeSpec, container, network, workspaceIP, gatewayIP string) error {
	exists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return err
	}
	if exists {
		if err := r.verifyOwnedProjectResource(ctx, "container", container, string(plan.Workspace.ID), projectWorkRole); err != nil {
			return err
		}
		observedSpec, err := r.projectContainerSpecHash(ctx, container)
		if err != nil {
			return err
		}
		if observedSpec != string(plan.Applied.ResolvedSpec) {
			if output, removeErr := r.runner.Output(ctx, []string{"rm", "-f", container}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove drifted final Workspace container: %w: %s", removeErr, boundedDiagnostic(output))
			}
			exists = false
		}
	}
	if exists {
		if err := r.ensureProjectContainerNetwork(ctx, container, network); err != nil {
			return err
		}
		component, err := r.inspectContainer(ctx, projectWorkRole, container)
		if err != nil {
			return err
		}
		if component.State != "running" {
			output, err := r.runner.Output(ctx, []string{"start", container}, os.Environ())
			if err != nil {
				return fmt.Errorf("start final Workspace container: %w: %s", err, boundedDiagnostic(output))
			}
		}
		return r.confirmFinalWorkspaceContainerNetwork(ctx, container, network, workspaceIP, gatewayIP)
	}
	uid, gid := currentIDs()
	sourceMount := "type=bind,src=" + spec.ProjectRoot + ",dst=" + spec.WorkspaceRoot
	if spec.SourceAccess == tobari.ManifestSourceAccessReadOnly {
		sourceMount += ",readonly"
	}
	args := []string{
		"create", "--name", container, "--hostname", container,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--env", "HOME=/var/lib/tobari", "--env", "TOBARI_INSIDE=1",
		"--env", "TOBARI_ID=" + string(plan.Workspace.ID), "--env", "TOBARI_CONTEXT_ID=" + string(plan.Workspace.ContextID),
		"--env", "TOBARI_ROOT=" + spec.WorkspaceRoot, "--env", "TOBARI_PROFILE=/opt/tobari/profile",
		"--env", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem", "--env", "REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem", "--env", "GIT_CONFIG_SYSTEM=" + projectGitContainerConfig,
		"--mount", "type=bind,src=" + spec.WorkspaceHome + ",dst=/var/lib/tobari",
		"--mount", sourceMount,
		"--mount", "type=bind,src=" + spec.GitDirectory + ",dst=" + projectGitContainerDirectory + ",readonly",
		"--mount", "type=bind,src=" + spec.ProfileDirectory + ",dst=/opt/tobari/profile,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.ProfileDirectory, "claude", "skills") + ",dst=/var/lib/tobari/.claude/skills,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.ProfileDirectory, "claude", "agents") + ",dst=/var/lib/tobari/.claude/agents,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.ProfileDirectory, "claude", "commands") + ",dst=/var/lib/tobari/.claude/commands,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.ProfileDirectory, "claude", "plugins.lock") + ",dst=/var/lib/tobari/.claude/plugins.lock,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.RuntimeDirectory, "browser", "tobari-open") + ",dst=/run/tobari-open,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.RuntimeDirectory, "browser", "tobari-open") + ",dst=/usr/local/bin/xdg-open,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.RuntimeDirectory, "helpers", "tobari-expose") + ",dst=/usr/local/bin/tobari-expose,readonly",
		"--mount", "type=bind,src=" + filepath.Join(spec.RuntimeDirectory, "helpers", "tobari-permission") + ",dst=/usr/local/bin/tobari-permission,readonly",
		"--mount", "type=volume,src=tobari-public-ca,dst=/run/tobari/ca-public,readonly",
		"--workdir", spec.WorkspaceRoot, "--network", network, "--ip", workspaceIP, "--dns", gatewayIP,
		"--health-cmd", "test -f /tmp/tobari-ready", "--health-interval", "2s", "--health-timeout", "2s", "--health-retries", "30",
		"--label", ownerLabel + "=" + ownerValue, "--label", componentLabel + "=tobari",
		"--label", projectIDLabel + "=" + string(plan.Workspace.ID), "--label", projectRoleLabel + "=" + projectWorkRole,
		"--label", projectSpecLabel + "=" + string(plan.Applied.ResolvedSpec),
	}
	args = append(args, projectResourceDockerArgs()...)
	for _, environment := range spec.AuthEnvironment {
		args = append(args, "--env", environment)
	}
	args = append(args, spec.ImageID)
	args = append(args, projectLifetimeCommand()...)
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("create final Workspace container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(ctx, []string{"start", container}, os.Environ()); err != nil {
		return fmt.Errorf("start final Workspace container: %w: %s", err, boundedDiagnostic(output))
	}
	return r.confirmFinalWorkspaceContainerNetwork(ctx, container, network, workspaceIP, gatewayIP)
}

func (r *Runtime) confirmFinalWorkspaceContainerNetwork(ctx context.Context, container, network, workspaceIP, gatewayIP string) error {
	observedAddress, err := r.workspaceNetworkAddress(ctx, container, network)
	if err != nil || observedAddress != workspaceIP {
		return fmt.Errorf("final Workspace static network address drifted: %w", err)
	}
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", `{{json .HostConfig.Dns}}`, container}, os.Environ())
	if err != nil {
		return fmt.Errorf("observe final Workspace DNS authority: %w: %s", err, boundedDiagnostic(output))
	}
	var dns []string
	if err := decodeStrictJSON(output, &dns); err != nil || len(dns) != 1 || dns[0] != gatewayIP {
		return fmt.Errorf("final Workspace DNS does not bind the exact Gateway address: %w", err)
	}
	return nil
}

func (r *Runtime) ensureFinalWorkspaceNetworkGuard(ctx context.Context, id tobari.WorkspaceID, container, network, subnet, gatewayIP string) error {
	expectedContainer, expectedNetwork, err := tobari.ProjectResourceNames(string(id))
	if err != nil || container != expectedContainer || network != expectedNetwork {
		return networkGuardFailure("final Workspace network guard target identity is invalid", err)
	}
	if err := r.verifyOwnedProjectResource(ctx, "container", container, string(id), projectWorkRole); err != nil {
		return networkGuardFailure("final Workspace network guard target ownership could not be verified", err)
	}
	imageID, err := r.gatewayRuntimeImageID(ctx)
	if err != nil {
		return networkGuardFailure("final Workspace network guard could not identify the helper image", err)
	}
	return r.runNetworkGuardHelper(ctx, imageID, container, "workspace", gatewayIP, subnet)
}

func (r *Runtime) observeFinalWorkspaceContainerForPlan(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan) (finalWorkspaceContainerObservation, error) {
	if r.finalWorkspaceDockerObserve != nil {
		return r.finalWorkspaceDockerObserve(ctx, plan.Clone())
	}
	container, _, err := tobari.ProjectResourceNames(string(plan.Workspace.ID))
	if err != nil {
		return finalWorkspaceContainerObservation{}, err
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}},` +
		`"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, container}, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) {
			return finalWorkspaceContainerObservation{}, tobari.ErrWorkspaceEntryRuntimeNotCurrent
		}
		return finalWorkspaceContainerObservation{}, fmt.Errorf("observe final Workspace container: %w: %s", err, boundedDiagnostic(output))
	}
	var observed finalWorkspaceContainerObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return observed, err
	}
	if err := observed.validateFor(plan.Workspace.ID, plan.Applied.ResolvedSpec, ""); err != nil {
		return observed, errors.Join(tobari.ErrWorkspaceEntryRuntimeNotCurrent, err)
	}
	return observed, nil
}

func (r *Runtime) ConfirmWorkspaceEntry(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, decisionRef string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	if err := validateFinalWorkspacePlan(plan); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	if err := validateFinalWorkspaceDecisionRef("workspace-entry", plan.Workspace.ID, decisionRef); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	record, present, err := r.readFinalWorkspaceEntryRecord(plan.Workspace.ID)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	if !present || record.DecisionRef != decisionRef || !reflect.DeepEqual(record.Plan, plan) {
		return tobari.WorkspaceEntryReconciliationReceipt{}, tobari.ErrWorkspaceEntryRuntimeNotCurrent
	}
	if err := r.confirmFinalWorkspaceEntryRecord(ctx, record); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	return record.Receipt, nil
}

func (r *Runtime) readFinalWorkspaceEntryRecord(id tobari.WorkspaceID) (finalWorkspaceEntryRecord, bool, error) {
	path, err := r.finalWorkspaceEntryRecordPath(id)
	if err != nil {
		return finalWorkspaceEntryRecord{}, false, err
	}
	var record finalWorkspaceEntryRecord
	if err := readStrictJSON(path, &record); errors.Is(err, os.ErrNotExist) {
		return finalWorkspaceEntryRecord{}, false, nil
	} else if err != nil {
		return finalWorkspaceEntryRecord{}, false, err
	}
	if record.SchemaVersion != finalWorkspaceRuntimeSchema || validateFinalWorkspacePlan(record.Plan) != nil || record.Receipt.ValidateFor(record.Plan) != nil ||
		validateFinalWorkspaceDecisionRef("workspace-entry", record.Plan.Workspace.ID, record.DecisionRef) != nil {
		return finalWorkspaceEntryRecord{}, false, fmt.Errorf("final Workspace entry receipt is invalid")
	}
	return record, true, nil
}

func (r *Runtime) confirmFinalWorkspaceEntryRecord(ctx context.Context, record finalWorkspaceEntryRecord) error {
	if err := record.Receipt.ValidateFor(record.Plan); err != nil {
		return err
	}
	home, _ := r.finalWorkspaceHome(record.Plan.Workspace.ID)
	if home != record.Plan.Workspace.Home || requirePrivateDirectory(home) != nil {
		return errors.Join(tobari.ErrWorkspaceEntryRuntimeNotCurrent, fmt.Errorf("final Workspace home is absent or unsafe"))
	}
	observed, err := r.observeFinalWorkspaceContainerForPlan(ctx, record.Plan)
	if err != nil {
		return err
	}
	if observed.ID != record.Receipt.ContainerID {
		return errors.Join(tobari.ErrWorkspaceEntryRuntimeNotCurrent, fmt.Errorf("final Workspace container identity changed"))
	}
	return nil
}

func (r *Runtime) ConfirmWorkspaceRetirementAllowed(ctx context.Context, workspace tobari.WorkspaceBinding, force bool) error {
	if err := validateFinalWorkspaceBinding(workspace); err != nil {
		return err
	}
	state, _, err := r.observeFinalWorkspaceSessionOwner(ctx, workspace.ContextID, workspace.ID)
	if err != nil {
		return err
	}
	if state == FinalWorkspaceSessionLive && !force {
		return tobari.ErrWorkspaceBindingProtected
	}
	container, network, _ := tobari.ProjectResourceNames(string(workspace.ID))
	for _, resource := range []struct{ kind, name, role string }{{"container", container, projectWorkRole}, {"network", network, projectNetRole}} {
		exists, err := r.projectResourceExists(ctx, resource.kind, resource.name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("final Workspace %s is missing before its durable retirement decision", resource.kind)
		}
		if err := r.verifyOwnedProjectResource(ctx, resource.kind, resource.name, string(workspace.ID), resource.role); err != nil {
			return err
		}
	}
	directory, _ := r.finalWorkspaceDirectory(workspace.ID)
	home, _ := r.finalWorkspaceHome(workspace.ID)
	if workspace.Home != home || requirePrivateDirectory(directory) != nil || requirePrivateDirectory(home) != nil {
		return fmt.Errorf("final Workspace private runtime state is missing or unsafe")
	}
	return nil
}

func validateFinalWorkspaceBinding(workspace tobari.WorkspaceBinding) error {
	if workspace.SchemaVersion != tobari.WorkspaceBindingSchemaVersion || workspace.ID.Validate() != nil || workspace.ContextID.Validate() != nil ||
		workspace.Home == "" || !filepath.IsAbs(workspace.Home) || filepath.Clean(workspace.Home) != workspace.Home || workspace.CreationDefaults.Validate() != nil {
		return fmt.Errorf("final Workspace retirement authority is invalid")
	}
	if workspace.LastSuccessfulEntry != nil {
		if err := workspace.LastSuccessfulEntry.Validate(); err != nil || workspace.LastSuccessfulEntry.ContextID != workspace.ContextID {
			return fmt.Errorf("final Workspace AppliedEntry is invalid: %w", err)
		}
	}
	return nil
}

func (r *Runtime) RetireWorkspace(ctx context.Context, workspace tobari.WorkspaceBinding, force bool, decisionRef string) error {
	if err := r.PrepareWorkspaceRetirement(ctx, workspace, force, decisionRef); err != nil {
		return err
	}
	return r.CompleteWorkspaceRetirement(ctx, workspace, force, decisionRef)
}

// PrepareWorkspaceRetirement is the pre-settlement half of final Workspace
// deletion. The parent mutation decision is already durable. This private
// phase binds the exact request before removing the target container, then
// waits for only that canonical owner to become absent. Gateway settlement may
// safely remove the principal only after this method returns.
func (r *Runtime) PrepareWorkspaceRetirement(ctx context.Context, workspace tobari.WorkspaceBinding, force bool, decisionRef string) error {
	if err := validateFinalWorkspaceBinding(workspace); err != nil {
		return err
	}
	if err := validateFinalWorkspaceDecisionRef("workspace-retirement", workspace.ID, decisionRef); err != nil {
		return err
	}
	return r.withProjectLock(ctx, func() error {
		if err := r.requireNoPredecessorProjectJournal(); err != nil {
			return err
		}
		if record, present, err := r.readFinalWorkspaceRetirementRecord(); err != nil {
			return err
		} else if present {
			if err := r.confirmFinalWorkspaceRetired(ctx, record); err != nil {
				return err
			}
			if err := r.clearCompletedFinalWorkspaceRetirementDecision(record); err != nil {
				return err
			}
			if record.DecisionRef == decisionRef && reflect.DeepEqual(record.Workspace, workspace) {
				return nil
			}
		}
		decision, present, err := r.readFinalWorkspaceRetirementDecision()
		if err != nil {
			return err
		}
		if present {
			if decision.DecisionRef != decisionRef || decision.Force != force || !reflect.DeepEqual(decision.Workspace, workspace) {
				return fmt.Errorf("another final Workspace retirement requires exact recovery")
			}
		} else {
			if err := r.ConfirmWorkspaceRetirementAllowed(ctx, workspace, force); err != nil {
				return err
			}
			if err := r.ensureFinalWorkspaceRuntimeRoot(); err != nil {
				return err
			}
			decision = finalWorkspaceRetirementDecision{
				SchemaVersion: finalWorkspaceRuntimeSchema, DecisionRef: decisionRef,
				Workspace: workspace, Force: force, Phase: finalWorkspaceRetirementPrepared,
			}
			if err := writeAtomicJSON(r.finalWorkspaceRetirementDecisionPath(), decision); err != nil {
				return err
			}
		}
		if decision.Phase == finalWorkspaceRetirementContainerRetired {
			return nil
		}
		if decision.Phase != finalWorkspaceRetirementPrepared {
			return fmt.Errorf("final Workspace retirement phase is invalid")
		}
		if !force {
			state, _, err := r.observeFinalWorkspaceSessionOwner(ctx, workspace.ContextID, workspace.ID)
			if err != nil {
				return err
			}
			if state == FinalWorkspaceSessionLive {
				return tobari.ErrWorkspaceBindingProtected
			}
		}
		container, _, _ := tobari.ProjectResourceNames(string(workspace.ID))
		if err := r.removeExactFinalWorkspaceResource(ctx, "container", container, workspace.ID, projectWorkRole); err != nil {
			return err
		}
		if r.finalWorkspaceAfterContainerRetirement != nil {
			if err := r.finalWorkspaceAfterContainerRetirement(); err != nil {
				return err
			}
		}
		if force {
			// Removing the exact owned container ends the selected docker exec
			// child. The canonical owner then closes through its existing WP07
			// cleanup path; only exact expiry+socket absence may be compacted.
			if err := r.waitFinalWorkspaceSessionOwnerAbsent(ctx, workspace.ContextID, workspace.ID); err != nil {
				return err
			}
		}
		decision.Phase = finalWorkspaceRetirementContainerRetired
		if err := writeAtomicJSON(r.finalWorkspaceRetirementDecisionPath(), decision); err != nil {
			return err
		}
		return nil
	})
}

// CompleteWorkspaceRetirement is called after the final Gateway/principal/OPA
// settlement has removed the Workspace authority. It retires the now-detached
// network and owner-only home, then publishes the secret-free terminal receipt.
func (r *Runtime) CompleteWorkspaceRetirement(ctx context.Context, workspace tobari.WorkspaceBinding, force bool, decisionRef string) error {
	if err := validateFinalWorkspaceBinding(workspace); err != nil {
		return err
	}
	if err := validateFinalWorkspaceDecisionRef("workspace-retirement", workspace.ID, decisionRef); err != nil {
		return err
	}
	return r.withProjectLock(ctx, func() error {
		if record, present, err := r.readFinalWorkspaceRetirementRecord(); err != nil {
			return err
		} else if present {
			if err := r.confirmFinalWorkspaceRetired(ctx, record); err != nil {
				return err
			}
			if err := r.clearCompletedFinalWorkspaceRetirementDecision(record); err != nil {
				return err
			}
			if record.DecisionRef == decisionRef && reflect.DeepEqual(record.Workspace, workspace) {
				return nil
			}
		}
		decision, present, err := r.readFinalWorkspaceRetirementDecision()
		if err != nil {
			return err
		}
		if !present || decision.DecisionRef != decisionRef || decision.Force != force || decision.Phase != finalWorkspaceRetirementContainerRetired || !reflect.DeepEqual(decision.Workspace, workspace) {
			return fmt.Errorf("final Workspace retirement was not prepared exactly")
		}
		_, network, _ := tobari.ProjectResourceNames(string(workspace.ID))
		if err := r.removeExactFinalWorkspaceResource(ctx, "network", network, workspace.ID, projectNetRole); err != nil {
			return err
		}
		directory, _ := r.finalWorkspaceDirectory(workspace.ID)
		home, _ := r.finalWorkspaceHome(workspace.ID)
		if workspace.Home != home {
			return fmt.Errorf("final Workspace home crosses its stable identity")
		}
		if err := requirePrivateDirectory(directory); err == nil {
			if err := requirePrivateDirectory(home); err != nil {
				return fmt.Errorf("final Workspace home is unsafe during retirement: %w", err)
			}
			if err := os.RemoveAll(directory); err != nil {
				return fmt.Errorf("remove final Workspace home and private runtime state: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("final Workspace private state is unsafe during retirement: %w", err)
		}
		if r.finalWorkspaceAfterHomeRetirement != nil {
			if err := r.finalWorkspaceAfterHomeRetirement(); err != nil {
				return err
			}
		}
		if err := syncDirectoryIfPresent(filepath.Dir(directory)); err != nil {
			return err
		}
		if err := r.ensureFinalWorkspaceRuntimeRoot(); err != nil {
			return err
		}
		record := finalWorkspaceRetirementRecord{SchemaVersion: finalWorkspaceRuntimeSchema, DecisionRef: decisionRef, Workspace: workspace}
		if err := writeAtomicJSON(r.finalWorkspaceRetirementRecordPath(), record); err != nil {
			return err
		}
		if err := os.Remove(r.finalWorkspaceRetirementDecisionPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := syncDirectory(r.finalWorkspaceRuntimeRoot()); err != nil {
			return err
		}
		return r.confirmFinalWorkspaceRetired(ctx, record)
	})
}

func (r *Runtime) readFinalWorkspaceRetirementDecision() (finalWorkspaceRetirementDecision, bool, error) {
	var decision finalWorkspaceRetirementDecision
	if err := readStrictJSON(r.finalWorkspaceRetirementDecisionPath(), &decision); errors.Is(err, os.ErrNotExist) {
		return finalWorkspaceRetirementDecision{}, false, nil
	} else if err != nil {
		return finalWorkspaceRetirementDecision{}, false, err
	}
	if decision.SchemaVersion != finalWorkspaceRuntimeSchema || validateFinalWorkspaceBinding(decision.Workspace) != nil ||
		validateFinalWorkspaceDecisionRef("workspace-retirement", decision.Workspace.ID, decision.DecisionRef) != nil ||
		(decision.Phase != finalWorkspaceRetirementPrepared && decision.Phase != finalWorkspaceRetirementContainerRetired) {
		return finalWorkspaceRetirementDecision{}, false, fmt.Errorf("final Workspace retirement decision is invalid")
	}
	return decision, true, nil
}

func (r *Runtime) clearCompletedFinalWorkspaceRetirementDecision(record finalWorkspaceRetirementRecord) error {
	decision, present, err := r.readFinalWorkspaceRetirementDecision()
	if err != nil || !present {
		return err
	}
	if decision.DecisionRef != record.DecisionRef || !reflect.DeepEqual(decision.Workspace, record.Workspace) {
		return nil
	}
	if err := os.Remove(r.finalWorkspaceRetirementDecisionPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(r.finalWorkspaceRuntimeRoot())
}

func (r *Runtime) removeExactFinalWorkspaceResource(ctx context.Context, kind, name string, id tobari.WorkspaceID, role string) error {
	exists, err := r.projectResourceExists(ctx, kind, name)
	if err != nil {
		return err
	}
	if !exists {
		// This method is reachable only after the parent mutation decision is
		// durable and the exact preflight proved presence and ownership. Absence
		// here is therefore an idempotent forward-recovery consequence.
		return nil
	}
	if err := r.verifyOwnedProjectResource(ctx, kind, name, string(id), role); err != nil {
		return err
	}
	args := []string{"rm", "-f", name}
	if kind == "network" {
		args = []string{"network", "rm", name}
	}
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("remove final Workspace %s: %w: %s", kind, err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) ConfirmWorkspaceRetired(ctx context.Context, workspace tobari.WorkspaceBinding, decisionRef string) error {
	if err := validateFinalWorkspaceBinding(workspace); err != nil {
		return err
	}
	if err := validateFinalWorkspaceDecisionRef("workspace-retirement", workspace.ID, decisionRef); err != nil {
		return err
	}
	record, present, err := r.readFinalWorkspaceRetirementRecord()
	if err != nil {
		return err
	}
	if !present || record.DecisionRef != decisionRef || !reflect.DeepEqual(record.Workspace, workspace) {
		return fmt.Errorf("final Workspace retirement has no exact terminal receipt")
	}
	return r.confirmFinalWorkspaceRetired(ctx, record)
}

func (r *Runtime) readFinalWorkspaceRetirementRecord() (finalWorkspaceRetirementRecord, bool, error) {
	var record finalWorkspaceRetirementRecord
	if err := readStrictJSON(r.finalWorkspaceRetirementRecordPath(), &record); errors.Is(err, os.ErrNotExist) {
		return finalWorkspaceRetirementRecord{}, false, nil
	} else if err != nil {
		return finalWorkspaceRetirementRecord{}, false, err
	}
	if record.SchemaVersion != finalWorkspaceRuntimeSchema || validateFinalWorkspaceBinding(record.Workspace) != nil ||
		validateFinalWorkspaceDecisionRef("workspace-retirement", record.Workspace.ID, record.DecisionRef) != nil {
		return finalWorkspaceRetirementRecord{}, false, fmt.Errorf("final Workspace retirement receipt is invalid")
	}
	return record, true, nil
}

func (r *Runtime) confirmFinalWorkspaceRetired(ctx context.Context, record finalWorkspaceRetirementRecord) error {
	container, network, _ := tobari.ProjectResourceNames(string(record.Workspace.ID))
	for _, resource := range []struct{ kind, name string }{{"container", container}, {"network", network}} {
		exists, err := r.projectResourceExists(ctx, resource.kind, resource.name)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("final Workspace %s remains after retirement", resource.kind)
		}
	}
	directory, _ := r.finalWorkspaceDirectory(record.Workspace.ID)
	if _, err := os.Lstat(directory); err == nil {
		return fmt.Errorf("final Workspace private state remains after retirement")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
