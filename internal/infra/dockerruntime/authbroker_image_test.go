package dockerruntime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const testAuthBrokerDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"

func TestVerifyAuthBrokerImagePullsAndChecksImmutableContract(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata:        authBrokerMetadata("arm64", "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest),
		server:          `{"Os":"linux","Arch":"aarch64"}`,
		firstInspectErr: errors.New("image is not present"),
	}
	runtime := &Runtime{runner: runner}
	if _, err := runtime.verifyAuthBrokerImage(context.Background(), "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest, true); err != nil {
		t.Fatal(err)
	}
	if runner.inspectCalls != 2 {
		t.Fatalf("image inspect calls = %d, want pull followed by re-inspect", runner.inspectCalls)
	}
	if len(runner.outputs) == 0 || runner.outputs[0].args[0] != "pull" {
		t.Fatalf("preflight output calls = %+v", runner.outputs)
	}
}

func TestVerifyAuthBrokerImageRejectsContractDimensions(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(string) string{
		"old api label": func(metadata string) string {
			return strings.Replace(metadata, `"io.tobari.auth-broker-api":"1"`, `"io.tobari.auth-broker-api":"2"`, 1)
		},
		"role label": func(metadata string) string {
			return strings.Replace(metadata, `"io.tobari.auth-broker-role":"credential-resolution"`, `"io.tobari.auth-broker-role":"other"`, 1)
		},
		"non-root user": func(metadata string) string {
			return strings.Replace(metadata, `"User":"1000:1000"`, `"User":"root"`, 1)
		},
		"entrypoint": func(metadata string) string {
			return strings.Replace(metadata, `"Entrypoint":["/opt/tobari/entrypoint.sh"]`, `"Entrypoint":["/bin/sh"]`, 1)
		},
		"declared volume": func(metadata string) string {
			return strings.Replace(metadata, `"Entrypoint":["/opt/tobari/entrypoint.sh"]`, `"Entrypoint":["/opt/tobari/entrypoint.sh"],"Volumes":{"/unreviewed":{}}`, 1)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &gatewayImageRunner{
				metadata: mutate(authBrokerMetadata("arm64", "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest)),
				server:   `{"Os":"linux","Arch":"arm64"}`,
			}
			runtime := &Runtime{runner: runner}
			_, err := runtime.verifyAuthBrokerImage(context.Background(), "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest, true)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "auth_broker_image_incompatible" {
				t.Fatalf("error = %v, public = %+v", err, public)
			}
			for _, call := range runner.outputs {
				if len(call.args) > 0 && call.args[0] == "pull" {
					t.Fatal("incompatible image was pulled")
				}
			}
		})
	}
}

func TestVerifyAuthBrokerImageRejectsEngineArchitectureMismatch(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: authBrokerMetadata("arm64", "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest),
		server:   `{"Os":"linux","Arch":"amd64"}`,
	}
	runtime := &Runtime{runner: runner}
	_, err := runtime.verifyAuthBrokerImage(context.Background(), "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest, true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_image_incompatible" {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
}

func TestPrepareAuthBrokerImageUsesInjectedLocalResolver(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{
		metadata: authBrokerMetadata("arm64", ""),
		server:   `{"Os":"linux","Arch":"arm64"}`,
	}
	runtime := &Runtime{
		runner: runner,
		images: testImageResolver{
			runtimeImage: "tobari-runtime:test",
			authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev", RequireDigest: false},
		},
	}
	image, err := runtime.prepareAuthBrokerImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if image != testAuthBrokerDigest {
		t.Fatalf("Auth Broker image = %q", image)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("local Auth Broker image was pulled: %v", runner.outputs)
		}
	}
}

func TestVerifyAuthBrokerImageRejectsOversizedMetadata(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{metadata: strings.Repeat("x", componentImageInspectLimit), server: `{"Os":"linux","Arch":"arm64"}`}
	_, err := (&Runtime{runner: runner}).verifyAuthBrokerImage(context.Background(), "tobari-auth-broker:test", false)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_image_unavailable" {
		t.Fatalf("error = %v public=%+v", err, public)
	}
}

func TestVerifyDigestAuthBrokerDoesNotPullAfterAmbiguousInspectFailure(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{firstInspectErr: errors.New("permission denied"), inspectDiagnostic: "permission denied"}
	_, err := (&Runtime{runner: runner}).verifyAuthBrokerImage(context.Background(), "ghcr.io/tasuku43/tobari/auth-broker@"+testAuthBrokerDigest, true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_image_unavailable" {
		t.Fatalf("error = %v public=%+v", err, public)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("ambiguous inspect triggered pull: %+v", runner.outputs)
		}
	}
}

func TestAuthBrokerBootstrapMarkerFailsBeforeDocker(t *testing.T) {
	t.Parallel()
	runner := &gatewayImageRunner{}
	runtime := &Runtime{runner: runner}
	_, err := runtime.verifyAuthBrokerImage(context.Background(), "unpublished", true)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_image_incompatible" || public.Retryable {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
	if len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("unpublished bootstrap marker reached Docker: outputs=%v runs=%v", runner.outputs, runner.runs)
	}
}

func TestClusterStartupRejectsAuthBrokerBootstrapMarkerBeforeDockerMutation(t *testing.T) {
	if !brokerRuntimeEnabled {
		t.Skip("Auth Broker startup exists only on the research surface")
	}
	t.Parallel()
	root := t.TempDir()
	runner := &gatewayImageRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:test",
		authBroker: sharedImageSelection{
			Image: "unpublished", RequireDigest: true,
		},
		gateway: sharedImageSelection{Image: "tobari-gateway:dev"},
	}
	_, err = runtime.ClusterUp(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_image_incompatible" {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
	if len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("unpublished bootstrap marker reached Docker: outputs=%v runs=%v", runner.outputs, runner.runs)
	}
	if _, exists, journalErr := runtime.readClusterJournal(); journalErr != nil || exists {
		t.Fatalf("cluster journal after preflight rejection = exists:%t error:%v", exists, journalErr)
	}
}

func authBrokerMetadata(architecture, repoDigest string) string {
	repoDigests := "[]"
	if repoDigest != "" {
		repoDigests = `["` + repoDigest + `"]`
	}
	return `{"Id":"` + testAuthBrokerDigest + `","RepoDigests":` + repoDigests + `,"Architecture":"` + architecture + `","Os":"linux","Config":{"User":"1000:1000","Labels":{"io.tobari.auth-broker-api":"1","io.tobari.auth-broker-role":"credential-resolution"},"Entrypoint":["/opt/tobari/entrypoint.sh"]}}`
}
