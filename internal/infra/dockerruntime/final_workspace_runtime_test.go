package dockerruntime

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type finalWorkspacePlanningRunner struct{}

type finalWorkspacePreparationRunner struct {
	*lifecycleObservationRunner
	builds   [][]string
	buildErr error
}

func (r *finalWorkspacePreparationRunner) Run(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) >= 2 && args[0] == "buildx" && args[1] == "build" {
		r.builds = append(r.builds, append([]string{}, args...))
		return r.buildErr
	}
	return r.lifecycleObservationRunner.Run(ctx, args, environment, in, out, errOut)
}

func (finalWorkspacePlanningRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

type finalWorkspaceAllocatedNetworkRunner struct{}

func (finalWorkspaceAllocatedNetworkRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return errors.New("unexpected Docker mutation")
}

func (finalWorkspaceAllocatedNetworkRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if slices.Equal(args, []string{"network", "ls", "--quiet", "--no-trunc"}) {
		return []byte(strings.Repeat("a", 64)), nil
	}
	if len(args) >= 2 && args[0] == "network" && args[1] == "inspect" {
		return []byte(`[{"Subnet":"10.20.0.0/16","Gateway":"10.20.0.1"}]`), nil
	}
	return nil, errors.New("unexpected Docker observation")
}

func TestFinalWorkspaceSubnetInventoryAcceptsExactDockerGatewayEvidence(t *testing.T) {
	runtime := &Runtime{runner: finalWorkspaceAllocatedNetworkRunner{}}
	prefixes, err := runtime.observeBoundedDockerIPv4Subnets(context.Background())
	if err != nil {
		t.Fatalf("observe Docker subnet with exact Gateway evidence: %v", err)
	}
	if len(prefixes) != 1 || prefixes[0].String() != "10.20.0.0/16" {
		t.Fatalf("unexpected bounded subnet inventory: %v", prefixes)
	}
}

type finalWorkspaceRetirementRunner struct {
	workspaceID         tobari.WorkspaceID
	containerPresent    bool
	networkPresent      bool
	containerRemoveFail bool
	containerRemoves    int
	networkRemoves      int
	onContainerRemove   func()
}

type finalWorkspaceContainerCreateRunner struct {
	created     bool
	createArgs  []string
	network     string
	workspaceIP string
	gatewayIP   string
}

func (r *finalWorkspaceContainerCreateRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *finalWorkspaceContainerCreateRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("missing Docker argv")
	}
	if args[0] == "create" {
		r.created = true
		r.createArgs = append([]string{}, args...)
		return []byte("created"), nil
	}
	if args[0] == "start" {
		return []byte{}, nil
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "{{.Id}}") {
		if !r.created {
			return []byte("Error: No such container"), errors.New("not found")
		}
		return []byte(strings.Repeat("c", 64)), nil
	}
	if strings.Contains(joined, ".NetworkSettings.Networks") {
		return []byte(`{"` + r.network + `":{"IPAddress":"` + r.workspaceIP + `"}}`), nil
	}
	if strings.Contains(joined, ".HostConfig.Dns") {
		return []byte(`["` + r.gatewayIP + `"]`), nil
	}
	return nil, errors.New("unexpected Docker observation")
}

func (r *finalWorkspaceRetirementRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *finalWorkspaceRetirementRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("missing Docker argv")
	}
	if args[0] == "rm" && len(args) == 3 {
		r.containerRemoves++
		r.containerPresent = false
		if r.onContainerRemove != nil {
			r.onContainerRemove()
		}
		if r.containerRemoveFail {
			r.containerRemoveFail = false
			return []byte("interrupted after remove"), errors.New("interrupted")
		}
		return []byte{}, nil
	}
	if args[0] == "network" && len(args) == 3 && args[1] == "rm" {
		r.networkRemoves++
		r.networkPresent = false
		return []byte{}, nil
	}
	kind := "container"
	present := r.containerPresent
	if args[0] == "network" {
		kind = "network"
		present = r.networkPresent
	}
	if !present {
		return []byte("Error: No such " + kind), errors.New("not found")
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, ownerLabel):
		return []byte(ownerValue), nil
	case strings.Contains(joined, projectIDLabel):
		return []byte(r.workspaceID), nil
	case strings.Contains(joined, projectRoleLabel):
		if kind == "network" {
			return []byte(projectNetRole), nil
		}
		return []byte(projectWorkRole), nil
	default:
		return []byte(strings.Repeat("a", 64)), nil
	}
}

func (finalWorkspacePlanningRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if slices.Equal(args, []string{"network", "ls", "--quiet", "--no-trunc"}) {
		return []byte{}, nil
	}
	if len(args) >= 2 && args[0] == "network" && args[1] == "inspect" {
		return []byte("Error: No such network"), errors.New("not found")
	}
	return nil, errors.New("unexpected Docker observation")
}

func TestFinalWorkspacePreparationBuildsOnlyCanonicalStandardRuntime(t *testing.T) {
	root := t.TempDir()
	observations := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runner := &finalWorkspacePreparationRunner{lifecycleObservationRunner: observations}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	bindEmptyFinalRuntimeProtection(t, runtime)
	image, err := runtimeassets.StandardRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: image, buildRuntime: true}
	manifest, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := manifest.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareWorkspaceRuntimeMaterial(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	if len(runner.builds) != 1 || !containsArgs(runner.builds[0], image) || !slices.Equal(runner.builds[0][:4], []string{"buildx", "build", "--progress=plain", "--load"}) {
		t.Fatalf("standard Runtime build calls=%v", runner.builds)
	}

	managedID := "018bcfe5-687b-7000-8000-000000000077"
	managed := installRuntimeLifecycleRevision(t, runtime, managedID, "custom", "sha256:"+strings.Repeat("b", 64), "FROM example.invalid/runtime\n")
	managedRevision := managed.Revisions[0]
	observations.images[managedRevision.Image] = lifecycleImageFixture{observation: managedLifecycleImage(managedID, managedRevision.Revision, managedRevision.Image)}
	managedBinding, err := managed.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareWorkspaceRuntimeMaterial(context.Background(), managedBinding); err != nil {
		t.Fatal(err)
	}
	if len(runner.builds) != 1 {
		t.Fatalf("custom Runtime triggered implicit build: %v", runner.builds)
	}
}

func TestFinalWorkspacePreparationClassifiesOnlyAttemptedStandardBuildAsUncertain(t *testing.T) {
	root := t.TempDir()
	observations := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runner := &finalWorkspacePreparationRunner{lifecycleObservationRunner: observations, buildErr: errors.New("synthetic BuildKit failure")}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	bindEmptyFinalRuntimeProtection(t, runtime)
	image, err := runtimeassets.StandardRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: image, buildRuntime: true}
	manifest, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := manifest.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareWorkspaceRuntimeMaterial(context.Background(), binding); !errors.Is(err, tobari.ErrWorkspaceRuntimePreparationUncertain) {
		t.Fatalf("standard Runtime build classification=%v", err)
	}
	if len(runner.builds) != 1 {
		t.Fatalf("standard Runtime build calls=%v", runner.builds)
	}
}

func finalWorkspaceRuntimeFixture(t *testing.T) (*Runtime, tobari.ContextAuthoritySnapshot, tobari.WorkspaceEntryReconciliationPlan) {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), finalWorkspacePlanningRunner{})
	if err != nil {
		t.Fatal(err)
	}
	projectRoot, err := filepath.EvalSymlinks(filepath.Join(root, "project"))
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(filepath.Join(root, "project"), 0o700); err != nil {
			t.Fatal(err)
		}
		projectRoot, err = filepath.EvalSymlinks(filepath.Join(root, "project"))
	}
	if err != nil {
		t.Fatal(err)
	}
	collection := finalProjectionCollectionFixture(t, "")
	record := collection.Contexts[0].Clone()
	record.Context.ProjectRoot = projectRoot
	snapshot := tobari.ContextAuthoritySnapshot{
		Context: record.Context, Template: collection.Templates[0].Clone(), PolicyMemory: record.PolicyMemory.Clone(),
		ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
		ActivePolicyMemoryRef: record.ActivePolicyMemoryRef,
	}
	imageID := "sha256:" + strings.Repeat("a", 64)
	runtime.finalWorkspaceRuntimeMaterial = func(_ context.Context, expected tobari.RuntimeBinding) (tobari.RuntimeBinding, string, string, error) {
		return expected, expected.Image, imageID, nil
	}
	version, err := runtimeassets.Version()
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"browser/tobari-open", "helpers/tobari-expose", "helpers/tobari-permission"} {
		path := filepath.Join(runtime.stateDirectory, "runtime", version, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	authority, err := tobari.DeriveWorkspaceTemplateEntryAuthority(snapshot.Template.Current)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := runtime.PlanWorkspaceEntry(context.Background(), snapshot, authority, finalProjectionWorkspaceA, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return runtime, snapshot, plan
}

func TestFinalWorkspaceEntryBindsDistinctStaticNetworkAndReplaysReceipt(t *testing.T) {
	runtime, _, plan := finalWorkspaceRuntimeFixture(t)
	if plan.Network.DockerGateway == plan.Network.GatewayIP || plan.Network.GatewayIP == plan.Network.WorkspaceIP || plan.Network.WorkspaceIP == plan.Network.DockerGateway {
		t.Fatalf("network authority is not distinct: %+v", plan.Network)
	}
	containerID := strings.Repeat("c", 64)
	reconciles := 0
	runtime.finalWorkspaceDockerReconcile = func(_ context.Context, got tobari.WorkspaceEntryReconciliationPlan, spec finalWorkspaceRuntimeSpec) (string, error) {
		reconciles++
		if got.Network != plan.Network || spec.Network != plan.Network {
			t.Fatalf("runtime topology changed: plan=%+v spec=%+v", got.Network, spec.Network)
		}
		return containerID, nil
	}
	runtime.finalWorkspaceDockerObserve = func(_ context.Context, got tobari.WorkspaceEntryReconciliationPlan) (finalWorkspaceContainerObservation, error) {
		return finalWorkspaceContainerObservation{ID: containerID, Owner: ownerValue, Component: "tobari", Workspace: string(got.Workspace.ID), Role: projectWorkRole, Spec: string(got.Applied.ResolvedSpec), Running: true, Health: "healthy"}, nil
	}
	decision := "workspace-entry:" + string(plan.Workspace.ID) + ":sha256:" + strings.Repeat("d", 64)
	first, err := runtime.ReconcileWorkspaceEntry(context.Background(), plan, decision)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.ReconcileWorkspaceEntry(context.Background(), plan, decision)
	if err != nil || first != second || reconciles != 1 {
		t.Fatalf("replay receipt=%+v err=%v effects=%d", second, err, reconciles)
	}
	if _, err := os.Lstat(filepath.Join(runtime.stateDirectory, "instances")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final entry wrote predecessor instance state: %v", err)
	}
}

func TestFinalWorkspaceContainerUsesDurableWorkspaceIPAndGatewayDNS(t *testing.T) {
	runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
	runner := &finalWorkspaceContainerCreateRunner{network: plan.Network.Network, workspaceIP: plan.Network.WorkspaceIP, gatewayIP: plan.Network.GatewayIP}
	runtime.runner = runner
	gitConfig, err := runtime.finalWorkspaceGitConfig(context.Background(), plan.Authority.SessionDefaults.GitIdentity, plan.Workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := runtime.finalWorkspaceSpec(plan.Authority, plan.CreationDefaults, plan.Network, snapshot.Context, plan.Workspace.ID, plan.Authority.Runtime.Image, "sha256:"+strings.Repeat("a", 64), gitConfig)
	if err != nil {
		t.Fatal(err)
	}
	spec.AuthEnvironment = []string{"SYNTHETIC_TOKEN=tobari-h1_" + strings.Repeat("A", 43)}
	container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
	if err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP); err != nil {
		t.Fatal(err)
	}
	for _, exact := range [][]string{{"--ip", plan.Network.WorkspaceIP}, {"--dns", plan.Network.GatewayIP}} {
		found := false
		for index := 0; index+1 < len(runner.createArgs); index++ {
			if runner.createArgs[index] == exact[0] && runner.createArgs[index+1] == exact[1] {
				found = true
			}
		}
		if !found {
			t.Fatalf("create args omit exact topology %v: %v", exact, runner.createArgs)
		}
	}
	if !slices.Contains(runner.createArgs, "SYNTHETIC_TOKEN=tobari-h1_"+strings.Repeat("A", 43)) {
		t.Fatalf("create args omit exact research authentication projection: %v", runner.createArgs)
	}
	if !slices.Contains(runner.createArgs, spec.ImageSelector) {
		t.Fatalf("create args omit exact managed Runtime selector: %v", runner.createArgs)
	}
}

func TestFinalWorkspaceEntryRuntimeDriftFailsBeforeHomeOrDockerEffect(t *testing.T) {
	runtime, _, plan := finalWorkspaceRuntimeFixture(t)
	runtime.finalWorkspaceRuntimeMaterial = func(_ context.Context, expected tobari.RuntimeBinding) (tobari.RuntimeBinding, string, string, error) {
		return expected, expected.Image, "sha256:" + strings.Repeat("b", 64), nil
	}
	effects := 0
	runtime.finalWorkspaceDockerReconcile = func(context.Context, tobari.WorkspaceEntryReconciliationPlan, finalWorkspaceRuntimeSpec) (string, error) {
		effects++
		return strings.Repeat("c", 64), nil
	}
	decision := "workspace-entry:" + string(plan.Workspace.ID) + ":sha256:" + strings.Repeat("e", 64)
	if _, err := runtime.ReconcileWorkspaceEntry(context.Background(), plan, decision); err == nil {
		t.Fatal("changed Runtime material was accepted")
	}
	directory, _ := runtime.finalWorkspaceDirectory(plan.Workspace.ID)
	if effects != 0 {
		t.Fatalf("Docker effects=%d", effects)
	}
	if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drift created final Workspace state: %v", err)
	}
}

func TestFinalWorkspaceEntryRetainsExistingCreationDefaultsAcrossTemplateAdvance(t *testing.T) {
	runtime, snapshot, _ := finalWorkspaceRuntimeFixture(t)
	revisionA := snapshot.Template.Current.Clone()
	bodyB := revisionA.Body.Clone()
	bootstrap, err := tobari.NewContextBootstrapSnapshot(1, tobari.ManifestAWSBootstrap{
		Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
		SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012",
		RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	bodyB.CreationDefaults.Bootstrap = &bootstrap
	revisionB, err := tobari.NewWorkspaceTemplateRevision(snapshot.Template.ID, 2, bodyB)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Template.Current = revisionB
	snapshot.Template.Retained = append(snapshot.Template.Retained, revisionB.Clone())
	home, err := runtime.finalWorkspaceHome(finalProjectionWorkspaceA)
	if err != nil {
		t.Fatal(err)
	}
	previous := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateRevision: revisionA.Revision,
		EntrySliceDigest: revisionA.Slices.EntrySliceDigest, RuntimeID: revisionA.Slices.RuntimeID,
		RuntimeRevision: revisionA.Slices.RuntimeRevision, ResolvedSpec: finalSessionDigest("7"), ReconciledAt: time.Unix(5, 0).UTC(),
	}
	snapshot.Workspace = &tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: finalProjectionWorkspaceA, ContextID: snapshot.Context.ID,
		ProjectRoot: snapshot.Context.ProjectRoot, Home: home, CreationDefaults: revisionA.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &previous,
	}
	authorityB, err := tobari.DeriveWorkspaceTemplateEntryAuthority(revisionB)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := runtime.PlanWorkspaceEntry(context.Background(), snapshot, authorityB, finalProjectionWorkspaceA, time.Unix(11, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if existing.CreationDefaults.Bootstrap != nil || existing.Authority.CreationDefaults.Bootstrap == nil || existing.Workspace.CreationDefaults != revisionA.Slices.CreationDefaultsDigest {
		t.Fatalf("existing Workspace creation authority changed: plan=%+v", existing)
	}
	snapshot.Workspace = nil
	created, err := runtime.PlanWorkspaceEntry(context.Background(), snapshot, authorityB, finalProjectionWorkspaceB, time.Unix(12, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if created.CreationDefaults.Bootstrap == nil || created.Workspace.CreationDefaults != revisionB.Slices.CreationDefaultsDigest {
		t.Fatalf("new Workspace did not consume current creation authority: plan=%+v", created)
	}
}

func preparedFinalWorkspaceForRetirement(t *testing.T) (*Runtime, tobari.WorkspaceBinding, *finalWorkspaceRetirementRunner) {
	t.Helper()
	runtime, _, plan := finalWorkspaceRuntimeFixture(t)
	containerID := strings.Repeat("c", 64)
	runtime.finalWorkspaceDockerReconcile = func(context.Context, tobari.WorkspaceEntryReconciliationPlan, finalWorkspaceRuntimeSpec) (string, error) {
		return containerID, nil
	}
	runtime.finalWorkspaceDockerObserve = func(_ context.Context, got tobari.WorkspaceEntryReconciliationPlan) (finalWorkspaceContainerObservation, error) {
		return finalWorkspaceContainerObservation{ID: containerID, Owner: ownerValue, Component: "tobari", Workspace: string(got.Workspace.ID), Role: projectWorkRole, Spec: string(got.Applied.ResolvedSpec), Running: true, Health: "healthy"}, nil
	}
	decision := "workspace-entry:" + string(plan.Workspace.ID) + ":sha256:" + strings.Repeat("d", 64)
	if _, err := runtime.ReconcileWorkspaceEntry(context.Background(), plan, decision); err != nil {
		t.Fatal(err)
	}
	runner := &finalWorkspaceRetirementRunner{workspaceID: plan.Workspace.ID, containerPresent: true, networkPresent: true}
	runtime.runner = runner
	return runtime, plan.Workspace, runner
}

func TestFinalWorkspaceRetirementRecoversExactContainerAndHomeAbsence(t *testing.T) {
	t.Run("container removal", func(t *testing.T) {
		runtime, workspace, runner := preparedFinalWorkspaceForRetirement(t)
		runner.containerRemoveFail = true
		decision := "workspace-retirement:" + string(workspace.ID) + ":sha256:" + strings.Repeat("e", 64)
		if err := runtime.ConfirmWorkspaceRetirementAllowed(context.Background(), workspace, false); err != nil {
			t.Fatal(err)
		}
		if err := runtime.PrepareWorkspaceRetirement(context.Background(), workspace, false, decision); err == nil || runner.containerPresent {
			t.Fatalf("container interruption err=%v present=%v", err, runner.containerPresent)
		}
		if err := runtime.PrepareWorkspaceRetirement(context.Background(), workspace, false, decision); err != nil {
			t.Fatal(err)
		}
		if err := runtime.CompleteWorkspaceRetirement(context.Background(), workspace, false, decision); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmWorkspaceRetired(context.Background(), workspace, decision); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("home removal", func(t *testing.T) {
		runtime, workspace, _ := preparedFinalWorkspaceForRetirement(t)
		decision := "workspace-retirement:" + string(workspace.ID) + ":sha256:" + strings.Repeat("f", 64)
		if err := runtime.PrepareWorkspaceRetirement(context.Background(), workspace, false, decision); err != nil {
			t.Fatal(err)
		}
		interrupt := true
		runtime.finalWorkspaceAfterHomeRetirement = func() error {
			if interrupt {
				interrupt = false
				return errors.New("interrupted after home removal")
			}
			return nil
		}
		if err := runtime.CompleteWorkspaceRetirement(context.Background(), workspace, false, decision); err == nil {
			t.Fatal("home interruption was not returned")
		}
		if err := runtime.CompleteWorkspaceRetirement(context.Background(), workspace, false, decision); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmWorkspaceRetired(context.Background(), workspace, decision); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFinalWorkspaceRetirementForceDoesNotAcceptPreDecisionAbsence(t *testing.T) {
	runtime, workspace, runner := preparedFinalWorkspaceForRetirement(t)
	runner.containerPresent = false
	if err := runtime.ConfirmWorkspaceRetirementAllowed(context.Background(), workspace, true); err == nil {
		t.Fatal("force accepted a missing container before the durable decision")
	}
	if runner.containerRemoves != 0 || runner.networkRemoves != 0 {
		t.Fatalf("preflight effects container=%d network=%d", runner.containerRemoves, runner.networkRemoves)
	}
}

func TestFinalWorkspaceForceRetiresOnlyTargetCanonicalOwnerAfterContainer(t *testing.T) {
	runtime, workspace, runner := preparedFinalWorkspaceForRetirement(t)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	target := tobari.InteractiveAttachmentSession{
		SchemaVersion: tobari.PermissionSessionSchema, WorkspaceManifestID: string(workspace.ContextID), WorkspaceID: string(workspace.ID),
		AttachmentID: "att_" + strings.Repeat("a", 32), OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: strings.Repeat("a", 64), OwnerPID: os.Getpid(),
		IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: "pws_" + strings.Repeat("a", 32) + ".sock", IngestionNonce: strings.Repeat("a", 64),
		CreatedAt: now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	unrelated := target
	unrelated.WorkspaceManifestID = "01912345-6789-7abc-8def-0123456789d1"
	unrelated.WorkspaceID = "01912345-6789-7abc-8def-0123456789d2"
	unrelated.AttachmentID = "att_" + strings.Repeat("b", 32)
	unrelated.FrozenPrincipalFingerprint = strings.Repeat("b", 64)
	unrelated.IngestionEndpoint = "pws_" + strings.Repeat("b", 32) + ".sock"
	unrelated.IngestionNonce = strings.Repeat("b", 64)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: runtime.interactiveAttachmentSocketPath(target), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtime.interactiveAttachmentSocketPath(target), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			request := make([]byte, permissionSessionHandshake)
			_, _ = io.ReadFull(connection, request)
			if len(request) == permissionSessionHandshake && request[0] == 'S' {
				_, _ = connection.Write([]byte("OK"))
			}
			_ = connection.Close()
		}
	}()
	registry := tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{target, unrelated}}
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), registry); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmWorkspaceRetirementAllowed(context.Background(), workspace, false); !errors.Is(err, tobari.ErrWorkspaceBindingProtected) {
		t.Fatalf("ordinary delete live owner error=%v", err)
	}
	if err := runtime.ConfirmWorkspaceRetirementAllowed(context.Background(), workspace, true); err != nil {
		t.Fatalf("force preflight: %v", err)
	}
	var ordering []string
	runner.onContainerRemove = func() {
		if runner.containerPresent {
			t.Error("target container was still present at owner cleanup boundary")
		}
		ordering = append(ordering, "container_absent")
		_ = listener.Close()
		<-done
		if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
			SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{unrelated},
		}); err != nil {
			t.Errorf("publish target owner cleanup: %v", err)
		}
		ordering = append(ordering, "owner_absent")
	}
	decision := "workspace-retirement:" + string(workspace.ID) + ":sha256:" + strings.Repeat("9", 64)
	if err := runtime.PrepareWorkspaceRetirement(context.Background(), workspace, true, decision); err != nil {
		t.Fatal(err)
	}
	var remaining tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &remaining); err != nil || len(remaining.Sessions) != 1 || !remaining.Sessions[0].SameAuthority(unrelated) {
		t.Fatalf("unrelated owner was changed: %+v err=%v", remaining, err)
	}
	if runner.containerPresent {
		t.Fatal("target container remained before settlement")
	}
	if !runner.networkPresent {
		t.Fatal("pre-settlement Prepare retired the target network")
	}
	home, _ := runtime.finalWorkspaceHome(workspace.ID)
	if err := requirePrivateDirectory(home); err != nil {
		t.Fatalf("pre-settlement Prepare retired the Workspace home: %v", err)
	}
	if !slices.Equal(ordering, []string{"container_absent", "owner_absent"}) {
		t.Fatalf("force preparation ordering=%v", ordering)
	}
}
