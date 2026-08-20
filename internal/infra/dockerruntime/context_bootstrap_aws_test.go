package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const syntheticAWSSharedConfig = `[profile engineering]
sso_session = company
sso_account_id = 123456789012
sso_role_name = Developer
region = ap-northeast-1
output = json

[sso-session company]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access
`

func writeSyntheticHostAWSConfig(t *testing.T, runtime *Runtime, contents string) {
	t.Helper()
	home := t.TempDir()
	runtime.hostHomeDirectory = home
	directory := filepath.Join(home, ".aws")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAWSBootstrapSnapshotIsStrictSecretFreeAndSemantic(t *testing.T) {
	t.Parallel()
	aws, err := parseHostAWSBootstrap([]byte(syntheticAWSSharedConfig), "engineering")
	if err != nil {
		t.Fatal(err)
	}
	first, err := tobari.NewContextBootstrapSnapshot(1, aws)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tobari.NewContextBootstrapSnapshot(9, aws)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision {
		t.Fatalf("generation changed semantic revision: %s != %s", first.Revision, second.Revision)
	}
	encoded, err := encodeProjectAWSConfig(aws)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"aws_access_key_id", "aws_secret_access_key", "credential_process", "source_profile", ".aws/sso/cache"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("projection contains forbidden authority %q: %s", forbidden, encoded)
		}
	}
	hostile := strings.ReplaceAll(syntheticAWSSharedConfig, "output = json", "credential_process = /tmp/helper")
	if _, err := parseHostAWSBootstrap([]byte(hostile), "engineering"); err == nil {
		t.Fatal("unknown executable directive was accepted")
	}
}

func TestHostAWSBootstrapRejectsSymlinkAndGroupWritableSource(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		prepare func(t *testing.T, config string)
	}{
		{name: "symlink", prepare: func(t *testing.T, config string) {
			t.Helper()
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, []byte(syntheticAWSSharedConfig), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, config); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "group writable", prepare: func(t *testing.T, config string) {
			t.Helper()
			if err := os.WriteFile(config, []byte(syntheticAWSSharedConfig), 0o620); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(config, 0o620); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := newProjectStateRuntime(t)
			home := t.TempDir()
			runtime.hostHomeDirectory = home
			directory := filepath.Join(home, ".aws")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			test.prepare(t, filepath.Join(directory, "config"))
			if _, err := runtime.PrepareContextAWSBootstrap(context.Background(), "engineering"); err == nil {
				t.Fatal("unsafe host AWS config was accepted")
			}
			discovered, err := runtime.DiscoverContextAWSBootstraps(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if discovered.State != tobari.ContextBootstrapDiscoveryRejected || len(discovered.Candidates) != 0 {
				t.Fatalf("unsafe discovery = %+v", discovered)
			}
		})
	}
}

func TestDiscoverAWSBootstrapsResolvesSharedSessionAndKeepsIndividualFailuresUnavailable(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	source := syntheticAWSSharedConfig + `
[profile production]
sso_session = company
sso_account_id = 210987654321
sso_role_name = ReadOnly
region = us-east-1

[profile broken]
sso_session = removed
sso_account_id = 123456789012
sso_role_name = Developer
`
	writeSyntheticHostAWSConfig(t, runtime, source)
	result, err := runtime.DiscoverContextAWSBootstraps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ContextBootstrapDiscoveryAvailable || len(result.Candidates) != 3 {
		t.Fatalf("discovery = %+v", result)
	}
	states := map[string]string{}
	for _, candidate := range result.Candidates {
		states[candidate.Profile] = candidate.State
		if candidate.State == tobari.ContextBootstrapCandidateAvailable && candidate.Snapshot.AWS.SSOSession != "company" {
			t.Fatalf("shared session was not resolved: %+v", candidate)
		}
	}
	if states["engineering"] != tobari.ContextBootstrapCandidateAvailable || states["production"] != tobari.ContextBootstrapCandidateAvailable || states["broken"] != tobari.ContextBootstrapCandidateUnavailable {
		t.Fatalf("candidate states = %+v", states)
	}
}

func TestDiscoverAWSBootstrapsRejectsMalformedWholeFileWithoutPartialCandidates(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig+"\n[profile engineering]\nsso_session = company\n")
	result, err := runtime.DiscoverContextAWSBootstraps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ContextBootstrapDiscoveryRejected || len(result.Candidates) != 0 || result.Reason == "" {
		t.Fatalf("malformed discovery = %+v", result)
	}
}

func TestDiscoverAWSBootstrapsDistinguishesMissingSource(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	runtime.hostHomeDirectory = t.TempDir()
	result, err := runtime.DiscoverContextAWSBootstraps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ContextBootstrapDiscoveryMissing || len(result.Candidates) != 0 {
		t.Fatalf("missing discovery = %+v", result)
	}
}

func TestSelectedAWSSemanticRevisionIgnoresUnrelatedProfileChanges(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig)
	reviewed, err := runtime.PrepareContextAWSBootstrap(context.Background(), "engineering")
	if err != nil {
		t.Fatal(err)
	}
	path, err := runtime.hostAWSConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	updated := syntheticAWSSharedConfig + `
[profile unrelated]
sso_session = company
sso_account_id = 210987654321
sso_role_name = ReadOnly
region = us-east-1
`
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshed, err := runtime.PrepareContextAWSBootstrap(context.Background(), "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Revision != reviewed.Revision {
		t.Fatalf("unrelated profile changed selected revision: %s != %s", refreshed.Revision, reviewed.Revision)
	}
}

func TestContextBootstrapAppliesOnceAndRefreshAffectsOnlyNewWorkspaces(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig)
	configured, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultContextName, "engineering", "", false)
	if err != nil {
		t.Fatal(err)
	}
	firstRevision := configured.Bootstrap.Revision
	if configured.Bootstrap.State != tobari.ContextBootstrapConfigured || firstRevision == "" {
		t.Fatalf("configured report = %+v", configured.Bootstrap)
	}

	firstRoot := filepath.Join(t.TempDir(), "first")
	if err := os.Mkdir(firstRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	first, _, err := runtime.ResolveOrCreateProjectInContext(context.Background(), firstRoot, tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	firstConfigPath := filepath.Join(runtime.projectHomePath(first.ID), ".aws", "config")
	firstBytes, err := os.ReadFile(firstConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if first.BootstrapRevision != firstRevision || !strings.Contains(string(firstBytes), "sso_role_name = Developer") {
		t.Fatalf("first Workspace projection = revision %q bytes %q", first.BootstrapRevision, firstBytes)
	}
	for _, path := range []string{filepath.Join(runtime.projectHomePath(first.ID), ".aws", "credentials"), filepath.Join(runtime.projectHomePath(first.ID), ".aws", "sso", "cache")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("forbidden AWS authority path exists: %s (%v)", path, err)
		}
	}

	hostPath, _ := runtime.hostAWSConfigPath()
	updated := strings.ReplaceAll(syntheticAWSSharedConfig, "sso_role_name = Developer", "sso_role_name = ReadOnly")
	if err := os.WriteFile(hostPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := runtime.PreviewContextAWSBootstrap(context.Background(), tobari.DefaultContextName, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0] != "aws.sso_role_name" {
		t.Fatalf("semantic changes = %+v", preview.Changes)
	}
	drifted := strings.ReplaceAll(updated, "sso_role_name = ReadOnly", "sso_role_name = Administrator")
	if err := os.WriteFile(hostPath, []byte(drifted), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultContextName, "", preview.Candidate.Revision, false); !errors.Is(err, tobari.ErrContextBootstrapSourceChanged) {
		t.Fatalf("stale reviewed candidate error = %v", err)
	}
	unchanged, err := runtime.ResolveContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Bootstrap == nil || unchanged.Bootstrap.Revision != firstRevision {
		t.Fatal("source drift changed the Context snapshot")
	}
	if err := os.WriteFile(hostPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshed, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultContextName, "", preview.Candidate.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Bootstrap.Generation != 2 || refreshed.Bootstrap.Revision == firstRevision {
		t.Fatalf("refreshed report = %+v", refreshed.Bootstrap)
	}
	afterRefresh, err := os.ReadFile(firstConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRefresh) != string(firstBytes) {
		t.Fatal("Context refresh rewrote an existing Workspace home")
	}

	secondRoot := filepath.Join(t.TempDir(), "second")
	if err := os.Mkdir(secondRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	second, _, err := runtime.ResolveOrCreateProjectInContext(context.Background(), secondRoot, tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(filepath.Join(runtime.projectHomePath(second.ID), ".aws", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if second.BootstrapRevision != refreshed.Bootstrap.Revision || !strings.Contains(string(secondBytes), "sso_role_name = ReadOnly") {
		t.Fatalf("second Workspace projection = revision %q bytes %q", second.BootstrapRevision, secondBytes)
	}
	manifest, err := runtime.ResolveContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	older, err := tobari.ResolveWorkspaceBootstrapReport(first.BootstrapRevision, manifest.Bootstrap)
	if err != nil || older.State != tobari.WorkspaceBootstrapOlder {
		t.Fatalf("first Workspace status = %+v, error=%v", older, err)
	}
	current, err := tobari.ResolveWorkspaceBootstrapReport(second.BootstrapRevision, manifest.Bootstrap)
	if err != nil || current.State != tobari.WorkspaceBootstrapCurrent {
		t.Fatalf("second Workspace status = %+v, error=%v", current, err)
	}
}
