package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	authDoctorRevision = "revision_doctor"
	authDoctorCanary   = "synthetic-auth-doctor-secret-canary"
)

type authDoctorRunner struct {
	brokerState    string
	companionState string
	bindingState   string
	bindingFrame   string
	controlCalls   [][]string
	inspectionErr  error
	inspectionData string
}

func (r *authDoctorRunner) Run(
	_ context.Context, args []string, _ []string, _ io.Reader, stdout, _ io.Writer,
) error {
	if !slices.Contains(args, "authbroker.control") {
		return nil
	}
	r.controlCalls = append(r.controlCalls, append([]string(nil), args...))
	switch {
	case slices.Contains(args, "health"):
		state := "unlocked"
		if r.brokerState == "locked" {
			state = "locked"
		}
		_, _ = io.WriteString(stdout, `{"schema_version":1,"ok":true,"state":"`+state+`"}`+"\n")
	case slices.Contains(args, "companion_status"):
		state := r.companionState
		if state == "" {
			state = "ready"
		}
		epoch := testCompanionEpoch
		if state == "absent" {
			epoch = ""
		}
		_, _ = io.WriteString(
			stdout,
			`{"schema_version":1,"ok":true,"state":"`+state+`","epoch_id":"`+epoch+`"}`+"\n",
		)
	case slices.Contains(args, "status"):
		provider := authDoctorArgument(args, "--provider")
		if provider == "github" {
			_, _ = io.WriteString(
				stdout,
				`{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"`+authDoctorRevision+`"}`+"\n",
			)
			break
		}
		_, _ = io.WriteString(
			stdout,
			`{"schema_version":1,"ok":true,"state":"not_configured","provider":"`+provider+`"}`+"\n",
		)
	case slices.Contains(args, "binding_status"):
		if r.bindingFrame != "" {
			_, _ = io.WriteString(stdout, r.bindingFrame+"\n")
			break
		}
		state := r.bindingState
		if state == "" {
			state = "ready"
		}
		provider := authDoctorArgument(args, "--provider")
		_, _ = io.WriteString(
			stdout,
			`{"schema_version":1,"ok":true,"state":"`+state+`","provider":"`+provider+`","revision":"`+authDoctorRevision+`"}`+"\n",
		)
	}
	return nil
}

func (r *authDoctorRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) != 0 && args[0] == "inspect" && slices.Contains(args, authBrokerContainer) {
		if r.inspectionErr != nil {
			return nil, r.inspectionErr
		}
		if r.inspectionData != "" {
			return []byte(r.inspectionData), nil
		}
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	return nil, nil
}

type authDoctorFixture struct {
	runtime  *Runtime
	runner   *authDoctorRunner
	project  tobari.ProjectInstance
	digest   string
	bindings []byte
}

func newAuthDoctorFixture(t *testing.T, runner *authDoctorRunner) authDoctorFixture {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	project, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtime.loadAuthProviders()
	if err != nil {
		t.Fatal(err)
	}
	_, bindings, digest, err := brokerBindingsForProvider(projection, "github")
	if err != nil {
		t.Fatal(err)
	}
	return authDoctorFixture{
		runtime: runtime, runner: runner, project: project, digest: digest, bindings: bindings,
	}
}

func (f authDoctorFixture) writeRegistry(t *testing.T, revision, digest string) {
	t.Helper()
	if err := writeAtomicJSON(f.runtime.projectAuthRegistryPath(f.project.ID), projectAuthRegistry{
		SchemaVersion: projectAuthRegistrySchema,
		ProjectID:     f.project.ID,
		Providers: []projectAuthProviderBinding{{
			Provider: "github", Revision: revision, BindingDigest: digest,
		}},
		Files: []projectAuthRegistryEntry{},
	}); err != nil {
		t.Fatal(err)
	}
}

func runAuthDiagnostics(runtime *Runtime) []doctor.Check {
	checks := make([]doctor.Check, 0, 5)
	runtime.addAuthDiagnostics(context.Background(), func(name string, status doctor.CheckStatus, detail string) {
		checks = append(checks, doctor.Check{Name: doctor.CheckID(name), Status: status, Detail: detail})
	})
	return checks
}

func requireAuthDiagnostic(t *testing.T, checks []doctor.Check, name string, status doctor.CheckStatus) doctor.Check {
	t.Helper()
	for _, check := range checks {
		if check.Name != doctor.CheckID(name) {
			continue
		}
		if check.Status != status {
			t.Fatalf("%s diagnostic = %+v, want status %q", name, check, status)
		}
		return check
	}
	t.Fatalf("diagnostics lack %q: %+v", name, checks)
	return doctor.Check{}
}

func authDoctorControlCalls(runner *authDoctorRunner, operation string) [][]string {
	result := make([][]string, 0)
	for _, call := range runner.controlCalls {
		if slices.Contains(call, operation) {
			result = append(result, call)
		}
	}
	return result
}

func authDoctorArgument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func assertAuthDoctorCanaryAbsent(t *testing.T, checks []doctor.Check) {
	t.Helper()
	for _, check := range checks {
		if strings.Contains(string(check.Name), authDoctorCanary) || strings.Contains(check.Detail, authDoctorCanary) {
			t.Fatalf("diagnostic exposed secret canary: %+v", check)
		}
	}
}

func TestAuthDoctorVerifiesMatchingProjectBindingWithExactHostOwnedDimensions(t *testing.T) {
	runner := &authDoctorRunner{brokerState: "ready", bindingState: "ready"}
	fixture := newAuthDoctorFixture(t, runner)
	fixture.writeRegistry(t, authDoctorRevision, fixture.digest)

	checks := runAuthDiagnostics(fixture.runtime)
	providerCheck := requireAuthDiagnostic(t, checks, "auth_provider_manifests", doctor.CheckStatusPass)
	wantProviderDetail := strconv.Itoa(len(authbroker.ActiveBuiltinProviderIDs())) + " credential-provider manifests normalize to projection schema V1"
	if providerCheck.Detail != wantProviderDetail {
		t.Fatalf("provider manifest diagnostic = %q", providerCheck.Detail)
	}
	requireAuthDiagnostic(t, checks, "auth_broker", doctor.CheckStatusPass)
	requireAuthDiagnostic(t, checks, "credential_companion", doctor.CheckStatusPass)
	requireAuthDiagnostic(t, checks, "auth_vault_integrity", doctor.CheckStatusPass)
	requireAuthDiagnostic(t, checks, "auth_project_handles", doctor.CheckStatusPass)
	assertAuthDoctorCanaryAbsent(t, checks)

	calls := authDoctorControlCalls(runner, "binding_status")
	if len(calls) != 1 {
		t.Fatalf("binding_status calls = %v, want exactly one", calls)
	}
	call := calls[0]
	for name, want := range map[string]string{
		"--context-id": fixture.project.ContextID,
		"--project-id": fixture.project.ID,
		"--provider":   "github",
		"--revision":   authDoctorRevision,
		"--bindings":   string(fixture.bindings),
	} {
		if got := authDoctorArgument(call, name); got != want {
			t.Fatalf("binding_status %s = %q, want %q; argv=%v", name, got, want, call)
		}
	}
}

func TestAuthDoctorReportsCompanionSeparatelyWithoutHidingStaticBrokerHealth(t *testing.T) {
	runner := &authDoctorRunner{brokerState: "ready", companionState: "absent"}
	fixture := newAuthDoctorFixture(t, runner)
	fixture.writeRegistry(t, authDoctorRevision, fixture.digest)

	checks := runAuthDiagnostics(fixture.runtime)
	requireAuthDiagnostic(t, checks, "auth_broker", doctor.CheckStatusPass)
	companion := requireAuthDiagnostic(t, checks, "credential_companion", doctor.CheckStatusWarn)
	if !strings.Contains(companion.Detail, "cluster up") {
		t.Fatalf("companion diagnostic = %q", companion.Detail)
	}
	requireAuthDiagnostic(t, checks, "auth_vault_integrity", doctor.CheckStatusPass)
	assertAuthDoctorCanaryAbsent(t, checks)
}

func TestAuthDoctorClassifiesProjectBindingDriftAsReentryWarning(t *testing.T) {
	tests := []struct {
		name             string
		bindingState     string
		registryRevision string
		registryDigest   func(string) string
		wantBindingCalls int
	}{
		{name: "broker binding missing", bindingState: "missing", registryRevision: authDoctorRevision, registryDigest: func(value string) string { return value }, wantBindingCalls: 1},
		{name: "broker binding stale", bindingState: "stale", registryRevision: authDoctorRevision, registryDigest: func(value string) string { return value }, wantBindingCalls: 1},
		{name: "registry revision stale", registryRevision: "revision_stale", registryDigest: func(value string) string { return value }, wantBindingCalls: 0},
		{name: "registry digest stale", registryRevision: authDoctorRevision, registryDigest: func(string) string { return "sha256:" + strings.Repeat("f", 64) }, wantBindingCalls: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &authDoctorRunner{brokerState: "ready", bindingState: test.bindingState}
			fixture := newAuthDoctorFixture(t, runner)
			fixture.writeRegistry(t, test.registryRevision, test.registryDigest(fixture.digest))

			checks := runAuthDiagnostics(fixture.runtime)
			check := requireAuthDiagnostic(t, checks, "auth_project_handles", doctor.CheckStatusWarn)
			if !strings.Contains(check.Detail, "require the next matching tobari entry") {
				t.Fatalf("project binding warning = %q", check.Detail)
			}
			if calls := authDoctorControlCalls(runner, "binding_status"); len(calls) != test.wantBindingCalls {
				t.Fatalf("binding_status calls = %v, want %d", calls, test.wantBindingCalls)
			}
			assertAuthDoctorCanaryAbsent(t, checks)
		})
	}
}

func TestAuthDoctorReportsMissingAndUnsafeProjectRegistriesWithoutContents(t *testing.T) {
	t.Run("missing registry requires reentry", func(t *testing.T) {
		runner := &authDoctorRunner{brokerState: "ready"}
		fixture := newAuthDoctorFixture(t, runner)
		checks := runAuthDiagnostics(fixture.runtime)
		requireAuthDiagnostic(t, checks, "auth_project_handles", doctor.CheckStatusWarn)
		if calls := authDoctorControlCalls(runner, "binding_status"); len(calls) != 0 {
			t.Fatalf("binding_status calls = %v, want none for a missing registry", calls)
		}
		assertAuthDoctorCanaryAbsent(t, checks)
	})

	t.Run("unsafe registry fails closed", func(t *testing.T) {
		runner := &authDoctorRunner{brokerState: "ready"}
		fixture := newAuthDoctorFixture(t, runner)
		registryPath := fixture.runtime.projectAuthRegistryPath(fixture.project.ID)
		if err := os.MkdirAll(filepath.Dir(registryPath), 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), authDoctorCanary)
		if err := os.WriteFile(target, []byte(authDoctorCanary), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, registryPath); err != nil {
			t.Fatal(err)
		}

		checks := runAuthDiagnostics(fixture.runtime)
		requireAuthDiagnostic(t, checks, "auth_project_handles", doctor.CheckStatusFail)
		if calls := authDoctorControlCalls(runner, "binding_status"); len(calls) != 0 {
			t.Fatalf("binding_status calls = %v, want none for an unsafe registry", calls)
		}
		assertAuthDoctorCanaryAbsent(t, checks)
	})
}

func TestAuthDoctorStopsAtLockedOrUnavailableBroker(t *testing.T) {
	tests := []struct {
		name   string
		runner *authDoctorRunner
		status doctor.CheckStatus
	}{
		{name: "locked", runner: &authDoctorRunner{brokerState: "locked"}, status: doctor.CheckStatusFail},
		{
			name: "unavailable",
			runner: &authDoctorRunner{
				brokerState: "unavailable", inspectionErr: errors.New("Docker unavailable: " + authDoctorCanary),
			},
			status: doctor.CheckStatusWarn,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntimeWithData(
				filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), test.runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			checks := runAuthDiagnostics(runtime)
			requireAuthDiagnostic(t, checks, "auth_broker", test.status)
			for _, forbidden := range []string{"credential_companion", "auth_vault_integrity", "auth_project_handles"} {
				for _, check := range checks {
					if check.Name == doctor.CheckID(forbidden) {
						t.Fatalf("%s broker diagnostic continued into %s", test.name, forbidden)
					}
				}
			}
			if calls := authDoctorControlCalls(test.runner, "status"); len(calls) != 0 {
				t.Fatalf("status calls = %v, want none", calls)
			}
			if calls := authDoctorControlCalls(test.runner, "binding_status"); len(calls) != 0 {
				t.Fatalf("binding_status calls = %v, want none", calls)
			}
			assertAuthDoctorCanaryAbsent(t, checks)
		})
	}
}

func TestAuthDoctorRedactsMalformedBrokerBindingResponse(t *testing.T) {
	runner := &authDoctorRunner{
		brokerState:  "ready",
		bindingFrame: `{"schema_version":1,"ok":false,"error":{"code":"transport_error","detail":"` + authDoctorCanary + `"}}`,
	}
	fixture := newAuthDoctorFixture(t, runner)
	fixture.writeRegistry(t, authDoctorRevision, fixture.digest)

	checks := runAuthDiagnostics(fixture.runtime)
	requireAuthDiagnostic(t, checks, "auth_project_handles", doctor.CheckStatusFail)
	if calls := authDoctorControlCalls(runner, "binding_status"); len(calls) != 1 {
		t.Fatalf("binding_status calls = %v, want one", calls)
	}
	assertAuthDoctorCanaryAbsent(t, checks)
}
