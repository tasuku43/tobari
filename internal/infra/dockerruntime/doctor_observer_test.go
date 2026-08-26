package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/doctorcmd"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type doctorObserverRunner struct {
	engineDown bool
	outputs    [][]string
	runs       [][]string
}

func (r *doctorObserverRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string{}, args...))
	switch {
	case len(args) > 0 && args[0] == "version":
		if r.engineDown {
			return nil, errors.New("synthetic engine unavailable")
		}
		return []byte("27.0.0\n"), nil
	case len(args) > 0 && args[0] == "context":
		return []byte("synthetic\n"), nil
	case len(args) > 0 && args[0] == "compose":
		return []byte("v2.29.0\n"), nil
	case len(args) > 0 && args[0] == "ps":
		return nil, nil
	default:
		return nil, errors.New("unexpected doctor Docker observation")
	}
}

func (r *doctorObserverRunner) Run(
	_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer,
) error {
	r.runs = append(r.runs, append([]string{}, args...))
	return errors.New("unexpected doctor Docker process")
}

func TestDoctorObserverDependencyMatrixAvoidsDockerFalseBlame(t *testing.T) {
	tests := map[string]struct {
		dockerCLI  bool
		engineDown bool
		want       map[doctor.CheckID]doctor.CheckStatus
		blockedBy  map[doctor.CheckID]doctor.CheckID
	}{
		"Docker CLI missing": {
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDDockerCLI: doctor.CheckStatusFail,
				doctor.CheckIDState:     doctor.CheckStatusWarn,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDDockerEngine:       doctor.CheckIDDockerCLI,
				doctor.CheckIDDockerContext:      doctor.CheckIDDockerCLI,
				doctor.CheckIDDockerCompose:      doctor.CheckIDDockerCLI,
				doctor.CheckIDPolicy:             doctor.CheckIDDockerEngine,
				doctor.CheckIDAuthBroker:         doctor.CheckIDState,
				doctor.CheckIDAuthVaultIntegrity: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles: doctor.CheckIDAuthVaultIntegrity,
				doctor.CheckIDOwnedResources:     doctor.CheckIDDockerEngine,
			},
		},
		"Docker Engine down": {
			dockerCLI: true, engineDown: true,
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDDockerCLI:     doctor.CheckStatusPass,
				doctor.CheckIDDockerEngine:  doctor.CheckStatusFail,
				doctor.CheckIDDockerContext: doctor.CheckStatusPass,
				doctor.CheckIDDockerCompose: doctor.CheckStatusPass,
				doctor.CheckIDState:         doctor.CheckStatusWarn,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDPolicy:             doctor.CheckIDDockerEngine,
				doctor.CheckIDAuthBroker:         doctor.CheckIDState,
				doctor.CheckIDAuthVaultIntegrity: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles: doctor.CheckIDAuthVaultIntegrity,
				doctor.CheckIDOwnedResources:     doctor.CheckIDDockerEngine,
			},
		},
		"cluster absent": {
			dockerCLI: true,
			want: map[doctor.CheckID]doctor.CheckStatus{
				doctor.CheckIDDockerCLI:      doctor.CheckStatusPass,
				doctor.CheckIDDockerEngine:   doctor.CheckStatusPass,
				doctor.CheckIDDockerContext:  doctor.CheckStatusPass,
				doctor.CheckIDDockerCompose:  doctor.CheckStatusPass,
				doctor.CheckIDState:          doctor.CheckStatusWarn,
				doctor.CheckIDPolicy:         doctor.CheckStatusWarn,
				doctor.CheckIDPolicyData:     doctor.CheckStatusWarn,
				doctor.CheckIDOwnedResources: doctor.CheckStatusPass,
			},
			blockedBy: map[doctor.CheckID]doctor.CheckID{
				doctor.CheckIDAuthBroker:         doctor.CheckIDState,
				doctor.CheckIDAuthVaultIntegrity: doctor.CheckIDAuthBroker,
				doctor.CheckIDAuthProjectHandles: doctor.CheckIDAuthVaultIntegrity,
			},
		},
	}
	if !capabilitysurface.Compiled().IncludesResearch() {
		for _, test := range tests {
			for id := range test.blockedBy {
				if isResearchDoctorCheck(id) {
					delete(test.blockedBy, id)
				}
			}
		}
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixtureRoot := t.TempDir()
			binDirectory := filepath.Join(fixtureRoot, "bin")
			if err := os.Mkdir(binDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			if test.dockerCLI {
				dockerPath := filepath.Join(binDirectory, "docker")
				if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", binDirectory)
			runner := &doctorObserverRunner{engineDown: test.engineDown}
			runtime, err := newRuntimeWithData(
				filepath.Join(fixtureRoot, "config"), filepath.Join(fixtureRoot, "state"),
				filepath.Join(fixtureRoot, "data"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			projectRoot := t.TempDir()
			report, err := doctorcmd.New(runtime).Run(
				context.Background(), operation.Intent{Command: "doctor", Effect: operation.EffectRead}, projectRoot,
			)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if err := report.Validate(); err != nil {
				t.Fatalf("report invalid: %v", err)
			}
			for id, status := range test.want {
				check := doctorObserverCheck(t, report, id)
				if check.Status != status {
					t.Errorf("%s status = %q, want %q (%s)", id, check.Status, status, check.Detail)
				}
			}
			for id, blocker := range test.blockedBy {
				check := doctorObserverCheck(t, report, id)
				if check.Status != doctor.CheckStatusBlocked || check.BlockedBy == nil || *check.BlockedBy != blocker {
					t.Errorf("%s = %+v, want blocked by %s", id, check, blocker)
				}
			}
			if len(runner.runs) != 0 {
				t.Fatalf("doctor crossed a Docker process/mutation boundary: %v", runner.runs)
			}
			for _, call := range runner.outputs {
				if !doctorDockerObservationAllowed(call) {
					t.Fatalf("doctor issued non-observational Docker argv: %v", call)
				}
			}
			if !test.dockerCLI && len(runner.outputs) != 0 {
				t.Fatalf("missing Docker CLI still caused Docker calls: %v", runner.outputs)
			}
		})
	}
}

func TestWorkspaceStartReadinessUsesOnlyExactGenericDockerReads(t *testing.T) {
	fixtureRoot := t.TempDir()
	installFakeDocker(t, fixtureRoot)
	runner := &doctorObserverRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(fixtureRoot, "config"), filepath.Join(fixtureRoot, "state"),
		filepath.Join(fixtureRoot, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	checks, _ := doctor.ReadinessChecks(doctor.ReadinessProfileWorkspaceStart)
	for _, id := range checks {
		observation, err := runtime.ObserveDoctorCheck(context.Background(), "", id)
		if err != nil || observation.Status != doctor.CheckStatusPass {
			t.Fatalf("readiness %s = %+v, %v", id, observation, err)
		}
	}
	want := [][]string{
		{"version", "--format", "{{.Server.Version}}"},
		{"context", "show"},
		{"compose", "version", "--short"},
	}
	if !reflect.DeepEqual(runner.outputs, want) || len(runner.runs) != 0 {
		t.Fatalf("readiness Docker boundaries = outputs %v runs %v", runner.outputs, runner.runs)
	}
	joined := strings.ToLower(fmt.Sprint(runner.outputs))
	for _, forbidden := range []string{"colima", "lima", "docker desktop", "rancher", "open", "systemctl", "launchctl", "socket"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("readiness inferred or managed a provider through %q: %v", forbidden, runner.outputs)
		}
	}
}

func TestDoctorObserverKeepsInvalidPolicyDistinctFromPolicyData(t *testing.T) {
	fixtureRoot := t.TempDir()
	installFakeDocker(t, fixtureRoot)
	runner := &doctorObserverRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(fixtureRoot, "config"), filepath.Join(fixtureRoot, "state"),
		filepath.Join(fixtureRoot, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "broken", tobari.BuiltinImageSelector, tobari.ManifestPolicyModeAdvanced, tobari.ManifestSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext("broken")
	if err != nil {
		t.Fatal(err)
	}
	invalidSource := "package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { data.tobari_contexts; input.schema_version == 3; input.schema_version == 4 }\n"
	if err := os.WriteFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), []byte(invalidSource), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := runRuntimeDoctor(context.Background(), runtime, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	policy := doctorObserverCheck(t, report, doctor.CheckIDPolicy)
	if policy.Status != doctor.CheckStatusFail || policy.Recovery == nil || policy.Recovery.NextCommand != "doctor" {
		t.Fatalf("policy = %+v, want independent failure with doctor recovery", policy)
	}
	if policyData := doctorObserverCheck(t, report, doctor.CheckIDPolicyData); policyData.Status != doctor.CheckStatusWarn || policyData.BlockedBy != nil {
		t.Fatalf("policy_data = %+v, want independent not-initialized warning", policyData)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("doctor used mutation/process runner: %v", runner.runs)
	}
	for _, call := range runner.outputs {
		if !doctorDockerObservationAllowed(call) {
			t.Fatalf("doctor issued non-observational Docker argv: %v", call)
		}
	}
}

func TestDoctorObserverBrokerLockedBlocksOnlyDependents(t *testing.T) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		t.Skip("research-surface doctor checks are not part of the release inventory")
	}
	installFakeDocker(t, t.TempDir())
	runner := &authDoctorRunner{brokerState: "locked"}
	fixture := newAuthDoctorFixture(t, runner)
	writeConfiguredDoctorState(t, fixture.runtime)
	report, err := runRuntimeDoctor(context.Background(), fixture.runtime, fixture.project.Root)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	broker := doctorObserverCheck(t, report, doctor.CheckIDAuthBroker)
	if broker.Status != doctor.CheckStatusFail || broker.Recovery == nil || broker.Recovery.NextCommand != "cluster up" {
		t.Fatalf("auth_broker = %+v, want locked failure with cluster recovery", broker)
	}
	for _, id := range []doctor.CheckID{
		doctor.CheckIDAuthVaultIntegrity, doctor.CheckIDAuthProjectHandles,
	} {
		check := doctorObserverCheck(t, report, id)
		if check.Status != doctor.CheckStatusBlocked || check.Recovery != nil {
			t.Fatalf("%s = %+v, want blocked without duplicate recovery", id, check)
		}
	}
	if provider := doctorObserverCheck(t, report, doctor.CheckIDAuthProviderManifests); provider.Status != doctor.CheckStatusPass {
		t.Fatalf("provider manifests did not continue independently: %+v", provider)
	}
	if calls := authDoctorControlCalls(runner, "status"); len(calls) != 0 {
		t.Fatalf("locked broker caused dependent status calls: %v", calls)
	}
}

func TestDoctorObserverHealthyWarningsRemainHealthy(t *testing.T) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		t.Skip("research-surface doctor checks are not part of the release inventory")
	}
	installFakeDocker(t, t.TempDir())
	runner := &authDoctorRunner{brokerState: "ready"}
	fixture := newAuthDoctorFixture(t, runner)
	fixture.writeRegistry(t, "revision_stale", fixture.digest)
	writeConfiguredDoctorState(t, fixture.runtime)
	report, err := runRuntimeDoctor(context.Background(), fixture.runtime, fixture.project.Root)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !report.Healthy() {
		for _, check := range report.Checks {
			if check.Status == doctor.CheckStatusFail {
				t.Errorf("unexpected failed warning fixture check: %+v", check)
			}
		}
		t.Fatal("warning fixture is unhealthy")
	}
	for _, id := range []doctor.CheckID{doctor.CheckIDRootSharing, doctor.CheckIDAuthProjectHandles} {
		if check := doctorObserverCheck(t, report, id); check.Status != doctor.CheckStatusWarn {
			t.Fatalf("%s = %+v, want warning", id, check)
		}
	}
	for _, check := range report.Checks {
		if check.Status == doctor.CheckStatusBlocked {
			t.Fatalf("healthy warning fixture unexpectedly blocked %s by %v", check.Name, check.BlockedBy)
		}
	}
}

func TestDoctorObserverFreshTreeIsExactlyReadOnly(t *testing.T) {
	fixtureRoot := t.TempDir()
	binDirectory := filepath.Join(t.TempDir(), "empty-bin")
	if err := os.Mkdir(binDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
	runner := &doctorObserverRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(fixtureRoot, "config"), filepath.Join(fixtureRoot, "state"),
		filepath.Join(fixtureRoot, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	before := doctorTreeSnapshot(t, fixtureRoot)
	report, err := runRuntimeDoctor(context.Background(), runtime, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	after := doctorTreeSnapshot(t, fixtureRoot)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("fresh doctor changed owned tree\nbefore=%v\nafter=%v", before, after)
	}
	for _, path := range []string{runtime.configDirectory, runtime.stateDirectory, runtime.dataDirectory} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("doctor created owned path %q: %v", path, err)
		}
	}
	if len(report.Checks) != len(doctor.CheckInventory()) || len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("fresh report/calls = %d/%v/%v", len(report.Checks), runner.outputs, runner.runs)
	}
}

func TestDoctorObserverRejectsUnknownCheckAndHonorsCancellation(t *testing.T) {
	runner := &doctorObserverRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"),
		filepath.Join(t.TempDir(), "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ObserveDoctorCheck(context.Background(), ".", doctor.CheckID("unknown")); err == nil {
		t.Fatal("unknown check succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.ObserveDoctorCheck(ctx, ".", doctor.CheckIDDockerEngine); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled observation error = %v", err)
	}
	if _, err := runtime.ObserveDoctorCheck(nil, ".", doctor.CheckIDDockerEngine); err == nil {
		t.Fatal("nil context observation succeeded")
	}
	if len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("rejected observations crossed runner: %v %v", runner.outputs, runner.runs)
	}
}

func installFakeDocker(t *testing.T, root string) {
	t.Helper()
	binDirectory := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDirectory, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
}

func isResearchDoctorCheck(id doctor.CheckID) bool {
	switch id {
	case doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths,
		doctor.CheckIDAuthRootKey, doctor.CheckIDAuthBroker,
		doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity,
		doctor.CheckIDAuthProjectHandles:
		return true
	default:
		return false
	}
}

func doctorDockerObservationAllowed(arguments []string) bool {
	if len(arguments) == 0 {
		return false
	}
	switch arguments[0] {
	case "version", "context", "compose", "ps":
		return true
	default:
		return false
	}
}

func writeConfiguredDoctorState(t *testing.T, runtime *Runtime) {
	t.Helper()
	state := runtimeState(t.TempDir())
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
}

func doctorTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	items := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		identity := ""
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			identity, err = os.Readlink(path)
		case info.Mode().IsRegular():
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			digest := sha256.Sum256(data)
			identity = hex.EncodeToString(digest[:])
		}
		if err != nil {
			return err
		}
		items = append(items, strings.Join([]string{relative, info.Mode().String(), fmt.Sprint(info.Size()), identity}, "|"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return items
}

func doctorObserverCheck(t *testing.T, report doctor.Report, id doctor.CheckID) doctor.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == id {
			return check
		}
	}
	t.Fatalf("report lacks %s", id)
	return doctor.Check{}
}

func runRuntimeDoctor(ctx context.Context, runtime *Runtime, root string) (doctor.Report, error) {
	return doctorcmd.New(runtime).Run(
		ctx, operation.Intent{Command: "doctor", Effect: operation.EffectRead}, root,
	)
}
